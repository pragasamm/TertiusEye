package agent

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"tertiuseye/agent/pkg/collector"
	"tertiuseye/agent/pkg/config"
	"tertiuseye/agent/pkg/model"
)

// PayloadHandler is a callback invoked whenever a discovery payload is produced.
type PayloadHandler func(ctx context.Context, payload *model.DiscoveryPayload) error

// Agent orchestrates startup initialization, ticker loop with randomized jitter, and discovery runs.
type Agent struct {
	cfg     *config.Config
	engine  *collector.Engine
	handler PayloadHandler
	rand    *rand.Rand
}

// NewAgent constructs a new Agent instance.
func NewAgent(cfg *config.Config, handler PayloadHandler) *Agent {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &Agent{
		cfg:     cfg,
		engine:  collector.NewEngine(cfg),
		handler: handler,
		rand:    r,
	}
}

// CalculateJitter returns a random duration between 0 and maxJitter.
func (a *Agent) CalculateJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	return time.Duration(a.rand.Int63n(int64(maxJitter)))
}

// RunOneShot executes a single discovery collection pass immediately.
func (a *Agent) RunOneShot(ctx context.Context) (*model.DiscoveryPayload, error) {
	payload, err := a.engine.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("discovery collection pass failed: %w", err)
	}

	if a.handler != nil {
		if err := a.handler(ctx, payload); err != nil {
			return payload, fmt.Errorf("payload handler execution failed: %w", err)
		}
	}

	return payload, nil
}

// Start Execution Loop: initial execution followed by periodic ticker with randomized jitter.
func (a *Agent) Start(ctx context.Context) error {
	fmt.Printf("[Agent] Starting Discovery Agent loop. TenantID: %s, DeviceUUID: %s\n", a.cfg.TenantID, a.cfg.DeviceUUID)
	fmt.Printf("[Agent] Ticker Base Interval: %s, Max Jitter: %s\n", a.cfg.ScanInterval, a.cfg.MaxJitter)

	// Execute initial discovery run immediately upon startup
	fmt.Println("[Agent] Performing initial startup discovery run...")
	if _, err := a.RunOneShot(ctx); err != nil {
		fmt.Printf("[Agent] Warning: initial discovery run returned error: %v\n", err)
	}

	ticker := time.NewTicker(a.cfg.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("[Agent] Shutdown signal received. Stopping execution loop.")
			return ctx.Err()

		case <-ticker.C:
			// Calculate randomized jitter to prevent thundering herd problem
			jitter := a.CalculateJitter(a.cfg.MaxJitter)
			fmt.Printf("[Agent] Ticker fired. Applying randomized jitter delay of %s before execution...\n", jitter)

			select {
			case <-ctx.Done():
				fmt.Println("[Agent] Context cancelled during jitter delay.")
				return ctx.Err()
			case <-time.After(jitter):
			}

			fmt.Println("[Agent] Starting scheduled discovery extraction...")
			if _, err := a.RunOneShot(ctx); err != nil {
				fmt.Printf("[Agent] Error during scheduled discovery extraction: %v\n", err)
			}
		}
	}
}
