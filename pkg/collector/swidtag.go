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
	TargetPaths []string
}

// NewSWIDCollector initializes a SWIDCollector with target search paths.
func NewSWIDCollector(paths []string) *SWIDCollector {
	return &SWIDCollector{
		TargetPaths: paths,
	}
}

// Collect walks configured target directories for .swidtag files AND scans system application folders for installed software.
func (s *SWIDCollector) Collect(ctx context.Context) (model.SoftwareInventory, error) {
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
	appTags := scanInstalledApplications(ctx)
	for _, tag := range appTags {
		addTag(tag)
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

		for _, dir := range appDirs {
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

				if entry.IsDir() && strings.HasSuffix(entry.Name(), ".app") {
					appPath := filepath.Join(dir, entry.Name())
					plistPath := filepath.Join(appPath, "Contents", "Info.plist")

					appName, version, bundleID := parseInfoPlist(plistPath)
					if appName == "" {
						appName = strings.TrimSuffix(entry.Name(), ".app")
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
						SourceFilePath: appPath,
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
					results = append(results, model.SWIDTagInfo{
						Name:           strings.Title(appName),
						Version:        "1.0.0",
						TagID:          "org.freedesktop." + appName,
						SourceFilePath: filepath.Join(dir, entry.Name()),
						Entities:       []model.SWIDEntity{{Name: "Open Source Community", Role: "softwareCreator"}},
					})
				}
			}
		}
	}

	return results
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
	} else if strings.Contains(lowerID, "microsoft") {
		return "Microsoft Corporation"
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

	return ParseSWIDTagXML(data, filePath)
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
