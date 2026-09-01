package saas

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type Mock429Transport struct {
	attempts int32
}

func (m *Mock429Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	current := atomic.AddInt32(&m.attempts, 1)
	if current < 3 {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     make(http.Header),
			Body:       http.NoBody,
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       http.NoBody,
	}, nil
}

func TestExecuteWithBackoffOn429(t *testing.T) {
	mockTrans := &Mock429Transport{}
	httpClient := &http.Client{Transport: mockTrans}

	graphClient := NewGraphClientWithHTTPClient(httpClient)
	graphClient.MaxRetries = 4

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://graph.microsoft.com/v1.0/me", nil)

	start := time.Now()
	resp, err := graphClient.ExecuteWithBackoff(context.Background(), req)
	if err != nil {
		t.Fatalf("ExecuteWithBackoff failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK after retries, got %d", resp.StatusCode)
	}

	if atomic.LoadInt32(&mockTrans.attempts) != 3 {
		t.Errorf("Expected 3 attempts, got %d", mockTrans.attempts)
	}

	elapsed := time.Since(start)
	if elapsed < 1*time.Second {
		t.Errorf("Expected backoff delay > 1s, got %v", elapsed)
	}
}
