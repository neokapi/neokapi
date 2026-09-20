package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// A minted id carries the prefix, a body ValidateID accepts, and enough entropy
// that a large batch minted in one process holds no duplicate.
func TestNewID(t *testing.T) {
	seen := make(map[string]bool, 2000)
	for range 2000 {
		id := NewID()
		require.True(t, strings.HasPrefix(id, IDPrefix), "%q opens with %q", id, IDPrefix)
		require.NoError(t, ValidateID(id), "a minted id validates: %q", id)
		body := strings.TrimPrefix(id, IDPrefix)
		require.Len(t, body, 26, "16 random bytes encode to 26 base32 characters")
		require.Equal(t, strings.ToLower(body), body, "the body is lowercase")
		require.False(t, seen[id], "minted twice: %q", id)
		seen[id] = true
	}
}

func TestValidateID(t *testing.T) {
	// 20 characters, the shortest body ValidateID accepts (100 bits).
	const body = "abcdefghijklmnopqrst"

	tests := []struct {
		name    string
		id      string
		wantErr string
	}{
		{name: "empty is a project identified by its name", id: ""},
		{name: "minted", id: NewID()},
		{name: "the shortest accepted body", id: IDPrefix + body},
		{name: "digits from the base32 alphabet", id: IDPrefix + "234567234567234567234567ab"},
		{name: "no prefix", id: body, wantErr: "is not a project id"},
		{name: "wrong prefix", id: "proj_" + body, wantErr: "is not a project id"},
		{name: "body one character short", id: IDPrefix + body[:19], wantErr: "is not a project id"},
		{name: "uppercase body", id: IDPrefix + strings.ToUpper(body), wantErr: "is not a project id"},
		{name: "a digit outside base32", id: IDPrefix + body[:19] + "1", wantErr: "is not a project id"},
		{name: "punctuation", id: IDPrefix + body[:19] + "-", wantErr: "is not a project id"},
		{name: "longer than the bound", id: IDPrefix + strings.Repeat("a", 65), wantErr: "is not a project id"},
		{name: "surrounding space", id: " " + IDPrefix + body, wantErr: "is not a project id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateID(tt.id)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// Identity is the id when there is one and the name otherwise, which is what
// keeps a recipe written before ids existed working unchanged.
func TestIdentity(t *testing.T) {
	id := NewID()
	tests := []struct {
		name string
		proj *KapiProject
		want string
	}{
		{name: "nil", proj: nil, want: ""},
		{name: "id and name", proj: &KapiProject{ID: id, Name: "Northsea"}, want: id},
		{name: "name alone", proj: &KapiProject{Name: "Northsea"}, want: "Northsea"},
		{name: "id alone", proj: &KapiProject{ID: id}, want: id},
		{name: "neither", proj: &KapiProject{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.proj.Identity())
		})
	}
}

// The loader checks the id's shape, so a hand-edited recipe carrying a
// half-pasted value is refused where it was written rather than silently
// becoming the key everything is recorded under.
func TestLoadValidatesID(t *testing.T) {
	tests := []struct {
		name    string
		recipe  string
		wantErr string
	}{
		{
			name:   "a minted id loads",
			recipe: "version: v1\nid: " + NewID() + "\nname: app\n",
		},
		{
			name:   "no id loads",
			recipe: "version: v1\nname: app\n",
		},
		{
			name:    "a malformed id is refused",
			recipe:  "version: v1\nid: prj_NOPE\nname: app\n",
			wantErr: "is not a project id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), RecipeFileName)
			require.NoError(t, os.WriteFile(path, []byte(tt.recipe), 0o644))
			proj, err := Load(path)
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, "app", proj.Name)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// A kapi that predates `id:` has no field for the key, so it lands in Extras.
// This reproduces that shape and holds the two properties a released binary
// reading a newer recipe depends on: the recipe still loads, and the near-miss
// reporter stays quiet about a key it cannot place.
func TestAnUnknownIDKeyLoadsWithoutComplaint(t *testing.T) {
	var node yaml.Node
	require.NoError(t, node.Encode(NewID()))

	proj := &KapiProject{
		Version: CurrentVersion,
		Name:    "app",
		Extras:  map[string]yaml.Node{"id": node},
	}
	require.NoError(t, proj.Validate(), "an unregistered top-level key round-trips")
	assert.Empty(t, proj.KeyWarnings(), "`id` resembles no field, so it is an extension and not a typo")
	assert.Equal(t, "app", proj.Identity(), "such a binary still identifies the project by its name")
}

// Saving a recipe that carries an id round-trips it, and the id is not a key
// the near-miss reporter mistakes for something else.
func TestIDRoundTripsThroughSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), RecipeFileName)
	id := NewID()
	require.NoError(t, os.WriteFile(path, []byte(
		"version: v1\n# the project's identity\nid: "+id+"\nname: app\n"), 0o644))

	proj, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, id, proj.ID)
	assert.Empty(t, proj.KeyWarnings(), "`id` is a field, so nothing reports it as a near miss")

	proj.Name = "renamed"
	require.NoError(t, Save(path, proj))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(after), "id: "+id)
	assert.Contains(t, string(after), "# the project's identity")

	reloaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, id, reloaded.ID)
	assert.Equal(t, "renamed", reloaded.Name)
}
