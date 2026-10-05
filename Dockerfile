# --- Build Stage ---
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/bin/agent ./cmd/agent
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/bin/ingestion-service ./cmd/ingestion
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/bin/cloud-discovery-service ./cmd/clouddisc
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/bin/saas-discovery-service ./cmd/saasdisc

# --- Agent & UI Container Target ---
FROM alpine:3.19 AS agent
WORKDIR /app
COPY --from=builder /app/bin/agent /app/agent
COPY config.example.json /app/config.json
ENTRYPOINT ["/app/agent"]

# --- Telemetry Ingestion Container Target ---
FROM alpine:3.19 AS ingestion-service
WORKDIR /app
COPY --from=builder /app/bin/ingestion-service /app/ingestion-service
ENTRYPOINT ["/app/ingestion-service"]

# --- Cloud Discovery Container Target ---
FROM alpine:3.19 AS cloud-discovery-service
WORKDIR /app
COPY --from=builder /app/bin/cloud-discovery-service /app/cloud-discovery-service
ENTRYPOINT ["/app/cloud-discovery-service"]

# --- SaaS Discovery Container Target ---
FROM alpine:3.19 AS saas-discovery-service
WORKDIR /app
COPY --from=builder /app/bin/saas-discovery-service /app/saas-discovery-service
ENTRYPOINT ["/app/saas-discovery-service"]
