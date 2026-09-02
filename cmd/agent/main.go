package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"tertiuseye/agent/pkg/agent"
	"tertiuseye/agent/pkg/config"
	"tertiuseye/agent/pkg/model"
	"tertiuseye/agent/pkg/network"
	"tertiuseye/agent/pkg/storage"
)

//go:embed web/index.html
var uiHTML []byte

func main() {
	configPath := flag.String("config", "config.json", "Path to provisioned agent configuration file")
	oneShot := flag.Bool("one-shot", false, "Execute a single discovery run and print output to stdout, then exit")
	verifyCert := flag.Bool("verify-cert", false, "Verify X.509 client certificate loading and exit")
	endpointURL := flag.String("endpoint", "", "Ingestion API endpoint URL (overrides default or mTLS endpoint)")
	dbPath := flag.String("sqlite-db", "offline_cache.db", "Path to local embedded SQLite offline queue database")
	showUI := flag.Bool("ui", false, "Launch interactive Web UI dashboard for one-shot telemetry demonstration")
	uiPort := flag.Int("port", 8090, "Port for the interactive Demo Web UI dashboard")
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

	// Initialize local SQLite offline queue (LLD Section 2.2)
	sqliteQueue, err := storage.NewSQLiteQueue(*dbPath)
	if err != nil {
		fmt.Printf("[Agent] Warning: failed to initialize SQLite offline queue at %s: %v\n", *dbPath, err)
	} else {
		defer sqliteQueue.Close()
	}

	// Payload output handler
	handler := func(ctx context.Context, payload *model.DiscoveryPayload) error {
		output, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format payload JSON: %w", err)
		}
		if *oneShot && !*showUI && *endpointURL == "" {
			fmt.Println(string(output))
		} else {
			fmt.Printf("[Agent] Collected telemetry payload: CPU model='%s', RAM total=%d, Processes=%d, SWID tags=%d\n",
				payload.Hardware.CPU.ModelName, payload.Hardware.RAM.TotalBytes, len(payload.Processes), payload.Software.TotalDiscovered)
		}
		return nil
	}

	ag := agent.NewAgent(cfg, handler)

	// Configure mTLS HTTP client if endpoint is specified or cert/key exist
	if *endpointURL != "" || (cfg.CertPath != "" && cfg.KeyPath != "") {
		targetURL := *endpointURL
		if targetURL == "" {
			targetURL = "https://ingest.yourdomain.com/api/v1/telemetry"
		}
		mtlsClient, err := network.NewClient(cfg.CertPath, cfg.KeyPath, targetURL, cfg.TenantID, sqliteQueue)
		if err != nil {
			fmt.Printf("[Agent] Warning: mTLS client setup error: %v\n", err)
		} else {
			ag.SetNetworkClient(mtlsClient, sqliteQueue)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Web UI Demo Mode
	if *showUI {
		uiURL := fmt.Sprintf("http://localhost:%d", *uiPort)
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(uiHTML)
		})

		mux.HandleFunc("/api/scan", func(w http.ResponseWriter, r *http.Request) {
			payload, err := ag.RunOneShot(r.Context())
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payload)
		})

		server := &http.Server{Addr: fmt.Sprintf(":%d", *uiPort), Handler: mux}

		go func() {
			fmt.Printf("\n===========================================================\n")
			fmt.Printf(" 🚀 TertiusEye Endpoint Agent Demo Dashboard Running!\n")
			fmt.Printf(" 🌐 Open in Browser: %s\n", uiURL)
			fmt.Printf("===========================================================\n\n")

			// Auto open browser on macOS/Linux/Windows
			switch runtime.GOOS {
			case "darwin":
				_ = exec.Command("open", uiURL).Start()
			case "windows":
				_ = exec.Command("cmd", "/c", "start", uiURL).Start()
			case "linux":
				_ = exec.Command("xdg-open", uiURL).Start()
			}

			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Printf("[Demo UI] Server error: %v\n", err)
			}
		}()

		<-ctx.Done()
		fmt.Println("\n[Demo UI] Shutting down web server...")
		_ = server.Shutdown(context.Background())
		return
	}

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
