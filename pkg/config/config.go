package config

import "crypto/tls"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Config represents the agent provisioned configuration parameters.
type Config struct {
	TenantID          string        `json:"tenant_id"`
	DeviceUUID        string        `json:"device_uuid"`
	CertPath          string        `json:"cert_path"`
	KeyPath           string        `json:"key_path"`
	ScanIntervalStr   string        `json:"scan_interval"`
	MaxJitterStr      string        `json:"max_jitter"`
	SoftwareScanPaths []string      `json:"software_scan_paths,omitempty"`
	ScanInterval      time.Duration `json:"-"`
	MaxJitter         time.Duration `json:"-"`
}

// DefaultConfig returns default configuration parameters.
func DefaultConfig() *Config {
	cfg := &Config{
		TenantID:          "",
		DeviceUUID:        "",
		CertPath:          "cert.pem",
		KeyPath:           "key.pem",
		ScanIntervalStr:   "4h",
		MaxJitterStr:      "30m",
		SoftwareScanPaths: defaultScanPaths(),
	}
	cfg.ParseDurations()
	return cfg
}

// LoadConfig loads configuration from a JSON file path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file at %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	if err := cfg.ParseDurations(); err != nil {
		return nil, err
	}

	if len(cfg.SoftwareScanPaths) == 0 {
		cfg.SoftwareScanPaths = defaultScanPaths()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// ParseDurations parses interval and jitter duration strings.
func (c *Config) ParseDurations() error {
	var err error
	if c.ScanIntervalStr != "" {
		c.ScanInterval, err = time.ParseDuration(c.ScanIntervalStr)
		if err != nil {
			return fmt.Errorf("invalid scan_interval '%s': %w", c.ScanIntervalStr, err)
		}
	} else {
		c.ScanInterval = 4 * time.Hour
	}

	if c.MaxJitterStr != "" {
		c.MaxJitter, err = time.ParseDuration(c.MaxJitterStr)
		if err != nil {
			return fmt.Errorf("invalid max_jitter '%s': %w", c.MaxJitterStr, err)
		}
	} else {
		c.MaxJitter = 30 * time.Minute
	}

	return nil
}

// Validate checks for required configuration fields.
func (c *Config) Validate() error {
	if c.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if c.DeviceUUID == "" {
		return fmt.Errorf("device_uuid is required")
	}
	if c.CertPath == "" {
		return fmt.Errorf("cert_path is required")
	}
	if c.KeyPath == "" {
		return fmt.Errorf("key_path is required")
	}
	return nil
}

// LoadTLSKeyPair loads and validates the X.509 client certificate and private key.
func (c *Config) LoadTLSKeyPair() (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(c.CertPath, c.KeyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load X.509 keypair from cert='%s' key='%s': %w", c.CertPath, c.KeyPath, err)
	}
	return cert, nil
}

// defaultScanPaths returns standard platform-specific installation paths for .swidtag scanning.
func defaultScanPaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Program Files`,
			`C:\Program Files (x86)`,
			`C:\ProgramData`,
		}
	case "darwin":
		return []string{
			"/Applications",
			"/Library",
			"/usr/local",
		}
	default: // linux and others
		return []string{
			"/usr/share",
			"/usr/local",
			"/var/lib",
			"/opt",
		}
	}
}

// CleanPaths expands and cleans configured scan paths.
func (c *Config) CleanPaths() []string {
	var cleaned []string
	for _, p := range c.SoftwareScanPaths {
		absPath, err := filepath.Abs(p)
		if err == nil {
			cleaned = append(cleaned, absPath)
		} else {
			cleaned = append(cleaned, filepath.Clean(p))
		}
	}
	return cleaned
}
