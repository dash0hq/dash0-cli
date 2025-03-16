package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/log"
	"github.com/dash0hq/dash0-cli/internal/metrics"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addQueryInstantMetricTool(server *server.MCPServer) {
	tool := mcp.NewTool(
		"dash0__query_instant_metric",
		mcp.WithDescription("Query the instant value of a metric from the Dash0 API. You can query Dash0's synthetic metrics to learn about spans and logs for any service. To query spans, you can use the `dash0_spans_total` metrics. Do query logs, you can use `dash0_logs_total`. You can group by a variety of span and log attributes to break down the data. Here a variety of labels you can group by `otel_scope_name`, `otel_scope_version`, `otel_log_time`, `otel_log_body`, `otel_log_severity_number`, `otel_log_severity_range`, `otel_log_severity_text`, `otel_trace_id`, `otel_span_id`, `otel_parent_id`, `otel_span_start_time`, `otel_span_duration`, `otel_span_end_time`, `otel_span_name`, `otel_span_kind`, `otel_span_status_code`, `otel_span_status_message`."),
    mcp.WithString("query", mcp.Description("PromQL query to execute")),
	)

	server.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract parameters
		query, ok := request.Params.Arguments["query"].(string)
		if !ok || query == "" {
			return nil, fmt.Errorf("query parameter is required")
		}

		// Optional parameters
		var dataset, queryTime, baseURL, authToken string
		if val, ok := request.Params.Arguments["dataset"].(string); ok {
			dataset = val
		}
		if val, ok := request.Params.Arguments["time"].(string); ok {
			queryTime = val
		}
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

		// Build the query URL
		apiURL, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		apiURL.Path = "/api/prometheus/api/v1/query"

		if len(queryTime) == 0 {
			queryTime = "now"
		}

		// Add query parameters
		params := url.Values{}
		params.Set("query", query)
		params.Set("time", queryTime)
		if dataset != "" {
			params.Set("dataset", dataset)
		}
		apiURL.RawQuery = params.Encode()

		// Create the HTTP request
		log.Logger.Debug().Str("url", apiURL.String()).Msg("Making API request")
		req, err := http.NewRequest("GET", apiURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}

		// Add authorization header
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", authToken))

		// Execute the request
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
		}
		defer resp.Body.Close()

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		// Check the response status
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse the response
		var response metrics.QueryInstantResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		// Check for API-reported errors
		if response.Status != "success" {
			return nil, fmt.Errorf("query failed: %s", response.Error)
		}

		// Format the results
		timestamp := time.Now().Unix()
		result := map[string]interface{}{
			"query": query,
			"time":  time.Unix(timestamp, 0).Format(time.RFC3339),
			"data":  response.Data,
		}

		resultJSON, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize result: %w", err)
		}

		// Use text result as JSON content is already serialized
		return mcp.NewToolResultText(string(resultJSON)), nil
	})
}
