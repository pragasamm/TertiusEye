package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"tertiuseye/agent/pkg/database"
	"tertiuseye/agent/pkg/model"
)

// IngestionTask represents a raw payload job submitted to the buffered worker pool.
type IngestionTask struct {
	TenantID string
	Payload  []byte
}

// Service manages HTTP endpoints and high-concurrency worker pool ingestion.
type Service struct {
	repo       *database.Repository
	workers    int
	taskChan   chan IngestionTask
	router     *chi.Mux
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewService initializes the Telemetry Ingestion Service with a buffered worker pool.
func NewService(repo *database.Repository, workers int, bufferSize int) *Service {
	if workers <= 0 {
		workers = 50 // Default worker pool size as per LLD 4.1
	}
	if bufferSize <= 0 {
		bufferSize = 1000
	}

	ctx, cancel := context.WithCancel(context.Background())

	svc := &Service{
		repo:       repo,
		workers:    workers,
		taskChan:   make(chan IngestionTask, bufferSize),
		router:     chi.NewRouter(),
		ctx:        ctx,
		cancelFunc: cancel,
	}

	svc.setupRoutes()
	svc.startWorkerPool()

	return svc
}

func (s *Service) setupRoutes() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)

	s.router.Post("/api/v1/telemetry", s.HandleTelemetry)
	s.router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

// Router returns the Chi HTTP handler.
func (s *Service) Router() http.Handler {
	return s.router
}

// HandleTelemetry extracts the X-Tenant-ID header injected by API Gateway mTLS verification and pushes job to worker pool.
func (s *Service) HandleTelemetry(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-Tenant-ID")
	if tenantID == "" {
		http.Error(w, "Missing X-Tenant-ID header", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, "Invalid or empty request payload", http.StatusBadRequest)
		return
	}

	task := IngestionTask{
		TenantID: tenantID,
		Payload:  body,
	}

	select {
	case s.taskChan <- task:
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"queued"}`))
	default:
		// Queue full (push backpressure)
		http.Error(w, "Ingestion buffer pool saturated", http.StatusServiceUnavailable)
	}
}

// startWorkerPool launches the fixed pool of goroutines processing buffered ingestion payloads.
func (s *Service) startWorkerPool() {
	for i := 0; i < s.workers; i++ {
		go func() {
			for {
				select {
				case <-s.ctx.Done():
					return
				case task, ok := <-s.taskChan:
					if !ok {
						return
					}
					s.processTask(task)
				}
			}
		}()
	}
}

// processTask validates JSON discovery schema and executes database insertion under RLS protection.
func (s *Service) processTask(task IngestionTask) {
	var payload model.DiscoveryPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		fmt.Printf("[Ingestion Worker] Error parsing discovery JSON schema for tenant %s: %v\n", task.TenantID, err)
		return
	}

	deviceID := payload.Metadata.DeviceUUID
	if deviceID == "" {
		deviceID = payload.Metadata.Hostname
	}
	if deviceID == "" {
		fmt.Printf("[Ingestion Worker] Missing device_uuid/hostname for tenant %s\n", task.TenantID)
		return
	}

	if s.repo != nil {
		err := s.repo.SaveDevice(s.ctx, task.TenantID, deviceID, payload.Metadata.Hostname, task.Payload)
		if err != nil {
			fmt.Printf("[Ingestion Worker] Error persisting device %s for tenant %s to DB: %v\n", deviceID, task.TenantID, err)
			return
		}
	}
}

// Close gracefully stops the worker pool.
func (s *Service) Close() error {
	s.cancelFunc()
	close(s.taskChan)
	return nil
}
