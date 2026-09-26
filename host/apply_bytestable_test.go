package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recipe `kapi apply` edits is human-authored source, and so is a voice
// profile somebody wrote by hand. Two properties hold over every write: the
// comments and key order survive a change, and a change that decides nothing
// writes nothing at all.
//
// The second is a gate, not tidiness. `scripts/check-sync-backed.sh` classifies
// a convergence run's tree into backing (`.kapi/` minus `.kapi/work`), derived
// and foreign, and asks for backing over the run as a WHOLE: one backing entry
// explains every derived change in that run. A `kapi apply` that rewrote a file
// under `.kapi/` only to reformat it would manufacture backing for artifacts
// nothing decided, which is the failure the gate exists to prevent. The recipe
// fails the other way: at the repo root it is foreign, so a run that rewrote it
// is refused by name even with backing present.
//
// An asset apply writes the project's stores, so the profile is reached through
// `kapi context import` and written back out by `kapi context snapshot`, which
// is where the commentary has to survive.

const commentedVoiceYAML = `# The voice the harbour docs are written in.
# Authored by hand: every line here is a decision someone made.
name: North Sea
description: |
  Plain, exact writing
  for harbour operations.
tone:
  # Restrained, because a berthing instruction is read under time pressure.
  personality: [clear, restrained]
  formality: neutral
style:
  prohibited_patterns:
    # Marketing register: never in operational prose.
    - regex: '(?i)\bworld-class\b'
      description: Marketing register
# The word rules this voice brings; importing it moves them into terms.
terms:
  - term: seamless
    replacement: uninterrupted
`

const commentedRecipe = `version: v1
name: northsea
# The languages this project is written in. One, for now.
defaults:
  source_language: en-GB
collections:
  # Every surface the harbour publishes to.
  - name: northsea-docs
    content:
      - path: "docs/**/*.md"
`

// newGovernanceProject writes a project whose recipe and voice profile are both
// commented, human-authored documents, and returns the app, command and paths.
func newGovernanceProject(t *testing.T) (a *App, cmd *EnvCommand, root, recipe, voice string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe = filepath.Join(real, project.RecipeFileName)
	voice = filepath.Join(real, project.RelStatePath("voice.yaml"))
	require.NoError(t, os.MkdirAll(filepath.Dir(voice), 0o755))
	require.NoError(t, os.WriteFile(recipe, []byte(commentedRecipe), 0o644))
	require.NoError(t, os.WriteFile(voice, []byte(commentedVoiceYAML), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(real, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "docs", "berths.md"), []byte("# Berths\n"), 0o644))
	t.Chdir(real)

	a = &App{}
	a.InitRegistries()
	cmd = NewEnvCommand(context.Background(), "apply")
	return a, cmd, real, recipe, voice
}

func digestOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// readGovernanceContext reads the fixture's authored profile into the project's
// store, which is what a person does once per checkout before anything in the
// file is in force.
func readGovernanceContext(t *testing.T, a *App, recipe string) {
	t.Helper()
	res, err := a.ImportProjectContext(context.Background(), recipe, ContextImportRequest{})
	require.NoError(t, err)
	require.Equal(t, 1, res.VoiceProfiles, "the authored profile reached the store")
}

// TestApplyTerm_NoOpIsByteStable: applying a word rule the import already
// moved into terms decides nothing, so neither governance file may move a byte.
func TestApplyTerm_NoOpIsByteStable(t *testing.T) {
	a, cmd, _, recipe, voice := newGovernanceProject(t)
	readGovernanceContext(t, a, recipe)
	voiceBefore, recipeBefore := digestOf(t, voice), digestOf(t, recipe)

	res := a.applyAssetEntry(context.Background(), cmd, changeEntry{
		Kind:        kindTerm,
		Term:        "seamless",
		Replacement: "uninterrupted",
		Locale:      "en-GB",
		Status:      "forbidden",
	})
	require.Equal(t, "skipped", res.Status, "detail: %s", res.Detail)

	assert.Equal(t, voiceBefore, digestOf(t, voice), "an applied no-op rewrote the voice profile")
	assert.Equal(t, recipeBefore, digestOf(t, recipe), "an applied no-op rewrote the recipe")
}

// TestSnapshotVoiceProfile_KeepsTheCommentary: a word rule lands in terms, and
// a snapshot writes the voice profile back out with every comment and the
// authored key order. A decision about one word that also deletes the voice
// file's explanation of itself is not a reviewable diff.
func TestSnapshotVoiceProfile_KeepsTheCommentary(t *testing.T) {
	a, cmd, _, recipe, voice := newGovernanceProject(t)
	readGovernanceContext(t, a, recipe)

	res := a.applyAssetEntry(context.Background(), cmd, changeEntry{
		Kind:        kindTerm,
		Term:        "mooring",
		Replacement: "berth",
		Locale:      "en-GB",
		Status:      "forbidden",
	})
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	_, err := a.SnapshotProjectContext(context.Background(), recipe, ContextSnapshotRequest{Out: filepath.Join(filepath.Dir(recipe), project.StateDirName)})
	require.NoError(t, err)

	after, err := os.ReadFile(voice)
	require.NoError(t, err)
	got := string(after)

	for _, comment := range []string{
		"# The voice the harbour docs are written in.",
		"# Authored by hand: every line here is a decision someone made.",
		"# Restrained, because a berthing instruction is read under time pressure.",
		"# Marketing register: never in operational prose.",
	} {
		assert.Contains(t, got, comment)
	}
	assert.Contains(t, got, "description: |", "the block scalar stayed a block scalar")
	termsFile, err := os.ReadFile(filepath.Join(filepath.Dir(voice), "terms.json"))
	require.NoError(t, err)
	assert.Contains(t, string(termsFile), "mooring", "the rule landed in terms")
	assert.Contains(t, string(termsFile), "seamless", "and the rule the voice file brought is there too")
}

// TestApplyRecipeField_NoOpIsByteStable: setting a recipe field to the value it
// already holds is the same no-op, and the recipe is repo-root — the erasure
// gate classifies it as foreign, so a run that rewrites it is refused by name.
func TestApplyRecipeField_NoOpIsByteStable(t *testing.T) {
	a, cmd, _, recipe, _ := newGovernanceProject(t)
	before := digestOf(t, recipe)

	res := a.applyAssetEntry(context.Background(), cmd, changeEntry{
		Kind:  kindRecipe,
		Op:    "set",
		Path:  "defaults.source_language",
		Value: []byte(`"en-GB"`),
	})
	require.NotEqual(t, "error", res.Status, "detail: %s", res.Detail)

	assert.Equal(t, before, digestOf(t, recipe), "setting a field to its own value rewrote the recipe")
}

// TestApplyRecipeField_KeepsTheCommentary: a recipe field that does change keeps
// the commented tutorial `kapi init` wrote around it.
func TestApplyRecipeField_KeepsTheCommentary(t *testing.T) {
	a, cmd, _, recipe, _ := newGovernanceProject(t)

	res := a.applyAssetEntry(context.Background(), cmd, changeEntry{
		Kind:  kindRecipe,
		Op:    "set",
		Path:  "defaults.source_language",
		Value: []byte(`"en-US"`),
	})
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	after, err := os.ReadFile(recipe)
	require.NoError(t, err)
	got := string(after)
	assert.Contains(t, got, "source_language: en-US", "the change landed")
	assert.Contains(t, got, "# The languages this project is written in. One, for now.")
	assert.Contains(t, got, "# Every surface the harbour publishes to.")
}

// TestApplyNoOp_LeavesNoBackingUnderKapi is the gate-integrity case, stated the
// way the erasure gate reads a tree: an apply that decided nothing leaves no
// changed file under `.kapi/` at all, so it cannot stand as the backing that
// lets a run's derived artifacts through.
func TestApplyNoOp_LeavesNoBackingUnderKapi(t *testing.T) {
	a, cmd, root, recipe, _ := newGovernanceProject(t)
	readGovernanceContext(t, a, recipe)
	stateDir := filepath.Join(root, project.StateDirName)

	// The derived store under `.kapi/work` is not backing (the gate excludes
	// it), and opening a project legitimately touches it.
	backing := func() map[string]string {
		t.Helper()
		out := map[string]string{}
		require.NoError(t, filepath.WalkDir(stateDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if rel, rerr := filepath.Rel(stateDir, path); rerr == nil && strings.HasPrefix(rel, "work") {
				return nil
			}
			out[path] = digestOf(t, path)
			return nil
		}))
		return out
	}

	before := backing()
	require.NotEmpty(t, before, "the fixture must have something under .kapi to be able to see it move")

	res := a.applyAssetEntry(context.Background(), cmd, changeEntry{
		Kind:        kindTerm,
		Term:        "seamless",
		Replacement: "uninterrupted",
		Locale:      "en-GB",
		Status:      "forbidden",
	})
	require.Equal(t, "skipped", res.Status, "detail: %s", res.Detail)

	assert.Equal(t, before, backing(), "a no-op apply manufactured backing under .kapi")
}
