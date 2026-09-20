package project

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setID drives SetField for the `id` path the way every caller does, through a
// JSON value.
func setID(t *testing.T, proj *KapiProject, v string) (bool, error) {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return SetField(proj, "id", raw)
}

// The id is settable exactly once through the recipe setter: a project with
// none can be given one, and a project that has one keeps it whatever a
// change-set says.
func TestSetFieldID(t *testing.T) {
	minted := NewID()
	other := NewID()

	tests := []struct {
		name        string
		start       string
		value       string
		wantChanged bool
		wantID      string
		wantErr     string
	}{
		{
			name:        "a project with no id takes one",
			value:       minted,
			wantChanged: true,
			wantID:      minted,
		},
		{
			name:        "setting the id it already has changes nothing",
			start:       minted,
			value:       minted,
			wantChanged: false,
			wantID:      minted,
		},
		{
			name:        "surrounding whitespace is trimmed",
			value:       "  " + minted + "  ",
			wantChanged: true,
			wantID:      minted,
		},
		{
			name:    "re-keying is refused",
			start:   minted,
			value:   other,
			wantID:  minted,
			wantErr: "already has the id " + minted,
		},
		{
			name:    "clearing is refused",
			start:   minted,
			value:   "",
			wantID:  minted,
			wantErr: "cannot be cleared",
		},
		{
			name:    "a malformed id is refused",
			value:   "prj_nope",
			wantErr: "is not a project id",
		},
		{
			name:    "an id without the prefix is refused",
			value:   "abcdefghijklmnopqrstuvwxyz",
			wantErr: "is not a project id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj := &KapiProject{Version: CurrentVersion, ID: tt.start, Name: "app"}
			changed, err := setID(t, proj, tt.value)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.False(t, changed)
				assert.Equal(t, tt.wantID, proj.ID, "a refused set leaves the recipe alone")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantChanged, changed)
			assert.Equal(t, tt.wantID, proj.ID)
			assert.NoError(t, proj.Validate(), "what the setter wrote loads again")
		})
	}
}

// The recipe setter rejects a non-string value for the id with the path in the
// message, as it does for every other field it owns.
func TestSetFieldIDWrongType(t *testing.T) {
	proj := &KapiProject{Version: CurrentVersion}
	_, err := SetField(proj, "id", json.RawMessage(`42`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id")
	assert.Empty(t, proj.ID)
}
