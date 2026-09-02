.PHONY: all build build-all clean test demo

BINARY_NAME=tertiuseye-agent
BUILD_DIR=bin
GOBUILD=go build -buildvcs=false -ldflags="-s -w"

all: test build-all

demo: build-agent-all
	./bin/tertiuseye-agent-darwin-arm64 -config config.example.json -ui -port 8090

test:
	CGO_ENABLED=1 go test -ldflags="-linkmode=external" -v ./...

build: build-agent build-ingestion build-saasdisc build-clouddisc

build-agent:
	CGO_ENABLED=0 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/agent

build-ingestion:
	CGO_ENABLED=0 $(GOBUILD) -o $(BUILD_DIR)/ingestion-service ./cmd/ingestion

build-saasdisc:
	CGO_ENABLED=0 $(GOBUILD) -o $(BUILD_DIR)/saas-discovery-service ./cmd/saasdisc

build-clouddisc:
	CGO_ENABLED=0 $(GOBUILD) -o $(BUILD_DIR)/cloud-discovery-service ./cmd/clouddisc

build-all: build-agent-all build-services

build-services: build-ingestion build-saasdisc build-clouddisc

build-agent-all:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/agent
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/agent
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/agent
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/agent
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/agent
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe ./cmd/agent

clean:
	rm -rf $(BUILD_DIR)
