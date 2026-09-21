package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// assetResponseTransport checks successful asset responses before the SDK loses
// the request context during decoding. Other endpoints and HTTP errors retain
// the SDK's normal handling. This does not validate user-supplied documents.
type assetResponseTransport struct {
	base http.RoundTripper
}

func (t *assetResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	schema := assetResponseSchema(req)
	if schema == nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated) {
		return resp, nil
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr == nil {
		readErr = schema.validate(body)
	}
	if readErr == nil && !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		readErr = fmt.Errorf("expected a JSON Content-Type")
	}
	if readErr != nil {
		// Report decoding failures when the SDK reads the body, not as transport
		// failures: replaying a successful write could create duplicate assets.
		resp.Body = io.NopCloser(&responseErrorReader{err: &malformedResponseError{
			asset: schema.asset, method: req.Method, path: req.URL.EscapedPath(), cause: readErr,
		}})
	} else {
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}
	return resp, nil
}

type responseErrorReader struct{ err error }

func (r *responseErrorReader) Read([]byte) (int, error) { return 0, r.err }

type malformedResponseError struct {
	asset, method, path string
	cause               error
}

func (e *malformedResponseError) Error() string {
	detail := e.cause.Error()
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(e.cause, &typeErr):
		field := typeErr.Field
		if field == "" {
			field = "$"
		}
		detail = fmt.Sprintf("field %q: expected %s, received %s", field, typeErr.Type, typeErr.Value)
	case errors.As(e.cause, &syntaxErr):
		detail = fmt.Sprintf("invalid JSON at byte %d: %s", syntaxErr.Offset, syntaxErr.Error())
	}
	return fmt.Sprintf("invalid API response for %s (%s %s):\n  %s\nHint: Update the CLI to the latest version; if this persists, report it with the command and endpoint.",
		e.asset, e.method, e.path, detail)
}

func (e *malformedResponseError) Unwrap() error { return e.cause }

type responseSchema struct {
	asset    string
	item     reflect.Type
	list     bool
	envelope string
	required []string
}

func assetResponseSchema(req *http.Request) *responseSchema {
	if req.Method != http.MethodGet && req.Method != http.MethodPost && req.Method != http.MethodPut {
		return nil
	}
	if !strings.HasPrefix(req.URL.Path, "/api/") {
		return nil
	}
	path := strings.TrimPrefix(req.URL.Path, "/api/")
	if strings.HasPrefix(path, "alerting/check-rules") {
		path = strings.TrimPrefix(path, "alerting/")
	}
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return nil
	}
	s := &responseSchema{list: req.Method == http.MethodGet && len(parts) == 1}
	s.required = []string{"kind", "metadata", "metadata.name", "spec"}
	switch parts[0] {
	case "dashboards":
		s.asset, s.item = "dashboard", reflect.TypeFor[dash0api.DashboardDefinition]()
		if s.list {
			s.item, s.required = reflect.TypeFor[dash0api.DashboardApiListItem](), []string{"id"}
		}
	case "check-rules":
		s.asset, s.item = "check rule", reflect.TypeFor[dash0api.PrometheusAlertRule]()
		s.required = []string{"name", "expression"}
		if s.list {
			s.item, s.required = reflect.TypeFor[dash0api.PrometheusAlertRuleApiListItem](), []string{"id"}
		}
	case "views":
		s.asset, s.item = "view", reflect.TypeFor[dash0api.ViewDefinition]()
		if s.list {
			s.item, s.required = reflect.TypeFor[dash0api.ViewApiListItem](), []string{"id", "type"}
		}
	case "synthetic-checks":
		s.asset, s.item = "synthetic check", reflect.TypeFor[dash0api.SyntheticCheckDefinition]()
		if s.list {
			s.item, s.required = reflect.TypeFor[dash0api.SyntheticChecksApiListItem](), []string{"id"}
		}
	case "recording-rules":
		s.asset, s.item = "recording rule", reflect.TypeFor[dash0api.RecordingRule]()
	case "slos":
		s.asset, s.item = "SLO", reflect.TypeFor[dash0api.SloDefinition]()
	case "notification-channels":
		s.asset, s.item = "notification channel", reflect.TypeFor[dash0api.NotificationChannelDefinition]()
	case "spam-filters":
		s.asset, s.item = "spam filter", reflect.TypeFor[dash0api.SpamFilter]()
		if s.list {
			s.envelope = "spamFilters"
		}
	case "teams":
		s.asset, s.item = "team", reflect.TypeFor[dash0api.TeamDefinitionV1Alpha1]()
		if s.list {
			s.item, s.required = reflect.TypeFor[dash0api.TeamsListItem](), []string{"id"}
		} else if req.Method == http.MethodGet {
			s.envelope = "team"
		}
	default:
		return nil
	}
	return s
}

func (s *responseSchema) validate(body []byte) error {
	if s.envelope != "" {
		if s.envelope == "team" {
			var team dash0api.GetTeamResponse
			if err := json.Unmarshal(body, &team); err != nil {
				return err
			}
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(body, &envelope); err != nil {
			return err
		}
		body = envelope[s.envelope]
		if absentJSON(body) {
			return fmt.Errorf("required field %q is missing or null", s.envelope)
		}
	}
	if absentJSON(body) {
		shape := "object"
		if s.list {
			shape = "list"
		}
		return fmt.Errorf("expected an asset %s, received an empty body or null", shape)
	}
	if s.list {
		var items []json.RawMessage
		if err := json.Unmarshal(body, &items); err != nil {
			return err
		}
		for i, item := range items {
			if err := s.validateItem(item); err != nil {
				return fmt.Errorf("list item %d:\n  %w", i, err)
			}
		}
		return nil
	}
	return s.validateItem(body)
}

func (s *responseSchema) validateItem(body []byte) error {
	itemType := s.item
	if s.asset == "spam filter" {
		var version struct {
			APIVersion string `json:"apiVersion"`
		}
		if err := json.Unmarshal(body, &version); err != nil {
			return err
		}
		if normalized, ok := dash0api.NormalizeDash0ApiVersion(version.APIVersion); ok && normalized == "v1alpha2" {
			itemType = reflect.TypeFor[dash0api.SpamFilterV1Alpha2]()
		}
	}
	if err := json.Unmarshal(body, reflect.New(itemType).Interface()); err != nil {
		return err
	}
	// Check the structural fields consumed by commands. Do not impose a full
	// schema or reject unknown fields: older assets and newer servers may
	// legitimately omit optional fields or add new ones.
	for _, field := range s.required {
		value := json.RawMessage(body)
		for _, key := range strings.Split(field, ".") {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(value, &object); err != nil {
				return err
			}
			value = object[key]
			if absentJSON(value) {
				return fmt.Errorf("required field %q is missing or null", field)
			}
		}
	}
	return nil
}

func absentJSON(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) == 0 || bytes.Equal(raw, []byte("null"))
}
