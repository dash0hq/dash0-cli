package tools

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/dash0hq/dash0-cli/internal/api/types/apicomparisons"
    d0time "github.com/dash0hq/dash0-cli/internal/api/types/d0time"
    "github.com/dash0hq/dash0-cli/internal/api/types/filtering"
    "github.com/dash0hq/dash0-cli/internal/api/types/otlpcommon"
    "github.com/dash0hq/dash0-cli/internal/config"
    "github.com/dash0hq/dash0-cli/internal/log"
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

func addTraceTriageTool(server *server.MCPServer) {
    tool := mcp.NewTool(
        "dash0__tracing_error_commonalities",
        mcp.WithDescription("Analyzes trace data using attribute comparison to identify patterns within errors. "),
        mcp.WithString("service_name", mcp.Description("The name of the service to analyze")),
        mcp.WithString("start", mcp.Description("Start time for selection (e.g. 'now-15m', 'now-1h', '2023-04-01T00:00:00Z')")),
        mcp.WithString("end", mcp.Description("End time for selection (e.g. 'now', 'now+15m', '2023-04-01T01:00:00Z')")),
    )

    server.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // Extract required parameters
        serviceName, ok := request.Params.Arguments["service_name"].(string)
        if !ok || serviceName == "" {
            return nil, fmt.Errorf("service_name parameter is required")
        }

        startTime, ok := request.Params.Arguments["start"].(string)
        if !ok || startTime == "" {
            return nil, fmt.Errorf("start parameter is required")
        }

        endTime, ok := request.Params.Arguments["end"].(string)
        if !ok || endTime == "" {
            return nil, fmt.Errorf("end parameter is required")
        }

        // Extract optional parameters
        var datasetName string = "default"

        if val, ok := request.Params.Arguments["dataset"].(string); ok {
            datasetName = val
        }

        // Optional parameters for auth
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

        // Build the AttributeComparisonRequest
        comparisonRequest := apicomparisons.AttributeComparisonRequest{
            BaselineSelection: apicomparisons.Selection{
                TimeRange: d0time.TimeReferenceRange{
                    From: startTime,
                    To:   endTime,
                },
                Filter: createServiceFilter(serviceName),
            },
        }

        // Add optional dataset
        if datasetName != "" {
            comparisonRequest.Dataset = &datasetName
        }

        // Add optional comparison selection
        comparisonSelection := apicomparisons.Selection{
            TimeRange: d0time.TimeReferenceRange{
                From: startTime,
                To:   endTime,
            },
        }
        // Use the same filter for comparison selection
        comparisonFilter := []filtering.AttributeFilter{
            createAttributeFilter("service.name", serviceName),
            createAttributeFilter("otel.span.status.code", "ERROR"),
        }
        comparisonSelection.Filter = &comparisonFilter
        comparisonRequest.ComparisonSelection = &comparisonSelection

        //var t = true
        //comparisonRequest.RetainCommonElements = &t

        // Build the request URL
        apiURL := fmt.Sprintf("%s/api/spans/comparison", baseURL)

        // Marshal the request body
        requestBody, err := json.Marshal(comparisonRequest)
        if err != nil {
            return nil, fmt.Errorf("failed to serialize request body: %w", err)
        }

        // Create the HTTP request
        log.Logger.Debug().Str("url", apiURL).Msg("Making API request")
        req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(requestBody)))
        if err != nil {
            return nil, fmt.Errorf("failed to create HTTP request: %w", err)
        }

        // Add headers
        req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", authToken))
        req.Header.Add("Content-Type", "application/json")
        req.Header.Add("Accept", "application/event-stream")

        // Execute the request
        client := &http.Client{Timeout: 5 * time.Minute} // Longer timeout for streaming responses
        resp, err := client.Do(req)
        if err != nil {
            return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
        }
        defer resp.Body.Close()

        // Check the response status
        if resp.StatusCode != http.StatusOK {
            return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
        }

        // Process the event stream
        responseMap := make(map[string]apicomparisons.AttributeComparisonResponse)
        scanner := bufio.NewScanner(resp.Body)
        allDone := false

        // Continue reading until we receive Done=true or encounter an error
        for !allDone {
            if !scanner.Scan() {
                // Check if we stopped scanning due to an error
                if err := scanner.Err(); err != nil {
                    return nil, fmt.Errorf("error reading response stream: %w", err)
                }
                // Otherwise we reached EOF without receiving Done=true
                break
            }

            line := scanner.Text()
            log.Logger.Info().Msg(line)

            // Skip empty lines and comments
            if line == "" || strings.HasPrefix(line, ":") {
                log.Logger.Info().Msg("Abort 1")
                continue
            }

            // Parse SSE event
            if !strings.HasPrefix(line, "data:") {
                log.Logger.Info().Msg("Abort 2")
                continue
            }

            // Extract the data payload
            data := strings.TrimPrefix(line, "data:")
            var response apicomparisons.AttributeComparisonResponse

            if err := json.Unmarshal([]byte(data), &response); err != nil {
                log.Logger.Error().Err(err).Str("data", data).Msg("Failed to parse event data")
                continue
            }

            // Store/update response by key
            responseMap[response.Key] = response

            // Check if we're done
            if response.Done {
                log.Logger.Debug().Str("key", response.Key).Msg("Received done event for key")
                allDone = true
            }
        }

        // Format results as specified
        var resultBuilder strings.Builder

        for _, response := range responseMap {
            if response.Attributes != nil {
                for _, attr := range *response.Attributes {
                    for _, value := range attr.Values {
                        selectionCount := -1
                        if value.SelectionCount != nil {
                            selectionCount = *value.SelectionCount
                        }

                        if selectionCount < 0 {
                            continue
                        }

                        // Convert AnyValue to string
                        valueStr, err := formatAnyValue(value.Value)
                        if err != nil {
                            log.Logger.Error().Err(err).Msg("Failed to format value")
                            continue
                        }

                        // Format: - ${attribute_key}=${attribute_value}: ${baselineCount}/${selectionCount}
                        resultBuilder.WriteString(fmt.Sprintf("- %s=%s: %d appears %d more often for errors\n",
                            attr.Key, valueStr, selectionCount-value.BaselineCount))

                        resultBuilder.WriteString("\n")
                    }
                }
            }
        }

        return mcp.NewToolResultText(resultBuilder.String()), nil
    })
}

// Helper function to create a service filter
func createServiceFilter(serviceName string) *filtering.FilterCriteria {
    // Create a single attribute filter for service name
    filter := []filtering.AttributeFilter{
        createAttributeFilter("service.name", serviceName),
    }
    return &filter
}

// Helper function to create an attribute filter
func createAttributeFilter(key, value string) filtering.AttributeFilter {
    // Create string value
    valueStr := value

    // Create attribute filter value
    filterValue := new(filtering.AttributeFilter_Value)
    _ = filterValue.FromAttributeFilterStringValue(valueStr)

    return filtering.AttributeFilter{
        Key:      key,
        Operator: filtering.AttributeFilterOperatorIs,
        Value:    filterValue,
    }
}

// Helper function to format AnyValue as a string
func formatAnyValue(value otlpcommon.AnyValue) (string, error) {
    if value.StringValue != nil {
        return *value.StringValue, nil
    } else if value.IntValue != nil {
        return *value.IntValue, nil
    } else if value.BoolValue != nil {
        return fmt.Sprintf("%t", *value.BoolValue), nil
    } else if value.DoubleValue != nil {
        return fmt.Sprintf("%f", *value.DoubleValue), nil
    } else if value.BytesValue != nil {
        return fmt.Sprintf("%v", *value.BytesValue), nil
    } else if value.ArrayValue != nil {
        arrayValues := make([]string, 0, len(value.ArrayValue.Values))
        for _, v := range value.ArrayValue.Values {
            strVal, err := formatAnyValue(v)
            if err != nil {
                return "", err
            }
            arrayValues = append(arrayValues, strVal)
        }
        return fmt.Sprintf("[%s]", strings.Join(arrayValues, ", ")), nil
    } else if value.KvlistValue != nil {
        kvValues := make([]string, 0, len(value.KvlistValue.Values))
        for _, kv := range value.KvlistValue.Values {
            strVal, err := formatAnyValue(kv.Value)
            if err != nil {
                return "", err
            }
            kvValues = append(kvValues, fmt.Sprintf("%s=%s", kv.Key, strVal))
        }
        return fmt.Sprintf("{%s}", strings.Join(kvValues, ", ")), nil
    }

    return "", fmt.Errorf("unknown value type")
}
