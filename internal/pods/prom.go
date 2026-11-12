package pods

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Dash0's Prometheus-compatible query API: POST /api/v1/query_range, the auth token as a bearer token, the dataset
// as a form field.

type promClient struct {
	base, token, userAgent string
	dataset                *string
	http                   *http.Client
}

type window struct {
	start, end time.Time
	step       time.Duration
}

// The sample times of a window, start to end, a step apart
func (w window) times() []time.Time {
	var out []time.Time
	for t := w.start; !t.After(w.end); t = t.Add(w.step) {
		out = append(out, t)
	}
	return out
}

type promAnswer struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Values [][2]any          `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// The query's series on the window's sample times, keyed by the value of label ("" keys the first series alone): nil
// where a series has no sample. No series at all is no data, not a failure
func (c promClient) queryRange(ctx context.Context, query string, w window, label string) (map[string][]*float64, error) {
	form := url.Values{
		"query": {query},
		"start": {strconv.FormatInt(w.start.Unix(), 10)},
		"end":   {strconv.FormatInt(w.end.Unix(), 10)},
		"step":  {strconv.Itoa(int(w.step.Seconds()))},
	}
	if c.dataset != nil {
		form.Set("dataset", *c.dataset)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.base, "/")+"/api/v1/query_range", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	var answer promAnswer
	if err := json.Unmarshal(body, &answer); err != nil || res.StatusCode != http.StatusOK || answer.Status != "success" {
		if answer.Error != "" {
			return nil, fmt.Errorf("%s: %s", res.Status, answer.Error)
		}
		var other struct {
			Error struct{ Message string } `json:"error"`
		}
		if json.Unmarshal(body, &other) == nil && other.Error.Message != "" {
			return nil, fmt.Errorf("%s: %s", res.Status, other.Error.Message)
		}
		return nil, fmt.Errorf("%s", res.Status)
	}
	times := w.times()
	out := map[string][]*float64{}
	for _, r := range answer.Data.Result {
		key := ""
		if label != "" {
			key = r.Metric[label]
		} else if len(out) > 0 {
			break
		}
		at := map[int64]float64{}
		for _, pair := range r.Values {
			ts, _ := pair[0].(float64)
			s, _ := pair[1].(string)
			// histogram_quantile answers NaN for a step without samples: a gap, never a number
			if v, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
				at[int64(ts)] = v
			}
		}
		values := make([]*float64, len(times))
		for i, t := range times {
			if v, ok := at[t.Unix()]; ok {
				values[i] = &v
			}
		}
		out[key] = values
	}
	if label == "" && len(out) == 0 {
		out[""] = make([]*float64, len(times))
	}
	return out, nil
}
