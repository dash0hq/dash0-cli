package asset

import (
	"context"
	"net/http"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeKeys struct {
	origin, id     string
	byOrigin, byID bool
}

func (k fakeKeys) GetOrigin() string                 { return k.origin }
func (k fakeKeys) GetID() string                     { return k.id }
func (k fakeKeys) GetName() string                   { return "" }
func (k fakeKeys) UpsertKeys() (byOrigin, byID bool) { return k.byOrigin, k.byID }

// Every branch is covered here, mismatch errors included, because a wrong
// answer PUTs one document's contents onto a different asset.
func TestResolveUpdateKey(t *testing.T) {
	originKeyed := func(origin, id string) UpdateKey {
		return UpdateKey{Noun: "spam filter", Upsert: fakeKeys{origin, id, true, true}}
	}
	originOnly := func(origin, id string) UpdateKey {
		return UpdateKey{Noun: "notification channel", Upsert: fakeKeys{origin, id, true, false}}
	}
	idOnly := func(id string) UpdateKey {
		return UpdateKey{Noun: "dashboard", Upsert: fakeKeys{"o", id, false, true}}
	}

	tests := []struct {
		name    string
		args    []string
		key     UpdateKey
		wantKey string
		wantErr string
	}{
		{
			name:    "arg only, file carries neither identifier",
			args:    []string{"any"},
			key:     originKeyed("", ""),
			wantKey: "any",
		},
		{
			name:    "file origin only",
			key:     originKeyed("o1", ""),
			wantKey: "o1",
		},
		{
			name:    "file id only",
			key:     originKeyed("", "i1"),
			wantKey: "i1",
		},
		{
			// Origin is the upsert key for the kinds that have one, so it wins
			// when both labels are present. `update` then addresses whatever
			// `apply` would have written.
			name:    "file origin preferred over file id",
			key:     originKeyed("o2", "i2"),
			wantKey: "o2",
		},
		{
			name:    "arg matches file origin",
			args:    []string{"o3"},
			key:     originKeyed("o3", "i3"),
			wantKey: "o3",
		},
		{
			name:    "arg matches file id",
			args:    []string{"i4"},
			key:     originKeyed("o4", "i4"),
			wantKey: "i4",
		},
		{
			name:    "arg matches neither, both identifiers present",
			args:    []string{"x"},
			key:     originKeyed("o5", "i5"),
			wantErr: `argument "x" matches neither the file's origin ("o5") nor its id ("i5")`,
		},
		{
			// A separate message, so it never mentions an identifier the file
			// does not carry.
			name:    "arg matches neither, origin-only file",
			args:    []string{"x"},
			key:     originKeyed("o6", ""),
			wantErr: `argument "x" does not match the file's origin ("o6")`,
		},
		{
			name:    "arg matches neither, id-only file",
			args:    []string{"x"},
			key:     originKeyed("", "i7"),
			wantErr: `argument "x" does not match the file's id ("i7")`,
		},
		{
			name:    "no arg and no identifiers, origin-keyed kind",
			key:     originKeyed("", ""),
			wantErr: `no spam filter origin or id given: pass one as an argument, or set metadata.labels["dash0.com/origin"] in the file`,
		},
		{
			// A kind with no origin upsert key must never be told to set an
			// origin label it does not use.
			name:    "no arg and no id, id-only kind",
			key:     idOnly(""),
			wantErr: "no dashboard id given: pass one as an argument, or set the id in the file",
		},
		{
			name:    "id-only kind, arg matches file id",
			args:    []string{"i8"},
			key:     idOnly("i8"),
			wantKey: "i8",
		},
		{
			name:    "id-only kind, arg mismatches file id",
			args:    []string{"x"},
			key:     idOnly("i9"),
			wantErr: `argument "x" does not match the file's id ("i9")`,
		},
		{
			name:    "origin-only kind, no arg and id-only file",
			key:     originOnly("", "i11"),
			wantErr: `no notification channel origin or id given`,
		},
		{
			name:    "origin-only kind, no arg, file has both",
			key:     originOnly("o12", "i12"),
			wantKey: "o12",
		},
		{
			// An exported document carries the server-assigned id, and the API path accepts it.
			name:    "origin-only kind, arg matches the id of an exported file",
			args:    []string{"i13"},
			key:     originOnly("o13", "i13"),
			wantKey: "i13",
		},
		{
			name:    "origin-only kind, arg names another asset",
			args:    []string{"other"},
			key:     originOnly("o14", ""),
			wantErr: `argument "other" does not match the file's origin ("o14")`,
		},
		{
			// The origin is provenance only for this kind, so it is never a key.
			name:    "id-only kind, arg matches only the origin label",
			args:    []string{"o"},
			key:     idOnly("i15"),
			wantErr: `argument "o" does not match the file's id ("i15")`,
		},
		{
			// A second positional argument is rejected by cobra's Args
			// validator before this runs; only args[0] is consulted.
			name:    "extra args ignored",
			args:    []string{"o10", "ignored"},
			key:     originKeyed("o10", ""),
			wantKey: "o10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveUpdateKey(tt.args, tt.key)
			if tt.wantErr != "" {
				require.Error(t, err)
				if tt.wantErr == "not found" {
					assert.True(t, dash0api.IsNotFound(err))
				} else {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKey, got)
		})
	}
}

// RouteUpdate must address the asset apply writes, or `update -f` and `apply`
// of the same file change different assets.
func TestRouteUpdate(t *testing.T) {
	key := UpdateKey{Noun: "team", Upsert: fakeKeys{"doc-origin", "the-id", true, true}}
	identity := Identity{ID: "the-id", Origin: "doc-origin"}
	notFound := &dash0api.APIError{StatusCode: http.StatusNotFound}

	tests := []struct {
		name    string
		args    []string
		idOnly  bool
		assets  map[string]storedOrigin
		wantKey string
		wantErr string
	}{
		{name: "id found wins over origin", assets: map[string]storedOrigin{"the-id": {"doc-origin"}, "doc-origin": {"doc-origin"}}, wantKey: "the-id"},
		{name: "id found without stored origin", assets: map[string]storedOrigin{"the-id": {""}}, wantKey: "the-id"},
		{name: "id not found falls back to origin", assets: map[string]storedOrigin{"doc-origin": {"doc-origin"}}, wantKey: "doc-origin"},
		{name: "neither found is not found", assets: map[string]storedOrigin{}, wantKey: "doc-origin", wantErr: "not found"},
		{name: "stored origin differs", assets: map[string]storedOrigin{"the-id": {"other"}}, wantKey: "the-id", wantErr: "the CLI never changes a stored origin"},
		{name: "id only, not found", idOnly: true, assets: map[string]storedOrigin{}, wantKey: "the-id", wantErr: "not found"},
		{name: "argument is the id and stored origin differs", args: []string{"the-id"}, assets: map[string]storedOrigin{"the-id": {"other"}}, wantKey: "the-id", wantErr: "the CLI never changes a stored origin"},
		{name: "argument is the origin, id's stored origin is not read", args: []string{"doc-origin"}, assets: map[string]storedOrigin{"the-id": {"other"}, "doc-origin": {"doc-origin"}}, wantKey: "doc-origin"},
		{name: "argument is used as given", args: []string{"doc-origin"}, assets: map[string]storedOrigin{"the-id": {"doc-origin"}, "doc-origin": {"doc-origin"}}, wantKey: "doc-origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := func(_ context.Context, k string) (storedOrigin, error) {
				if a, ok := tt.assets[k]; ok {
					return a, nil
				}
				return storedOrigin{}, notFound
			}
			id := identity
			if tt.idOnly {
				id.Origin = ""
			}
			gotKey, _, err := RouteUpdate(context.Background(), tt.args, key, id, get)
			assert.Equal(t, tt.wantKey, gotKey)
			if tt.wantErr != "" {
				require.Error(t, err)
				if tt.wantErr == "not found" {
					assert.True(t, dash0api.IsNotFound(err))
				} else {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

type storedOrigin struct{ origin string }

func (s storedOrigin) GetOrigin() string { return s.origin }

func TestCheckStoredOrigin(t *testing.T) {
	identity := Identity{ID: "i", Origin: "o"}

	assert.NoError(t, CheckStoredOrigin("i", identity, fakeKeys{origin: "o"}))
	assert.Error(t, CheckStoredOrigin("i", identity, fakeKeys{origin: "other"}))
	assert.NoError(t, CheckStoredOrigin("o", identity, fakeKeys{origin: "other"}), "a write by origin is not checked")
	assert.ErrorContains(t, CheckStoredOrigin("i", identity, struct{}{}), "cannot read the stored origin")
}
