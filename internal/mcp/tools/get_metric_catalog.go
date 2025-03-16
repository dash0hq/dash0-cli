package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	d0time "github.com/dash0hq/dash0-cli/internal/api/types/d0time"
	"github.com/dash0hq/dash0-cli/internal/api/types/filtering"
	"github.com/dash0hq/dash0-cli/internal/api/types/metrics"
	"github.com/dash0hq/dash0-cli/internal/api/types/pagination"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/log"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addGetMetricCatalogTool(server *server.MCPServer) {
	tool := mcp.NewTool(
		"dash0__get_metric_catalog",
		mcp.WithDescription("Get the catalog of available metrics from the Dash0 API, optionally filtered by service name."),
		mcp.WithString("service_name", mcp.Description("Service name to filter the catalog by")),
	)

	server.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract service name parameter
		serviceName, _ := request.Params.Arguments["service_name"].(string)

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

		// Set up initial request
		apiURL := fmt.Sprintf("%s/api/metrics/catalog", baseURL)

		// Create fixed time references for the time range
		reqBody := metrics.GetMetricCatalogRequest{
			TimeRange: d0time.TimeReferenceRange{
				From: "now-1h",
				To:   "now",
			},
		}

		// Add service name filter if provided
		if serviceName != "" {
			filters := make(filtering.FilterCriteria, 0)

			// Create attribute filter for service name
			filter := filtering.AttributeFilter{
				Key:      "service.name",
				Operator: filtering.AttributeFilterOperatorIs,
			}

			// Create value for the filter
			var value filtering.AttributeFilter_Value
			if err := value.FromAttributeFilterStringValue(serviceName); err != nil {
				return nil, fmt.Errorf("failed to create filter value: %w", err)
			}
			filter.Value = &value

			// Add the filter to the criteria
			filters = append(filters, filter)
			reqBody.Filter = &filters
		}

		// Collected metrics
		allMetrics := make(map[string]metrics.MetricCatalogItem)

		// Cursor for pagination
		var cursor string

		// Iterate through all pages using cursors
		for {
			// Update pagination with cursor if we have one
			if cursor != "" {
				reqBody.Pagination = &pagination.CursorPagination{
					Cursor: &cursor,
				}
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
			log.Logger.Debug().Str("url", apiURL).Msg("Making API request to metrics catalog")
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
			var response metrics.GetMetricCatalogResponse
			if err := json.Unmarshal(body, &response); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			// Add metrics to our collection
			for _, metric := range response.Catalog {
				allMetrics[metric.Name] = metric
			}

			// Check if we have more pages
			if response.Cursors.After == nil {
				break
			}

			// Update cursor for next page
			cursor = *response.Cursors.After
		}

		// Prepare summary of metrics
		var summaryBuilder strings.Builder
		summaryBuilder.WriteString(fmt.Sprintf("Found %d metrics", len(allMetrics)))
		if serviceName != "" {
			summaryBuilder.WriteString(fmt.Sprintf(" for service '%s'", serviceName))
		}
		summaryBuilder.WriteString(":\n\n")

		// Group metrics by type
		metricsByType := make(map[metrics.MetricType][]string)
		for _, metric := range allMetrics {
			metricsByType[metric.Type] = append(metricsByType[metric.Type], metric.Name)
		}

		// Add metrics to summary, grouped by type
		for metricType, metricNames := range metricsByType {
			summaryBuilder.WriteString(fmt.Sprintf("Type: %s (%d metrics)\n", metricType, len(metricNames)))
			for _, name := range metricNames {
				metric := allMetrics[name]
				unit := "no unit"
				if metric.Unit != nil {
					unit = *metric.Unit
				}
				summaryBuilder.WriteString(fmt.Sprintf("  - %s (unit: %s, cardinality: %d)\n",
					name, unit, metric.Volume.Cardinality))
			}
			summaryBuilder.WriteString("\n")
		}

		return mcp.NewToolResultText(summaryBuilder.String()), nil
	})
}
