package kpz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms/ktb"
)

// The context profile of the container: a package holding what a project's
// store keeps as authored context, and nothing of its content.

// contextPackage is a small package of the context profile, carrying one
// profile of each member kind the profile adds.
func contextPackage() *Package {
	return &Package{
		Kind:  KindContext,
		Terms: ktb.FromConcepts(nil),
		Voice: []VoiceDoc{
			{
				Path:    VoiceDir + "acme.yaml",
				ID:      "acme",
				Binding: ".kapi/voice.yaml",
				Profile: &profile.VoiceProfile{
					ID:   "acme",
					Name: "Acme",
					Tone: profile.ToneProfile{Formality: "neutral"},
				},
			},
		},
		Decisions: []DecisionDoc{
			{Path: DecisionsDir + "d-docs.jsonl", Data: []byte(`{"unit":"greeting","variant":"nb"}` + "\n")},
		},
	}
}

// TestContextPackage_RoundTripsItsMembers: voice profiles and decision shards
// come back as they went in, identities and bindings included.
func TestContextPackage_RoundTripsItsMembers(t *testing.T) {
	pkg := contextPackage()
	require.True(t, pkg.HasContent(), "voice and decisions are content")

	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)

	assert.Equal(t, KindContext, got.Kind)
	require.Len(t, got.Voice, 1)
	assert.Equal(t, VoiceDir+"acme.yaml", got.Voice[0].Path)
	assert.Equal(t, "acme", got.Voice[0].ID)
	assert.Equal(t, ".kapi/voice.yaml", got.Voice[0].Binding)
	require.NotNil(t, got.Voice[0].Profile)
	assert.Equal(t, "Acme", got.Voice[0].Profile.Name)
	assert.Equal(t, "neutral", got.Voice[0].Profile.Tone.Formality)

	require.Len(t, got.Decisions, 1)
	assert.Equal(t, DecisionsDir+"d-docs.jsonl", got.Decisions[0].Path)
	assert.Equal(t, pkg.Decisions[0].Data, got.Decisions[0].Data)
}

// TestContextPackage_IsDeterministic: two marshals of the same package are the
// same bytes, which is what lets a bundle be compared rather than merely read.
func TestContextPackage_IsDeterministic(t *testing.T) {
	first, err := contextPackage().Marshal()
	require.NoError(t, err)
	second, err := contextPackage().Marshal()
	require.NoError(t, err)
	assert.Equal(t, first, second)

	hash, err := contextPackage().RootHash()
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

// TestContextPackage_RefusesABindingOutsideTheProject: a binding is a path a
// restore writes to, so it is validated with every other path in the manifest.
func TestContextPackage_RefusesABindingOutsideTheProject(t *testing.T) {
	tests := []struct {
		name    string
		binding string
	}{
		{name: "climbing out", binding: "../../.kapi/voice.yaml"},
		{name: "absolute", binding: "/etc/kapi/voice.yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkg := contextPackage()
			pkg.Voice[0].Binding = tc.binding
			data, err := pkg.Marshal()
			require.NoError(t, err)

			_, err = Unmarshal(data)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "voice binding")
		})
	}
}

// TestPackage_RefusesAnUnknownKind names the three profiles it does read, so a
// package of a fourth is refused with the list rather than a bare no.
func TestPackage_RefusesAnUnknownKind(t *testing.T) {
	pkg := contextPackage()
	pkg.Kind = "kapi-something-else"
	data, err := pkg.Marshal()
	require.NoError(t, err)

	_, err = Unmarshal(data)
	require.Error(t, err)
	for _, kind := range []string{KindProject, KindInterchange, KindContext} {
		assert.Contains(t, err.Error(), kind)
	}
}
