package host

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/kpz"
)

// A .kpz is packed, handed to an outside translator or reviewer, and returned
// to be merged. These tests hold the boundary that makes that safe: a package
// may only name paths inside the project it is merged into, and the intent it
// carries is inert until the recipient adopts it deliberately.

// TestContainedJoinRefusesEscapes: one rule, one helper. unpack, merge and the
// workspace flow all resolve package-supplied paths through this, so they
// cannot drift apart on what "inside the project" means.
func TestContainedJoinRefusesEscapes(t *testing.T) {
	root := t.TempDir()

	for _, tc := range []struct{ name, in, wantErr string }{
		{"parent traversal", "../../victim.txt", "escapes the project root"},
		{"absolute", "/etc/victim.txt", "escapes the project root"},
		{"traversal mid-path", "src/../../victim.txt", "escapes the project root"},
		{"bare dot-dot", "..", "escapes the project root"},
		{"empty", "", "no usable relative path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := containedJoin(root, tc.in, "test: member")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}

	for _, ok := range []string{"src/en.json", "a/b/c/deep.json", "./src/en.json", "file.txt"} {
		got, err := containedJoin(root, ok, "test: member")
		require.NoErrorf(t, err, "%q is an ordinary project-relative path", ok)
		assert.True(t, strings.HasPrefix(got, root+string(filepath.Separator)),
			"%q must resolve inside the root", ok)
	}
}

// TestCheckPathSegmentRefusesPaths: a locale and a project name are
// identifiers, and they are interpolated into an output path as ONE component.
// A value carrying a separator or ".." would silently redirect the write.
func TestCheckPathSegmentRefusesPaths(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../../etc", "nb/NO", `nb\NO`, "/abs"} {
		require.Errorf(t, checkPathSegment(bad, "test: locale"),
			"%q must not be usable as a path segment", bad)
	}
	for _, good := range []string{"nb", "nb-NO", "zh-Hans-CN", "qps"} {
		assert.NoErrorf(t, checkPathSegment(good, "test: locale"), "%q is an ordinary locale", good)
	}
}

// TestResolveMergeOutputPathRefusesEscapingSource: the source path decides both
// which file merge re-reads and where it writes the result. It can come from a
// package's manifest, so an escaping name must stop the merge rather than
// resolve to somewhere outside the project.
func TestResolveMergeOutputPathRefusesEscapingSource(t *testing.T) {
	root := t.TempDir()

	for _, tc := range []struct{ name, source, locale string }{
		{"source climbs out", "../../../../etc/cron.d/kapi", "nb"},
		{"source is absolute", "/etc/cron.d/kapi", "nb"},
		{"locale is a path", "src/en.json", "../../.."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := &project.ExtractionFile{Source: tc.source}
			_, err := resolveMergeOutputPath(entry, nil, root, model.LocaleID(tc.locale))
			require.Error(t, err, "a merge destination outside the project must not be produced")
		})
	}

	// The ordinary case still resolves, and lands inside the project.
	entry := &project.ExtractionFile{Source: "src/en.json"}
	got, err := resolveMergeOutputPath(entry, nil, root, "nb")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "src", "nb", "en.json"), got)
}

// TestResolveMergeOutputPathKeepsAnAbsoluteRecipeTarget: the target template
// comes from the LOCAL recipe — the project owner's own file — so writing
// outside the tree stays a supported thing for them to ask for. Only the
// package-supplied half is constrained.
func TestResolveMergeOutputPathKeepsAnAbsoluteRecipeTarget(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "published")
	proj := &project.KapiProject{Content: []project.ContentCollection{{
		Items: []project.ContentItem{{Path: "src/en.json", Target: outside + "/{lang}.json"}},
	}}}

	got, err := resolveMergeOutputPath(&project.ExtractionFile{Source: "src/en.json"}, proj, root, "nb")
	require.NoError(t, err, "the project's own recipe may name an absolute destination")
	assert.Equal(t, filepath.Join(outside, "nb.json"), got)
}

// TestMergeOutputPathRefusesLocaleAsPath: the kpz workspace merge interpolates
// the locale into every layout branch, and the locale list comes from the
// package's own recipe.
func TestMergeOutputPathRefusesLocaleAsPath(t *testing.T) {
	_, err := mergeOutputPath("l10n/{lang}/{name}.{ext}", "src/en.json", "../../../etc", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path segment")

	got, err := mergeOutputPath("", "src/en.json", "nb", false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("nb", "en.json"), got)
}

// TestLoadWorkspaceStripsExecClassSteps: sanitizing at pack time asks the
// PACKER to be honest, which a hostile one will not be. LoadWorkspace is the
// single ingest point for every .kpz this binary opens, so the guarantee holds
// there instead — on a package hand-built to skip the pack-side sweep.
func TestLoadWorkspaceStripsExecClassSteps(t *testing.T) {
	recipe := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "hostile",
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{
				{Tool: "external-command", Config: map[string]any{"command": "/bin/sh"}},
				{Tool: "translate"},
			}},
		},
	}
	require.NoError(t, kpz.SetRecipeWorkspaceMeta(recipe, kpz.WorkspaceMeta{
		Out: "/etc/cron.d/{name}", Workspace: true,
	}))

	path := filepath.Join(t.TempDir(), "hostile.kpz")
	require.NoError(t, saveWorkspace(&kpz.Package{
		Kind:     kpz.KindProject,
		Recipe:   recipe,
		Overlays: []kpz.OverlayDoc{{Kind: "targets/nb", BlockHash: "b1", Payload: []byte(`{}`)}},
	}, path))

	pkg, err := LoadWorkspace(path)
	require.NoError(t, err, "a package with a hostile recipe still opens — the recipe is made inert, not fatal")

	require.NotNil(t, pkg.Recipe)
	steps := pkg.Recipe.Flows["translate"].Steps
	require.Len(t, steps, 1, "the step that runs a command it names must not survive ingest")
	assert.Equal(t, "translate", steps[0].Tool)
	assert.Empty(t, kpz.RecipeWorkspaceMeta(pkg.Recipe).Out,
		"an output layout naming somewhere absolute falls back to the default")
	assert.Equal(t, "hostile", pkg.Recipe.Name, "the rest of the recipe is untouched")
}

// TestConfirmAdoptRecipeNeedsConsent: adopting a package's recipe makes every
// later `kapi run` here execute steps someone else wrote, so it is a decision
// the recipient makes rather than a side effect of unpacking.
func TestConfirmAdoptRecipeNeedsConsent(t *testing.T) {
	snapshot := "handoff.kpz"

	t.Run("--yes adopts without asking", func(t *testing.T) {
		cmd := NewEnvCommand(context.Background(), "unpack")
		ok, err := (&App{AssumeYes: true}).confirmAdoptRecipe(cmd, snapshot)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("non-interactive declines and says how", func(t *testing.T) {
		cmd := NewEnvCommand(context.Background(), "unpack")
		var errOut strings.Builder
		cmd.SetErr(&errOut)
		ok, err := (&App{isTTY: func() bool { return false }}).confirmAdoptRecipe(cmd, snapshot)
		require.NoError(t, err)
		assert.False(t, ok, "an unattended run must not adopt a recipe it never showed anyone")
		assert.Contains(t, errOut.String(), "--yes")
	})

	for _, tc := range []struct {
		name, answer string
		want         bool
	}{
		{"accepted", "y\n", true},
		{"accepted by default", "\n", true},
		{"declined", "n\n", false},
	} {
		t.Run("interactive "+tc.name, func(t *testing.T) {
			cmd := NewEnvCommand(context.Background(), "unpack")
			cmd.SetIn(strings.NewReader(tc.answer))
			var errOut strings.Builder
			cmd.SetErr(&errOut)

			ok, err := (&App{isTTY: func() bool { return true }}).confirmAdoptRecipe(cmd, snapshot)
			require.NoError(t, err)
			assert.Equal(t, tc.want, ok)
			assert.Contains(t, errOut.String(), "handoff.kpz",
				"the prompt must name what is being adopted")
		})
	}
}

// TestReconstitutedProjectPathRefusesAHostileName: the recipe's name becomes a
// directory beside the snapshot, and everything that follows — the block store,
// the content memory, the terms — lands inside it. A name spelled as a path
// would place all of that somewhere the recipient never chose, so the snapshot's
// own base name is used instead.
func TestReconstitutedProjectPathRefusesAHostileName(t *testing.T) {
	snapshot := filepath.Join(t.TempDir(), "handoff.kpz")
	safe := filepath.Join(filepath.Dir(snapshot), "handoff", project.RecipeFileName)

	for _, name := range []string{"../../../../tmp/pwned", "/etc/cron.d", "..", "a/b"} {
		pkg := &kpz.Package{Recipe: &project.KapiProject{Name: name}}
		assert.Equalf(t, safe, reconstitutedProjectPath(snapshot, pkg),
			"recipe name %q must not choose the directory", name)
	}

	ordinary := &kpz.Package{Recipe: &project.KapiProject{Name: "archive-demo"}}
	assert.Equal(t,
		filepath.Join(filepath.Dir(snapshot), "archive-demo", project.RecipeFileName),
		reconstitutedProjectPath(snapshot, ordinary),
		"an ordinary project name still names its folder")
}
