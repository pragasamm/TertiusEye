package network

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"tertiuseye/agent/pkg/storage"
)

// Client handles mTLS authenticated HTTPS transmission and offline flushing.
type Client struct {
	httpClient  *http.Client
	endpointURL string
	tenantID    string
	queue       *storage.SQLiteQueue
}

// NewClient initializes an mTLS HTTP client using MDM-provisioned certificate and key.
func NewClient(certPath, keyPath, endpointURL, tenantID string, queue *storage.SQLiteQueue) (*Client, error) {
	var certificates []tls.Certificate

	if certPath != "" && keyPath != "" {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load X.509 keypair (cert='%s', key='%s'): %w", certPath, keyPath, err)
		}
		certificates = append(certificates, cert)
	}

	tlsConfig := &tls.Config{
		Certificates:       certificates,
		InsecureSkipVerify: true, // Configurable for dev/test environments
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &Client{
		httpClient:  client,
		endpointURL: endpointURL,
		tenantID:    tenantID,
		queue:       queue,
	}, nil
}

// Transmit sends a discovery JSON payload to the SaaS backend over mTLS.
// If transmission fails, it enqueues the payload into SQLite for offline caching.
func (c *Client) Transmit(ctx context.Context, payloadJSON string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointURL, bytes.NewBufferString(payloadJSON))
	if err != nil {
		c.cacheOffline(payloadJSON)
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.tenantID != "" {
		req.Header.Set("X-Tenant-ID", c.tenantID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Connection failed (e.g., laptop is offline). Enqueue payload into SQLite.
		c.cacheOffline(payloadJSON)
		return fmt.Errorf("outbound mTLS connection failed (cached to offline queue): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		c.cacheOffline(payloadJSON)
		return fmt.Errorf("backend server returned error status %d: %s (cached to offline queue)", resp.StatusCode, string(body))
	}

	return nil
}

// cacheOffline enqueues a payload string into local SQLite database.
func (c *Client) cacheOffline(payloadJSON string) {
	if c.queue != nil {
		_ = c.queue.Enqueue(payloadJSON)
	}
}

// FlushOfflineCache queries SQLite for pending payloads, flushes them to the backend, and deletes them upon success.
func (c *Client) FlushOfflineCache(ctx context.Context) (int, error) {
	if c.queue == nil {
		return 0, nil
	}

	items, err := c.queue.DequeueAll()
	if err != nil || len(items) == 0 {
		return 0, err
	}

	var flushedIDs []int64

	for _, item := range items {
		select {
		case <-ctx.Done():
			return len(flushedIDs), ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointURL, bytes.NewBufferString(item.Payload))
		if err != nil {
			break
		}
		req.Header.Set("Content-Type", "application/json")
		if c.tenantID != "" {
			req.Header.Set("X-Tenant-ID", c.tenantID)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Still offline or host unreachable
			break
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			flushedIDs = append(flushedIDs, item.ID)
		} else {
			// Stop flushing on error
			break
		}
	}

	if len(flushedIDs) > 0 {
		if err := c.queue.Delete(flushedIDs); err != nil {
			return len(flushedIDs), fmt.Errorf("failed to delete flushed records from sqlite: %w", err)
		}
	}

	return len(flushedIDs), nil
}

// StartBackgroundFlusher launches a background goroutine polling for connectivity to flush offline queue upon reconnection.
func (c *Client) StartBackgroundFlusher(ctx context.Context, pollInterval time.Duration) {
	if pollInterval <= 0 {
		pollInterval = 1 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count, _ := c.queue.Count()
				if count > 0 {
					_, _ = c.FlushOfflineCache(ctx)
				}
			}
		}
	}()
}
