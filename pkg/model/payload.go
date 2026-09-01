package model

import "time"

// AgentMetadata contains device identification and environment info.
type AgentMetadata struct {
	TenantID   string    `json:"tenant_id"`
	DeviceUUID string    `json:"device_uuid"`
	Hostname   string    `json:"hostname"`
	OS         string    `json:"os"`
	Platform   string    `json:"platform"`
	Arch       string    `json:"arch"`
	Timestamp  time.Time `json:"timestamp"`
}

// CPUInfo represents CPU specification and utilization metrics.
type CPUInfo struct {
	ModelName      string    `json:"model_name"`
	Cores          int32     `json:"cores"`
	LogicalCores   int       `json:"logical_cores"`
	Mhz            float64   `json:"mhz"`
	VendorID       string    `json:"vendor_id"`
	CPUUsagePercent float64  `json:"cpu_usage_percent"`
}

// RAMInfo represents system memory telemetry.
type RAMInfo struct {
	TotalBytes     uint64  `json:"total_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

// HardwareTelemetry aggregates CPU and RAM telemetry.
type HardwareTelemetry struct {
	CPU CPUInfo `json:"cpu"`
	RAM RAMInfo `json:"ram"`
}

// ProcessInfo represents running process details.
type ProcessInfo struct {
	PID            int32   `json:"pid"`
	Name           string  `json:"name"`
	ExecutablePath string  `json:"executable_path"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryBytes    uint64  `json:"memory_bytes"`
	MemoryPercent  float32 `json:"memory_percent"`
	Status         string  `json:"status"`
	Username       string  `json:"username"`
	CreateTime     int64   `json:"create_time_ms"`
}

// SWIDEntity represents software tag entity metadata (ISO/IEC 19770-2).
type SWIDEntity struct {
	Name  string `json:"name" xml:"name,attr"`
	RegID string `json:"regid,omitempty" xml:"regid,attr"`
	Role  string `json:"role,omitempty" xml:"role,attr"`
}

// SWIDLink represents links specified in software tag.
type SWIDLink struct {
	Rel  string `json:"rel,omitempty" xml:"rel,attr"`
	Href string `json:"href,omitempty" xml:"href,attr"`
}

// SWIDTagInfo represents parsed ISO/IEC 19770-2 software identification tag.
type SWIDTagInfo struct {
	Name           string       `json:"name"`
	Version        string       `json:"version"`
	TagID          string       `json:"tag_id"`
	Patch          bool         `json:"is_patch"`
	Supplemental   bool         `json:"is_supplemental"`
	Entities       []SWIDEntity `json:"entities,omitempty"`
	Links          []SWIDLink   `json:"links,omitempty"`
	SourceFilePath string       `json:"source_file_path"`
}

// SoftwareInventory contains all discovered software tags.
type SoftwareInventory struct {
	SWIDTags        []SWIDTagInfo `json:"swid_tags"`
	TotalDiscovered int           `json:"total_discovered"`
}

// DiscoveryPayload is the full payload sent during discovery scan.
type DiscoveryPayload struct {
	Metadata  AgentMetadata     `json:"metadata"`
	Hardware  HardwareTelemetry `json:"hardware"`
	Processes []ProcessInfo     `json:"processes"`
	Software  SoftwareInventory `json:"software"`
}
