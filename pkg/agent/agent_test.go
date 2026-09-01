package agent

import (
	"context"
	"testing"
	"time"

	"tertiuseye/agent/pkg/config"
	"tertiuseye/agent/pkg/model"
)

func TestCalculateJitter(t *testing.T) {
	cfg := &config.Config{
		TenantID:   "test-tenant",
		DeviceUUID: "test-device",
		MaxJitter:  30 * time.Minute,
	}

	ag := NewAgent(cfg, nil)

	for i := 0; i < 100; i++ {
		jitter := ag.CalculateJitter(cfg.MaxJitter)
		if jitter < 0 {
			t.Errorf("Jitter cannot be negative: %v", jitter)
		}
		if jitter >= cfg.MaxJitter {
			t.Errorf("Jitter %v must be strictly less than MaxJitter %v", jitter, cfg.MaxJitter)
		}
	}
}

func TestAgentRunOneShot(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		TenantID:          "tenant-unit-test",
		DeviceUUID:        "device-unit-test",
		SoftwareScanPaths: []string{tempDir},
	}

	var captured *model.DiscoveryPayload
	handler := func(ctx context.Context, payload *model.DiscoveryPayload) error {
		captured = payload
		return nil
	}

	ag := NewAgent(cfg, handler)
	payload, err := ag.RunOneShot(context.Background())
	if err != nil {
		t.Fatalf("RunOneShot failed: %v", err)
	}

	if payload == nil || captured == nil {
		t.Fatal("Expected non-nil discovery payload")
	}

	if captured.Metadata.TenantID != "tenant-unit-test" {
		t.Errorf("Expected TenantID 'tenant-unit-test', got '%s'", captured.Metadata.TenantID)
	}
	if captured.Metadata.DeviceUUID != "device-unit-test" {
		t.Errorf("Expected DeviceUUID 'device-unit-test', got '%s'", captured.Metadata.DeviceUUID)
	}
}

func TestAgentLoopShutdown(t *testing.T) {
	cfg := &config.Config{
		TenantID:          "tenant-test",
		DeviceUUID:        "device-test",
		ScanInterval:      50 * time.Millisecond,
		MaxJitter:         5 * time.Millisecond,
		SoftwareScanPaths: []string{t.TempDir()},
	}

	ag := NewAgent(cfg, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := ag.Start(ctx)
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("Expected context cancellation error, got %v", err)
	}
}
