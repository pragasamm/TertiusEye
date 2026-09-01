package collector

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// Collect walks the configured target directories searching for .swidtag files and parses them.
func (s *SWIDCollector) Collect(ctx context.Context) (model.SoftwareInventory, error) {
	var inventory model.SoftwareInventory

	for _, root := range s.TargetPaths {
		select {
		case <-ctx.Done():
			return inventory, ctx.Err()
		default:
		}

		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Skip permission denied or inaccessible files/directories
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Don't follow symlinked directories to prevent infinite loops
			if d.IsDir() {
				if d.Type()&os.ModeSymlink != 0 {
					return filepath.SkipDir
				}
				return nil
			}

			if strings.HasSuffix(strings.ToLower(d.Name()), ".swidtag") {
				tag, err := ParseSWIDTagFile(path)
				if err == nil {
					inventory.SWIDTags = append(inventory.SWIDTags, tag)
				}
			}

			return nil
		})

		if err != nil && err != ctx.Err() {
			// Log error or continue to next target path
		}
	}

	inventory.TotalDiscovered = len(inventory.SWIDTags)
	return inventory, nil
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
