package collector

import (
	"context"
	"testing"
)

func TestHardwareCollector(t *testing.T) {
	collector := NewHardwareCollector()
	data, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Hardware collection failed: %v", err)
	}

	if data.RAM.TotalBytes == 0 {
		t.Error("Expected RAM TotalBytes > 0")
	}

	if data.CPU.LogicalCores <= 0 {
		t.Errorf("Expected CPU LogicalCores > 0, got %d", data.CPU.LogicalCores)
	}
}
