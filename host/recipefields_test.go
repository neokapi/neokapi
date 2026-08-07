package host

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/project/projecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// registerVenue registers a platform's venue extension for the duration of a
// test. The framework finds the venue through the registry, never by key name,
// so a test asking whether a recipe is connected has to have one.
func registerVenue(t *testing.T) {
	t.Helper()
	projecttest.ResetExtensions()
	t.Cleanup(projecttest.ResetExtensions)
	project.RegisterExtension(project.Extension{
		Name:  "bowrain",
		Scope: project.ScopeProject,
		Group: "bowrain",
		Venue: true,
	})
}

// venueExtras builds the venue block a connected recipe carries. The framework
// knows the key only through the registry — the block is a plugin extension.
func venueExtras(t *testing.T) map[string]yaml.Node {
	t.Helper()
	var node yaml.Node
	require.NoError(t, node.Encode(map[string]string{"url": "https://example.invalid"}))
	return map[string]yaml.Node{"bowrain": node}
}

// TestWarnUnsyncedCoordinates covers what is left of the venue caveat now that
// the context space syncs.
//
// The warning used to fire for any governance a profile carried on a connected
// project, because none of it reached the server. The context content type
// carries the collections, their points and the voice governing each, so a
// voice bound by a profile now resolves the same in both venues and must NOT
// warn. A `terms:` binding is still a local path with nothing on the wire, and
// still must.
func TestWarnUnsyncedCoordinates(t *testing.T) {
	registerVenue(t)

	voiceProfiles := map[string]project.Profile{
		"kapi": {
			Channels: []project.Channel{{ID: "docs"}},
			Voice:    &project.VoiceBinding{ProfileFile: "voice.yaml"},
		},
	}
	termsProfiles := map[string]project.Profile{
		"kapi": {
			Channels: []project.Channel{{ID: "docs"}},
			Terms:    "terms/kapi.json",
		},
	}

	tests := []struct {
		name  string
		proj  *project.KapiProject
		quiet bool
		want  bool
	}{
		{
			name: "connected and binding terms by profile: warn",
			proj: &project.KapiProject{
				Version: "v1", Profiles: termsProfiles, Extras: venueExtras(t),
			},
			want: true,
		},
		{
			name: "a profile that declares no channels warns too",
			proj: &project.KapiProject{
				Version:  "v1",
				Profiles: map[string]project.Profile{"kapi": {Terms: "terms/all.json"}},
				Extras:   venueExtras(t),
			},
			want: true,
		},
		{
			name: "connected and binding only a voice: the context content type carries it, nothing to warn about",
			proj: &project.KapiProject{
				Version: "v1", Profiles: voiceProfiles, Extras: venueExtras(t),
			},
		},
		{
			name: "standalone: the recipe is the only venue, nothing to warn about",
			proj: &project.KapiProject{Version: "v1", Profiles: termsProfiles},
		},
		{
			name: "connected but ungoverned: no profiles to diverge over",
			proj: &project.KapiProject{Version: "v1", Extras: venueExtras(t)},
		},
		{
			name: "quiet suppresses it like every other run warning",
			proj: &project.KapiProject{
				Version: "v1", Profiles: termsProfiles, Extras: venueExtras(t),
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

// TestWarnUnsyncedCoordinates_UnregisteredVenue is the seam: the same recipe,
// in a binary that links no platform, binds no venue at all. A key kapi cannot
// name is a key it grows no opinion about, so there is nothing to warn against.
func TestWarnUnsyncedCoordinates_UnregisteredVenue(t *testing.T) {
	projecttest.ResetExtensions()
	t.Cleanup(projecttest.ResetExtensions)

	proj := &project.KapiProject{
		Version: "v1",
		Profiles: map[string]project.Profile{
			"kapi": {Terms: "terms/kapi.json"},
		},
		Extras: venueExtras(t),
	}

	var buf bytes.Buffer
	(&App{}).WarnUnsyncedCoordinates(&buf, proj)
	assert.Empty(t, buf.String())
}
