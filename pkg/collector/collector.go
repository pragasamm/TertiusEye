package collector

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"tertiuseye/agent/pkg/config"
	"tertiuseye/agent/pkg/model"
)

// Engine orchestrates all telemetry and inventory collectors.
type Engine struct {
	cfg       *config.Config
	hw        *HardwareCollector
	proc      *ProcessCollector
	swid      *SWIDCollector
}

// NewEngine creates a new Engine initialized with agent configuration.
func NewEngine(cfg *config.Config) *Engine {
	return &Engine{
		cfg:  cfg,
		hw:   NewHardwareCollector(),
		proc: NewProcessCollector(),
		swid: NewSWIDCollector(cfg.CleanPaths()),
	}
}

// Collect executes a full discovery pass across hardware, processes, and software inventory.
func (e *Engine) Collect(ctx context.Context) (*model.DiscoveryPayload, error) {
	hostname, _ := os.Hostname()

	payload := &model.DiscoveryPayload{
		Metadata: model.AgentMetadata{
			TenantID:   e.cfg.TenantID,
			DeviceUUID: e.cfg.DeviceUUID,
			Hostname:   hostname,
			OS:         runtime.GOOS,
			Platform:   runtime.GOOS,
			Arch:       runtime.GOARCH,
			Timestamp:  time.Now().UTC(),
		},
	}

	// 1. Collect Hardware Telemetry
	hwData, err := e.hw.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("hardware collection failed: %w", err)
	}
	payload.Hardware = hwData

	// 2. Collect Process Telemetry
	procData, err := e.proc.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("process collection failed: %w", err)
	}
	payload.Processes = procData

	// 3. Collect SWID Tag Software Inventory
	swData, err := e.swid.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("software inventory collection failed: %w", err)
	}
	payload.Software = swData

	return payload, nil
}
