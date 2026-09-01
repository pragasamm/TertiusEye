# TertiusEye - Endpoint Discovery Agent (InvMan)

TertiusEye is an enterprise endpoint discovery system. The Endpoint Discovery Agent is a statically compiled Go binary designed for deployment across Windows, macOS, and Linux endpoints via MDM solutions (e.g., Microsoft Intune, Jamf Pro).

---

## Key Features

- **Core Execution Loop with Jitter**:
  - Uses `time.NewTicker` to wake up periodically (e.g., every 4 hours).
  - Includes a **randomized jitter** of up to 30 minutes to prevent the "thundering herd" problem across fleet deployments.
  - Supports graceful shutdown on OS signals (`SIGINT`, `SIGTERM`).

- **Provisioned Device Identity & Mutual TLS**:
  - Initializes via provisioned configuration containing `Tenant ID`, `Device UUID`, and local paths to X.509 client certificate and private key (`cert.pem`, `key.pem`).

- **Hardware Telemetry Extraction**:
  - Leverages `github.com/shirou/gopsutil/v3` for cross-platform CPU metrics (model name, core count, utilization) and RAM stats (total, available, used, percentage).

- **Process Inventory Extraction**:
  - Concurrent worker-pool process collector for sub-second table extraction (PID, process name, executable path, CPU utilization %, RSS memory bytes/%, process status, user/owner, creation timestamp).

- **Software Inventory (.swidtag)**:
  - Iterates through target installation directories using `filepath.WalkDir`.
  - Discovers and parses ISO/IEC 19770-2 compliant `.swidtag` XML files (software name, version, tag ID, patch status, entity roles, links).

---

## Directory & Package Architecture

```
TertiusEye/
├── cmd/
│   └── agent/
│       └── main.go           # CLI entry point, flag parsing & signal handling
├── pkg/
│   ├── agent/
│   │   ├── agent.go          # Core ticker loop with randomized jitter
│   │   └── agent_test.go     # Agent unit tests
│   ├── collector/
│   │   ├── collector.go      # Unified discovery pass aggregator
│   │   ├── hardware.go       # gopsutil CPU & RAM collector
│   │   ├── hardware_test.go  # Hardware collector tests
│   │   ├── process.go        # Concurrent process collector
│   │   ├── process_test.go   # Process collector tests
│   │   ├── swidtag.go        # ISO/IEC 19770-2 .swidtag XML parser & walker
│   │   └── swidtag_test.go   # SWID tag parser tests
│   ├── config/
│   │   ├── config.go         # Config loader & X.509 cert validator
│   │   └── config_test.go    # Config unit tests
│   └── model/
│       └── payload.go        # Discovery payload data schemas
├── config.example.json       # Sample provisioned configuration
├── Makefile                  # Cross-platform compilation targets
├── go.mod
└── go.sum
```

---

## Configuration

Configuration parameters are provisioned via JSON:

```json
{
  "tenant_id": "tenant-8f92a1b0-4c31-11ee-be56-0242ac120002",
  "device_uuid": "device-3c90a1b0-4c31-11ee-be56-0242ac120002",
  "cert_path": "/etc/tertiuseye/cert.pem",
  "key_path": "/etc/tertiuseye/key.pem",
  "scan_interval": "4h",
  "max_jitter": "30m",
  "software_scan_paths": [
    "/Applications",
    "/Library",
    "/usr/local"
  ]
}
```

---

## Building & Testing

### 1. Run Unit Tests
```bash
go test -v ./...
```

### 2. Build Cross-Platform Static Binaries (`CGO_ENABLED=0`)
```bash
make build-all
```

Binaries will be output to `bin/`:
- `bin/tertiuseye-agent-linux-amd64`
- `bin/tertiuseye-agent-linux-arm64`
- `bin/tertiuseye-agent-darwin-amd64`
- `bin/tertiuseye-agent-darwin-arm64`
- `bin/tertiuseye-agent-windows-amd64.exe`
- `bin/tertiuseye-agent-windows-arm64.exe`

---

## Usage Modes

### Daemon Mode (Default Ticker Loop)
```bash
./bin/tertiuseye-agent-darwin-arm64 -config config.json
```

### One-Shot Discovery Pass
```bash
./bin/tertiuseye-agent-darwin-arm64 -config config.json -one-shot
```

### Verify TLS Client Certificate
```bash
./bin/tertiuseye-agent-darwin-arm64 -config config.json -verify-cert
```
