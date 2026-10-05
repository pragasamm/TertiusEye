# TertiusEye - Enterprise System Architecture & Component Diagrams

This document details the high-level and subsystem architecture diagrams for **TertiusEye**, an enterprise IT Asset Management (ITAM) distributed SaaS platform built with Golang microservices, multi-platform agent binaries, AWS EKS/ECS Fargate, and PostgreSQL.

---

## 1. High-Level System Architecture Overview

The platform unifies multi-platform endpoint telemetry, SaaS application license discovery, and AWS cloud infrastructure scanning into a central multi-tenant PostgreSQL database protected with Row-Level Security (RLS) and AWS KMS envelope encryption.

The **Presentation Layer (Enterprise Management Web Console)** allows IT administrators and SOC security teams to securely query, inspect, and analyze all ingested ITAM data (Endpoint Telemetry, AWS EC2/S3 Assets, SaaS License Usage) isolated by tenant context.

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

## 2. Presentation Layer & Management Console Retrieval Flow

The Presentation Layer provides IT Administrators with real-time visibility into ingested AWS Cloud Assets (EC2, S3), Endpoint Telemetry, and SaaS License Allocations while maintaining strict multi-tenant Row-Level Security (RLS) and Envelope Decryption.

```mermaid
sequenceDiagram
    autonumber
    participant Admin as IT Admin / SOC User
    participant Browser as Web Browser (Presentation UI)
    participant APIGW as AWS API Gateway (JWT Auth)
    participant Svc as Management API & Web Server (cmd/webconsole)
    participant DB as PostgreSQL (RLS Enabled)
    participant KMS as AWS KMS

    Admin->>Browser: Open Enterprise ITAM Dashboard
    Browser->>APIGW: GET /api/v1/assets/cloud (Bearer OIDC JWT Token)
    APIGW->>APIGW: Validate JWT & Extract Tenant ID ('tenant-abc')
    APIGW->>Svc: Forward Request with X-Tenant-ID: tenant-abc
    
    Svc->>DB: Begin DB Transaction
    Svc->>DB: SET LOCAL app.current_tenant_id = 'tenant-abc'
    Svc->>DB: SELECT * FROM cloud_credentials, devices, saas_inventory
    DB-->>Svc: Return Encrypted Credentials & Ingested AWS Assets (EC2, S3)
    
    Svc->>KMS: Decrypt DEK via AWS KMS API
    KMS-->>Svc: Return Plaintext DEK
    Svc->>Svc: Decrypt AES-256-GCM Payload & Zero Plaintext DEK in RAM
    
    Svc-->>Browser: Return JSON ITAM Asset & AWS Cloud Inventory Data
    Browser-->>Admin: Render AWS EC2/S3 Assets, Devices & SaaS License Visualizations
```

---

## 3. Endpoint Agent Internal Architecture

The endpoint agent operates cross-platform (Windows, macOS, Linux) in continuous daemon mode, one-shot mode, or interactive local Web UI mode.

```mermaid
flowchart TD
    subgraph AgentCore["Agent Runtime Core (pkg/agent)"]
        Ticker["Ticker Engine<br/>(Default: 4h execution)"]
        Jitter["Randomized Jitter Generator<br/>(0 to 30 min delay)"]
        Runner["Discovery Executor"]
        
        Ticker --> Jitter --> Runner
    end

    subgraph TelemetryCollectors["Telemetry Collectors (pkg/collector)"]
        HW["Hardware Collector<br/>(gopsutil/v3 CPU, RAM, Host)"]
        Proc["Process Collector<br/>(Worker Pool Process Table)"]
        SWID["SWIDtag Collector<br/>(ISO/IEC 19770-2 XML & OS Apps)"]

        Runner --> HW
        Runner --> Proc
        Runner --> SWID
    end

    subgraph DataPipeline["Data Processing & Transport (pkg/network & pkg/storage)"]
        Aggregator["Telemetry Payload Aggregator<br/>(pkg/model)"]
        NetworkCheck{"mTLS Connection<br/>Available?"}
        MTLSClient["mTLS HTTP Client<br/>(X.509 Certificate)"]
        SQLiteQueue[("Offline SQLite Queue<br/>(modernc.org/sqlite)")]
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

## 4. High-Concurrency Telemetry Ingestion Pipeline

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

## 5. Multi-Tenant Persistence & Envelope Encryption Flow

Tenant data isolation is enforced at the PostgreSQL engine level via Row-Level Security (RLS). Sensitive credentials use AWS KMS Envelope Encryption with memory-clearing safeguards.

```mermaid
flowchart TD
    subgraph AppLayer["Application Layer (pkg/crypto & pkg/database)"]
        Repo["PostgreSQL RLS Repository"]
        Crypto["Crypto Manager (pkg/crypto)"]
        PlaintextToken["Sensitive OAuth Token / Credential"]
    end

    subgraph KMSEnvelope["AWS KMS Envelope Encryption Flow"]
        GenerateDEK["Generate 32-Byte Data Encryption Key (DEK)<br/>(crypto/rand)"]
        EncryptData["Encrypt Payload using AES-256-GCM<br/>(crypto/aes + cipher.NewGCM)"]
        KMSWrap["AWS KMS Encrypt API<br/>(Wraps DEK with AWS Master Key)"]
        ZeroMemory["Safeguard: Zero Plaintext DEK in RAM<br/>(memclear / byte zeroing)"]

        PlaintextToken --> GenerateDEK
        GenerateDEK --> EncryptData
        GenerateDEK --> KMSWrap
        EncryptData --> ZeroMemory
        KMSWrap --> ZeroMemory
    end

    subgraph StorageLayer["Multi-Tenant Persistence (PostgreSQL)"]
        DBTx["Database Transaction<br/>(SET LOCAL app.current_tenant_id = $1)"]
        TableDevices[("devices Table<br/>(RLS Policy: tenant_id = current_setting)")]
        TableTokens[("oauth_tokens Table<br/>(Stores Encrypted DEK + Ciphertext)")]

        ZeroMemory --> DBTx
        DBTx --> TableDevices
        DBTx --> TableTokens
    end

    Repo --> DBTx
```

---

## 6. SaaS & Cloud Discovery Microservices Flow

```mermaid
flowchart LR
    subgraph SaaSDisc["SaaS Discovery Service (cmd/saasdisc)"]
        OAuth2["OAuth2 Client Credentials<br/>(golang.org/x/oauth2)"]
        GraphClient["MS Graph API Poller"]
        BackoffEngine["Exponential Backoff & Jitter<br/>(Handles HTTP 429 Too Many Requests)"]

        OAuth2 --> GraphClient --> BackoffEngine
    end

    subgraph CloudDisc["Cloud Discovery Service (cmd/clouddisc)"]
        DBLookup["Query Target Tenant Role ARN & ExternalId"]
        STSAssume["AWS STS AssumeRole<br/>(Validates ExternalId for Confused Deputy Protection)"]
        EC2Scanner["EC2 Instance Scanner"]
        S3Scanner["S3 Bucket Scanner"]

        DBLookup --> STSAssume
        STSAssume --> EC2Scanner
        STSAssume --> S3Scanner
    end

    subgraph Targets["Target Cloud Providers"]
        EntraID["Microsoft Entra ID / Graph API"]
        TargetAWS["Target AWS Account Resources"]
    end

    subgraph DB["PostgreSQL Database"]
        TenantDB[("Multi-Tenant DB (RLS)")]
    end

    BackoffEngine <--> EntraID
    EC2Scanner <--> TargetAWS
    S3Scanner <--> TargetAWS

    BackoffEngine -- "Write SaaS Inventory" --> TenantDB
    EC2Scanner -- "Write Cloud Inventory" --> TenantDB
    S3Scanner -- "Write Cloud Inventory" --> TenantDB
```

---

## 7. Security Architecture & Controls

TertiusEye incorporates defense-in-depth security mechanisms across transport, storage, cross-account authorization, and application runtime layers.

```mermaid
flowchart TB
    subgraph DefenseDepth["Defense-in-Depth Security Controls"]
        subgraph NetworkSec["1. Transport Security"]
            mTLS["Mutual TLS (mTLS X.509 Certs)<br/>• MDM-provisioned Client Certs<br/>• Gateway Cert Verification"]
        end

        subgraph TenantSec["2. Multi-Tenant Isolation"]
            RLS["PostgreSQL Row-Level Security (RLS)<br/>• SET LOCAL app.current_tenant_id = $1<br/>• Engine-level Table Isolation"]
        end

        subgraph CryptoSec["3. Cryptographic Protection"]
            KMS["AWS KMS Envelope Encryption<br/>• AES-256-GCM Payload Encryption<br/>• Immediate RAM DEK Zeroing"]
        end

        subgraph CloudSec["4. Cross-Account Authorization"]
            STS["AWS STS AssumeRole<br/>• Mandatory ExternalId Verification<br/>• Confused Deputy Defense"]
        end

        subgraph ResiliencySec["5. DDoS & Rate Protection"]
            Jitter["Execution Jitter & Backoff<br/>• 30m Ticker Jitter Engine<br/>• HTTP 429 Exponential Backoff"]
        end
    end
```

### Key Security Specifications

1. **Zero-Trust Mutual TLS (mTLS)**:
   - Endpoint agents authenticate to AWS API Gateway using client X.509 certificates provisioned via Enterprise MDM.
   - API Gateway verifies client certificate authenticity and injects `X-Tenant-ID` into downstream headers.

2. **Engine-Level Multi-Tenant Isolation (PostgreSQL RLS)**:
   - Every database table (`devices`, `oauth_tokens`, `cloud_credentials`, `saas_inventory`) enforces `ENABLE ROW LEVEL SECURITY`.
   - Go repository transactions ([`pkg/database/repository.go`](pkg/database/repository.go)) execute `SET LOCAL app.current_tenant_id = $1` prior to executing any SQL queries, preventing cross-tenant data leaks at the database engine level.

3. **Envelope Encryption & RAM Protection**:
   - Generates 32-byte Data Encryption Keys (DEKs) via `crypto/rand`.
   - Encrypts sensitive credentials using AES-256-GCM (`crypto/aes`).
   - Wraps DEKs using AWS KMS `Encrypt` API.
   - **RAM Safeguard**: Plaintext DEK byte slices are explicitly zeroed out in memory (`pkg/crypto/envelope.go`) immediately after encryption to mitigate cold-boot memory scraping.

4. **Confused Deputy Defense**:
   - Cross-account AWS scanning strictly enforces `ExternalId` verification in AWS STS `AssumeRole` calls ([`pkg/cloud/aws.go`](pkg/cloud/aws.go)), protecting target cloud accounts from unauthorized third-party role assumption.

---

## 8. Data Portability & Interoperability Architecture

TertiusEye is architected to avoid vendor lock-in, leveraging open standards, canonical schemas, and standard database DDL for maximum data portability.

```mermaid
flowchart LR
    subgraph IngestionSources["Ingested Data Specs"]
        SWIDtag["ISO/IEC 19770-2 SWIDtag XML<br/>(Standard Software Schema)"]
        TelemetryJSON["Canonical JSON Schemas<br/>(Hardware, Processes, Host)"]
        SQLiteCache[("Embedded Offline SQLite<br/>(offline_cache.db)")]
    end

    subgraph PlatformEngine["TertiusEye Core Engine"]
        Postgres[("Multi-Tenant PostgreSQL<br/>(JSONB GIN Indexing)")]
    end

    subgraph ExportPortability["Open Data Export & Integration"]
        RESTAPI["REST / JSON Open APIs<br/>(go-chi/v5 Endpoints)"]
        ITAMExport["Enterprise ITAM Systems<br/>(ServiceNow, Flexera, Snow)"]
        SIEMIntegration["SIEM / Security Logging<br/>(Splunk, Datadog, CloudWatch)"]
    end

    SWIDtag & TelemetryJSON <--> SQLiteCache
    SQLiteCache --> Postgres
    Postgres --> RESTAPI
    RESTAPI --> ITAMExport
    RESTAPI --> SIEMIntegration
```

### Key Data Portability Features

1. **Standard ISO/IEC 19770-2 SWIDtag Support**:
   - Software discovery leverages international ISO/IEC 19770-2 XML standards ([`pkg/collector/swidtag.go`](pkg/collector/swidtag.go)), ensuring full compatibility and data exportability to enterprise ITAM platforms (ServiceNow, Flexera, Snow Software).

2. **Canonical Open JSON Schemas**:
   - Telemetry metrics use transparent, un-vendor-locked JSON schemas ([`pkg/model/payload.go`](pkg/model/payload.go)), allowing seamless parsing by external analytics or ETL tools.

3. **Offline Caching & Storage Queue**:
   - Embedded SQLite (`modernc.org/sqlite` in [`pkg/storage`](pkg/storage)) queues serialized JSON payloads locally during network outages, maintaining data integrity without data loss.

4. **Database Engine Independence**:
   - Uses standard PostgreSQL 16 DDL with JSONB GIN indexing ([`migrations/001_initial_schema.sql`](migrations/001_initial_schema.sql)).
   - Data can be migrated without code changes across AWS Aurora PostgreSQL, Azure Database for PostgreSQL, Google Cloud SQL, or self-hosted PostgreSQL clusters.

---

## 9. Directory Architecture & Module Mapping

| Directory / Package | Architectural Responsibility | Key Interfaces & Dependencies |
| :--- | :--- | :--- |
| `cmd/agent` | Endpoint Discovery Agent binary entrypoint | Command flags (`-ui`, `-one-shot`), runtime lifecycle |
| `cmd/ingestion` | Telemetry Ingestion Service daemon entrypoint | `go-chi/v5`, PostgreSQL pool, worker configuration |
| `cmd/saasdisc` | SaaS Discovery Service entrypoint | Microsoft Entra ID OAuth2 configuration |
| `cmd/clouddisc` | Cloud Discovery Service entrypoint | AWS SDK v2, STS AssumeRole client |
| `cmd/webconsole` | Presentation Layer Management Console Service | OIDC JWT authentication, RLS query engine, REST API |
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
| `web/` | Presentation Layer UI (Admin Web Dashboard & Agent UI) | Single-file embedded HTML/CSS/JS dashboard |
