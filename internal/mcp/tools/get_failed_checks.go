package tools

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    "github.com/dash0hq/dash0-cli/internal/api/types/alerting"
    d0time "github.com/dash0hq/dash0-cli/internal/api/types/d0time"
    "github.com/dash0hq/dash0-cli/internal/api/types/otlpcommon"
    "github.com/dash0hq/dash0-cli/internal/config"
    "github.com/dash0hq/dash0-cli/internal/log"
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

func addGetFailedChecksTool(server *server.MCPServer) {
    tool := mcp.NewTool(
        "dash0__get_failed_checks",
        mcp.WithDescription("Get failed check issues from the Dash0 API"),
    )

    server.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // Extract optional parameters
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

        // Create the request body
        issuesReq := alerting.GetIssuesRequest{
            // Use default dataset and timeRange from the last hour
            TimeRange: createDefaultTimeRange(),
        }

        // Marshal the request to JSON
        requestBody, err := json.Marshal(issuesReq)
        if err != nil {
            return nil, fmt.Errorf("failed to serialize request body: %w", err)
        }

        // Build the API endpoint URL
        apiURL := fmt.Sprintf("%s/api/alerting/issues", baseURL)

        // Create the HTTP request
        log.Logger.Debug().Str("url", apiURL).Msg("Making API request")
        req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(requestBody))
        if err != nil {
            return nil, fmt.Errorf("failed to create HTTP request: %w", err)
        }

        // Add headers
        req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", authToken))
        req.Header.Add("Content-Type", "application/json")

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
        var response alerting.GetIssuesResponse
        if err := json.Unmarshal(body, &response); err != nil {
            return nil, fmt.Errorf("failed to parse response: %w", err)
        }

        // Format and summarize the issues
        summary := formatIssuesSummary(response)

        // Return the summary
        return mcp.NewToolResultText(summary), nil
    })
}

// createDefaultTimeRange creates a time range for the last hour
func createDefaultTimeRange() d0time.TimeReferenceRange {
    // This is a simplified implementation - adjust as needed
    return d0time.TimeReferenceRange{
        From: "now-1h",
        To:   "now",
    }
}

// formatIssuesSummary creates a human-readable summary of the issues
func formatIssuesSummary(response alerting.GetIssuesResponse) string {
    if len(response.Issues) == 0 {
        return "No issues found in the specified time range."
    }

    var summary string
    summary = fmt.Sprintf("Found %d issues:\n\n", len(response.Issues))

    for i, issue := range response.Issues {
        summary += fmt.Sprintf("%d. %s\n", i+1, issue.Summary)
        summary += fmt.Sprintf("   ID: %s\n", issue.Id)
        summary += fmt.Sprintf("   Started: %s\n", issue.Start)
        if issue.End != nil {
            summary += fmt.Sprintf("   Ended: %s\n", *issue.End)
        } else {
            summary += "   Ongoing\n"
        }
        summary += fmt.Sprintf("   Check: %s (version: %d)\n", issue.CheckRule.Name, issue.CheckRule.Version)

        // Add description if available
        if issue.Description != "" {
            summary += fmt.Sprintf("   Description: %s\n", issue.Description)
        }

        // Add affected resources information
        if len(issue.AffectedResourceSummaries) > 0 {
            summary += "   Affected Resources:\n"
            for _, resourceSummary := range issue.AffectedResourceSummaries {
                // Add services information
                if resourceSummary.Services != nil && len(*resourceSummary.Services) > 0 {
                    summary += "     Services:\n"
                    for _, service := range *resourceSummary.Services {
                        // Extract information from attributes
                        serviceName := extractAttributeValue(service.Attributes, "service.name")
                        count := extractAttributeValue(service.Attributes, "count")
                        summary += fmt.Sprintf("       - %s: %s instances\n", serviceName, count)
                    }
                }

                // Add clouds information
                if resourceSummary.Clouds != nil && len(*resourceSummary.Clouds) > 0 {
                    summary += "     Clouds:\n"
                    for _, cloud := range *resourceSummary.Clouds {
                        // Extract information from attributes
                        cloudProvider := extractAttributeValue(cloud.Attributes, "cloud.provider")
                        count := extractAttributeValue(cloud.Attributes, "count")
                        summary += fmt.Sprintf("       - %s: %s instances\n", cloudProvider, count)
                    }
                }

                // Add platforms information
                if resourceSummary.Platforms != nil && len(*resourceSummary.Platforms) > 0 {
                    summary += "     Platforms:\n"
                    for _, platform := range *resourceSummary.Platforms {
                        // Extract information from attributes
                        platformName := extractAttributeValue(platform.Attributes, "platform.name")
                        count := extractAttributeValue(platform.Attributes, "count")
                        summary += fmt.Sprintf("       - %s: %s instances\n", platformName, count)
                    }
                }

                // Add runtimes information
                if resourceSummary.Runtimes != nil && len(*resourceSummary.Runtimes) > 0 {
                    summary += "     Runtimes:\n"
                    for _, runtime := range *resourceSummary.Runtimes {
                        // Extract information from attributes
                        runtimeName := extractAttributeValue(runtime.Attributes, "runtime.name")
                        count := extractAttributeValue(runtime.Attributes, "count")
                        summary += fmt.Sprintf("       - %s: %s instances\n", runtimeName, count)
                    }
                }
            }
        }

        // Add labels
        if len(issue.Labels) > 0 {
            summary += "   Labels:\n"
            for _, label := range issue.Labels {
                summary += fmt.Sprintf("     - %s: %s\n", label.Key, extractValue(label.Value))
            }
        }

        // Add annotations
        if len(issue.Annotations) > 0 {
            summary += "   Annotations:\n"
            for _, annotation := range issue.Annotations {
                summary += fmt.Sprintf("     - %s: %s\n", annotation.Key, extractValue(annotation.Value))
            }
        }

        summary += "\n"
    }

    return summary
}

// extractAttributeValue extracts a specific key's value from a list of KeyValue attributes
func extractAttributeValue(attributes []otlpcommon.KeyValue, key string) string {
    for _, attr := range attributes {
        if attr.Key == key {
            return extractValue(attr.Value)
        }
    }
    return "unknown"
}

// extractValue extracts a human-readable string from an AnyValue
func extractValue(value otlpcommon.AnyValue) string {
    if value.StringValue != nil {
        return *value.StringValue
    } else if value.IntValue != nil {
        return *value.IntValue
    } else if value.DoubleValue != nil {
        return fmt.Sprintf("%f", *value.DoubleValue)
    } else if value.BoolValue != nil {
        return fmt.Sprintf("%t", *value.BoolValue)
    } else if value.ArrayValue != nil && len(value.ArrayValue.Values) > 0 {
        // For array values, we'll join the first few elements
        var result []string
        maxElements := 3 // Limit to first 3 elements for brevity
        count := len(value.ArrayValue.Values)
        if count > maxElements {
            count = maxElements
        }

        for i := 0; i < count; i++ {
            result = append(result, extractValue(value.ArrayValue.Values[i]))
        }

        if len(value.ArrayValue.Values) > maxElements {
            result = append(result, fmt.Sprintf("...(%d more)", len(value.ArrayValue.Values)-maxElements))
        }

        return "[" + strings.Join(result, ", ") + "]"
    } else if value.KvlistValue != nil && len(value.KvlistValue.Values) > 0 {
        // For key-value lists, format as a simple map
        var result []string
        maxElements := 3 // Limit to first 3 key-value pairs for brevity
        count := len(value.KvlistValue.Values)
        if count > maxElements {
            count = maxElements
        }

        for i := 0; i < count; i++ {
            kv := value.KvlistValue.Values[i]
            result = append(result, fmt.Sprintf("%s: %s", kv.Key, extractValue(kv.Value)))
        }

        if len(value.KvlistValue.Values) > maxElements {
            result = append(result, fmt.Sprintf("...(%d more)", len(value.KvlistValue.Values)-maxElements))
        }

        return "{" + strings.Join(result, ", ") + "}"
    }

    // Default case when the value type is unknown or null
    return "<empty>"
}
