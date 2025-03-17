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

func addGetOperationsTool(server *server.MCPServer) {
	tool := mcp.NewTool(
		"dash0__get_operations",
		mcp.WithDescription("Get a list of operations from the Dash0 API with information about RED metrics grouped by operation and service name."),
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

		// Create a request body with RED metrics and grouping by operation name, type, and service name
		groupBy := grouping.GroupingCriteria{
			"dash0.operation.name",
			"dash0.operation.type",
			"service.name",
		}

		reqBody := filtering.GetGroupedResourcesRequest{
			TimeRange: d0time.TimeReferenceRange{
				From: "now-1h",
				To:   "now",
			},
			GroupBy: &groupBy,
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
		log.Logger.Debug().Str("url", apiURL).Msg("Making API request to get operations")
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

		// Prepare operations summary
		var summaryBuilder strings.Builder
		summaryBuilder.WriteString(fmt.Sprintf("Operations Catalog (%d operations):\n\n", len(response)))

		// Process each operation in the response
		for _, operation := range response {
			// Find operation name, type, and service name from attributes
			operationName := "unknown"
			operationType := "unknown"
			serviceName := "unknown"

			for _, attr := range operation.Attributes {
				if attr.Key == "dash0.operation.name" && attr.Value.StringValue != nil {
					operationName = *attr.Value.StringValue
				} else if attr.Key == "dash0.operation.type" && attr.Value.StringValue != nil {
					operationType = *attr.Value.StringValue
				} else if attr.Key == "service.name" && attr.Value.StringValue != nil {
					serviceName = *attr.Value.StringValue
				}
			}

			// Start operation section
			summaryBuilder.WriteString(fmt.Sprintf("## %s\n", operationName))
			summaryBuilder.WriteString(fmt.Sprintf("Type: %s\n", operationType))
			summaryBuilder.WriteString(fmt.Sprintf("Service: %s\n", serviceName))
			summaryBuilder.WriteString(fmt.Sprintf("Resource Count: %d\n", operation.ResourceCount))

			// Add RED metrics if available
			if operation.Metrics != nil {
				summaryBuilder.WriteString("\nRED Metrics:\n")
				
				if requestsRate, ok := (*operation.Metrics)[string(metrics.NamedMetricQueryRequestsRate)]; ok {
					summaryBuilder.WriteString(fmt.Sprintf("- Requests Rate: %s\n", requestsRate))
				}
				
				if errorsPercentage, ok := (*operation.Metrics)[string(metrics.NamedMetricQueryErrorsPercentage)]; ok {
					summaryBuilder.WriteString(fmt.Sprintf("- Error Rate: %s\n", errorsPercentage))
				}
				
				if durationP95, ok := (*operation.Metrics)[string(metrics.NamedMetricQueryDurationP95Seconds)]; ok {
					summaryBuilder.WriteString(fmt.Sprintf("- P95 Duration: %s\n", durationP95))
				}
			}

			// Add separator for next operation
			summaryBuilder.WriteString("\n---\n\n")
		}

		return mcp.NewToolResultText(summaryBuilder.String()), nil
	})
}