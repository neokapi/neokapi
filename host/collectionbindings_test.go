package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The platform voice carries a channel override, so a test can prove the
// channel coordinate reaches profile resolution rather than being carried and
// dropped.
const platformVoiceYAML = `id: platform
name: Platform Voice
tone:
  formality: neutral
channels:
  landing:
    tone:
      formality: casual
`

// governedProject writes a recipe declaring a two-product context space, a
// profile governing one of them, and the voice YAMLs they bind. platformContext
// is the point the platform collection declares; nil leaves that collection at
// the project's default point. Returns the recipe path and root.
func governedProject(t *testing.T, platformContext map[string]string) (recipe, root string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "voice.yaml"),
		[]byte("id: house\nname: House Style\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "platform-voice.yaml"),
		[]byte(platformVoiceYAML), 0o644))

	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Governed",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
			Voice:           &project.VoiceBinding{ProfileFile: "voice.yaml"},
		},
		Coordinates: project.Coordinates{
			"product": {{ID: "framework"}, {ID: "platform"}},
			"channel": {{ID: "docs"}, {ID: "landing"}},
		},
		Profiles: []project.ProfileBinding{{
			When:  map[string]string{"product": "platform"},
			Voice: &project.VoiceBinding{ProfileFile: "platform-voice.yaml"},
		}},
		Content: []project.ContentCollection{
			{Name: "framework-docs", Items: []project.ContentItem{{Path: "docs/**/*.md"}}},
			{Name: "platform-docs", Context: platformContext, Items: []project.ContentItem{{Path: "platform/**/*.md"}}},
		},
	}
	recipe = filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	return recipe, dir
}

// bindingsCmd is a project-bound command for the resolvers, the same shape the
// embedded orchestrator builds (NewEnvCommand + the project flag).
func bindingsCmd(t *testing.T, recipe string) Command {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "bindings-test")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set(projectFlagName, recipe))
	return cmd
}

func TestGroupInputsByBinding(t *testing.T) {
	defaultVoice := &project.VoiceBinding{ProfileFile: "voice.yaml"}
	coords := project.Coordinates{
		"product": {{ID: "platform"}, {ID: "press"}, {ID: "house"}},
		"channel": {{ID: "docs"}, {ID: "landing"}},
		"market":  {{ID: "us"}, {ID: "de"}},
	}
	profiles := []project.ProfileBinding{
		{
			When:  map[string]string{"product": "platform"},
			Voice: &project.VoiceBinding{ProfileFile: "platform-voice.yaml"},
		},
		{
			When:  map[string]string{"product": "press"},
			Voice: &project.VoiceBinding{Pack: "marketing-blog"},
			Terms: "press-terms.db",
		},
		{
			// A profile binding exactly what the project defaults bind: the
			// default voice, and no standalone terms — so the project's own
			// store governs its vocabulary, exactly as at the default point.
			When:  map[string]string{"product": "house"},
			Voice: defaultVoice,
		},
	}
	root := filepath.FromSlash("/repo")

	newProj := func(content ...project.ContentCollection) *project.KapiProject {
		return &project.KapiProject{
			Version:     "v1",
			Defaults:    project.Defaults{Voice: defaultVoice},
			Coordinates: coords,
			Profiles:    profiles,
			Content:     content,
		}
	}
	abs := func(rel ...string) []string {
		out := make([]string, 0, len(rel))
		for _, r := range rel {
			out = append(out, filepath.Join(root, filepath.FromSlash(r)))
		}
		return out
	}

	tests := []struct {
		name   string
		proj   *project.KapiProject
		inputs []string
		want   []bindingGroup
	}{
		{
			name: "no collection names a context: one ungrouped run",
			proj: newProj(
				project.ContentCollection{Name: "docs", Items: []project.ContentItem{{Path: "docs/**/*.md"}}},
				project.ContentCollection{Name: "app", Items: []project.ContentItem{{Path: "app/**/*.json"}}},
			),
			inputs: abs("docs/a.md", "app/b.json"),
			want:   []bindingGroup{{Collection: "", Inputs: abs("docs/a.md", "app/b.json")}},
		},
		{
			name: "two products: one group each, in input order",
			proj: newProj(
				project.ContentCollection{Name: "docs", Items: []project.ContentItem{{Path: "docs/**/*.md"}}},
				project.ContentCollection{
					Name:    "platform",
					Context: map[string]string{"product": "platform"},
					Items:   []project.ContentItem{{Path: "platform/**/*.md"}},
				},
			),
			inputs: abs("docs/a.md", "platform/b.md", "docs/c.md"),
			want: []bindingGroup{
				{Collection: "", Inputs: abs("docs/a.md", "docs/c.md")},
				{Collection: "platform", Inputs: abs("platform/b.md")},
			},
		},
		{
			name: "collections at the same point share one group",
			proj: newProj(
				project.ContentCollection{
					Name:    "platform-docs",
					Context: map[string]string{"product": "platform", "channel": "docs"},
					Items:   []project.ContentItem{{Path: "docs/**/*.md"}},
				},
				project.ContentCollection{
					Name:    "platform-notes",
					Context: map[string]string{"product": "platform", "channel": "docs"},
					Items:   []project.ContentItem{{Path: "notes/**/*.md"}},
				},
			),
			inputs: abs("docs/a.md", "notes/b.md"),
			want:   []bindingGroup{{Collection: "platform-docs", Inputs: abs("docs/a.md", "notes/b.md")}},
		},
		{
			name: "two points governed by one profile share one group",
			proj: newProj(
				project.ContentCollection{
					Name:    "platform-us",
					Context: map[string]string{"product": "platform", "market": "us"},
					Items:   []project.ContentItem{{Path: "us/**/*.md"}},
				},
				project.ContentCollection{
					Name:    "platform-de",
					Context: map[string]string{"product": "platform", "market": "de"},
					Items:   []project.ContentItem{{Path: "de/**/*.md"}},
				},
			),
			inputs: abs("us/a.md", "de/b.md"),
			want:   []bindingGroup{{Collection: "platform-us", Inputs: abs("us/a.md", "de/b.md")}},
		},
		{
			name: "same profile, different channel: different governance, different groups",
			proj: newProj(
				project.ContentCollection{
					Name:    "platform-docs",
					Context: map[string]string{"product": "platform", "channel": "docs"},
					Items:   []project.ContentItem{{Path: "docs/**/*.md"}},
				},
				project.ContentCollection{
					Name:    "platform-landing",
					Context: map[string]string{"product": "platform", "channel": "landing"},
					Items:   []project.ContentItem{{Path: "landing/**/*.md"}},
				},
			),
			inputs: abs("docs/a.md", "landing/b.md"),
			want: []bindingGroup{
				{Collection: "platform-docs", Inputs: abs("docs/a.md")},
				{Collection: "platform-landing", Inputs: abs("landing/b.md")},
			},
		},
		{
			name: "a point that resolves to the defaults resolves as the defaults",
			proj: newProj(project.ContentCollection{
				Name:    "docs",
				Context: map[string]string{"product": "house"},
				Items:   []project.ContentItem{{Path: "docs/**/*.md"}},
			}),
			inputs: abs("docs/a.md"),
			want:   []bindingGroup{{Collection: "", Inputs: abs("docs/a.md")}},
		},
		{
			name: "a profile with its own terms splits too",
			proj: newProj(
				project.ContentCollection{Name: "docs", Items: []project.ContentItem{{Path: "docs/**/*.md"}}},
				project.ContentCollection{
					Name:    "press",
					Context: map[string]string{"product": "press"},
					Items:   []project.ContentItem{{Path: "press/**/*.md"}},
				},
			),
			inputs: abs("docs/a.md", "press/b.md"),
			want: []bindingGroup{
				{Collection: "", Inputs: abs("docs/a.md")},
				{Collection: "press", Inputs: abs("press/b.md")},
			},
		},
		{
			name: "unclaimed and outside-the-root paths sit at the default point",
			proj: newProj(
				project.ContentCollection{
					Name:    "platform",
					Context: map[string]string{"product": "platform"},
					Items:   []project.ContentItem{{Path: "platform/**/*.md"}},
				},
			),
			inputs: append(abs("scripts/build.sh", "platform/b.md"), filepath.FromSlash("/elsewhere/c.md")),
			want: []bindingGroup{
				{Collection: "", Inputs: append(abs("scripts/build.sh"), filepath.FromSlash("/elsewhere/c.md"))},
				{Collection: "platform", Inputs: abs("platform/b.md")},
			},
		},
		{
			name:   "no project is one group",
			proj:   nil,
			inputs: abs("docs/a.md"),
			want:   []bindingGroup{{Collection: "", Inputs: abs("docs/a.md")}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.proj != nil {
				require.NoError(t, tt.proj.Validate(), "the fixture must be a loadable recipe")
			}
			got, err := groupInputsByBinding(tt.proj, root, tt.inputs)
			require.NoError(t, err)
			require.Len(t, got, len(tt.want))
			for i := range tt.want {
				assert.Equal(t, tt.want[i].Collection, got[i].Collection, "group %d collection", i)
				assert.Equal(t, tt.want[i].Inputs, got[i].Inputs, "group %d inputs", i)
			}
		})
	}
}

// TestResolveProjectBindings_PerContextVoice is the point of the feature: two
// collections of one project resolve two different voice profiles, and a
// collection sitting at the project's default point still resolves the
// project-wide one.
func TestResolveProjectBindings_PerContextVoice(t *testing.T) {
	recipe, _ := governedProject(t, map[string]string{"product": "platform"})
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	a := &App{}
	cmd := bindingsCmd(t, recipe)

	defaults, err := a.resolveProjectBindings(cmd, proj, recipe, "")
	require.NoError(t, err)
	require.NotNil(t, defaults)
	require.NotNil(t, defaults.profile)
	assert.Equal(t, "House Style", defaults.profile.Name)

	framework, err := a.resolveProjectBindings(cmd, proj, recipe, "framework-docs")
	require.NoError(t, err)
	require.NotNil(t, framework.profile)
	assert.Equal(t, "House Style", framework.profile.Name,
		"a collection that names no context keeps the project voice")

	platform, err := a.resolveProjectBindings(cmd, proj, recipe, "platform-docs")
	require.NoError(t, err)
	require.NotNil(t, platform.profile)
	assert.Equal(t, "Platform Voice", platform.profile.Name,
		"the profile matching the collection's point must win")

	unknown, err := a.resolveProjectBindings(cmd, proj, recipe, "no-such-collection")
	require.NoError(t, err)
	require.NotNil(t, unknown.profile)
	assert.Equal(t, "House Style", unknown.profile.Name)
}

// TestResolveProjectBindings_ExplicitProfileWinsOverTheRecipe is the precedence
// the recipe must not break: its match governs at the *collection* tier of the
// shared resolution chain (core/profile), under an explicit per-call profile.
// A flow step naming its own profile keeps it; the recipe fills the gap when it
// does not.
func TestResolveProjectBindings_ExplicitProfileWinsOverTheRecipe(t *testing.T) {
	recipe, _ := governedProject(t, map[string]string{"product": "platform"})
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	a := &App{}
	b, err := a.resolveProjectBindings(bindingsCmd(t, recipe), proj, recipe, "platform-docs")
	require.NoError(t, err)
	require.NotNil(t, b)
	require.NotNil(t, b.profile)
	require.Equal(t, "Platform Voice", b.profile.Name, "the collection's coordinates select profile A")

	bound := a.applyBindings(b, "translate", nil, map[string]any{})
	assert.Equal(t, b.profile, bound["profile"],
		"with nothing explicit, the collection tier supplies the voice")

	caller := &profile.VoiceProfile{ID: "caller", Name: "Caller Voice"}
	explicit := a.applyBindings(b, "translate", nil, map[string]any{"profile": caller})
	assert.Equal(t, caller, explicit["profile"],
		"an explicit per-call profile outranks the recipe's collection-tier binding")
}

// TestRecipeGovernanceEntersTheChain is the unification: the profile a
// collection's coordinates select is not resolved beside the framework's
// precedence chain, it *is* the chain's collection tier. So it outranks the
// bindings a server-governed project carries on its stream, project and
// workspace rows, and is outranked by an explicit per-call profile — one
// ordering, whichever surface the project is governed from.
func TestRecipeGovernanceEntersTheChain(t *testing.T) {
	recipe, root := governedProject(t, map[string]string{"product": "platform"})
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	a := &App{}
	b, err := a.resolveProjectBindings(bindingsCmd(t, recipe), proj, recipe, "platform-docs")
	require.NoError(t, err)
	require.NotNil(t, b)
	require.NotNil(t, b.profile)
	require.Equal(t, "Platform Voice", b.profile.Name, "the collection's coordinates select profile A")

	// A real store holding what the tiers under (and over) the collection name.
	store, err := openVoiceStoreAt(filepath.Join(root, "brand.db"))
	require.NoError(t, err)
	defer store.Close()
	for _, p := range []*profile.VoiceProfile{
		{ID: "explicit-voice", Name: "Explicit Voice", WorkspaceID: LocalWorkspace},
		{ID: "stream-voice", Name: "Stream Voice", WorkspaceID: LocalWorkspace},
		{ID: "project-voice", Name: "Project Voice", WorkspaceID: LocalWorkspace},
		{ID: "workspace-voice", Name: "Workspace Voice", WorkspaceID: LocalWorkspace},
	} {
		require.NoError(t, store.CreateProfile(t.Context(), p))
	}

	rc := profile.ResolveContext{
		CollectionProfile:  b.profile,
		StreamProperties:   map[string]string{profile.PropertyProfileID: "stream-voice"},
		ProjectProperties:  map[string]string{profile.PropertyProfileID: "project-voice"},
		WorkspaceProfileID: "workspace-voice",
	}

	got, err := profile.ResolveProfileFromContext(t.Context(), rc, store)
	require.NoError(t, err)
	assert.Equal(t, "Platform Voice", got.Name,
		"the recipe's coordinate governs over stream, project and workspace bindings")

	rc.ExplicitProfileID = "explicit-voice"
	got, err = profile.ResolveProfileFromContext(t.Context(), rc, store)
	require.NoError(t, err)
	assert.Equal(t, "Explicit Voice", got.Name,
		"and yields to an explicit per-call profile")
}

// TestResolveProjectBindings_ChannelSelectsTheOverride proves the well-known
// axis takes effect: after a profile is selected, the channel selects the
// override inside that profile's voice (profile.VoiceProfile.Channels), so one
// authored voice covers every channel instead of one voice file per
// product-and-channel pair.
func TestResolveProjectBindings_ChannelSelectsTheOverride(t *testing.T) {
	a := &App{}

	docs, dRecipe := resolveAtChannel(t, a, "docs")
	assert.Equal(t, "neutral", docs.Tone.Formality,
		"a channel the profile declares no override for leaves the base voice unchanged")

	landing, lRecipe := resolveAtChannel(t, a, "landing")
	assert.Equal(t, "casual", landing.Tone.Formality,
		"the profile's landing override must be applied at the landing coordinate")

	assert.NotEqual(t, dRecipe, lRecipe, "each subcase builds its own recipe")
}

// resolveAtChannel resolves the platform collection's profile with the
// collection placed at (platform, channel). Returns the profile and the recipe.
func resolveAtChannel(t *testing.T, a *App, channel string) (*profile.VoiceProfile, string) {
	t.Helper()
	recipe, _ := governedProject(t, map[string]string{"product": "platform", "channel": channel})
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	b, err := a.resolveProjectBindings(bindingsCmd(t, recipe), proj, recipe, "platform-docs")
	require.NoError(t, err)
	require.NotNil(t, b)
	require.NotNil(t, b.profile)
	return b.profile, recipe
}

// TestResolveGroupBindings_SingleRunWhenUngoverned is the regression guard: a
// project where no collection names a context must resolve its bindings once —
// one group, one tool chain, the run it has always been.
func TestResolveGroupBindings_SingleRunWhenUngoverned(t *testing.T) {
	recipe, root := governedProject(t, nil)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	inputs := []string{
		filepath.Join(root, "docs", "a.md"),
		filepath.Join(root, "platform", "b.md"),
	}
	groups, err := groupInputsByBinding(proj, root, inputs)
	require.NoError(t, err)
	require.Len(t, groups, 1, "no context anywhere must not split the run")
	assert.Empty(t, groups[0].Collection, "the single group resolves the project-wide bindings")
	assert.Equal(t, inputs, groups[0].Inputs)

	a := &App{}
	require.NoError(t, a.resolveGroupBindings(bindingsCmd(t, recipe), proj, recipe, groups))
	require.NotNil(t, groups[0].bindings)
	assert.Equal(t, "House Style", groups[0].bindings.profile.Name)

	// And with a context, the same inputs split into two — so the assertion
	// above is about the recipe, not about grouping being inert.
	governed, gRoot := governedProject(t, map[string]string{"product": "platform"})
	gProj, err := project.LoadWithOptions(governed, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	gGroups, err := groupInputsByBinding(gProj, gRoot, []string{
		filepath.Join(gRoot, "docs", "a.md"),
		filepath.Join(gRoot, "platform", "b.md"),
	})
	require.NoError(t, err)
	assert.Len(t, gGroups, 2)
}
