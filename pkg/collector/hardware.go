package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	"tertiuseye/agent/pkg/model"
)

// HardwareCollector collects CPU and RAM telemetry using gopsutil.
type HardwareCollector struct{}

// NewHardwareCollector creates a new HardwareCollector instance.
func NewHardwareCollector() *HardwareCollector {
	return &HardwareCollector{}
}

// Collect extracts CPU and RAM metrics.
func (h *HardwareCollector) Collect(ctx context.Context) (model.HardwareTelemetry, error) {
	var telemetry model.HardwareTelemetry

	// Collect RAM metrics
	vmem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return telemetry, fmt.Errorf("failed to extract virtual memory metrics: %w", err)
	}

	telemetry.RAM = model.RAMInfo{
		TotalBytes:     vmem.Total,
		AvailableBytes: vmem.Available,
		UsedBytes:      vmem.Used,
		UsedPercent:    vmem.UsedPercent,
	}

	// Collect CPU metrics
	infos, err := cpu.InfoWithContext(ctx)
	if err == nil && len(infos) > 0 {
		info := infos[0]
		telemetry.CPU.ModelName = info.ModelName
		telemetry.CPU.Cores = info.Cores
		telemetry.CPU.Mhz = info.Mhz
		telemetry.CPU.VendorID = info.VendorID
	}

	logicalCores, err := cpu.CountsWithContext(ctx, true)
	if err == nil {
		telemetry.CPU.LogicalCores = logicalCores
	}

	// Fetch short sampling of CPU usage
	percents, err := cpu.PercentWithContext(ctx, 100*time.Millisecond, false)
	if err == nil && len(percents) > 0 {
		telemetry.CPU.CPUUsagePercent = percents[0]
	}

	return telemetry, nil
}
