# TertiusEye - Comprehensive Application Build & Implementation Plan

This document serves as the master technical blueprint for **TertiusEye**, an enterprise IT Asset Management (ITAM) distributed SaaS platform. It explains the system architecture, codebase layout, security models, build pipelines, and execution roadmap based on the current codebase.

---

## 1. Architectural Overview & Design Goals

TertiusEye delivers unified IT asset management by combining endpoint agent telemetry, cloud asset discovery, and SaaS application usage polling into a multi-tenant PostgreSQL platform.

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
└───────────────────────────┘    ┌─────────────────────────────────┐    └───────────────────────────┘
                                                  ▲
                                                  │ AWS KMS DEK Envelope Encryption
                                 ┌────────────────┴────────────────┐
                                 │   KMS Envelope Encryption       │
                                 │   (AES-256-GCM + DEK Zeroing)   │
                                 └─────────────────────────────────┘
```

### Core Design Principles
1. **Multi-Tenancy Isolation**: Strictly enforced at the database layer using PostgreSQL Row-Level Security (RLS) with `SET LOCAL app.current_tenant_id`.
2. **Resilient Telemetry Ingestion**: High-throughput `go-chi` HTTP ingestion backed by a 50-worker goroutine pool and client-side offline SQLite queuing.
3. **Thundering Herd Protection**: Ticker-based agent discovery loops with randomized execution jitter up to 30 minutes.
4. **Envelope Cryptography**: Zero-trust AES-256-GCM encryption with AWS KMS DEK wrapping and RAM byte-slice zeroing.

---

## 2. Directory & Module Structure

```
TertiusEye/
├── cmd/
│   ├── agent/                 # Agent entrypoint (flag parsing, daemon/one-shot/UI modes)
│   ├── ingestion/             # Telemetry Ingestion Service entrypoint
│   ├── saasdisc/              # SaaS Discovery Service entrypoint
│   └── clouddisc/             # Cloud Discovery Service entrypoint
├── pkg/
│   ├── agent/                 # Core discovery ticker loop with randomized jitter
│   ├── cloud/                 # AWS STS AssumeRole client & resource scanners (EC2, S3)
│   ├── collector/             # Telemetry collectors (hardware, processes, SWIDtags)
│   ├── config/                # JSON config parser & TLS keypair validator
│   ├── crypto/                # AWS KMS Envelope Encryption (AES-256-GCM + DEK zeroing)
│   ├── database/              # PostgreSQL RLS repository (SET LOCAL app.current_tenant_id)
│   ├── ingestion/             # go-chi HTTP ingestion handlers & 50-worker pool
│   ├── model/                 # Data models & JSON telemetry payload schemas
│   ├── network/               # mTLS HTTP client & offline reconnection flusher
│   ├── saas/                  # MS Graph API client with HTTP 429 exponential backoff + jitter
│   └── storage/               # Offline SQLite queue (modernc.org/sqlite)
├── migrations/
│   └── 001_initial_schema.sql # PostgreSQL DDL, GIN indexes, RLS policies
├── web/
│   └── index.html             # Embedded single-file interactive Demo Web UI dashboard
├── config.example.json        # Agent runtime config template
├── docker-compose.yml         # Local PostgreSQL 16 test environment
├── Makefile                   # Build automation & cross-compilation pipeline
└── go.mod                     # Go module definitions (Go 1.24)
```

---

## 3. Subsystem Detailed Specification

### 3.1 Endpoint Discovery Agent (`cmd/agent`, `pkg/agent`, `pkg/collector`)
- **Execution Modes**:
  - `-one-shot`: Executes a single discovery collection pass and outputs JSON telemetry to stdout.
  - `-ui`: Starts an embedded HTTP dashboard at `http://localhost:8090` rendering telemetry interactively.
  - *Daemon Mode*: Continuous ticker loop with randomized jitter preventing API gateway spikes.
- **Telemetry Collectors**:
  - `HardwareCollector`: Collects CPU specs, RAM usage, host identifiers via `gopsutil/v3`.
  - `ProcessCollector`: Captures process table snapshots using concurrent worker pools.
  - `SWIDCollector`: Parses ISO/IEC 19770-2 `.swidtag` XML files and system installed applications (`/Applications`, `.desktop` entries).
- **Offline Resiliency**: When outbound mTLS POST fails, payloads are serialized into an embedded SQLite database (`offline_cache.db`). A background flusher re-transmits queued payloads once network connectivity recovers.

### 3.2 Telemetry Ingestion Microservice (`cmd/ingestion`, `pkg/ingestion`)
- Built on `go-chi/chi/v5` for allocation-free HTTP routing.
- Authenticates API requests via mTLS (`X-Tenant-ID` header injected by AWS API Gateway).
- Uses a buffered `chan IngestionTask` feeding a fixed 50-worker pool for backpressure control during high concurrency.

### 3.3 SaaS Discovery Service (`cmd/saasdisc`, `pkg/saas`)
- Authenticates against Microsoft Entra ID using `client_credentials` grant.
- Polls Microsoft Graph API for user license allocations and app activity.
- Handles HTTP 429 `TooManyRequests` with randomized backoff jitter up to 4 retry attempts.

### 3.4 Cloud Discovery Service (`cmd/clouddisc`, `pkg/cloud`)
- Retrieves tenant `RoleArn` and `ExternalId` from PostgreSQL.
- Executes AWS STS `AssumeRole` validating `ExternalId` to eliminate Confused Deputy attack vectors.
- Scans target AWS account EC2 instances and S3 buckets.

### 3.5 Security & Data Persistence Layer (`pkg/database`, `pkg/crypto`, `migrations`)
- **PostgreSQL RLS**: All tables (`devices`, `oauth_tokens`, `cloud_credentials`) enforce `ENABLE ROW LEVEL SECURITY`. Transactions execute `SET LOCAL app.current_tenant_id = $1` before any data query.
- **Envelope Encryption**: Generates 32-byte DEKs via `crypto/rand`, encrypts credentials with AES-256-GCM, wraps DEK via AWS KMS `Encrypt`, and explicitly zeroes out plaintext DEKs from memory (`pkg/crypto/envelope.go`).

---

## 4. Build, Compilation & Execution Instructions

### 4.1 Prerequisites
- Go 1.24+
- Docker & Docker Compose (for local PostgreSQL testing)
- CGO Enabled for running unit tests (due to SQLite test driver requirements)

### 4.2 Building Binaries via Makefile
```bash
# 1. Run full test suite
make test

# 2. Build local binaries for all 4 services
make build

# 3. Cross-compile static agent binaries for all target OS/ARCH combinations
make build-agent-all

# 4. Build all microservices and agent binaries
make build-all
```

**Compiled Artifact Locations (`bin/`)**:
- `bin/ingestion-service`
- `bin/saas-discovery-service`
- `bin/cloud-discovery-service`
- `bin/tertiuseye-agent-linux-amd64`
- `bin/tertiuseye-agent-linux-arm64`
- `bin/tertiuseye-agent-darwin-amd64`
- `bin/tertiuseye-agent-darwin-arm64`
- `bin/tertiuseye-agent-windows-amd64.exe`
- `bin/tertiuseye-agent-windows-arm64.exe`

### 4.3 Running Local Development Environment

1. **Spin up local PostgreSQL instance**:
   ```bash
   docker-compose up -d
   ```
2. **Run Interactive Demo Agent**:
   ```bash
   make demo
   # Opens browser dashboard at http://localhost:8090
   ```
3. **Run Telemetry Ingestion Service**:
   ```bash
   ./bin/ingestion-service -port 8080 -db "postgres://postgres:postgres@localhost:5432/tertiuseye?sslmode=disable" -workers 50
   ```

---

## 5. Development Roadmap & Implementation Tasks

### Phase 1: Test Suite Stabilization & Isolation Fix (Completed)
- [x] **Fix `SWIDCollector` Test Isolation**: Modified `SWIDCollector` in `pkg/collector/swidtag.go` to add an `IncludeSystemApps` flag so unit tests in isolated temporary directories do not walk host OS `/Applications`.
- [x] **Fix `TestSWIDCollectorWalkDir`**: Verified `go test -v ./...` passes 100% cleanly across all packages.

### Phase 2: Unit Test Coverage Expansion
- [ ] **Database RLS Package (`pkg/database`)**: Add mock unit tests verifying `SET LOCAL app.current_tenant_id` SQL query generation and connection pooling.
- [ ] **Ingestion Package (`pkg/ingestion`)**: Add HTTP test handlers validating payload parsing and worker queue buffer bounds.
- [ ] **Network mTLS Package (`pkg/network`)**: Add test cases for client cert loading and reconnect/flush logic.

### Phase 3: Observability & Production Readiness
- [ ] **Health Check Endpoints**: Implement `/healthz` and `/readyz` endpoints in `ingestion-service`.
- [ ] **Prometheus Metrics Exporter**: Expose ingestion task rates, worker pool saturation, and SQLite cache depths.
- [ ] **Helm Deployment Charts**: Provide Kubernetes manifests for deploying services to AWS EKS with AWS Load Balancer Controller mTLS termination.

---

## 6. Verification Plan

### Automated Verification
Run the complete test suite:
```bash
make test
```

### Manual Verification
1. Run static agent cross-compilation:
   ```bash
   make build-all
   ```
2. Execute agent demo mode and verify Web UI components:
   ```bash
   make demo
   ```
