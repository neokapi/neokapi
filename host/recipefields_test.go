package host

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// serverExtras builds the `server:` block a connected recipe carries. The
// framework knows the key, never the schema — the block is a plugin extension.
func serverExtras(t *testing.T) map[string]yaml.Node {
	t.Helper()
	var node yaml.Node
	require.NoError(t, node.Encode(map[string]string{"url": "https://example.invalid"}))
	return map[string]yaml.Node{"server": node}
}

// TestWarnUnsyncedCoordinates covers what is left of the venue caveat now that
// coordinates sync.
//
// The warning used to fire for any coordinate governance on a connected
// project, because none of it reached the server. The context content type
// carries the collections, their points and the voice governing each, so a
// voice bound by coordinates now resolves the same in both venues and must NOT
// warn. A `terms:` binding is still a local path with nothing on the wire, and
// still must.
func TestWarnUnsyncedCoordinates(t *testing.T) {
	coordinated := project.Coordinates{"product": {{ID: "kapi"}}}
	voiceProfiles := []project.ProfileBinding{{
		When:  map[string]string{"product": "kapi"},
		Voice: &project.BrandVoiceBinding{ProfileFile: "voice.yaml"},
	}}
	termsProfiles := []project.ProfileBinding{{
		When:  map[string]string{"product": "kapi"},
		Terms: "terms/kapi.json",
	}}

	tests := []struct {
		name  string
		proj  *project.KapiProject
		quiet bool
		want  bool
	}{
		{
			name: "connected and binding terms by coordinates: warn",
			proj: &project.KapiProject{
				Version: "v1", Coordinates: coordinated, Profiles: termsProfiles,
				Extras: serverExtras(t),
			},
			want: true,
		},
		{
			name: "a terms binding at the base point warns too",
			proj: &project.KapiProject{
				Version:  "v1",
				Profiles: []project.ProfileBinding{{Terms: "terms/all.json"}},
				Extras:   serverExtras(t),
			},
			want: true,
		},
		{
			name: "connected and binding only a voice: the context content type carries it, nothing to warn about",
			proj: &project.KapiProject{
				Version: "v1", Coordinates: coordinated, Profiles: voiceProfiles,
				Extras: serverExtras(t),
			},
		},
		{
			name: "standalone: the recipe is the only venue, nothing to warn about",
			proj: &project.KapiProject{Version: "v1", Coordinates: coordinated, Profiles: termsProfiles},
		},
		{
			name: "connected but ungoverned: no coordinates to diverge over",
			proj: &project.KapiProject{Version: "v1", Extras: serverExtras(t)},
		},
		{
			name: "quiet suppresses it like every other run warning",
			proj: &project.KapiProject{
				Version: "v1", Coordinates: coordinated, Profiles: termsProfiles,
				Extras: serverExtras(t),
			},
			quiet: true,
		},
		{
			name: "no project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			(&App{Quiet: tt.quiet}).WarnUnsyncedCoordinates(&buf, tt.proj)
			if !tt.want {
				assert.Empty(t, buf.String())
				return
			}
			assert.Equal(t, UnsyncedCoordinatesWarning+"\n", buf.String())
			assert.Contains(t, buf.String(), "local runs only")
		})
	}
}

// TestKapiProject_GovernsByCoordinates pins what "governed by coordinates"
// means: either half of the model counts, and a recipe written before the model
// existed is untouched by it.
func TestKapiProject_GovernsByCoordinates(t *testing.T) {
	assert.False(t, (&project.KapiProject{Version: "v1"}).GovernsByCoordinates())
	assert.True(t, (&project.KapiProject{
		Coordinates: project.Coordinates{"tenant": {}},
	}).GovernsByCoordinates(), "an open axis is still a declared context space")
	assert.True(t, (&project.KapiProject{
		Profiles: []project.ProfileBinding{{Terms: "terms.json"}},
	}).GovernsByCoordinates())
}

// TestKapiProject_BindsTermsByCoordinates pins the narrower question the venue
// warning now asks: not "is there a context space?" but "does it bind something
// that cannot travel?".
func TestKapiProject_BindsTermsByCoordinates(t *testing.T) {
	assert.False(t, (&project.KapiProject{Version: "v1"}).BindsTermsByCoordinates())
	assert.False(t, (&project.KapiProject{
		Profiles: []project.ProfileBinding{{Voice: &project.BrandVoiceBinding{ProfileFile: "voice.yaml"}}},
	}).BindsTermsByCoordinates(), "a voice binding travels with the push")
	assert.True(t, (&project.KapiProject{
		Profiles: []project.ProfileBinding{
			{Voice: &project.BrandVoiceBinding{ProfileFile: "voice.yaml"}},
			{Terms: "terms.json"},
		},
	}).BindsTermsByCoordinates(), "one terms binding anywhere is enough")
}
