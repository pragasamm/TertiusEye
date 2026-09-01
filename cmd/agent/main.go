package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tertiuseye/agent/pkg/agent"
	"tertiuseye/agent/pkg/config"
	"tertiuseye/agent/pkg/model"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to provisioned agent configuration file")
	oneShot := flag.Bool("one-shot", false, "Execute a single discovery run and print output to stdout, then exit")
	verifyCert := flag.Bool("verify-cert", false, "Verify X.509 client certificate loading and exit")
	flag.Parse()

	if *configPath == "" {
		fmt.Println("Error: -config path is required")
		os.Exit(1)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("Initialization Error: %v\n", err)
		os.Exit(1)
	}

	if *verifyCert {
		_, err := cfg.LoadTLSKeyPair()
		if err != nil {
			fmt.Printf("TLS X.509 KeyPair Validation Failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("TLS X.509 KeyPair loaded successfully!")
		return
	}

	// Payload output handler
	handler := func(ctx context.Context, payload *model.DiscoveryPayload) error {
		output, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format payload JSON: %w", err)
		}
		if *oneShot {
			fmt.Println(string(output))
		} else {
			fmt.Printf("[Agent] Collected telemetry payload: CPU model='%s', RAM total=%d, Processes=%d, SWID tags=%d\n",
				payload.Hardware.CPU.ModelName, payload.Hardware.RAM.TotalBytes, len(payload.Processes), payload.Software.TotalDiscovered)
		}
		return nil
	}

	ag := agent.NewAgent(cfg, handler)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if *oneShot {
		fmt.Println("[Agent] Running in one-shot discovery mode...")
		if _, err := ag.RunOneShot(ctx); err != nil {
			fmt.Printf("Error executing one-shot scan: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := ag.Start(ctx); err != nil && err != context.Canceled {
		fmt.Printf("Agent exited with error: %v\n", err)
		os.Exit(1)
	}
}
