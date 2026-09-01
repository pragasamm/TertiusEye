package collector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSWIDTagXML(t *testing.T) {
	xmlPayload := `<?xml version="1.0" encoding="utf-8"?>
<SoftwareIdentity
    xmlns="http://standards.iso.org/iso/19770/-2/2015/schema.xsd"
    name="TertiusEye Endpoint Security Agent"
    version="2.1.0"
    tagId="com.tertiuseye.agent-2.1.0"
    patch="false"
    supplemental="false">
    <Entity name="TertiusEye Security Inc" regid="tertiuseye.com" role="softwareCreator licensor"/>
    <Entity name="Enterprise IT Admin" role="tagCreator"/>
    <Link rel="installation" href="https://tertiuseye.com/agent/2.1.0"/>
</SoftwareIdentity>`

	tag, err := ParseSWIDTagXML([]byte(xmlPayload), "/path/to/sample.swidtag")
	if err != nil {
		t.Fatalf("ParseSWIDTagXML failed: %v", err)
	}

	if tag.Name != "TertiusEye Endpoint Security Agent" {
		t.Errorf("Expected Name 'TertiusEye Endpoint Security Agent', got '%s'", tag.Name)
	}
	if tag.Version != "2.1.0" {
		t.Errorf("Expected Version '2.1.0', got '%s'", tag.Version)
	}
	if tag.TagID != "com.tertiuseye.agent-2.1.0" {
		t.Errorf("Expected TagID 'com.tertiuseye.agent-2.1.0', got '%s'", tag.TagID)
	}
	if tag.Patch {
		t.Errorf("Expected Patch false, got true")
	}
	if len(tag.Entities) != 2 {
		t.Fatalf("Expected 2 entities, got %d", len(tag.Entities))
	}
	if tag.Entities[0].Name != "TertiusEye Security Inc" {
		t.Errorf("Expected Entity Name 'TertiusEye Security Inc', got '%s'", tag.Entities[0].Name)
	}
	if len(tag.Links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(tag.Links))
	}
	if tag.Links[0].Href != "https://tertiuseye.com/agent/2.1.0" {
		t.Errorf("Expected Link Href 'https://tertiuseye.com/agent/2.1.0', got '%s'", tag.Links[0].Href)
	}
}

func TestSWIDCollectorWalkDir(t *testing.T) {
	tempDir := t.TempDir()

	subDir := filepath.Join(tempDir, "installed_apps", "app1")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subDir: %v", err)
	}

	swidContent := `<?xml version="1.0" encoding="utf-8"?>
<SoftwareIdentity name="Sample Application" version="1.0.0" tagId="sample-app-1.0">
    <Entity name="Vendor Corp" role="publisher"/>
</SoftwareIdentity>`

	swidPath := filepath.Join(subDir, "app.swidtag")
	if err := os.WriteFile(swidPath, []byte(swidContent), 0644); err != nil {
		t.Fatalf("Failed to write swidtag file: %v", err)
	}

	// Create non-swidtag file to test filtering
	if err := os.WriteFile(filepath.Join(subDir, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("Failed to write txt file: %v", err)
	}

	collector := NewSWIDCollector([]string{tempDir})
	inventory, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if inventory.TotalDiscovered != 1 {
		t.Fatalf("Expected 1 swidtag discovered, got %d", inventory.TotalDiscovered)
	}

	discovered := inventory.SWIDTags[0]
	if discovered.Name != "Sample Application" {
		t.Errorf("Expected discovered tag name 'Sample Application', got '%s'", discovered.Name)
	}
}
