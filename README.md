# TertiusEye - Enterprise ITAM Distributed SaaS Platform

TertiusEye is an enterprise IT Asset Management (ITAM) platform built with Golang microservices and agent binaries, Amazon EKS container orchestration, and PostgreSQL for multi-tenant persistence.

---

## System Architecture Overview

```
                                 ┌─────────────────────────────────┐
                                 │   Endpoint Discovery Agent      │
                                 │   (Windows / macOS / Linux)     │
                                 └────────────────┬────────────────┘
                                                  │ mTLS (X.509) / Offline SQLite
                                                  ▼
                                 ┌─────────────────────────────────┐
                                 │      AWS API Gateway mTLS       │
                                 │  (Injects X-Tenant-ID Header)   │
                                 └────────────────┬────────────────┘
                                                  │ VPC Link
                                                  ▼
                                 ┌─────────────────────────────────┐
                                 │  Telemetry Ingestion Service    │
                                 │  (go-chi + 50 Goroutine Pool)   │
                                 └────────────────┬────────────────┘
                                                  │ RLS Tx (SET LOCAL app.current_tenant_id)
                                                  ▼
┌───────────────────────────┐    ┌─────────────────────────────────┐    ┌───────────────────────────┐
│   SaaS Discovery Service  │───►│       PostgreSQL Database       │◄───│  Cloud Discovery Service  │
│(MS Graph / Entra ID + 429)│    │(JSONB GIN Index + RLS Policies) │    │  (AWS STS AssumeRole)     │
└───────────────────────────┘    └─────────────────────────────────┘    └───────────────────────────┘
                                                  ▲
                                                  │ AWS KMS DEK Envelope Encryption
                                 ┌────────────────┴────────────────┐
                                 │   KMS Envelope Encryption       │
                                 │   (AES-256-GCM + DEK Zeroing)   │
                                 └─────────────────────────────────┘
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

### 3. Local PostgreSQL Environment (Docker)
```bash
docker-compose up -d
```
