package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ResolveUpdateKey decides which identifier an `update -f <file>` invocation
// addresses. Every branch is covered here, including the mismatch errors,
// because a wrong answer PUTs one document's contents onto a different asset.
func TestResolveUpdateKey(t *testing.T) {
	originKeyed := func(origin, id string) UpdateKey {
		return UpdateKey{Noun: "spam filter", UsesOrigin: true, Origin: origin, ID: id}
	}
	idOnly := func(id string) UpdateKey {
		return UpdateKey{Noun: "dashboard", ID: id}
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
			// whenever both labels are present — `update` then addresses
			// whatever `apply` would have written.
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
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKey, got)
		})
	}
}
