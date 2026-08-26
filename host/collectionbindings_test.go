package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
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

// governedProject writes a recipe declaring two products, a profile governing
// one of them, and the voice YAMLs they bind. platformChannel is the channel
// reference the platform collection declares; empty leaves that collection at
// the project's default point. Returns the recipe path and root.
func governedProject(t *testing.T, platformChannel string) (recipe, root string) {
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
		Profiles: map[string]project.Profile{
			"framework": {Channels: []project.Channel{{ID: "docs"}}},
			"platform": {
				Channels: []project.Channel{{ID: "docs"}, {ID: "landing"}},
				Voice:    &project.VoiceBinding{ProfileFile: "platform-voice.yaml"},
			},
		},
		Collections: []project.Collection{
			{Name: "framework-docs", Content: []project.ContentItem{{Path: "docs/**/*.md"}}},
			{Name: "platform-docs", Channel: platformChannel, Content: []project.ContentItem{{Path: "platform/**/*.md"}}},
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

// wantGroup is one expected partition: the point its files are governed at,
// written `profile/channel` (empty for the project's default point), and the
// files themselves. The expectation is the resolved point rather than a
// collection label, because grouping is by what a point resolves to.
type wantGroup struct {
	point  string
	inputs []string
}

// resolvedPoint renders what a group's representative point resolves to, which
// is what the group is: the governance every file in it shares.
func resolvedPoint(t *testing.T, proj *project.KapiProject, pt project.GovernancePoint) string {
	t.Helper()
	if proj == nil {
		return ""
	}
	rc, err := proj.ResolveGovernanceFor(pt)
	require.NoError(t, err)
	return rc.Ref().String()
}

func TestGroupInputsByBinding(t *testing.T) {
	defaultVoice := &project.VoiceBinding{ProfileFile: "voice.yaml"}
	profiles := map[string]project.Profile{
		"platform": {
			Channels: []project.Channel{{ID: "docs"}, {ID: "landing"}, {ID: "notes"}},
			Voice:    &project.VoiceBinding{ProfileFile: "platform-voice.yaml"},
		},
		"press": {
			Channels:  []project.Channel{{ID: "news"}},
			Voice:     &project.VoiceBinding{Pack: "marketing-blog"},
			TermStore: "press-terms.db",
		},
		// A profile binding exactly what the project defaults bind: the default
		// voice, and no standalone terms — so the project's own store governs
		// its vocabulary, exactly as at the default point.
		"house": {
			Channels: []project.Channel{{ID: "home"}},
			Voice:    defaultVoice,
		},
	}
	root := filepath.FromSlash("/repo")

	newProj := func(collections ...project.Collection) *project.KapiProject {
		return &project.KapiProject{
			Version:     "v1",
			Defaults:    project.Defaults{Voice: defaultVoice},
			Profiles:    profiles,
			Collections: collections,
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
		want   []wantGroup
	}{
		{
			name: "no collection names a channel: one ungrouped run",
			proj: newProj(
				project.Collection{Name: "docs", Content: []project.ContentItem{{Path: "docs/**/*.md"}}},
				project.Collection{Name: "app", Content: []project.ContentItem{{Path: "app/**/*.json"}}},
			),
			inputs: abs("docs/a.md", "app/b.json"),
			want:   []wantGroup{{point: "", inputs: abs("docs/a.md", "app/b.json")}},
		},
		{
			name: "two products: one group each, in input order",
			proj: newProj(
				project.Collection{Name: "docs", Content: []project.ContentItem{{Path: "docs/**/*.md"}}},
				project.Collection{
					Name:    "platform",
					Channel: "platform/docs",
					Content: []project.ContentItem{{Path: "platform/**/*.md"}},
				},
			),
			inputs: abs("docs/a.md", "platform/b.md", "docs/c.md"),
			want: []wantGroup{
				{point: "", inputs: abs("docs/a.md", "docs/c.md")},
				{point: "platform/docs", inputs: abs("platform/b.md")},
			},
		},
		{
			name: "collections at the same point share one group",
			proj: newProj(
				project.Collection{
					Name:    "platform-docs",
					Channel: "platform/docs",
					Content: []project.ContentItem{{Path: "docs/**/*.md"}},
				},
				project.Collection{
					Name:    "platform-more-docs",
					Channel: "platform/docs",
					Content: []project.ContentItem{{Path: "more-docs/**/*.md"}},
				},
			),
			inputs: abs("docs/a.md", "more-docs/b.md"),
			want:   []wantGroup{{point: "platform/docs", inputs: abs("docs/a.md", "more-docs/b.md")}},
		},
		{
			name: "two collections at one point share one group",
			proj: newProj(
				project.Collection{
					Name:    "platform-notes",
					Channel: "platform/notes",
					Content: []project.ContentItem{{Path: "notes/**/*.md"}},
				},
				project.Collection{
					Name:    "platform-notes-more",
					Channel: "platform/notes",
					Content: []project.ContentItem{{Path: "more-notes/**/*.md"}},
				},
			),
			inputs: abs("notes/a.md", "more-notes/b.md"),
			want:   []wantGroup{{point: "platform/notes", inputs: abs("notes/a.md", "more-notes/b.md")}},
		},
		{
			name: "same profile, different channel: different governance, different groups",
			proj: newProj(
				project.Collection{
					Name:    "platform-docs",
					Channel: "platform/docs",
					Content: []project.ContentItem{{Path: "docs/**/*.md"}},
				},
				project.Collection{
					Name:    "platform-landing",
					Channel: "platform/landing",
					Content: []project.ContentItem{{Path: "landing/**/*.md"}},
				},
			),
			inputs: abs("docs/a.md", "landing/b.md"),
			want: []wantGroup{
				{point: "platform/docs", inputs: abs("docs/a.md")},
				{point: "platform/landing", inputs: abs("landing/b.md")},
			},
		},
		{
			// The house profile binds exactly what the project defaults bind, so
			// only the channel tells the two apart — and it is enough, because
			// the channel selects an override inside that voice.
			name: "a channel still splits when its profile binds the defaults",
			proj: newProj(
				project.Collection{Name: "app", Content: []project.ContentItem{{Path: "app/**/*.json"}}},
				project.Collection{
					Name:    "docs",
					Channel: "house/home",
					Content: []project.ContentItem{{Path: "docs/**/*.md"}},
				},
			),
			inputs: abs("app/a.json", "docs/b.md"),
			want: []wantGroup{
				{point: "", inputs: abs("app/a.json")},
				{point: "house/home", inputs: abs("docs/b.md")},
			},
		},
		{
			name: "a profile with its own terms splits too",
			proj: newProj(
				project.Collection{Name: "docs", Content: []project.ContentItem{{Path: "docs/**/*.md"}}},
				project.Collection{
					Name:    "press",
					Channel: "press/news",
					Content: []project.ContentItem{{Path: "press/**/*.md"}},
				},
			),
			inputs: abs("docs/a.md", "press/b.md"),
			want: []wantGroup{
				{point: "", inputs: abs("docs/a.md")},
				{point: "press/news", inputs: abs("press/b.md")},
			},
		},
		{
			name: "unclaimed and outside-the-root paths sit at the default point",
			proj: newProj(
				project.Collection{
					Name:    "platform",
					Channel: "platform/docs",
					Content: []project.ContentItem{{Path: "platform/**/*.md"}},
				},
			),
			inputs: append(abs("scripts/build.sh", "platform/b.md"), filepath.FromSlash("/elsewhere/c.md")),
			want: []wantGroup{
				{point: "", inputs: append(abs("scripts/build.sh"), filepath.FromSlash("/elsewhere/c.md"))},
				{point: "platform/docs", inputs: abs("platform/b.md")},
			},
		},
		{
			name:   "no project is one group",
			proj:   nil,
			inputs: abs("docs/a.md"),
			want:   []wantGroup{{point: "", inputs: abs("docs/a.md")}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.proj != nil {
				require.NoError(t, tt.proj.Validate(), "the fixture must be a loadable recipe")
			}
			a := &App{}
			got, err := a.groupInputsByBinding(nil, tt.proj, root, tt.inputs)
			require.NoError(t, err)
			require.Len(t, got, len(tt.want))
			for i := range tt.want {
				assert.Equal(t, tt.want[i].point, resolvedPoint(t, tt.proj, got[i].Point), "group %d point", i)
				assert.Equal(t, tt.want[i].inputs, got[i].Inputs, "group %d inputs", i)
			}
		})
	}
}

// TestResolveProjectBindings_PerChannelVoice is the point of the feature: two
// collections of one project resolve two different voice profiles, and a
// collection sitting at the project's default point still resolves the
// project-wide one.
func TestResolveProjectBindings_PerChannelVoice(t *testing.T) {
	recipe, _ := governedProject(t, "platform/docs")
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	a := &App{}
	cmd := bindingsCmd(t, recipe)

	defaults, err := a.resolveProjectBindings(cmd, proj, recipe, project.GovernancePoint{})
	require.NoError(t, err)
	require.NotNil(t, defaults)
	require.NotNil(t, defaults.profile)
	assert.Equal(t, "House Style", defaults.profile.Name)

	framework, err := a.resolveProjectBindings(cmd, proj, recipe, project.GovernancePoint{Collection: "framework-docs"})
	require.NoError(t, err)
	require.NotNil(t, framework.profile)
	assert.Equal(t, "House Style", framework.profile.Name,
		"a collection that names no channel keeps the project voice")

	platform, err := a.resolveProjectBindings(cmd, proj, recipe, project.GovernancePoint{Collection: "platform-docs"})
	require.NoError(t, err)
	require.NotNil(t, platform.profile)
	assert.Equal(t, "Platform Voice", platform.profile.Name,
		"the profile matching the collection's point must win")

	unknown, err := a.resolveProjectBindings(cmd, proj, recipe, project.GovernancePoint{Collection: "no-such-collection"})
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
	recipe, _ := governedProject(t, "platform/docs")
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	a := &App{}
	b, err := a.resolveProjectBindings(bindingsCmd(t, recipe), proj, recipe, project.GovernancePoint{Collection: "platform-docs"})
	require.NoError(t, err)
	require.NotNil(t, b)
	require.NotNil(t, b.profile)
	require.Equal(t, "Platform Voice", b.profile.Name, "the collection's channel selects the platform profile")

	bound := a.applyBindings(b, "translate", nil, map[string]any{})
	assert.Equal(t, b.profile, bound["profile"],
		"with nothing explicit, the collection tier supplies the voice")

	caller := &profile.VoiceProfile{ID: "caller", Name: "Caller Voice"}
	explicit := a.applyBindings(b, "translate", nil, map[string]any{"profile": caller})
	assert.Equal(t, caller, explicit["profile"],
		"an explicit per-call profile outranks the recipe's collection-tier binding")
}

// TestResolveProjectBindings_CarriesThePointToTheProducers: a fill has to know
// where it is happening. The record can hold two reviewed answers for one
// source, one approved in each collection that carries the string, and the
// corpus can only prefer the local one if the caller says which one it is. The
// point comes from the same resolution the voice does.
//
// It reaches translate as well as recycle, which it did not always. The old
// reasoning was that translate produces a new translation rather than choosing
// between approved ones. That is true of the output and beside the point once a
// block's own previous version became reference material: wording approved for
// one surface must not steer another.
func TestResolveProjectBindings_CarriesThePointToTheProducers(t *testing.T) {
	recipe, _ := governedProject(t, "platform/docs")
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	a := &App{}
	b, err := a.resolveProjectBindings(bindingsCmd(t, recipe), proj, recipe,
		project.GovernancePoint{Collection: "platform-docs"})
	require.NoError(t, err)
	require.NotNil(t, b)

	want := memory.NewPoint("platform", "docs", "")
	assert.Equal(t, want, b.point, "the group's product and channel, as the recipe binds them")

	recycled := a.applyBindings(b, "recycle", nil, map[string]any{})
	assert.Equal(t, want, recycled["point"], "recycle asks the corpus from where it is")

	translated := a.applyBindings(b, "translate", nil, map[string]any{})
	assert.Equal(t, want, translated["point"],
		"translate reads this block's history, and only from where the block sits")
}

// TestRecipeGovernanceEntersTheChain is the unification: the profile a
// collection's channel selects is not resolved beside the framework's
// precedence chain, it *is* the chain's collection tier. So it outranks the
// bindings a server-governed project carries on its stream, project and
// workspace rows, and is outranked by an explicit per-call profile — one
// ordering, whichever surface the project is governed from.
func TestRecipeGovernanceEntersTheChain(t *testing.T) {
	recipe, root := governedProject(t, "platform/docs")
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	a := &App{}
	b, err := a.resolveProjectBindings(bindingsCmd(t, recipe), proj, recipe, project.GovernancePoint{Collection: "platform-docs"})
	require.NoError(t, err)
	require.NotNil(t, b)
	require.NotNil(t, b.profile)
	require.Equal(t, "Platform Voice", b.profile.Name, "the collection's channel selects the platform profile")

	// A real store holding what the tiers under (and over) the collection name.
	store, err := openVoiceStoreAt(filepath.Join(root, "voice.db"))
	require.NoError(t, err)
	defer store.Close()
	for _, p := range []*profile.VoiceProfile{
		{ID: "explicit-voice", Name: "Explicit Voice", Scope: LocalScope},
		{ID: "stream-voice", Name: "Stream Voice", Scope: LocalScope},
		{ID: "project-voice", Name: "Project Voice", Scope: LocalScope},
		{ID: "workspace-voice", Name: "Workspace Voice", Scope: LocalScope},
	} {
		require.NoError(t, store.CreateProfile(t.Context(), p))
	}

	rc := profile.ResolveContext{
		CollectionProfile: b.profile,
		StreamProperties:  map[string]string{profile.PropertyProfileID: "stream-voice"},
		ProjectProperties: map[string]string{profile.PropertyProfileID: "project-voice"},
		RootProfileID:     "workspace-voice",
	}

	got, err := profile.ResolveProfileFromContext(t.Context(), rc, store)
	require.NoError(t, err)
	assert.Equal(t, "Platform Voice", got.Name,
		"the recipe's channel governs over stream, project and workspace bindings")

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
		"the profile's landing override must be applied at the landing channel")

	assert.NotEqual(t, dRecipe, lRecipe, "each subcase builds its own recipe")
}

// resolveAtChannel resolves the platform collection's profile with the
// collection placed at (platform, channel). Returns the profile and the recipe.
func resolveAtChannel(t *testing.T, a *App, channel string) (*profile.VoiceProfile, string) {
	t.Helper()
	recipe, _ := governedProject(t, "platform/"+channel)
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	b, err := a.resolveProjectBindings(bindingsCmd(t, recipe), proj, recipe, project.GovernancePoint{Collection: "platform-docs"})
	require.NoError(t, err)
	require.NotNil(t, b)
	require.NotNil(t, b.profile)
	return b.profile, recipe
}

// TestResolveGroupBindings_SingleRunWhenUngoverned is the regression guard: a
// project where no collection names a channel must resolve its bindings once —
// one group, one tool chain, the run it has always been.
func TestResolveGroupBindings_SingleRunWhenUngoverned(t *testing.T) {
	recipe, root := governedProject(t, "")
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	inputs := []string{
		filepath.Join(root, "docs", "a.md"),
		filepath.Join(root, "platform", "b.md"),
	}
	a := &App{}
	groups, err := a.groupInputsByBinding(nil, proj, root, inputs)
	require.NoError(t, err)
	require.Len(t, groups, 1, "no channel anywhere must not split the run")
	assert.Empty(t, resolvedPoint(t, proj, groups[0].Point), "the single group resolves the project-wide bindings")
	assert.Equal(t, inputs, groups[0].Inputs)

	require.NoError(t, a.resolveGroupBindings(bindingsCmd(t, recipe), proj, recipe, groups))
	require.NotNil(t, groups[0].bindings)
	assert.Equal(t, "House Style", groups[0].bindings.profile.Name)

	// And with a channel, the same inputs split into two — so the assertion
	// above is about the recipe, not about grouping being inert.
	governed, gRoot := governedProject(t, "platform/docs")
	gProj, err := project.LoadWithOptions(governed, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	gGroups, err := a.groupInputsByBinding(nil, gProj, gRoot, []string{
		filepath.Join(gRoot, "docs", "a.md"),
		filepath.Join(gRoot, "platform", "b.md"),
	})
	require.NoError(t, err)
	assert.Len(t, gGroups, 2)
}
