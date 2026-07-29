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

// TestWarnUnsyncedCoordinates covers the venue caveat: coordinates are resolved
// by the run that reads the recipe, and only a local run reads it. A connected
// project therefore has to be told that its coordinate governance stops at the
// machine it ran on — the same content coming back in two voices depending on
// where the loop ran is what the warning exists to pre-empt.
func TestWarnUnsyncedCoordinates(t *testing.T) {
	coordinated := project.Coordinates{"product": {{ID: "kapi"}}}
	profiles := []project.ProfileBinding{{
		When:  map[string]string{"product": "kapi"},
		Voice: &project.BrandVoiceBinding{ProfileFile: "voice.yaml"},
	}}

	tests := []struct {
		name  string
		proj  *project.KapiProject
		quiet bool
		want  bool
	}{
		{
			name: "connected and governed by coordinates: warn",
			proj: &project.KapiProject{
				Version: "v1", Coordinates: coordinated, Profiles: profiles,
				Extras: serverExtras(t),
			},
			want: true,
		},
		{
			name: "profiles alone are coordinate governance too",
			proj: &project.KapiProject{
				Version: "v1",
				Profiles: []project.ProfileBinding{
					{Voice: &project.BrandVoiceBinding{ProfileFile: "voice.yaml"}},
				},
				Extras: serverExtras(t),
			},
			want: true,
		},
		{
			name: "standalone: the recipe is the only venue, nothing to warn about",
			proj: &project.KapiProject{Version: "v1", Coordinates: coordinated, Profiles: profiles},
		},
		{
			name: "connected but ungoverned: no coordinates to diverge over",
			proj: &project.KapiProject{Version: "v1", Extras: serverExtras(t)},
		},
		{
			name: "quiet suppresses it like every other run warning",
			proj: &project.KapiProject{
				Version: "v1", Coordinates: coordinated, Profiles: profiles,
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
// means for the warning above: either half of the model counts, and a recipe
// written before the model existed is untouched by it.
func TestKapiProject_GovernsByCoordinates(t *testing.T) {
	assert.False(t, (&project.KapiProject{Version: "v1"}).GovernsByCoordinates())
	assert.True(t, (&project.KapiProject{
		Coordinates: project.Coordinates{"tenant": {}},
	}).GovernsByCoordinates(), "an open axis is still a declared context space")
	assert.True(t, (&project.KapiProject{
		Profiles: []project.ProfileBinding{{Terms: "terms.json"}},
	}).GovernsByCoordinates())
}
