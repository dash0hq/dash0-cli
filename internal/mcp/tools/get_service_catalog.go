package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	filtering "github.com/dash0hq/dash0-cli/internal/api/types/api"
	d0time "github.com/dash0hq/dash0-cli/internal/api/types/d0time"
	"github.com/dash0hq/dash0-cli/internal/api/types/grouping"
	"github.com/dash0hq/dash0-cli/internal/api/types/metrics"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/log"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addGetServiceCatalogTool(server *server.MCPServer) {
	tool := mcp.NewTool(
		"dash0__get_service_catalog",
		mcp.WithDescription("Get a service catalog from the Dash0 API with information about RED metrics and Kubernetes deployment context."),
	)

	server.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Optional parameters
		var baseURL, authToken string
		if val, ok := request.Params.Arguments["base_url"].(string); ok {
			baseURL = val
		}
		if val, ok := request.Params.Arguments["auth_token"].(string); ok {
			authToken = val
		}

		// Resolve configuration with overrides
		cfg, err := config.ResolveConfiguration(baseURL, authToken)
		if err != nil {
			return nil, err
		}

		// Use resolved configuration values
		baseURL = cfg.BaseURL
		authToken = cfg.AuthToken

		// Create HTTP client
		client := &http.Client{Timeout: 30 * time.Second}

		// Set up API URL
		apiURL := fmt.Sprintf("%s/api/resources/grouped", baseURL)

		// Create a request body with RED metrics and resource attribute breakdown
		groupBy := grouping.GroupingCriteria{"service.name"}
		reqBody := filtering.GetGroupedResourcesRequest{
			TimeRange: d0time.TimeReferenceRange{
				From: "now-1h",
				To:   "now",
			},
			GroupBy: &groupBy,
			IncludeResourceAttributeBreakdown: boolPtr(true),
			Metrics: &metrics.NamedMetricQueries{
				metrics.NamedMetricQueryRequestsRate,
				metrics.NamedMetricQueryErrorsPercentage,
				metrics.NamedMetricQueryDurationP95Seconds,
			},
		}

		// Serialize request body
		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize request body: %w", err)
		}

		// Create request
		req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(jsonBody)))
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}

		// Add headers
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", authToken))
		req.Header.Add("Content-Type", "application/json")

		// Execute request
		log.Logger.Debug().Str("url", apiURL).Msg("Making API request to get service catalog")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		// Check response status
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var response filtering.GetGroupedResourcesResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		// Prepare service catalog summary
		var summaryBuilder strings.Builder
		summaryBuilder.WriteString(fmt.Sprintf("Service Catalog (%d services):\n\n", len(response)))

		// Process each service in the response
		for _, service := range response {
			// Find service name
			serviceName := "unknown"
			for _, attr := range service.Attributes {
				if attr.Key == "service.name" {
					if attr.Value.StringValue != nil {
						serviceName = *attr.Value.StringValue
						break
					}
				}
			}

			// Start service section
			summaryBuilder.WriteString(fmt.Sprintf("## %s\n", serviceName))
			summaryBuilder.WriteString(fmt.Sprintf("Resource Count: %d\n", service.ResourceCount))

			// Add RED metrics if available
			if service.Metrics != nil {
				summaryBuilder.WriteString("\nRED Metrics:\n")
				
				if requestsRate, ok := (*service.Metrics)[string(metrics.NamedMetricQueryRequestsRate)]; ok {
					summaryBuilder.WriteString(fmt.Sprintf("- Requests Rate: %s\n", requestsRate))
				}
				
				if errorsPercentage, ok := (*service.Metrics)[string(metrics.NamedMetricQueryErrorsPercentage)]; ok {
					summaryBuilder.WriteString(fmt.Sprintf("- Error Rate: %s\n", errorsPercentage))
				}
				
				if durationP95, ok := (*service.Metrics)[string(metrics.NamedMetricQueryDurationP95Seconds)]; ok {
					summaryBuilder.WriteString(fmt.Sprintf("- P95 Duration: %s\n", durationP95))
				}
			}

			// Add deployment context
			if service.Orchestrations != nil && len(*service.Orchestrations) > 0 {
				summaryBuilder.WriteString("\nKubernetes Context:\n")
				
				for _, orch := range *service.Orchestrations {
					for _, attr := range orch.Attributes {
						switch attr.Key {
						case "k8s.deployment.name", "k8s.namespace.name", "k8s.cluster.name":
							if attr.Value.StringValue != nil {
								summaryBuilder.WriteString(fmt.Sprintf("- %s: %s\n", attr.Key, *attr.Value.StringValue))
							}
						}
					}
				}
			}

			// Add language and other context
			if service.Languages != nil && len(*service.Languages) > 0 {
				summaryBuilder.WriteString("\nRuntime Context:\n")
				
				for _, lang := range *service.Languages {
					for _, attr := range lang.Attributes {
						switch attr.Key {
						case "telemetry.sdk.language", "telemetry.sdk.name", "telemetry.sdk.version":
							if attr.Value.StringValue != nil {
								summaryBuilder.WriteString(fmt.Sprintf("- %s: %s\n", attr.Key, *attr.Value.StringValue))
							}
						}
					}
				}
			}

			// Add cloud context
			if service.Clouds != nil && len(*service.Clouds) > 0 {
				summaryBuilder.WriteString("\nCloud Context:\n")
				
				for _, cloud := range *service.Clouds {
					for _, attr := range cloud.Attributes {
						switch attr.Key {
						case "cloud.provider", "cloud.region", "cloud.availability_zone":
							if attr.Value.StringValue != nil {
								summaryBuilder.WriteString(fmt.Sprintf("- %s: %s\n", attr.Key, *attr.Value.StringValue))
							}
						}
					}
				}
			}

			// Add separator for next service
			summaryBuilder.WriteString("\n---\n\n")
		}

		return mcp.NewToolResultText(summaryBuilder.String()), nil
	})
}

// Helper function to create a bool pointer
func boolPtr(b bool) *bool {
	return &b
}