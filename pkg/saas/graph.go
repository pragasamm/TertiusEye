package saas

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"golang.org/x/oauth2/clientcredentials"
)

// GraphClient executes Microsoft Entra ID / Graph API discovery with rate-limit exponential backoff and jitter.
type GraphClient struct {
	httpClient *http.Client
	rand       *rand.Rand
	MaxRetries int
}

// NewGraphClient initializes an OAuth2 client credentials client for Microsoft Entra ID.
func NewGraphClient(ctx context.Context, tenantID, clientID, clientSecret string) *GraphClient {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)

	cfg := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       []string{"https://graph.microsoft.com/.default"},
	}

	oauthClient := cfg.Client(ctx)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	return &GraphClient{
		httpClient: oauthClient,
		rand:       r,
		MaxRetries: 5,
	}
}

// NewGraphClientWithHTTPClient initializes GraphClient with a custom http.Client (useful for testing).
func NewGraphClientWithHTTPClient(client *http.Client) *GraphClient {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &GraphClient{
		httpClient: client,
		rand:       r,
		MaxRetries: 5,
	}
}

// ExecuteWithBackoff executes an HTTP request with exponential backoff & randomized jitter on HTTP 429 (LLD 4.2).
func (g *GraphClient) ExecuteWithBackoff(ctx context.Context, req *http.Request) (*http.Response, error) {
	backoff := time.Second
	maxRetries := g.MaxRetries

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()

			// LLD Section 4.2: Rate Limit Backoff & Randomized Jitter implementation
			jitter := time.Duration(g.rand.Int63n(1000)) * time.Millisecond
			sleepDuration := backoff + jitter

			fmt.Printf("[SaaS Discovery] Received HTTP 429 Too Many Requests. Retrying in %v (attempt %d/%d)...\n", sleepDuration, i+1, maxRetries)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepDuration):
			}

			backoff *= 2 // Exponential increase
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("max retries (%d) exceeded due to persistent rate limiting (HTTP 429)", maxRetries)
}
