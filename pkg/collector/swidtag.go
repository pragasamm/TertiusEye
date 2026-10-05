package collector

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"tertiuseye/agent/pkg/model"
)

// SWIDTagXML defines the XML structure for ISO/IEC 19770-2 Software Identification Tags.
type SWIDTagXML struct {
	XMLName      xml.Name        `xml:""`
	Name         string          `xml:"name,attr"`
	Version      string          `xml:"version,attr"`
	TagID        string          `xml:"tagId,attr"`
	TagIDAlt     string          `xml:"uniqueId,attr"`
	Patch        bool            `xml:"patch,attr"`
	Supplemental bool            `xml:"supplemental,attr"`
	Entities     []SWIDEntityXML `xml:"Entity"`
	EntitiesAlt  []SWIDEntityXML `xml:"entity"`
	Links        []SWIDLinkXML   `xml:"Link"`
	LinksAlt     []SWIDLinkXML   `xml:"link"`
}

type SWIDEntityXML struct {
	Name  string `xml:"name,attr"`
	RegID string `xml:"regid,attr"`
	Role  string `xml:"role,attr"`
}

type SWIDLinkXML struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

// SWIDCollector scans target directories for ISO/IEC 19770-2 .swidtag XML files.
type SWIDCollector struct {
	TargetPaths       []string
	IncludeSystemApps bool
}

// NewSWIDCollector initializes a SWIDCollector with target search paths.
func NewSWIDCollector(paths []string) *SWIDCollector {
	return &SWIDCollector{
		TargetPaths:       paths,
		IncludeSystemApps: len(paths) == 0,
	}
}

// Collect walks configured target directories for .swidtag files AND optionally scans system application folders for installed software.
func (s *SWIDCollector) Collect(ctx context.Context) (model.SoftwareInventory, error) {
	return s.CollectWithProcesses(ctx, nil)
}

// CollectWithProcesses collects software inventory and cross-references active running processes to update LastUsed for running apps.
func (s *SWIDCollector) CollectWithProcesses(ctx context.Context, processes []model.ProcessInfo) (model.SoftwareInventory, error) {
	var inventory model.SoftwareInventory
	seen := make(map[string]bool)

	addTag := func(tag model.SWIDTagInfo) {
		key := strings.ToLower(tag.TagID)
		if key == "" {
			key = strings.ToLower(tag.Name)
		}
		if key != "" && !seen[key] {
			seen[key] = true
			inventory.SWIDTags = append(inventory.SWIDTags, tag)
		}
	}

	// 1. Scan configured SWID tag directories
	for _, root := range s.TargetPaths {
		select {
		case <-ctx.Done():
			return inventory, ctx.Err()
		default:
		}

		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if d.IsDir() {
				if d.Type()&os.ModeSymlink != 0 {
					return filepath.SkipDir
				}
				return nil
			}

			if strings.HasSuffix(strings.ToLower(d.Name()), ".swidtag") {
				tag, err := ParseSWIDTagFile(path)
				if err == nil {
					addTag(tag)
				}
			}

			return nil
		})
	}

	// 2. Scan installed system applications (macOS .app bundles, Linux desktop apps, Windows Program Files)
	if s.IncludeSystemApps {
		appTags := scanInstalledApplications(ctx)
		for _, tag := range appTags {
			addTag(tag)
		}
	}

	// 3. Cross-reference running processes to update LastUsed for currently active applications
	if len(processes) > 0 {
		nowStr := time.Now().Format("2006-01-02 15:04:05")
		for i, tag := range inventory.SWIDTags {
			tagPathLower := strings.ToLower(tag.SourceFilePath)
			tagNameLower := strings.ToLower(tag.Name)

			for _, p := range processes {
				execLower := strings.ToLower(p.ExecutablePath)
				procNameLower := strings.ToLower(p.Name)

				if (tagPathLower != "" && execLower != "" && strings.Contains(execLower, tagPathLower)) ||
					(tagNameLower != "" && procNameLower != "" && strings.EqualFold(tagNameLower, procNameLower)) {
					inventory.SWIDTags[i].LastUsed = nowStr
					break
				}
			}
		}
	}

	inventory.TotalDiscovered = len(inventory.SWIDTags)
	return inventory, nil
}

// scanInstalledApplications discovers native system applications from OS standard application directories.
func scanInstalledApplications(ctx context.Context) []model.SWIDTagInfo {
	var results []model.SWIDTagInfo

	if runtime.GOOS == "darwin" {
		appDirs := []string{"/Applications", "/System/Applications"}
		if homeDir, err := os.UserHomeDir(); err == nil {
			appDirs = append(appDirs, filepath.Join(homeDir, "Applications"))
		}

		var scanDir func(dir string, depth int)
		scanDir = func(dir string, depth int) {
			if depth > 2 {
				return
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return
			}
			for _, entry := range entries {
				select {
				case <-ctx.Done():
					return
				default:
				}

				name := entry.Name()
				if strings.HasPrefix(name, ".") {
					continue
				}

				fullPath := filepath.Join(dir, name)
				if strings.HasSuffix(name, ".app") {
					plistPath := filepath.Join(fullPath, "Contents", "Info.plist")
					appName, version, bundleID := parseInfoPlist(plistPath)
					if appName == "" {
						appName = strings.TrimSuffix(name, ".app")
					}
					if version == "" {
						version = "1.0.0"
					}
					if bundleID == "" {
						bundleID = "com.apple.application." + strings.ToLower(appName)
					}
					publisher := resolvePublisher(bundleID, appName)
					results = append(results, model.SWIDTagInfo{
						Name:           appName,
						Version:        version,
						TagID:          bundleID,
						Patch:          false,
						Supplemental:   false,
						SourceFilePath: fullPath,
						LastUsed:       determineLastUsed(fullPath),
						Entities: []model.SWIDEntity{
							{
								Name: publisher,
								Role: "softwareCreator",
							},
						},
					})
				} else if entry.IsDir() {
					scanDir(fullPath, depth+1)
				}
			}
		}

		for _, dir := range appDirs {
			scanDir(dir, 0)
		}
	} else if runtime.GOOS == "linux" {
		desktopDirs := []string{"/usr/share/applications", "/var/lib/snapd/desktop/applications"}
		for _, dir := range desktopDirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".desktop") {
					appName := strings.TrimSuffix(entry.Name(), ".desktop")
					deskPath := filepath.Join(dir, entry.Name())
					results = append(results, model.SWIDTagInfo{
						Name:           strings.Title(appName),
						Version:        "1.0.0",
						TagID:          "org.freedesktop." + appName,
						SourceFilePath: deskPath,
						LastUsed:       determineLastUsed(deskPath),
						Entities:       []model.SWIDEntity{{Name: "Open Source Community", Role: "softwareCreator"}},
					})
				}
			}
		}
	} else if runtime.GOOS == "windows" {
		winDirs := []string{}
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			winDirs = append(winDirs, pf)
		}
		if pfx86 := os.Getenv("ProgramFiles(x86)"); pfx86 != "" {
			winDirs = append(winDirs, pfx86)
		}
		if localAppData := os.Getenv("LocalAppData"); localAppData != "" {
			winDirs = append(winDirs, filepath.Join(localAppData, "Programs"))
		}

		for _, dir := range winDirs {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				continue
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}

			for _, entry := range entries {
				select {
				case <-ctx.Done():
					return results
				default:
				}

				if entry.IsDir() {
					appName := entry.Name()
					if strings.HasPrefix(appName, "Common Files") || strings.HasPrefix(appName, "Windows") {
						continue
					}
					appFolder := filepath.Join(dir, appName)
					exePath := appFolder

					if files, readErr := os.ReadDir(appFolder); readErr == nil {
						for _, f := range files {
							if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".exe") {
								exePath = filepath.Join(appFolder, f.Name())
								break
							}
						}
					}

					bundleID := "com.microsoft.windows." + strings.ToLower(strings.ReplaceAll(appName, " ", "."))
					publisher := resolvePublisher(bundleID, appName)

					results = append(results, model.SWIDTagInfo{
						Name:           appName,
						Version:        "1.0.0",
						TagID:          bundleID,
						Patch:          false,
						Supplemental:   false,
						SourceFilePath: exePath,
						LastUsed:       determineLastUsed(exePath),
						Entities: []model.SWIDEntity{
							{
								Name: publisher,
								Role: "softwareCreator",
							},
						},
					})
				}
			}
		}
	}

	return results
}

func determineLastUsed(filePath string) string {
	if filePath == "" {
		return time.Now().Format("2006-01-02 15:04:05")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return time.Now().Format("2006-01-02 15:04:05")
	}
	return info.ModTime().Format("2006-01-02 15:04:05")
}

func parseInfoPlist(plistPath string) (name string, version string, bundleID string) {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return "", "", ""
	}
	content := string(data)

	name = extractPlistKey(content, "CFBundleDisplayName")
	if name == "" {
		name = extractPlistKey(content, "CFBundleName")
	}

	version = extractPlistKey(content, "CFBundleShortVersionString")
	if version == "" {
		version = extractPlistKey(content, "CFBundleVersion")
	}

	bundleID = extractPlistKey(content, "CFBundleIdentifier")
	return name, version, bundleID
}

func extractPlistKey(content string, key string) string {
	keyTag := "<key>" + key + "</key>"
	idx := strings.Index(content, keyTag)
	if idx == -1 {
		return ""
	}
	sub := content[idx+len(keyTag):]
	valStart := strings.Index(sub, "<string>")
	valEnd := strings.Index(sub, "</string>")
	if valStart != -1 && valEnd != -1 && valEnd > valStart {
		return strings.TrimSpace(sub[valStart+8 : valEnd])
	}
	return ""
}

func resolvePublisher(bundleID string, appName string) string {
	lowerID := strings.ToLower(bundleID)
	lowerName := strings.ToLower(appName)

	if strings.Contains(lowerID, "apple") || strings.Contains(lowerName, "safari") {
		return "Apple Inc."
	} else if strings.Contains(lowerID, "google") || strings.Contains(lowerName, "chrome") {
		return "Google LLC"
	} else if strings.Contains(lowerID, "microsoft") || strings.Contains(lowerID, "vscode") {
		return "Microsoft Corporation"
	} else if strings.Contains(lowerID, "whatsapp") || strings.Contains(lowerName, "whatsapp") {
		return "Meta Platforms Inc."
	} else if strings.Contains(lowerID, "zoom") || strings.Contains(lowerName, "zoom") {
		return "Zoom Video Communications Inc."
	} else if strings.Contains(lowerID, "audacity") || strings.Contains(lowerName, "audacity") {
		return "Audacity Team"
	} else if strings.Contains(lowerID, "docker") {
		return "Docker Inc"
	} else if strings.Contains(lowerID, "slack") {
		return "Slack Technologies"
	} else if strings.Contains(lowerID, "antigravity") {
		return "Antigravity Systems"
	}

	parts := strings.Split(bundleID, ".")
	if len(parts) >= 2 {
		return strings.Title(parts[1])
	}
	return "Software Publisher"
}

// ParseSWIDTagFile reads and unmarshals an ISO/IEC 19770-2 .swidtag file.
func ParseSWIDTagFile(filePath string) (model.SWIDTagInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return model.SWIDTagInfo{}, fmt.Errorf("failed to read swidtag file %s: %w", filePath, err)
	}

	info, err := ParseSWIDTagXML(data, filePath)
	if err == nil {
		info.LastUsed = determineLastUsed(filePath)
	}
	return info, err
}

// ParseSWIDTagXML unmarshals raw XML payload into SWIDTagInfo data structure.
func ParseSWIDTagXML(data []byte, filePath string) (model.SWIDTagInfo, error) {
	var raw SWIDTagXML
	if err := xml.Unmarshal(data, &raw); err != nil {
		return model.SWIDTagInfo{}, fmt.Errorf("failed to parse swidtag XML: %w", err)
	}

	tagID := raw.TagID
	if tagID == "" {
		tagID = raw.TagIDAlt
	}

	info := model.SWIDTagInfo{
		Name:           raw.Name,
		Version:        raw.Version,
		TagID:          tagID,
		Patch:          raw.Patch,
		Supplemental:   raw.Supplemental,
		SourceFilePath: filePath,
	}

	// Consolidate Entity elements
	entities := append(raw.Entities, raw.EntitiesAlt...)
	for _, e := range entities {
		info.Entities = append(info.Entities, model.SWIDEntity{
			Name:  e.Name,
			RegID: e.RegID,
			Role:  e.Role,
		})
	}

	// Consolidate Link elements
	links := append(raw.Links, raw.LinksAlt...)
	for _, l := range links {
		info.Links = append(info.Links, model.SWIDLink{
			Rel:  l.Rel,
			Href: l.Href,
		})
	}

	return info, nil
}
