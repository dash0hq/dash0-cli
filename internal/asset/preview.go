package asset

import (
	"encoding/json"
	"fmt"
)

// PreviewUpdate returns stored as it would look after a partial update: the
// update's values win, and anything the update leaves out keeps its stored value.
// It serves kinds whose PUT keeps omitted spec fields, so a dry-run diff does
// not show omissions as removals.
func PreviewUpdate[T any](stored, update any) (*T, error) {
	base, err := toMap(stored)
	if err != nil {
		return nil, err
	}
	over, err := toMap(update)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(mergeMaps(base, over))
	if err != nil {
		return nil, err
	}
	var merged T
	if err := json.Unmarshal(data, &merged); err != nil {
		return nil, fmt.Errorf("failed to build the update preview: %w", err)
	}
	return &merged, nil
}

func toMap(value any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// mergeMaps overlays over onto base: nested objects merge, everything else is replaced.
func mergeMaps(base, over map[string]any) map[string]any {
	for key, value := range over {
		if overMap, ok := value.(map[string]any); ok {
			if baseMap, ok := base[key].(map[string]any); ok {
				base[key] = mergeMaps(baseMap, overMap)
				continue
			}
		}
		base[key] = value
	}
	return base
}
