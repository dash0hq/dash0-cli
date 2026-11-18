package asset

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type storedAsset struct{ origin string }

// fakeStore answers reads from a map and records every key read.
type fakeStore struct {
	assets map[string]storedAsset
	err    error
	reads  []string
}

func (s *fakeStore) get(_ context.Context, key string) (storedAsset, error) {
	s.reads = append(s.reads, key)
	if s.err != nil {
		return storedAsset{}, s.err
	}
	if stored, ok := s.assets[key]; ok {
		return stored, nil
	}
	return storedAsset{}, &dash0api.APIError{StatusCode: http.StatusNotFound}
}

func originOf(stored storedAsset) string { return stored.origin }

func TestRouteDocument(t *testing.T) {
	tests := []struct {
		name       string
		identity   Identity
		stored     map[string]storedAsset
		wantKey    string
		wantCreate bool
		wantReads  []string
	}{
		{
			name:       "origin only, not stored",
			identity:   Identity{Origin: "o", Inferred: "inferred"},
			wantKey:    "o",
			wantCreate: true,
			wantReads:  []string{"o"},
		},
		{
			name:      "origin only, stored",
			identity:  Identity{Origin: "o", Inferred: "inferred"},
			stored:    map[string]storedAsset{"o": {origin: "o"}},
			wantKey:   "o",
			wantReads: []string{"o"},
		},
		{
			name:      "id hit",
			identity:  Identity{ID: "i", Origin: "o", Inferred: "inferred"},
			stored:    map[string]storedAsset{"i": {origin: "o"}},
			wantKey:   "i",
			wantReads: []string{"i"},
		},
		{
			name:      "id hit, stored origin set, document has none",
			identity:  Identity{ID: "i", Inferred: "inferred"},
			stored:    map[string]storedAsset{"i": {origin: "api-123"}},
			wantKey:   "i",
			wantReads: []string{"i"},
		},
		{
			name:      "id hit, stored without origin",
			identity:  Identity{ID: "i", Origin: "o", Inferred: "inferred"},
			stored:    map[string]storedAsset{"i": {}},
			wantKey:   "i",
			wantReads: []string{"i"},
		},
		{
			name:       "id not found creates by the document's origin",
			identity:   Identity{ID: "i", Origin: "o", Inferred: "inferred"},
			wantKey:    "o",
			wantCreate: true,
			wantReads:  []string{"i", "o"},
		},
		{
			name:       "id not found creates by the inferred origin",
			identity:   Identity{ID: "i", Inferred: "inferred"},
			wantKey:    "inferred",
			wantCreate: true,
			wantReads:  []string{"i", "inferred"},
		},
		{
			name:      "id not found, origin stored, updates by origin",
			identity:  Identity{ID: "i", Inferred: "inferred"},
			stored:    map[string]storedAsset{"inferred": {origin: "inferred"}},
			wantKey:   "inferred",
			wantReads: []string{"i", "inferred"},
		},
		{
			name:       "no identifier creates by the inferred origin",
			identity:   Identity{Inferred: "inferred"},
			wantKey:    "inferred",
			wantCreate: true,
			wantReads:  []string{"inferred"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{assets: tt.stored}
			route, err := RouteDocument(context.Background(), tt.identity, store.get, originOf)
			require.NoError(t, err)
			assert.Equal(t, tt.wantKey, route.Key)
			assert.Equal(t, tt.wantCreate, route.Create)
			assert.Equal(t, tt.wantReads, store.reads)
		})
	}
}

func TestRouteDocument_FailsWhenTheStoredOriginDiffers(t *testing.T) {
	store := &fakeStore{assets: map[string]storedAsset{"i": {origin: "stored"}}}
	_, err := RouteDocument(context.Background(), Identity{ID: "i", Origin: "o"}, store.get, originOf)
	var mismatch *OriginMismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Contains(t, err.Error(), `"stored"`)
	assert.Contains(t, err.Error(), `"o"`)
	assert.Equal(t, []string{"i"}, store.reads, "a mismatch must not read or write anything else")
}

// The id read decides where the write goes, so its failure stops the write.
func TestRouteDocument_IDReadErrorStops(t *testing.T) {
	store := &fakeStore{err: errors.New("boom")}
	route, err := RouteDocument(context.Background(), Identity{ID: "i", Inferred: "inferred"}, store.get, originOf)
	require.EqualError(t, err, "boom")
	assert.Empty(t, route.Key)
}

// The origin read only says whether the write is a create, so a caller may
// still write to the returned key.
func TestRouteDocument_OriginReadErrorKeepsTheKey(t *testing.T) {
	store := &fakeStore{err: errors.New("boom")}
	route, err := RouteDocument(context.Background(), Identity{Inferred: "inferred"}, store.get, originOf)
	require.EqualError(t, err, "boom")
	assert.Equal(t, "inferred", route.Key)
	assert.True(t, route.Create)
}

func TestInferOrigin(t *testing.T) {
	origin := InferOrigin("Dashboard", "checkout", "default")
	assert.True(t, strings.HasPrefix(origin, "dash0-cli_"), origin)
	assert.Equal(t, origin, InferOrigin("Dashboard", "checkout", "default"), "the same input gives the same origin")
	assert.Equal(t, origin, InferOrigin("dashboard", "checkout", "default"), "the kind is normalized")
	assert.NotEqual(t, origin, InferOrigin("Dashboard", "checkout", "staging"))
	assert.NotEqual(t, origin, InferOrigin("Dashboard", "payments", "default"))
	assert.NotEqual(t, origin, InferOrigin("EphemeralDashboard", "checkout", "default"))
	assert.NotEqual(t, InferOrigin("Dashboard", "a", "bc"), InferOrigin("Dashboard", "ab", "c"))
	// Pinned, so a change to the formula, which would duplicate every asset
	// created without an identifier, cannot pass unnoticed.
	assert.Equal(t, "dash0-cli_039d7a42761ac5ca4b3d987260cebfd2", origin)
}

func TestDatasetName(t *testing.T) {
	assert.Equal(t, "default", DatasetName(nil))
	assert.Equal(t, "default", DatasetName(dash0api.DatasetPtr("")))
	assert.Equal(t, "staging", DatasetName(dash0api.DatasetPtr("staging")))
}

func TestCheckInferredOriginCollisions(t *testing.T) {
	doc := func(path, raw string) Document {
		return Document{Kind: "Dashboard", Raw: []byte(raw), FilePath: path, DocIndex: 1, DocCount: 1}
	}
	same := "kind: Dashboard\nmetadata:\n  name: checkout\nspec: {}\n"
	other := "kind: Dashboard\nmetadata:\n  name: payments\nspec: {}\n"
	withOrigin := "kind: Dashboard\nmetadata:\n  name: checkout\n  labels:\n    dash0.com/origin: o\nspec: {}\n"

	assert.Empty(t, CheckInferredOriginCollisions([]Document{doc("a.yaml", same), doc("b.yaml", other)}, nil))
	assert.Empty(t, CheckInferredOriginCollisions([]Document{doc("a.yaml", same), doc("b.yaml", withOrigin)}, nil))

	collisions := CheckInferredOriginCollisions([]Document{doc("a.yaml", same), doc("b.yaml", same)}, nil)
	require.Len(t, collisions, 1)
	assert.Contains(t, collisions[0], "a.yaml")
	assert.Contains(t, collisions[0], "b.yaml")
	assert.Contains(t, collisions[0], InferOrigin("Dashboard", "checkout", "default"))

	// An id that is not found falls back to the inferred origin.
	withID := func(id string) string {
		return "kind: Dashboard\nmetadata:\n  name: checkout\n  labels:\n    dash0.com/id: " + id + "\nspec: {}\n"
	}
	assert.Len(t, CheckInferredOriginCollisions([]Document{doc("a.yaml", withID("i1")), doc("b.yaml", withID("i2"))}, nil), 1)
}
