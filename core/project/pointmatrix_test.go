package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The point matrix: one recipe, every point it declares, and the COMPLETE
// governance that must resolve at each.
//
// The tests around it check one resolution step each — MergeCoordinates ranks
// layers, ResolveChannel parses a ref, ValidateContextSpace rejects a bad space.
// This one checks their composition, which is the question a reader of a recipe
// actually has: given this file, what governs my content here? Nothing asserted
// that end to end, and it is the class of failure the coordinate model created
// the possibility of — every part correct, the whole resolving somewhere else.
//
// Two rules make it a matrix rather than a pile of cases.
//
// Every case states the WHOLE expected governance, never a field or two. A
// partial assertion passes just as happily when a binding is broader than it
// should be — the profile you asserted is there, and so is the channel override
// you did not think to look at — which is exactly how a too-broad binding hides.
//
// And every declared point appears exactly once, asserted by
// TestPointMatrixCoversEveryDeclaredPoint. A matrix with a hole is a matrix that
// agrees with whatever the resolver does at the point nobody wrote down.

// matrixRecipe declares a context space with the shapes that break resolvers:
// two profiles sharing a channel name, a profile-level voice and a
// profile-level termstore that do not travel together, an item overriding its
// collection's channel, and a collection bound to nothing.
const matrixRecipe = `version: v1
name: point-matrix
defaults:
  source_language: en
  target_languages: [nb]
  voice:
    profile_file: .kapi/voice.yaml
profiles:
  acme:
    channels: [web, support]
    voice: .kapi/profiles/acme/voice.yaml
    termstore: .kapi/profiles/acme/terms.json
  other:
    channels: [web, email]
    voice: .kapi/profiles/other/voice.yaml
collections:
  - name: acme-web
    channel: acme/web
    content:
      - path: sites/acme/**/*.mdx
  - name: acme-support
    channel: acme/support
    content:
      - path: support/acme/**/*.md
  - name: other-web
    channel: other/web
    content:
      - path: sites/other/**/*.mdx
  - name: mixed
    channel: acme/web
    content:
      - path: mixed/plain/**/*.md
      - path: mixed/mail/**/*.md
        channel: other/email
  - name: loose
    content:
      - path: README.md
`

// governanceAtPoint is the whole answer a point resolves to. Every field the
// recipe can govern is here, so a case that leaves one out is a case that
// asserts it empty rather than a case that ignores it.
type governanceAtPoint struct {
	Profile    string
	Channel    string
	VoiceFile  string
	VoiceField string
	TermStore  string
}

func resolvedAt(t *testing.T, rc *ResolvedGovernance) governanceAtPoint {
	t.Helper()
	got := governanceAtPoint{
		Profile:    rc.Profile,
		Channel:    rc.Channel,
		VoiceField: rc.VoiceField,
		TermStore:  rc.TermStore,
	}
	if rc.Voice != nil {
		got.VoiceFile = rc.Voice.ProfileFile
	}
	return got
}

func TestPointMatrix(t *testing.T) {
	p := loadRecipe(t, matrixRecipe)

	// The default point's answer, named once so the cases that expect it cannot
	// drift apart from each other.
	defaultPoint := governanceAtPoint{
		VoiceFile:  ".kapi/voice.yaml",
		VoiceField: DefaultVoiceField,
	}

	tests := []struct {
		name       string
		collection string
		want       governanceAtPoint
	}{
		{
			name:       "a bound collection resolves its profile's voice and termstore",
			collection: "acme-web",
			want: governanceAtPoint{
				Profile:    "acme",
				Channel:    "web",
				VoiceFile:  ".kapi/profiles/acme/voice.yaml",
				VoiceField: "profiles.acme.voice",
				TermStore:  ".kapi/profiles/acme/terms.json",
			},
		},
		{
			// Same profile, different channel. The channel moves and everything
			// governed at the profile rung stays put — the case a resolver that
			// keys governance on the channel alone gets wrong.
			name:       "a second channel under one profile keeps the profile's governance",
			collection: "acme-support",
			want: governanceAtPoint{
				Profile:    "acme",
				Channel:    "support",
				VoiceFile:  ".kapi/profiles/acme/voice.yaml",
				VoiceField: "profiles.acme.voice",
				TermStore:  ".kapi/profiles/acme/terms.json",
			},
		},
		{
			// `web` is declared by BOTH profiles. A resolver that matches on the
			// channel name rather than the qualified ref lands on whichever it
			// walked into first, and the two differ in exactly one field.
			name:       "a channel name two profiles share resolves by its profile",
			collection: "other-web",
			want: governanceAtPoint{
				Profile:    "other",
				Channel:    "web",
				VoiceFile:  ".kapi/profiles/other/voice.yaml",
				VoiceField: "profiles.other.voice",
				// `other` declares no termstore, so the project's own governs.
				// Asserted empty rather than omitted: inheriting acme's here
				// would be a leak between profiles and would pass any test that
				// only checked the voice.
				TermStore: "",
			},
		},
		{
			name:       "a collection bound to nothing sits at the default point",
			collection: "loose",
			want:       defaultPoint,
		},
		{
			name:       "a name no collection claims resolves the default point",
			collection: "nothing-declares-this",
			want:       defaultPoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, err := p.ResolveGovernance(tt.collection)
			require.NoError(t, err)
			assert.Equal(t, tt.want, resolvedAt(t, rc))
		})
	}
}

func TestPointMatrixByPath(t *testing.T) {
	p := loadRecipe(t, matrixRecipe)

	tests := []struct {
		name string
		path string
		want governanceAtPoint
	}{
		{
			name: "a file takes its collection's point",
			path: "mixed/plain/notes.md",
			want: governanceAtPoint{
				Profile:    "acme",
				Channel:    "web",
				VoiceFile:  ".kapi/profiles/acme/voice.yaml",
				VoiceField: "profiles.acme.voice",
				TermStore:  ".kapi/profiles/acme/terms.json",
			},
		},
		{
			// One item overriding its collection's channel moves the whole
			// answer, profile included — the finer rung wins, and it wins for
			// every field rather than for the channel alone.
			name: "an item's own channel overrides its collection's, profile and all",
			path: "mixed/mail/welcome.md",
			want: governanceAtPoint{
				Profile:    "other",
				Channel:    "email",
				VoiceFile:  ".kapi/profiles/other/voice.yaml",
				VoiceField: "profiles.other.voice",
				TermStore:  "",
			},
		},
		{
			name: "a path no item claims resolves the default point",
			path: "somewhere/unclaimed.md",
			want: governanceAtPoint{
				VoiceFile:  ".kapi/voice.yaml",
				VoiceField: DefaultVoiceField,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, err := p.ResolveGovernanceForPath(tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, resolvedAt(t, rc))
		})
	}
}

// TestPointMatrixCoversEveryDeclaredPoint is what makes the matrix a matrix.
//
// Every collection the recipe declares must be asserted above. Without this,
// adding a collection to matrixRecipe — or, in the shape that actually happens,
// adding one to a real recipe and copying this file as the template — leaves a
// point whose resolution nothing states, and the resolver agrees with itself
// there forever.
func TestPointMatrixCoversEveryDeclaredPoint(t *testing.T) {
	p := loadRecipe(t, matrixRecipe)

	asserted := map[string]bool{
		"acme-web":     true,
		"acme-support": true,
		"other-web":    true,
		"mixed":        true, // covered by path, both of its points
		"loose":        true,
	}

	for _, c := range p.Collections {
		assert.True(t, asserted[c.Name],
			"collection %q is declared by the recipe and asserted by no case in the matrix", c.Name)
	}
	assert.Len(t, asserted, len(p.Collections),
		"the matrix asserts a collection the recipe does not declare")
}
