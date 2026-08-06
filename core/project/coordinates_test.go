package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// twoProductProject is the case the context model exists for: one recipe, two
// products, several channels, two markets. The base voice is stated once, each
// product refines it, and one product refines it again for one market — so no
// voice is copied into a second file that can drift from the first.
func twoProductProject() *KapiProject {
	return &KapiProject{
		Version: "v1",
		Name:    "Two products",
		Defaults: Defaults{
			SourceLanguage: "en",
			BrandVoice:     &BrandVoiceBinding{ProfileFile: "context/base-voice.yaml"},
		},
		Coordinates: Coordinates{
			"product": {{ID: "kapi"}, {ID: "bowrain", Concept: "term:9a1c0f42b7"}},
			"channel": {{ID: "docs"}, {ID: "landing"}, {ID: "email"}},
			"market":  {{ID: "us"}, {ID: "de"}},
			"tenant":  {},
		},
		Profiles: []ProfileBinding{
			{
				When:  map[string]string{"product": "kapi"},
				Voice: &BrandVoiceBinding{ProfileFile: "context/kapi-voice.yaml"},
			},
			{
				When:  map[string]string{"product": "bowrain"},
				Voice: &BrandVoiceBinding{ProfileFile: "context/bowrain-voice.yaml"},
				Terms: "context/bowrain-terms.json",
			},
			{
				When:  map[string]string{"product": "bowrain", "market": "de"},
				Voice: &BrandVoiceBinding{ProfileFile: "context/bowrain-de.yaml"},
				Terms: "context/de-terms.json",
			},
		},
		Content: []ContentCollection{
			{
				Name:    "docs",
				Context: map[string]string{"product": "kapi", "channel": "docs"},
				Items:   []ContentItem{{Path: "web/docs/**/*.md"}},
			},
			{
				Name:    "docs-bowrain",
				Context: map[string]string{"product": "bowrain", "channel": "docs"},
				Items:   []ContentItem{{Path: "bowrain/web/docs/**/*.md"}},
			},
			{
				Name:    "landing",
				Context: map[string]string{"product": "bowrain", "channel": "landing"},
				Items:   []ContentItem{{Path: "bowrain/web/src/pages/*.tsx"}},
			},
			{
				Name:    "landing-de",
				Context: map[string]string{"product": "bowrain", "channel": "landing", "market": "de"},
				Items:   []ContentItem{{Path: "bowrain/web/src/pages/de/*.tsx"}},
			},
			{
				Name:  "emails",
				Items: []ContentItem{{Path: "context/emails/*.json"}},
			},
			{Path: "README.md", Target: "README.{lang}.md"},
		},
	}
}

func TestKapiProject_ResolveGovernance(t *testing.T) {
	proj := twoProductProject()
	base := proj.Defaults.BrandVoice

	tests := []struct {
		name       string
		collection string
		want       ResolvedGovernance
	}{
		{
			name:       "empty name sits at the project default point",
			collection: "",
			want:       ResolvedGovernance{Voice: base, VoiceField: "defaults.brand_voice"},
		},
		{
			name:       "unknown name sits at the project default point",
			collection: "no-such-collection",
			want:       ResolvedGovernance{Voice: base, VoiceField: "defaults.brand_voice"},
		},
		{
			name:       "a collection declaring no coordinates inherits",
			collection: "emails",
			want:       ResolvedGovernance{Voice: base, VoiceField: "defaults.brand_voice"},
		},
		{
			name:       "a matched profile binding no terms leaves the project store governing",
			collection: "docs",
			want: ResolvedGovernance{
				Channel:    "docs",
				Voice:      &BrandVoiceBinding{ProfileFile: "context/kapi-voice.yaml"},
				VoiceField: "profiles[0].voice",
				Profile:    "kapi",
			},
		},
		{
			name:       "a matched profile binds voice and terms",
			collection: "docs-bowrain",
			want: ResolvedGovernance{
				Channel:    "docs",
				Voice:      &BrandVoiceBinding{ProfileFile: "context/bowrain-voice.yaml"},
				Terms:      "context/bowrain-terms.json",
				VoiceField: "profiles[1].voice",
				Profile:    "bowrain",
			},
		},
		{
			name:       "one voice, two channels: the same profile, a different override",
			collection: "landing",
			want: ResolvedGovernance{
				Channel:    "landing",
				Voice:      &BrandVoiceBinding{ProfileFile: "context/bowrain-voice.yaml"},
				Terms:      "context/bowrain-terms.json",
				VoiceField: "profiles[1].voice",
				Profile:    "bowrain",
			},
		},
		{
			name:       "the most specific match wins over the broader one",
			collection: "landing-de",
			want: ResolvedGovernance{
				Channel:    "landing",
				Voice:      &BrandVoiceBinding{ProfileFile: "context/bowrain-de.yaml"},
				Terms:      "context/de-terms.json",
				VoiceField: "profiles[2].voice",
				Profile:    "de-bowrain",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, err := proj.ResolveGovernance(tt.collection)
			require.NoError(t, err)
			assert.Equal(t, &tt.want, rc)
		})
	}
}

// TestKapiProject_ResolveGovernance_BaseProfile covers the `when: {}` base: it
// governs every point no profile claims more specifically, and always loses to
// one that does. It also pins what "wins" means — one profile governs, and what
// it leaves unbound stays unbound, rather than being inherited from the broader
// profile it beat.
func TestKapiProject_ResolveGovernance_BaseProfile(t *testing.T) {
	proj := &KapiProject{
		Version: "v1",

		Coordinates: Coordinates{"product": {{ID: "kapi"}, {ID: "bowrain"}}},
		Profiles: []ProfileBinding{
			{Voice: &BrandVoiceBinding{ProfileFile: "base.yaml"}, Terms: "base-terms.json"},
			{
				When:  map[string]string{"product": "bowrain"},
				Voice: &BrandVoiceBinding{ProfileFile: "bowrain.yaml"},
			},
		},
		Content: []ContentCollection{
			{Name: "plain", Items: []ContentItem{{Path: "a/*.md"}}},
			{
				Name:    "platform",
				Context: map[string]string{"product": "bowrain"},
				Items:   []ContentItem{{Path: "b/*.md"}},
			},
		},
	}
	require.NoError(t, proj.Validate())

	plain, err := proj.ResolveGovernance("plain")
	require.NoError(t, err)
	assert.Equal(t, "base.yaml", plain.Voice.ProfileFile, "the base profile governs an unclaimed point")
	assert.Equal(t, "base-terms.json", plain.Terms)

	platform, err := proj.ResolveGovernance("platform")
	require.NoError(t, err)
	assert.Equal(t, "bowrain.yaml", platform.Voice.ProfileFile, "a non-empty match beats the base")
	assert.Empty(t, platform.Terms,
		"the winning profile binds no terms, so the project's own store governs — profiles select, they do not layer")
}

// TestKapiProject_ResolveGovernance_NoProfiles covers the project that declares
// coordinates but binds no governance to them: every point resolves the project
// defaults, and an unbound default resolves to nothing rather than to something
// borrowed.
func TestKapiProject_ResolveGovernance_NoProfiles(t *testing.T) {
	proj := &KapiProject{
		Version:     "v1",
		Coordinates: Coordinates{"product": {{ID: "kapi"}}},
		Content: []ContentCollection{{
			Name:    "docs",
			Context: map[string]string{"product": "kapi"},
			Items:   []ContentItem{{Path: "a/*.md"}},
		}},
	}

	rc, err := proj.ResolveGovernance("docs")
	require.NoError(t, err)
	assert.Nil(t, rc.Voice)
	assert.Empty(t, rc.Terms)
	assert.Equal(t, "defaults.brand_voice", rc.VoiceField)
}

// TestValidate_Tie is the ambiguity this model refuses to settle by luck: two
// profiles matching one collection on the same number of coordinates. Which
// voice a piece of content is written in is not a map-order question, so the
// recipe does not load.
func TestValidate_Tie(t *testing.T) {
	proj := twoProductProject()
	proj.Profiles = append(proj.Profiles, ProfileBinding{
		When:  map[string]string{"channel": "landing"},
		Voice: &BrandVoiceBinding{ProfileFile: "context/landing-voice.yaml"},
	})

	err := proj.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `content[2]: collection "landing" is matched by`)
	assert.Contains(t, err.Error(), "profiles[1] (when: {product: bowrain})")
	assert.Contains(t, err.Error(), "profiles[3] (when: {channel: landing})")
	assert.Contains(t, err.Error(), "with equal specificity")
	assert.NotContains(t, err.Error(), `collection "landing-de"`,
		"the market coordinate makes profiles[2] the more specific match there, so it is not ambiguous")
}

// TestValidate_Tie_AtLoad proves the ambiguity check runs on the real load
// path, not only on a hand-built struct.
func TestValidate_Tie_AtLoad(t *testing.T) {
	proj := twoProductProject()
	proj.Profiles = append(proj.Profiles, ProfileBinding{
		When:  map[string]string{"channel": "landing"},
		Voice: &BrandVoiceBinding{ProfileFile: "context/landing-voice.yaml"},
	})
	path := filepath.Join(t.TempDir(), RecipeFileName)
	// Save does not validate; the failure has to come from Load.
	require.NoError(t, Save(path, proj))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "with equal specificity")
}

func TestValidate_Coordinates(t *testing.T) {
	tests := []struct {
		name    string
		coords  Coordinates
		wantErr string
	}{
		{
			name:   "slug axes and slug values",
			coords: Coordinates{"product": {{ID: "kapi"}, {ID: "bowrain-cloud"}}, "market": {{ID: "de"}}},
		},
		{
			name:   "an axis with no values is open",
			coords: Coordinates{"tenant": {}},
		},
		{
			name:    "an axis name must be a slug",
			coords:  Coordinates{"Product Line": {{ID: "kapi"}}},
			wantErr: `coordinates: axis name "Product Line" is not a slug (lowercase letters, digits and hyphens, starting with a letter or digit)`,
		},
		{
			name:    "an axis name cannot start with a hyphen",
			coords:  Coordinates{"-product": {{ID: "kapi"}}},
			wantErr: `coordinates: axis name "-product" is not a slug`,
		},
		{
			name:    "a value must be a slug",
			coords:  Coordinates{"product": {{ID: "Bowrain Cloud"}}},
			wantErr: `coordinates.product: value "Bowrain Cloud" is not a slug`,
		},
		{
			name:    "a value cannot be a concept reference",
			coords:  Coordinates{"product": {{ID: "term:9a1c0f42b7"}}},
			wantErr: `coordinates.product: value "term:9a1c0f42b7" is not a slug`,
		},
		{
			name:    "a value needs an id",
			coords:  Coordinates{"product": {{Concept: "term:9a1c0f42b7"}}},
			wantErr: "coordinates.product[0]: a value needs an id",
		},
		{
			name:    "a value cannot be declared twice",
			coords:  Coordinates{"product": {{ID: "kapi"}, {ID: "kapi", Concept: "term:9a1c0f42b7"}}},
			wantErr: `coordinates.product: value "kapi" is declared twice`,
		},
		{
			name:   "a concept reference is carried",
			coords: Coordinates{"product": {{ID: "kapi", Concept: "term:9a1c0f42b7"}}},
		},
		{
			name:    "a concept reference must be an id",
			coords:  Coordinates{"product": {{ID: "kapi", Concept: "the kapi framework"}}},
			wantErr: `coordinates.product: value "kapi" has an invalid concept reference "the kapi framework" (want a concept id, e.g. term:9a1c0f42b7)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&KapiProject{Version: "v1", Coordinates: tt.coords}).Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestValidate_Point covers the coordinates a collection and a profile name:
// both are checked against the declared axes, and both name what the recipe
// does declare — the failure is nearly always a typo, and a silent fall-back
// would govern that content in a plausible-looking wrong voice.
func TestValidate_Point(t *testing.T) {
	declared := Coordinates{
		"product": {{ID: "kapi"}, {ID: "bowrain"}},
		"channel": {{ID: "docs"}},
		"tenant":  {},
	}
	voice := &BrandVoiceBinding{ProfileFile: "voice.yaml"}

	tests := []struct {
		name    string
		context map[string]string
		when    map[string]string
		wantErr string
	}{
		{
			name:    "a declared axis and a declared value",
			context: map[string]string{"product": "bowrain", "channel": "docs"},
		},
		{
			name:    "an undeclared axis in a context",
			context: map[string]string{"brand": "bowrain"},
			wantErr: `content[0]: collection "docs" sets coordinate "brand", which coordinates: does not declare (declared: channel, product, tenant)`,
		},
		{
			name:    "an undeclared value in a context",
			context: map[string]string{"product": "browrain"},
			wantErr: `content[0]: collection "docs" sets product to "browrain", which coordinates.product does not declare (declared: kapi, bowrain)`,
		},
		{
			name:    "a non-slug value in a context",
			context: map[string]string{"tenant": "ACME Corp"},
			wantErr: `content[0]: collection "docs" sets tenant to "ACME Corp", which is not a slug (lowercase letters, digits and hyphens, starting with a letter or digit)`,
		},
		{
			name:    "an open axis accepts any well-formed slug",
			context: map[string]string{"tenant": "acme-corp"},
		},
		{
			name:    "an undeclared axis in a profile match",
			when:    map[string]string{"brand": "bowrain"},
			wantErr: `profiles[0]: when sets coordinate "brand", which coordinates: does not declare (declared: channel, product, tenant)`,
		},
		{
			name:    "an undeclared value in a profile match",
			when:    map[string]string{"channel": "carrier-pigeon"},
			wantErr: `profiles[0]: when sets channel to "carrier-pigeon", which coordinates.channel does not declare (declared: docs)`,
		},
		{
			name:    "a non-slug value in a profile match",
			when:    map[string]string{"tenant": "ACME"},
			wantErr: `profiles[0]: when sets tenant to "ACME", which is not a slug`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj := &KapiProject{
				Version:     "v1",
				Coordinates: declared,
				Content: []ContentCollection{{
					Name:    "docs",
					Context: tt.context,
					Items:   []ContentItem{{Path: "a/*.md"}},
				}},
			}
			if tt.when != nil {
				proj.Profiles = []ProfileBinding{{When: tt.when, Voice: voice}}
			}
			err := proj.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestValidate_UndeclaredAxis_NoCoordinates covers naming an axis in a recipe
// that declares no coordinates at all — the same error, saying so plainly.
func TestValidate_UndeclaredAxis_NoCoordinates(t *testing.T) {
	proj := &KapiProject{
		Version: "v1",
		Content: []ContentCollection{{
			Name:    "docs",
			Context: map[string]string{"product": "kapi"},
			Items:   []ContentItem{{Path: "a/*.md"}},
		}},
	}

	err := proj.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "(declared: none)")
}

func TestValidate_Profiles(t *testing.T) {
	coords := Coordinates{"product": {{ID: "kapi"}, {ID: "bowrain"}}}

	tests := []struct {
		name     string
		profiles []ProfileBinding
		content  []ContentCollection
		wantErr  string
	}{
		{
			name: "a profile binding a voice",
			profiles: []ProfileBinding{{
				When:  map[string]string{"product": "kapi"},
				Voice: &BrandVoiceBinding{Pack: "technical-docs"},
			}},
		},
		{
			name:     "a profile binding only terms",
			profiles: []ProfileBinding{{When: map[string]string{"product": "kapi"}, Terms: "kapi-terms.json"}},
		},
		{
			name:     "a profile must bind something",
			profiles: []ProfileBinding{{When: map[string]string{"product": "kapi"}}},
			wantErr:  "profiles[0]: a profile must bind a voice, terms, or both",
		},
		{
			name: "a profile voice takes exactly one source",
			profiles: []ProfileBinding{{
				Voice: &BrandVoiceBinding{ProfileFile: "voice.yaml", Pack: "technical-docs"},
			}},
			wantErr: "profiles[0].voice: profile_file, profile, and pack are mutually exclusive",
		},
		{
			name:     "a profile voice with no source is rejected",
			profiles: []ProfileBinding{{Voice: &BrandVoiceBinding{}}},
			wantErr:  "profiles[0].voice: specify one of profile_file, profile, or pack",
		},
		{
			name: "two profiles cannot claim the same point",
			profiles: []ProfileBinding{
				{When: map[string]string{"product": "kapi"}, Voice: &BrandVoiceBinding{ProfileFile: "a.yaml"}},
				{When: map[string]string{"product": "bowrain"}, Voice: &BrandVoiceBinding{ProfileFile: "b.yaml"}},
				{When: map[string]string{"product": "kapi"}, Voice: &BrandVoiceBinding{ProfileFile: "c.yaml"}},
			},
			wantErr: "profiles[2]: when {product: kapi} is already bound by profiles[0]; one point cannot have two profiles",
		},
		{
			name: "two base profiles are the same ambiguity",
			profiles: []ProfileBinding{
				{Voice: &BrandVoiceBinding{ProfileFile: "a.yaml"}},
				{Voice: &BrandVoiceBinding{ProfileFile: "b.yaml"}},
			},
			wantErr: "profiles[1]: when {} is already bound by profiles[0]",
		},
		{
			name: "a bare entry cannot name a context",
			profiles: []ProfileBinding{{
				When:  map[string]string{"product": "kapi"},
				Voice: &BrandVoiceBinding{Pack: "technical-docs"},
			}},
			content: []ContentCollection{{Path: "a/*.md", Context: map[string]string{"product": "kapi"}}},
			wantErr: "content[0]: context requires a named collection (add `name:`)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&KapiProject{
				Version:     "v1",
				Coordinates: coords,
				Profiles:    tt.profiles,
				Content:     tt.content,
			}).Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestCoordinateValue_Forms covers both authored forms of a declared value, and
// the rule that keeps them interchangeable: the concept is display metadata, so
// a value carrying one matches exactly as the bare slug does.
func TestCoordinateValue_Forms(t *testing.T) {
	const recipe = `version: v1
coordinates:
  product:
    - kapi
    - id: bowrain
      concept: term:9a1c0f42b7
profiles:
  - when: {product: bowrain}
    voice: context/bowrain-voice.yaml
content:
  - name: landing
    context: {product: bowrain}
    items:
      - path: web/*.tsx
`
	var proj KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(recipe), &proj))
	require.NoError(t, proj.Validate())

	assert.Equal(t, []CoordinateValue{
		{ID: "kapi"},
		{ID: "bowrain", Concept: "term:9a1c0f42b7"},
	}, proj.Coordinates["product"], "both forms decode to the same shape")

	rc, err := proj.ResolveGovernance("landing")
	require.NoError(t, err)
	require.NotNil(t, rc.Voice)
	assert.Equal(t, "context/bowrain-voice.yaml", rc.Voice.ProfileFile,
		"the short voice form is the profile file, and the concept does not affect matching")
}

// TestContextSpace_RoundTrip proves the recipe carries the coordinates, the
// profiles and each collection's point through Save/Load — governance the
// framework dropped on save would silently revert a project to one voice — and
// that the authored short forms survive the trip.
func TestContextSpace_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), RecipeFileName)
	require.NoError(t, Save(path, twoProductProject()))

	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(saved), "- kapi", "a value with no concept saves as the bare slug")
	assert.Contains(t, string(saved), "voice: context/kapi-voice.yaml",
		"a voice bound to a file saves as the bare path")

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, twoProductProject().Coordinates, loaded.Coordinates)
	require.Len(t, loaded.Profiles, 3)

	rc, err := loaded.ResolveGovernance("landing-de")
	require.NoError(t, err)
	assert.Equal(t, "landing", rc.Channel)
	assert.Equal(t, "context/bowrain-de.yaml", rc.Voice.ProfileFile)
	assert.Equal(t, "context/de-terms.json", rc.Terms)
}

func TestKapiProject_CollectionForPath(t *testing.T) {
	proj := twoProductProject()

	tests := []struct {
		name string
		path string
		want string
	}{
		{"first collection", "web/docs/intro.md", "docs"},
		{"nested match", "bowrain/web/docs/guide/setup.md", "docs-bowrain"},
		{"another collection's pattern", "context/emails/welcome.json", "emails"},
		{"bare entry has no name", "README.md", ""},
		{"no pattern matches", "scripts/build.sh", ""},
		{"a directory prefix is not a match", "web/docs", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, proj.CollectionForPath(tt.path))
		})
	}
}
