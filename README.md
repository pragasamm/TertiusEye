# TertiusEye - Enterprise ITAM Distributed SaaS Platform

TertiusEye is an enterprise IT Asset Management (ITAM) platform built with Golang microservices and agent binaries, Amazon EKS container orchestration, and PostgreSQL for multi-tenant persistence.

## Overview & Core Architecture

* **Endpoint Agent (`cmd/agent`)**: Cross-platform (Windows, macOS, Linux) agent collecting hardware metrics, process data, and ISO/IEC 19770-2 `.swidtag` software inventory. Features mutual TLS (mTLS) authentication and offline SQLite payload queuing (`pkg/storage`).
* **Telemetry Ingestion Service (`cmd/ingestion`)**: High-concurrency `go-chi` microservice with a 50-goroutine worker pool for ingesting telemetry from endpoints.
* **SaaS Discovery Service (`cmd/saasdisc`)**: Integrates with Microsoft Entra ID / MS Graph API to discover SaaS application usage and licenses.
* **Cloud Discovery Service (`cmd/clouddisc`)**: Performs cross-account AWS cloud asset discovery (EC2, S3) using AWS STS `AssumeRole`.
* **Multi-Tenant Persistence (`migrations/001_initial_schema.sql`)**: PostgreSQL database utilizing Row-Level Security (RLS) for multi-tenant data isolation and JSONB GIN indexing.
* **Envelope Encryption (`pkg/crypto`)**: AWS KMS envelope encryption for sensitive OAuth tokens and credentials using AES-256-GCM with memory-clearing safeguards.

---

## System Architecture Overview

*For full subsystem diagrams and component flows, see [ARCHITECTURE.md](ARCHITECTURE.md).*

```mermaid
flowchart TB
    %% --- Subgraphs ---
    subgraph Presentation["1. Presentation & Management UI"]
        UserBrowser["IT Administrators & SOC Security Teams<br/>(Web Browser / OIDC Auth)"]
        ManagementConsole["Enterprise Management Console<br/>(cmd/webconsole & web/)<br/>• ITAM Dashboard & Cloud Asset Explorer<br/>• Telemetry & SaaS Usage Analytics"]

        UserBrowser -- "HTTPS / OIDC JWT Auth" --> ManagementConsole
    end

    subgraph Endpoints["2. Endpoint Infrastructure (Win / macOS / Linux)"]
        AgentDaemon["Endpoint Discovery Agent<br/>(cmd/agent)<br/>• Hardware, Process & SWIDtag Collectors<br/>• Ticker with 30m Execution Jitter"]
        SQLiteCache[("Offline SQLite Queue<br/>(offline_cache.db)")]
        
        AgentDaemon <--> SQLiteCache
    end

    subgraph Ingress["3. AWS Ingress Layer"]
        APIGateway["AWS API Gateway<br/>• mTLS Client Cert Verification<br/>• OIDC JWT Tenant Claim Validation<br/>• Injects X-Tenant-ID Header"]
        VPCLink["AWS VPC Link / ALB"]
        
        APIGateway --> VPCLink
    end

    subgraph Microservices["4. Microservice Engine (ECS / EKS)"]
        IngestionSvc["Telemetry Ingestion Service<br/>(cmd/ingestion)<br/>• 50-Goroutine Worker Pool"]
        WebConsoleSvc["Management API Server<br/>(cmd/webconsole)<br/>• RLS Tenant Query Engine"]
        SaaSDiscSvc["SaaS Discovery Service<br/>(cmd/saasdisc)<br/>• Entra ID / Graph Poller"]
        CloudDiscSvc["Cloud Discovery Service<br/>(cmd/clouddisc)<br/>• AWS STS AssumeRole Scanner"]
    end

    subgraph ExternalServices["5. Cloud Provider Integrations"]
        MSGraph["Microsoft Entra ID & Graph API"]
        AWSSTS["AWS Target Accounts (EC2 & S3)"]
    end

    subgraph Persistence["6. Persistence & Security Layer"]
        PostgreSQL[("PostgreSQL 16 Database<br/>• Tenant Row-Level Security (RLS)<br/>• JSONB GIN Hardware Indexing")]
        AWSKMS["AWS KMS<br/>• AES-256-GCM Envelope Encryption<br/>• RAM DEK Zeroing Safeguard"]
    end

    %% --- Flow Connections ---
    AgentDaemon -- "mTLS HTTPS POST" --> APIGateway
    ManagementConsole -- "HTTPS API Requests" --> APIGateway
    
    VPCLink --> IngestionSvc
    VPCLink --> WebConsoleSvc
    
    IngestionSvc -- "Write Telemetry (RLS Tx)" --> PostgreSQL
    WebConsoleSvc -- "Read Assets & Cloud (RLS Tx)" --> PostgreSQL
    
    SaaSDiscSvc <--> MSGraph
    SaaSDiscSvc -- "Store SaaS Inventory" --> PostgreSQL

    CloudDiscSvc <--> AWSSTS
    CloudDiscSvc -- "Store Cloud Inventory" --> PostgreSQL

    PostgreSQL <--> AWSKMS
    WebConsoleSvc -- "Envelope Decryption Request" --> AWSKMS

    %% --- Modern Color Styling ---
    style Presentation fill:#0f172a,stroke:#818cf8,stroke-width:2px,color:#f8fafc
    style Endpoints fill:#0f172a,stroke:#38bdf8,stroke-width:2px,color:#f8fafc
    style Ingress fill:#0f172a,stroke:#fb7185,stroke-width:2px,color:#f8fafc
    style Microservices fill:#0f172a,stroke:#a78bfa,stroke-width:2px,color:#f8fafc
    style ExternalServices fill:#0f172a,stroke:#f59e0b,stroke-width:2px,color:#f8fafc
    style Persistence fill:#0f172a,stroke:#34d399,stroke-width:2px,color:#f8fafc

    style UserBrowser fill:#4f46e5,stroke:#818cf8,stroke-width:2px,color:#ffffff
    style ManagementConsole fill:#6366f1,stroke:#a5b4fc,stroke-width:2px,color:#ffffff
    style AgentDaemon fill:#0284c7,stroke:#38bdf8,stroke-width:2px,color:#ffffff
    style SQLiteCache fill:#7c3aed,stroke:#a78bfa,stroke-width:2px,color:#ffffff
    style APIGateway fill:#e11d48,stroke:#fb7185,stroke-width:2px,color:#ffffff
    style VPCLink fill:#d97706,stroke:#fbbf24,stroke-width:2px,color:#ffffff
    style IngestionSvc fill:#059669,stroke:#34d399,stroke-width:2px,color:#ffffff
    style SaaSDiscSvc fill:#0284c7,stroke:#38bdf8,stroke-width:2px,color:#ffffff
    style CloudDiscSvc fill:#d97706,stroke:#fbbf24,stroke-width:2px,color:#ffffff
    style WebConsoleSvc fill:#6366f1,stroke:#a5b4fc,stroke-width:2px,color:#ffffff
    style MSGraph fill:#4f46e5,stroke:#818cf8,stroke-width:2px,color:#ffffff
    style AWSSTS fill:#e11d48,stroke:#fb7185,stroke-width:2px,color:#ffffff
    style AWSKMS fill:#059669,stroke:#34d399,stroke-width:2px,color:#ffffff
    style PostgreSQL fill:#2563eb,stroke:#60a5fa,stroke-width:2px,color:#ffffff
```

---

## Key Modules & Features

### 1. Endpoint Discovery Agent (`cmd/agent`)
- **Ticker & Randomized Jitter**: `time.NewTicker` execution loop (e.g. 4 hours) with randomized jitter up to 30 minutes preventing thundering herd spikes.
- **Hardware & Process Telemetry**: Uses `gopsutil/v3` to extract CPU specs, RAM metrics, and concurrent worker-pool process table data.
- **Software Inventory**: Parses ISO/IEC 19770-2 `.swidtag` XML payloads found during filesystem traversal (`filepath.WalkDir`).
- **Offline Caching (SQLite)**: If outbound network connectivity fails, JSON discovery payloads are serialized and enqueued into an embedded SQLite database (`modernc.org/sqlite`). A background goroutine polls network connectivity and flushes cached payloads upon reconnection.
- **mTLS Network Client**: Mutual TLS authentication using MDM-provisioned X.509 certificate and private key.

### 2. Telemetry Ingestion Microservice (`cmd/ingestion`)
- Powered by `go-chi/chi/v5` for fast, allocation-free HTTP routing.
- Extracts `X-Tenant-ID` header injected by AWS API Gateway mTLS clientCert verification.
- Uses a buffered channel (`chan IngestionTask`) feeding a fixed pool of 50 worker goroutines per pod to handle high-concurrency spikes.

### 3. SaaS Discovery Microservice (`cmd/saasdisc`)
- Authenticates against Microsoft Entra ID via `golang.org/x/oauth2/clientcredentials`.
- Implements rate-limit backoff with randomized jitter on HTTP 429 `TooManyRequests`.

### 4. Cloud Discovery Microservice (`cmd/clouddisc`)
- Retrieves tenant `RoleArn` and `ExternalId` from PostgreSQL.
- Calls AWS STS `AssumeRole`, strictly validating `ExternalId` to mitigate Confused Deputy vulnerabilities.
- Scans cross-account EC2 instances and S3 bucket resources.

### 5. Multi-Tenant Persistence Layer (`migrations/001_initial_schema.sql`, `pkg/database`)
- Multi-tenant PostgreSQL database (`tenants`, `devices`, `oauth_tokens`, `cloud_credentials`).
- High-speed GIN index on `hardware_specs` JSONB column (`idx_devices_hardware_specs`).
- Strict Row-Level Security (RLS): Enforces tenant data isolation (`ENABLE ROW LEVEL SECURITY`). Go database repository executes `SET LOCAL app.current_tenant_id = $1` inside transactions prior to any query.

### 6. Envelope Cryptography (`pkg/crypto`)
- Generates 32-byte DEKs using `crypto/rand`.
- Encrypts OAuth tokens using AES-256-GCM (`crypto/aes`, `cipher.NewGCM`).
- Wraps DEK using AWS KMS `Encrypt` / `Decrypt` API.
- **Security Safeguard**: Immediately zeroes out plaintext DEK byte slices in RAM upon completion to prevent memory scraping.

---

## Directory Architecture

```
TertiusEye/
├── cmd/
│   ├── agent/                # Endpoint Discovery Agent CLI
│   ├── ingestion/            # Telemetry Ingestion Service (go-chi + worker pool)
│   ├── saasdisc/             # SaaS Discovery Service (Entra ID / MS Graph)
│   └── clouddisc/            # Cloud Discovery Service (AWS STS AssumeRole)
├── pkg/
│   ├── agent/                # Agent core loop & ticker with jitter
│   ├── cloud/                # AWS STS AssumeRole client & resource discovery
│   ├── collector/            # Hardware, Process, and SWIDtag collectors
│   ├── config/               # Config parsing & TLS keypair validator
│   ├── crypto/               # AWS KMS Envelope Encryption & DEK zeroing
│   ├── database/             # PostgreSQL RLS repository (SET LOCAL tenant_id)
│   ├── ingestion/            # Ingestion worker pool & HTTP handlers
│   ├── model/                # Payload schemas & telemetry models
│   ├── network/              # mTLS HTTP client & reconnection flusher
│   ├── saas/                 # Microsoft Graph API polling with 429 backoff
│   └── storage/              # Offline SQLite database storage queue
├── migrations/
│   └── 001_initial_schema.sql # PostgreSQL DDL, GIN indexes, RLS policies
├── docker-compose.yml        # PostgreSQL 16 local testing environment
├── config.example.json       # Agent configuration example
└── Makefile                  # Build & test automation targets
```

---

## Building & Running

### 1. Run Unit Test Suite
```bash
go test -v ./...
```

### 2. Build All Microservices & Static Agent Binaries
```bash
make build-all
```

Binaries will be output to `bin/`:
- `bin/ingestion-service`
- `bin/saas-discovery-service`
- `bin/cloud-discovery-service`
- `bin/tertiuseye-agent-linux-amd64`
- `bin/tertiuseye-agent-linux-arm64`
- `bin/tertiuseye-agent-darwin-amd64`
- `bin/tertiuseye-agent-darwin-arm64`
- `bin/tertiuseye-agent-windows-amd64.exe`
- `bin/tertiuseye-agent-windows-arm64.exe`

### 3. Running the Binaries

#### A. Endpoint Discovery Agent (`tertiuseye-agent-*`)
Run the binary matching your platform (e.g. `tertiuseye-agent-darwin-arm64` on Apple Silicon macOS):

- **Interactive Demo Web UI Mode** (Zero DB/AWS dependency - opens live dashboard in browser at `http://localhost:8090`):
  ```bash
  make demo
  # OR
  ./bin/tertiuseye-agent-darwin-arm64 -config config.example.json -ui -port 8090
  ```

- **One-Shot Discovery Mode** (Executes a single discovery run and outputs JSON telemetry to stdout):
  ```bash
  ./bin/tertiuseye-agent-darwin-arm64 -config config.example.json -one-shot
  ```

- **Continuous Daemon Mode** (Runs in background using ticker with randomized jitter):
  ```bash
  ./bin/tertiuseye-agent-darwin-arm64 -config config.example.json
  ```

- **Agent Command-Line Flags**:
  - `-ui`: Launch interactive embedded Web UI dashboard (opens `http://localhost:8090`).
  - `-port <number>`: Port for the Demo Web UI server (default `8090`).
  - `-config <path>`: Path to agent configuration file (default `config.json`).
  - `-one-shot`: Run a single telemetry collection cycle and output JSON to stdout.
  - `-endpoint <url>`: Override default ingestion service URL endpoint.
  - `-verify-cert`: Verify X.509 client certificate loading and exit.
  - `-sqlite-db <path>`: Path to offline queue SQLite database (default `offline_cache.db`).

#### B. Telemetry Ingestion Microservice (`ingestion-service`)
Starts the high-concurrency `go-chi` HTTP ingestion server:
```bash
./bin/ingestion-service -port 8080 -db "postgres://postgres:postgres@localhost:5432/tertiuseye?sslmode=disable" -workers 50
```

#### C. SaaS Discovery Microservice (`saas-discovery-service`)
Runs Microsoft Entra ID / Graph API discovery:
```bash
./bin/saas-discovery-service -tenant "your-tenant-id" -client-id "your-client-id" -client-secret "your-secret"
```

#### D. Cloud Discovery Microservice (`cloud-discovery-service`)
Runs AWS STS `AssumeRole` cross-account EC2 and S3 discovery:
```bash
./bin/cloud-discovery-service -role-arn "arn:aws:iam::123456789012:role/TertiusEyeRole"
```

### 4. Local PostgreSQL Environment (Docker)
```bash
docker-compose up -d
```
