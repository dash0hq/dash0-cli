package rawapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// supportedMethods lists the HTTP methods accepted by `dash0 api`.
// The set intentionally excludes TRACE and CONNECT: neither has a defined
// use in the Dash0 API and allowing them would only expand the surface
// without adding value.
var supportedMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodHead:    {},
	http.MethodOptions: {},
}

// parseMethodAndPath reads the positional arguments.
// It accepts either "<path>" (method defaults to GET) or "<METHOD> <path>".
// The method is normalized to uppercase and validated against supportedMethods.
func parseMethodAndPath(args []string) (string, string, error) {
	switch len(args) {
	case 1:
		return http.MethodGet, args[0], nil
	case 2:
		method := strings.ToUpper(args[0])
		if _, ok := supportedMethods[method]; !ok {
			return "", "", fmt.Errorf("unsupported HTTP method %q", args[0])
		}
		return method, args[1], nil
	default:
		return "", "", fmt.Errorf("expected <path> or <method> <path>")
	}
}

// readBody reads the request body from the file path given on `-f`.
// A value of "-" reads from the provided reader (stdin).
// An empty path returns a nil slice, meaning "no body".
func readBody(file string, stdin io.Reader) ([]byte, error) {
	if file == "" {
		return nil, nil
	}
	if file == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read body file: %w", err)
	}
	return data, nil
}

// parsedHeader represents a single `-H` value.
type parsedHeader struct {
	Key   string
	Value string
}

// parseHeaders parses `-H "Key: Value"` flag values.
// Setting Authorization (case-insensitive) is a hard error — authentication is
// managed by the active profile.
func parseHeaders(raw []string) ([]parsedHeader, error) {
	result := make([]parsedHeader, 0, len(raw))
	for _, h := range raw {
		idx := strings.IndexByte(h, ':')
		if idx <= 0 {
			return nil, fmt.Errorf("invalid header %q: expected 'Key: Value'", h)
		}
		key := strings.TrimSpace(h[:idx])
		value := strings.TrimSpace(h[idx+1:])
		if key == "" {
			return nil, fmt.Errorf("invalid header %q: key is empty", h)
		}
		if strings.EqualFold(key, "Authorization") {
			return nil, fmt.Errorf("cannot set Authorization header: authentication is managed by the active profile")
		}
		result = append(result, parsedHeader{Key: key, Value: value})
	}
	return result, nil
}

type buildRequestInput struct {
	BaseURL          string
	Path             string
	Method           string
	ProfileDataset   string
	FlagDataset      string
	DatasetFlagSet   bool
	Body             []byte
	Headers          []parsedHeader
	AuthToken        string
	AuthTokenFromFlag bool
	UserAgent        string
}

// buildRequest constructs the outbound http.Request. It resolves the final URL
// (including dataset auto-injection), sets headers, and populates the body.
func buildRequest(in buildRequestInput) (*http.Request, error) {
	finalURL, err := resolveURL(in.BaseURL, in.Path)
	if err != nil {
		return nil, err
	}

	if err := checkHostMismatch(in.BaseURL, in.Path, finalURL, in.AuthTokenFromFlag); err != nil {
		return nil, err
	}

	if err := injectDataset(finalURL, in.ProfileDataset, in.FlagDataset, in.DatasetFlagSet); err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if in.Body != nil {
		bodyReader = bytes.NewReader(in.Body)
	}

	req, err := http.NewRequest(in.Method, finalURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+in.AuthToken)
	req.Header.Set("User-Agent", in.UserAgent)

	hasContentType := false
	for _, h := range in.Headers {
		if strings.EqualFold(h.Key, "Content-Type") {
			hasContentType = true
		}
		req.Header.Add(h.Key, h.Value)
	}
	if in.Body != nil && !hasContentType {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// checkHostMismatch returns an error when the user passes an absolute URL
// whose host differs from the profile's api-url and the auth token was not
// explicitly provided via --auth-token. Silently sending a profile token to
// an unrelated host is almost always a mistake (e.g., a dev token hitting
// production) and leads to confusing 401 errors.
func checkHostMismatch(baseURL, path string, finalURL *url.URL, authTokenFromFlag bool) error {
	if authTokenFromFlag {
		return nil
	}
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		return nil
	}
	baseU, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	if strings.EqualFold(baseU.Host, finalURL.Host) {
		return nil
	}
	return fmt.Errorf(
		"absolute URL host %q does not match the profile's api-url host %q; "+
			"the profile's auth token may not be valid for this host — "+
			"pass --auth-token explicitly or switch to a profile configured for %s",
		finalURL.Host, baseU.Host, finalURL.Host,
	)
}

// resolveURL combines the base URL and the path argument. Absolute URLs
// (http:// or https://) are returned verbatim. Relative paths have "/api/"
// prepended and are resolved against the base URL.
func resolveURL(baseURL, path string) (*url.URL, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		u, err := url.Parse(path)
		if err != nil {
			return nil, fmt.Errorf("invalid URL %q: %w", path, err)
		}
		return u, nil
	}

	normalized := "/" + strings.TrimLeft(path, "/")
	if !strings.HasPrefix(normalized, "/api/") {
		return nil, fmt.Errorf("relative path must start with /api/ (got %q)", path)
	}

	u, err := url.Parse(strings.TrimRight(baseURL, "/") + normalized)
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", path, err)
	}
	return u, nil
}

// injectDataset applies the dataset=<value> query parameter to u according to
// the resolution rules:
//
//   - --dataset "" (explicit empty): do not inject.
//   - --dataset <value> (explicit non-empty): inject that value.
//   - --dataset not provided: inject the profile/env dataset if non-empty.
//
// If the URL already contains a dataset= query parameter and the resolved
// dataset is non-empty, the function returns an error.
func injectDataset(u *url.URL, profileDataset, flagDataset string, flagSet bool) error {
	var effective string
	switch {
	case flagSet && flagDataset == "":
		return nil
	case flagSet:
		effective = flagDataset
	default:
		effective = profileDataset
	}

	if effective == "" {
		return nil
	}

	q := u.Query()
	if q.Has("dataset") {
		return fmt.Errorf("dataset is set in both the path query and --dataset; remove one (pass --dataset \"\" to keep the query)")
	}
	q.Set("dataset", effective)
	u.RawQuery = q.Encode()
	return nil
}
