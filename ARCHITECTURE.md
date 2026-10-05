# TertiusEye - Enterprise System Architecture & Component Diagrams

This document details the high-level and subsystem architecture diagrams for **TertiusEye**, an enterprise IT Asset Management (ITAM) distributed SaaS platform built with Golang microservices, multi-platform agent binaries, AWS EKS, and PostgreSQL.

---

## 1. High-Level System Architecture Overview

The platform unifies multi-platform endpoint telemetry, SaaS application license discovery, and AWS cloud infrastructure scanning into a central multi-tenant PostgreSQL database protected with Row-Level Security (RLS) and AWS KMS envelope encryption.

```mermaid
flowchart TB
    subgraph Endpoints["Endpoint Infrastructure (Windows / macOS / Linux)"]
        AgentDaemon["Endpoint Discovery Agent (cmd/agent)\n- Ticker + 30m Jitter Loop\n- Hardware / Process / SWIDtag Collectors"]
        SQLiteCache[("Offline SQLite Cache\n(offline_cache.db)")]
        LocalUI["Demo Web UI Dashboard\n(http://localhost:8090)"]

        AgentDaemon <--> SQLiteCache
        AgentDaemon --> LocalUI
    end

    subgraph Ingress["AWS Infrastructure & Ingress Layer"]
        APIGateway["AWS API Gateway (mTLS)\n- Verifies X.509 Client Certs\n- Injects X-Tenant-ID Header"]
        VPCLink["AWS VPC Link / ALB"]
        
        APIGateway --> VPCLink
    end

    subgraph Microservices["Kubernetes / EKS Microservice Layer"]
        IngestionSvc["Telemetry Ingestion Service (cmd/ingestion)\n- go-chi/v5 Router\n- 50-Goroutine Worker Pool\n- Backpressure Channel Buffer"]
        SaaSDiscSvc["SaaS Discovery Service (cmd/saasdisc)\n- MS Entra ID Client Credentials\n- Graph API + HTTP 429 Jitter Backoff"]
        CloudDiscSvc["Cloud Discovery Service (cmd/clouddisc)\n- AWS STS AssumeRole (ExternalId)\n- EC2 & S3 Resource Scanners"]
    end

    subgraph ExternalServices["External Cloud Integrations"]
        MSGraph["Microsoft Graph API / Entra ID"]
        AWSSTS["AWS Target Accounts (STS / EC2 / S3)"]
        AWSKMS["AWS KMS (Envelope Encryption)"]
    end

    subgraph Persistence["Persistence & Security Layer"]
        PostgreSQL[("PostgreSQL Database (v16)\n- Tenant Row-Level Security (RLS)\n- JSONB GIN Hardware Indexing\n- Devices / SaaS / Cloud Schemas")]
    end

    %% Flow Connections
    AgentDaemon -- "mTLS POST Payload" --> APIGateway
    VPCLink --> IngestionSvc
    IngestionSvc -- "SET LOCAL app.current_tenant_id\nRLS Transaction" --> PostgreSQL
    
    SaaSDiscSvc <--> MSGraph
    SaaSDiscSvc -- "Store SaaS Inventory" --> PostgreSQL

    CloudDiscSvc <--> AWSSTS
    CloudDiscSvc -- "Store Cloud Inventory" --> PostgreSQL

    PostgreSQL <--> AWSKMS
```

---

## 2. Endpoint Agent Internal Architecture

The endpoint agent operates cross-platform (Windows, macOS, Linux) in continuous daemon mode, one-shot mode, or interactive local Web UI mode.

```mermaid
flowchart TD
    subgraph AgentCore["Agent Runtime Core (pkg/agent)"]
        Ticker["Ticker Engine\n(Default: 4h execution)"]
        Jitter["Randomized Jitter Generator\n(0 to 30 min delay)"]
        Runner["Discovery Executor"]
        
        Ticker --> Jitter --> Runner
    end

    subgraph TelemetryCollectors["Telemetry Collectors (pkg/collector)"]
        HW["Hardware Collector\n(gopsutil/v3 CPU, RAM, Host)"]
        Proc["Process Collector\n(Worker Pool Process Table)"]
        SWID["SWIDtag Collector\n(ISO/IEC 19770-2 XML & OS Apps)"]

        Runner --> HW
        Runner --> Proc
        Runner --> SWID
    end

    subgraph DataPipeline["Data Processing & Transport (pkg/network & pkg/storage)"]
        Aggregator["Telemetry Payload Aggregator\n(pkg/model)"]
        NetworkCheck{"mTLS Connection\nAvailable?"}
        MTLSClient["mTLS HTTP Client\n(X.509 Certificate)"]
        SQLiteQueue[("Offline SQLite Queue\n(modernc.org/sqlite)")]
        BackgroundFlusher["Background Reconnection Flusher"]

        HW --> Aggregator
        Proc --> Aggregator
        SWID --> Aggregator

        Aggregator --> NetworkCheck
        NetworkCheck -- "Yes" --> MTLSClient
        NetworkCheck -- "No (Offline)" --> SQLiteQueue
        SQLiteQueue <--> BackgroundFlusher
        BackgroundFlusher -- "Flushes Cached Payloads" --> MTLSClient
    end

    subgraph IngestTarget["Ingestion Ingress"]
        APIGW["AWS API Gateway mTLS"]
        MTLSClient -- "HTTPS mTLS POST" --> APIGW
    end
```

---

## 3. High-Concurrency Telemetry Ingestion Pipeline

The Telemetry Ingestion Service handles incoming agent telemetry via a bounded buffer channel feeding a fixed 50-worker goroutine pool.

```mermaid
sequenceDiagram
    autonumber
    participant Agent as Endpoint Agent
    participant APIGW as AWS API Gateway (mTLS)
    participant Chi as go-chi HTTP Router
    participant Queue as Buffered Task Channel (chan IngestionTask)
    participant Pool as Worker Pool (50 Goroutines)
    participant DB as PostgreSQL (RLS Enabled)

    Agent->>APIGW: POST /api/v1/telemetry (mTLS X.509 Cert)
    APIGW->>APIGW: Validate Cert & Inject X-Tenant-ID Header
    APIGW->>Chi: HTTP POST with X-Tenant-ID header
    Chi->>Chi: Validate Payload Schema & Tenant Header
    Chi->>Queue: Enqueue IngestionTask to channel
    Chi-->>Agent: HTTP 202 Accepted (Immediate Response)
    
    loop Worker Processing Loop
        Pool->>Queue: Dequeue IngestionTask
        Pool->>DB: Begin DB Transaction
        Pool->>DB: SET LOCAL app.current_tenant_id = 'tenant-123'
        Pool->>DB: UPSERT Device Record & Hardware Specs (JSONB)
        Pool->>DB: Commit Transaction
    end
```

---

## 4. Multi-Tenant Persistence & Envelope Encryption Flow

Tenant data isolation is enforced at the PostgreSQL engine level via Row-Level Security (RLS). Sensitive credentials use AWS KMS Envelope Encryption with memory-clearing safeguards.

```mermaid
flowchart TD
    subgraph AppLayer["Application Layer (pkg/crypto & pkg/database)"]
        Repo["PostgreSQL RLS Repository"]
        Crypto["Crypto Manager (pkg/crypto)"]
        PlaintextToken["Sensitive OAuth Token / Credential"]
    end

    subgraph KMSEnvelope["AWS KMS Envelope Encryption Flow"]
        GenerateDEK["Generate 32-Byte Data Encryption Key (DEK)\n(crypto/rand)"]
        EncryptData["Encrypt Payload using AES-256-GCM\n(crypto/aes + cipher.NewGCM)"]
        KMSWrap["AWS KMS Encrypt API\n(Wraps DEK with AWS Master Key)"]
        ZeroMemory["Safeguard: Zero Plaintext DEK in RAM\n(memclear / byte zeroing)"]

        PlaintextToken --> GenerateDEK
        GenerateDEK --> EncryptData
        GenerateDEK --> KMSWrap
        EncryptData & KMSWrap --> ZeroMemory
    end

    subgraph StorageLayer["Multi-Tenant Persistence (PostgreSQL)"]
        DBTx["Database Transaction\n(SET LOCAL app.current_tenant_id = $1)"]
        TableDevices[("devices Table\n(RLS Policy: tenant_id = current_setting)")]
        TableTokens[("oauth_tokens Table\n(Stores Encrypted DEK + Ciphertext)")]

        ZeroMemory --> DBTx
        DBTx --> TableDevices & TableTokens
    end

    Repo --> DBTx
```

---

## 5. SaaS & Cloud Discovery Microservices Flow

```mermaid
flowchart LR
    subgraph SaaSDisc["SaaS Discovery Service (cmd/saasdisc)"]
        OAuth2["OAuth2 Client Credentials\n(golang.org/x/oauth2)"]
        GraphClient["MS Graph API Poller"]
        BackoffEngine["Exponential Backoff & Jitter\n(Handles HTTP 429 Too Many Requests)"]

        OAuth2 --> GraphClient --> BackoffEngine
    end

    subgraph CloudDisc["Cloud Discovery Service (cmd/clouddisc)"]
        DBLookup["Query Target Tenant Role ARN & ExternalId"]
        STSAssume["AWS STS AssumeRole\n(Validates ExternalId for Confused Deputy Protection)"]
        EC2Scanner["EC2 Instance Scanner"]
        S3Scanner["S3 Bucket Scanner"]

        DBLookup --> STSAssume --> EC2Scanner & S3Scanner
    end

    subgraph Targets["Target Cloud Providers"]
        EntraID["Microsoft Entra ID / Graph API"]
        TargetAWS["Target AWS Account Resources"]
    end

    subgraph DB["PostgreSQL Database"]
        TenantDB[("Multi-Tenant DB (RLS)")]
    end

    BackoffEngine <--> EntraID
    EC2Scanner & S3Scanner <--> TargetAWS

    BackoffEngine -- "Write SaaS Inventory" --> TenantDB
    EC2Scanner & S3Scanner -- "Write Cloud Inventory" --> TenantDB
```

---

## 6. Directory Architecture & Module Mapping

| Directory / Package | Architectural Responsibility | Key Interfaces & Dependencies |
| :--- | :--- | :--- |
| `cmd/agent` | Endpoint Discovery Agent binary entrypoint | Command flags (`-ui`, `-one-shot`), runtime lifecycle |
| `cmd/ingestion` | Telemetry Ingestion Service daemon entrypoint | `go-chi/v5`, PostgreSQL pool, worker configuration |
| `cmd/saasdisc` | SaaS Discovery Service entrypoint | Microsoft Entra ID OAuth2 configuration |
| `cmd/clouddisc` | Cloud Discovery Service entrypoint | AWS SDK v2, STS AssumeRole client |
| `pkg/agent` | Core ticker discovery engine with jitter | `time.Ticker`, `math/rand` jitter engine |
| `pkg/collector` | Hardware, Process, and SWIDtag collectors | `gopsutil/v3`, ISO/IEC 19770-2 XML parser |
| `pkg/config` | Config JSON parsing and TLS cert validator | `crypto/tls`, `encoding/json` |
| `pkg/crypto` | AWS KMS Envelope Encryption & DEK zeroing | `crypto/aes`, `crypto/cipher`, AWS KMS SDK |
| `pkg/database` | PostgreSQL multi-tenant repository with RLS | `database/sql`, `SET LOCAL app.current_tenant_id` |
| `pkg/ingestion` | Ingestion HTTP router & 50-worker pool | `go-chi/v5`, buffered channel `chan IngestionTask` |
| `pkg/model` | Canonical JSON telemetry models & schemas | Go structs |
| `pkg/network` | mTLS HTTP client & offline buffer flusher | `net/http`, X.509 cert pool |
| `pkg/saas` | MS Graph API client with HTTP 429 backoff | `golang.org/x/oauth2/clientcredentials` |
| `pkg/storage` | Embedded offline SQLite storage queue | `modernc.org/sqlite` |
| `migrations/` | PostgreSQL DDL, GIN indexes, RLS security policies | SQL migrations (`001_initial_schema.sql`) |
| `web/` | Embedded single-file interactive Demo Web UI | Embedded HTML/JS dashboard |
