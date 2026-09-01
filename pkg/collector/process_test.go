package collector

import (
	"context"
	"os"
	"testing"
)

func TestProcessCollector(t *testing.T) {
	collector := NewProcessCollector()
	procs, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Process collection failed: %v", err)
	}

	if len(procs) == 0 {
		t.Fatal("Expected at least 1 running process")
	}

	// Verify current process PID is in the extracted process list
	currentPID := int32(os.Getpid())
	foundCurrent := false
	for _, p := range procs {
		if p.PID == currentPID {
			foundCurrent = true
			if p.PID <= 0 {
				t.Errorf("Invalid PID: %d", p.PID)
			}
			break
		}
	}

	if !foundCurrent {
		t.Errorf("Current process PID %d was not found in extracted process list", currentPID)
	}
}
