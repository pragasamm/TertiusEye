package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"

	"tertiuseye/agent/pkg/saas"
)

func main() {
	tenantID := flag.String("tenant", "", "Entra ID Tenant ID")
	clientID := flag.String("client-id", "", "Entra ID Client ID")
	clientSecret := flag.String("client-secret", "", "Entra ID Client Secret")
	flag.Parse()

	if *tenantID == "" || *clientID == "" || *clientSecret == "" {
		fmt.Println("[SaaS Discovery Service] Running in dry-run simulation mode (pass -tenant, -client-id, -client-secret for live polling).")
		graphClient := saas.NewGraphClientWithHTTPClient(http.DefaultClient)

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://httpbin.org/status/200", nil)
		resp, err := graphClient.ExecuteWithBackoff(context.Background(), req)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		defer resp.Body.Close()
		fmt.Printf("[SaaS Discovery Service] Simulation response status: %d\n", resp.StatusCode)
		return
	}

	ctx := context.Background()
	graphClient := saas.NewGraphClient(ctx, *tenantID, *clientID, *clientSecret)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://graph.microsoft.com/v1.0/users", nil)
	resp, err := graphClient.ExecuteWithBackoff(ctx, req)
	if err != nil {
		fmt.Printf("Graph API request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("[SaaS Discovery Service] Successfully polled Microsoft Graph API! Status: %d\n", resp.StatusCode)
}
