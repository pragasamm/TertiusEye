package collector

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v3/process"

	"tertiuseye/agent/pkg/model"
)

// ProcessCollector extracts system running processes via gopsutil using concurrent workers.
type ProcessCollector struct {
	Workers int
}

// NewProcessCollector creates a new ProcessCollector instance.
func NewProcessCollector() *ProcessCollector {
	workers := runtime.NumCPU() * 2
	if workers < 4 {
		workers = 4
	}
	if workers > 32 {
		workers = 32
	}
	return &ProcessCollector{
		Workers: workers,
	}
}

// Collect lists all active processes and extracts telemetry concurrently.
func (p *ProcessCollector) Collect(ctx context.Context) ([]model.ProcessInfo, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list processes: %w", err)
	}

	numProcs := len(procs)
	if numProcs == 0 {
		return nil, nil
	}

	procChan := make(chan *process.Process, numProcs)
	for _, proc := range procs {
		procChan <- proc
	}
	close(procChan)

	resultsChan := make(chan model.ProcessInfo, numProcs)
	var wg sync.WaitGroup

	for i := 0; i < p.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for proc := range procChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				info := model.ProcessInfo{
					PID: proc.Pid,
				}

				if name, err := proc.NameWithContext(ctx); err == nil {
					info.Name = name
				}

				if exe, err := proc.ExeWithContext(ctx); err == nil {
					info.ExecutablePath = exe
				}

				if cpuPercent, err := proc.CPUPercentWithContext(ctx); err == nil {
					info.CPUPercent = cpuPercent
				}

				if memInfo, err := proc.MemoryInfoWithContext(ctx); err == nil && memInfo != nil {
					info.MemoryBytes = memInfo.RSS
				}

				if memPercent, err := proc.MemoryPercentWithContext(ctx); err == nil {
					info.MemoryPercent = memPercent
				}

				if status, err := proc.StatusWithContext(ctx); err == nil {
					info.Status = strings.Join(status, ",")
				}

				if user, err := proc.UsernameWithContext(ctx); err == nil {
					info.Username = user
				}

				if createTime, err := proc.CreateTimeWithContext(ctx); err == nil {
					info.CreateTime = createTime
				}

				resultsChan <- info
			}
		}()
	}

	wg.Wait()
	close(resultsChan)

	var results []model.ProcessInfo
	for res := range resultsChan {
		results = append(results, res)
	}

	return results, nil
}
