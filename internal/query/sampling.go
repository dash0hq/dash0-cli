package query

import (
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// PrecisionFlagDescription is the help text for the `--precision` flag.
// It is shared by every query command that accepts the flag.
const PrecisionFlagDescription = `Sampling mode for the request: "adaptive" (server default; samples records during query execution to keep queries fast on large datasets while returning statistically representative results) or "disabled" (returns every matching record; slower on wide time ranges but deterministic for narrow filters)`

// ParsePrecision turns the raw value of the `--precision` flag into a sampling
// spec for the API request. The empty string means "use the server default"
// and returns nil so the request omits the field.
func ParsePrecision(value string, timeRange dash0api.TimeReferenceRange) (*dash0api.Sampling, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return nil, nil
	case string(dash0api.SamplingModeAdaptive):
		return &dash0api.Sampling{
			Mode:      dash0api.SamplingModeAdaptive,
			TimeRange: timeRange,
		}, nil
	case string(dash0api.SamplingModeDisabled):
		return &dash0api.Sampling{
			Mode:      dash0api.SamplingModeDisabled,
			TimeRange: timeRange,
		}, nil
	default:
		return nil, fmt.Errorf("invalid --precision value %q: must be \"adaptive\" or \"disabled\"", value)
	}
}
