package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/spf13/cobra"
)

// QueryInstantResponse represents the response from the Prometheus instant query API
type QueryInstantResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

// NewMetricsCmd creates a new query command
func NewMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Query Dash0 metrics",
		Long:  `Query metrics from the Dash0 API`,
	}

	// Add subcommands
	cmd.AddCommand(newInstantQueryCmd())

	return cmd
}

// newInstantQueryCmd creates a new instant query command
func newInstantQueryCmd() *cobra.Command {
	var apiUrl string
	var authToken string
	var queryExpr string
	var dataset string
	var queryTime string

	cmd := &cobra.Command{
		Use:   "instant",
		Short: "Run an instant query",
		Long: `Query the instant value of a metric from the Dash0 API.` + internal.CONFIG_HINT,
		Example: `  # Query a PromQL expression
  dash0 metrics instant --query 'up'

  # Query with a specific dataset
  dash0 metrics instant --query 'rate(http_requests_total[5m])' --dataset production

  # Query at a specific time
  dash0 metrics instant --query 'process_cpu_seconds_total' --time 2024-01-25T10:00:00Z`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve configuration with overrides
			cfg, err := config.ResolveConfiguration(apiUrl, authToken)
			if err != nil {
				return err
			}

			// Use resolved configuration values
			apiUrl = cfg.ApiUrl
			authToken = cfg.AuthToken

			// Resolve dataset from config if not provided via flag
			if dataset == "" {
				dataset = cfg.Dataset
			}

			// Validate required parameters
			if queryExpr == "" {
				return fmt.Errorf("query expression is required")
			}

			// Create and execute the request
			timestamp := time.Now().Unix()

			// Build the query URL
			apiURL, err := url.Parse(apiUrl)
			if err != nil {
				return fmt.Errorf("invalid API URL: %w", err)
			}
			apiURL.Path = "/api/prometheus/api/v1/query"

			if len(queryTime) == 0 {
				queryTime = "now"
			}

			// Add query parameters
			query := url.Values{}
			query.Set("query", queryExpr)
			query.Set("time", queryTime)
			if dataset != "" && dataset != "default" {
				query.Set("dataset", dataset)
			}
			apiURL.RawQuery = query.Encode()

			// Create the HTTP request
			req, err := http.NewRequest("GET", apiURL.String(), nil)
			if err != nil {
				return fmt.Errorf("failed to create HTTP request: %w", err)
			}

			// Add authorization header
			req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", authToken))

			// Execute the request
			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed to execute HTTP request: %w", err)
			}
			defer resp.Body.Close()

			// Read the response body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response body: %w", err)
			}

			// Check the response status
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
			}

			// Parse the response
			var response QueryInstantResponse
			if err := json.Unmarshal(body, &response); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			// Check for API-reported errors
			if response.Status != "success" {
				return fmt.Errorf("query failed: %s", response.Error)
			}

			// Print the results in a user-friendly format
			fmt.Println("Query:", queryExpr)
			fmt.Printf("Time: %s\n\n", time.Unix(timestamp, 0).Format(time.RFC3339))

			if len(response.Data.Result) == 0 {
				fmt.Println("No results found.")
				return nil
			}

			for _, result := range response.Data.Result {
				// Print the metric labels
				fmt.Println("Metric:")
				for k, v := range result.Metric {
					fmt.Printf("  %s: %s\n", k, v)
				}

				// Print the value (typically an array where first element is timestamp and second is value)
				if len(result.Value) >= 2 {
					fmt.Printf("Value: %v\n", result.Value[1])
				} else {
					fmt.Printf("Value: %v\n", result.Value)
				}
				fmt.Println()
			}

			return nil
		},
	}

	// Register flags
	cmd.Flags().StringVar(&queryExpr, "query", "", "PromQL query expression (required)")
	cmd.Flags().StringVar(&dataset, "dataset", "", "Dataset to query (optional)")
	cmd.Flags().StringVar(&queryTime, "time", "", "Evaluation timestamp (optional, defaults to now). Supports relative time ranges")
	cmd.Flags().StringVar(&apiUrl, "api-url", "", "API URL for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Auth token for the Dash0 API (overrides active profile)")

	cmd.MarkFlagRequired("query")

	return cmd
}
