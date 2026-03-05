package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// PersesDashboard represents the Perses Operator PersesDashboard CRD
// (perses.dev/v1alpha1 and perses.dev/v1alpha2).
type PersesDashboard struct {
	APIVersion string                    `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                    `yaml:"kind" json:"kind"`
	Metadata   PersesDashboardMetadata   `yaml:"metadata" json:"metadata"`
	Spec       map[string]interface{}    `yaml:"spec" json:"spec"`
}

// PersesDashboardMetadata contains metadata for a PersesDashboard.
type PersesDashboardMetadata struct {
	Name        string            `yaml:"name,omitempty" json:"name,omitempty"`
	Namespace   string            `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// ConvertToDashboard converts a PersesDashboard CRD into a Dash0
// DashboardDefinition. It normalizes v1alpha1/v1alpha2 differences (the
// v1alpha2 CRD wraps the spec in a "config" key) and ensures a display name
// is set, falling back to metadata.name.
func ConvertToDashboard(perses *PersesDashboard) *dash0api.DashboardDefinition {
	spec := perses.Spec
	if spec == nil {
		spec = make(map[string]interface{})
	}

	// Normalize v1alpha1/v1alpha2: if spec.config exists, unwrap it.
	// v1alpha2 wraps the dashboard content in spec.config; v1alpha1 puts it
	// directly under spec. After normalization both look the same.
	if configRaw, ok := spec["config"]; ok {
		if config, ok := configRaw.(map[string]interface{}); ok {
			spec = config
		}
	}

	// Ensure display section exists
	displayRaw, hasDisplay := spec["display"]
	if !hasDisplay {
		spec["display"] = map[string]interface{}{
			"name": perses.Metadata.Name,
		}
	} else if display, ok := displayRaw.(map[string]interface{}); ok {
		// Set display.name to metadata.name if missing
		if _, hasName := display["name"]; !hasName {
			display["name"] = perses.Metadata.Name
		}
	}

	displayName := extractDisplayName(spec)
	if displayName == "" {
		displayName = perses.Metadata.Name
	}

	dashboard := &dash0api.DashboardDefinition{
		Kind: dash0api.Dashboard,
		Metadata: dash0api.DashboardMetadata{
			Name: displayName,
		},
		Spec: spec,
	}

	// Copy dash0.com/id from labels into dash0Extensions.id
	if perses.Metadata.Labels != nil {
		if id := perses.Metadata.Labels["dash0.com/id"]; id != "" {
			dashboard.Metadata.Dash0Extensions = &dash0api.DashboardMetadataExtensions{
				Id: &id,
			}
		}
	}

	return dashboard
}

// extractDisplayName reads spec.display.name from a dashboard spec map.
func extractDisplayName(spec map[string]interface{}) string {
	display, ok := spec["display"].(map[string]interface{})
	if !ok {
		return ""
	}
	name, ok := display["name"].(string)
	if !ok {
		return ""
	}
	return name
}

// ImportPersesDashboard converts a PersesDashboard CRD to a Dash0 dashboard
// and imports it. It returns the import result (created or updated).
func ImportPersesDashboard(ctx context.Context, apiClient dash0api.Client, perses *PersesDashboard, dataset *string) (ImportResult, error) {
	dashboard := ConvertToDashboard(perses)
	return ImportDashboard(ctx, apiClient, dashboard, dataset)
}

// ExtractPersesDashboardName returns the display name from the Perses spec,
// falling back to metadata.name.
func ExtractPersesDashboardName(perses *PersesDashboard) string {
	if perses.Spec != nil {
		// Check after normalization: handle both v1alpha1 and v1alpha2
		spec := perses.Spec
		if configRaw, ok := spec["config"]; ok {
			if config, ok := configRaw.(map[string]interface{}); ok {
				spec = config
			}
		}
		if name := extractDisplayName(spec); name != "" {
			return name
		}
	}
	return perses.Metadata.Name
}

// ExtractPersesDashboardID returns the dash0.com/id label value if present.
func ExtractPersesDashboardID(perses *PersesDashboard) string {
	if perses.Metadata.Labels != nil {
		return perses.Metadata.Labels["dash0.com/id"]
	}
	return ""
}
