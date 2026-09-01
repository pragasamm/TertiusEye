package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"tertiuseye/agent/pkg/database"
	"tertiuseye/agent/pkg/ingestion"
)

func main() {
	port := flag.Int("port", 8080, "HTTP server port")
	dbConn := flag.String("db", "postgres://postgres:postgres@localhost:5432/tertiuseye?sslmode=disable", "PostgreSQL connection string")
	workers := flag.Int("workers", 50, "Number of worker goroutines in ingestion pool")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Println("[Ingestion Service] Initializing PostgreSQL connection pool...")
	repo, err := database.NewRepository(ctx, *dbConn)
	if err != nil {
		fmt.Printf("[Ingestion Service] Warning: PostgreSQL pool initialization failed: %v (running in dry-run mode)\n", err)
	} else {
		defer repo.Close()
	}

	svc := ingestion.NewService(repo, *workers, 2000)
	defer svc.Close()

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: svc.Router(),
	}

	go func() {
		fmt.Printf("[Ingestion Service] Listening on :%d with %d worker goroutines...\n", *port, *workers)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[Ingestion Service] Server error: %v\n", err)
		}
	}()

	<-ctx.Done()
	fmt.Println("[Ingestion Service] Shutting down...")
	_ = server.Shutdown(context.Background())
}
