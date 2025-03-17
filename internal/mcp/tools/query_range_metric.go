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

func addQueryRangeMetricTool(server *server.MCPServer) {
	tool := mcp.NewTool(
		"dash0__query_range_metric",
		mcp.WithDescription("Query metrics over a time range from the Dash0 API. Similar to query_instant_metric, but returns data points over a specified time range."),
		mcp.WithString("query", mcp.Description("PromQL query to execute")),
		mcp.WithString("start", mcp.Description("Start time (defaults to 1 hour ago). Can use relative time like 'now-2h' or ISO timestamps.")),
		mcp.WithString("end", mcp.Description("End time (defaults to now). Can use relative time like 'now' or ISO timestamps.")),
		mcp.WithString("step", mcp.Description("Query resolution step width in duration format or float number of seconds (defaults to 60s).")),
	)

	server.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract required parameters
		query, ok := request.Params.Arguments["query"].(string)
		if !ok || query == "" {
			return nil, fmt.Errorf("query parameter is required")
		}

		// Optional parameters
		var start, end, step, dataset, baseURL, authToken string
		if val, ok := request.Params.Arguments["start"].(string); ok {
			start = val
		} else {
			start = "now-1h" // Default to 1 hour ago
		}
		
		if val, ok := request.Params.Arguments["end"].(string); ok {
			end = val
		} else {
			end = "now" // Default to now
		}
		
		if val, ok := request.Params.Arguments["step"].(string); ok {
			step = val
		} else {
			step = "60s" // Default to 60 seconds
		}

		if val, ok := request.Params.Arguments["dataset"].(string); ok {
			dataset = val
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
		apiURL.Path = "/api/prometheus/api/v1/query_range"

		// Add query parameters
		params := url.Values{}
		params.Set("query", query)
		params.Set("start", start)
		params.Set("end", end)
		params.Set("step", step)
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
		var response metrics.QueryRangeResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		// Check for API-reported errors
		if response.Status != "success" {
			return nil, fmt.Errorf("query failed: %s", response.Error)
		}

		// Format the results
		result := map[string]interface{}{
			"query": query,
			"start": start,
			"end":   end,
			"step":  step,
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