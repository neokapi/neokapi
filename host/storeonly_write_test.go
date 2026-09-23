package host

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
)

// The write half of the store-only model. Every change to a project's context
// lands in the store and is recorded as an operation; the checkout's files are
// read by one command and written by two, and nothing else opens one.

// TestImport_RunTwiceReadsNothingTheSecondTime: an import skips a source whose
// bytes have not moved since this checkout read it, and `--force` reads it
// again.
func TestImport_RunTwiceReadsNothingTheSecondTime(t *testing.T) {
	c := newTwoCheckoutProject(t)
	ctx := context.Background()
	writeTermsSource(t, c.aRoot, map[string]string{"widget": "dings"})

	first, err := c.a.ImportProjectContext(ctx, c.aRecipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.Equal(t, 1, first.Concepts)
	assert.Zero(t, first.Unchanged)

	second, err := c.a.ImportProjectContext(ctx, c.aRecipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.False(t, second.Read(), "a second run of the same import reads nothing")
	assert.Equal(t, 1, second.Unchanged)
	var report strings.Builder
	require.NoError(t, second.FormatText(&report))
	assert.Contains(t, report.String(), "Nothing to read")

	forced, err := c.a.ImportProjectContext(ctx, c.aRecipe, ContextImportRequest{Force: true})
	require.NoError(t, err)
	assert.Equal(t, 1, forced.Concepts, "--force reads a source at bytes it has read before")
	assert.Zero(t, forced.Unchanged)
}

// TestImport_EachCheckoutReadsItsOwnFiles is the write half of #2918. The skip
// stamps used to sit in the checkout while the rows they gate are shared, so
// one clone's import could decide another clone's file had been read. Both
// branches reach the store, and the log names both imports.
func TestImport_EachCheckoutReadsItsOwnFiles(t *testing.T) {
	c := newTwoCheckoutProject(t)
	ctx := context.Background()
	writeTermsSource(t, c.aRoot, map[string]string{"widget": "dings"})
	writeTermsSource(t, c.bRoot, map[string]string{"sidebar": "sidefelt"})

	fromA, err := c.a.ImportProjectContext(ctx, c.aRecipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.Equal(t, 1, fromA.Concepts)

	fromB, err := c.b.ImportProjectContext(ctx, c.bRecipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.Equal(t, 1, fromB.Concepts, "the second checkout reads its own branch's bundle")

	for _, checkout := range []struct {
		name   string
		app    *App
		recipe string
	}{
		{"checkout A", c.a, c.aRecipe},
		{"checkout B", c.b, c.bRecipe},
	} {
		t.Run(checkout.name, func(t *testing.T) {
			assert.ElementsMatch(t, []string{"widget", "sidebar"},
				conceptTexts(t, checkout.app, checkout.recipe),
				"one store holds what both branches read in")
		})
	}

	ops := recordedOps(t, c.a, c.aRecipe)
	require.Len(t, ops, 2, "the log names both imports")
	for _, op := range ops {
		assert.Equal(t, contextop.KindImport, op.Kind)
		assert.Equal(t, contextop.StatusEstablished, op.Status, "what a person imports is established")
		require.NotEmpty(t, op.Evidence)
		assert.Equal(t, ".kapi/terms.json", op.Evidence[0].Path)
		assert.True(t, strings.HasPrefix(op.Evidence[0].Quote, "sha256:"),
			"the evidence carries the bytes that were read, got %q", op.Evidence[0].Quote)
		assert.Contains(t, op.Subject.Text, "terms bundle .kapi/terms.json, 1 concept")
	}
	assert.NotEqual(t, ops[0].Evidence[0].Quote, ops[1].Evidence[0].Quote,
		"two branches' bundles have two digests")
}

// TestImport_RefusesAnAgent: reading a checkout's context files puts them in
// force for everyone working in the project, so a person runs it. An agent is
// turned away before the store moves.
func TestImport_RefusesAnAgent(t *testing.T) {
	c := newTwoCheckoutProject(t)
	ctx := context.Background()
	writeTermsSource(t, c.aRoot, map[string]string{"widget": "dings"})
	t.Setenv(EnvActor, string(contextop.ActorAgent))
	t.Setenv(EnvAgentName, "test-agent")

	_, err := c.a.ImportProjectContext(ctx, c.aRecipe, ContextImportRequest{})
	require.ErrorIs(t, err, contextop.ErrRefused)
	assert.Empty(t, conceptTexts(t, c.a, c.aRecipe), "and the store did not move")
}

// TestConfirm_ChangesNoFileAndReachesBothCheckouts: confirming a rule in one
// checkout writes the store, leaves every file where it was, and is in force
// in the other checkout the same moment.
func TestConfirm_ChangesNoFileAndReachesBothCheckouts(t *testing.T) {
	c := newTwoCheckoutProject(t)
	ctx := context.Background()

	// Both stores opened first, so the machine state each checkout keeps is
	// already there when the trees are read.
	require.Empty(t, conceptTexts(t, c.a, c.aRecipe))
	require.Empty(t, conceptTexts(t, c.b, c.bRecipe))
	beforeA, beforeB := treeOf(t, c.aRoot), treeOf(t, c.bRoot)

	proposed, err := c.a.RecordContextObservation(ctx, ContextObserveRequest{
		Project: c.aRecipe,
		Term:    "use", InsteadOf: []string{"utilise"},
	})
	require.NoError(t, err)
	_, err = c.a.KeepContextOperation(ctx, ContextKeepRequest{Project: c.aRecipe, ID: proposed.ID})
	require.NoError(t, err)

	assert.Equal(t, []string{"use", "utilise"}, sorted(conceptTexts(t, c.b, c.bRecipe)),
		"the other checkout is held to the rule with no file passing between them")
	assert.Equal(t, beforeA, treeOf(t, c.aRoot), "confirming wrote no file in this checkout")
	assert.Equal(t, beforeB, treeOf(t, c.bRoot), "and none in the other")
}

// TestApply_LandsInTheStoreAndIsRecorded: `kapi apply` writes a term, a
// content-memory pair and a voice rule into the project's stores, writes no
// file but the recipe, and leaves each one on the project's record.
func TestApply_LandsInTheStoreAndIsRecorded(t *testing.T) {
	a, cmd, root, recipe := newApplyAssetProject(t)
	ownWorkspace(t, a)
	ctx := context.Background()
	_, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	before := treeOf(t, root)

	for _, e := range []changeEntry{
		{Kind: kindTerm, Op: "upsert", Term: "sign in", Locale: "en", Status: "preferred", Replacement: "log in"},
		{Kind: kindMemory, Op: "add", Source: "Save", Target: "Enregistrer", SourceLocale: "en", TargetLocale: "fr"},
		{Kind: kindVoice, Op: "add-rule", List: "forbidden", Term: "utilise", Replacement: "use"},
	} {
		res := a.applyRecordedAssetEntry(ctx, cmd, e)
		require.Equal(t, "applied", res.Status, "kind %s detail: %s", e.Kind, res.Detail)
		assert.NotContains(t, res.Detail, "not recorded", "the change is on the record")
	}

	// The recipe is configuration, and a voice rule binds a profile in it. Every
	// other file in the checkout stands where it was.
	after := treeOf(t, root)
	for path, content := range after {
		if path == project.RecipeFileName {
			continue
		}
		assert.Equal(t, before[path], content, "apply changed %s", path)
	}
	for path := range before {
		assert.Contains(t, after, path, "apply removed %s", path)
	}

	ops := recordedOps(t, a, recipe)
	require.Len(t, ops, 2, "each applied term and memory pair is one edit")
	edited := map[contextop.SubjectKind]int{}
	for _, op := range ops {
		if op.Kind == contextop.KindEdit {
			edited[op.Subject.Kind]++
		}
	}
	assert.Equal(t, map[contextop.SubjectKind]int{
		contextop.SubjectTerm: 1, contextop.SubjectMemory: 1,
	}, edited, "a voice-profile vocabulary rule is recorded by the profile's own history")
}

// TestVoiceEdit_ReadsTheEditedProfileBackAsOneOperation drives the editor with
// a script, the way a person drives it with their own.
func TestVoiceEdit_ReadsTheEditedProfileBackAsOneOperation(t *testing.T) {
	a, cmd, root, recipe := newApplyAssetProject(t)
	ownWorkspace(t, a)
	ctx := context.Background()
	seedVoiceProfile(t, a, cmd)

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", writingEditor(t, "name: Edited Voice\ntone:\n  formality: casual\n"))

	res, err := a.EditVoiceProfile(ctx, cmd, VoiceEditRequest{})
	require.NoError(t, err)
	require.True(t, res.Changed, "the edited document differed from the one opened")
	assert.NotEmpty(t, res.Recorded)

	store, release, err := a.ProjectVoiceStore(ctx, root)
	require.NoError(t, err)
	defer release()
	prof, err := store.GetProfile(ctx, res.ID)
	require.NoError(t, err)
	assert.Equal(t, "casual", prof.Tone.Formality, "what the editor saved is what the store holds")
	assert.Equal(t, "Edited Voice", prof.Name)

	ops := recordedOps(t, a, recipe)
	edit := ops[len(ops)-1]
	assert.Equal(t, contextop.KindEdit, edit.Kind)
	assert.Contains(t, edit.Subject.Text, "edited whole")
}

// TestVoiceEdit_AnUnchangedDocumentChangesNothing: a person who opens the
// profile, reads it and quits leaves the store and the record where they were,
// and so does an editor that exits with an error.
func TestVoiceEdit_AnUnchangedDocumentChangesNothing(t *testing.T) {
	a, cmd, _, recipe := newApplyAssetProject(t)
	ownWorkspace(t, a)
	ctx := context.Background()
	seedVoiceProfile(t, a, cmd)
	held := len(recordedOps(t, a, recipe))

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", quietEditor(t))
	res, err := a.EditVoiceProfile(ctx, cmd, VoiceEditRequest{})
	require.NoError(t, err)
	assert.False(t, res.Changed, "a document saved as it was opened is not an edit")
	assert.Empty(t, res.Recorded)

	t.Setenv("EDITOR", refusingEditor(t))
	_, err = a.EditVoiceProfile(ctx, cmd, VoiceEditRequest{})
	require.Error(t, err)
	assert.Len(t, recordedOps(t, a, recipe), held, "and the record stands")
}

// TestVoiceEdit_RefusesADocumentThatDoesNotCheckOut: a profile that would not
// pass `kapi voice validate` never reaches the store, and the person is told
// where their edit is.
func TestVoiceEdit_RefusesADocumentThatDoesNotCheckOut(t *testing.T) {
	a, cmd, _, _ := newApplyAssetProject(t)
	ownWorkspace(t, a)
	ctx := context.Background()
	seedVoiceProfile(t, a, cmd)

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", writingEditor(t, "name: Edited Voice\ntone:\n  formality: extremely\n"))
	_, err := a.EditVoiceProfile(ctx, cmd, VoiceEditRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Your edit is at ")
}

// TestVoiceEdit_ClearsConstraintsTheEditDropped: the store reads a profile
// carrying no constraints as "leave the ones you have", which is right for a
// partial write. A full-profile edit states the profile entire, so a section
// the person deleted is gone.
func TestVoiceEdit_ClearsConstraintsTheEditDropped(t *testing.T) {
	a, cmd, root, _ := newApplyAssetProject(t)
	ownWorkspace(t, a)
	ctx := context.Background()
	seedVoiceProfile(t, a, cmd)

	store, release, err := a.ProjectVoiceStore(ctx, root)
	require.NoError(t, err)
	defer release()
	profiles, err := store.ListProfiles(ctx, LocalScope)
	require.NoError(t, err)
	require.NotEmpty(t, profiles)
	prof, err := store.GetProfile(ctx, profiles[0].ID)
	require.NoError(t, err)
	prof.Constraints = []coreprofile.Constraint{{
		ID: "c1", Version: 1, Source: "style guide", Kind: "forbid",
		Statement: "Never open a sentence with a conjunction.",
	}}
	require.NoError(t, store.UpdateProfile(ctx, prof))
	seeded, err := store.GetProfile(ctx, prof.ID)
	require.NoError(t, err)
	require.Len(t, seeded.Constraints, 1, "the store holds the constraint to be dropped")

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", writingEditor(t, "name: Edited Voice\ntone:\n  formality: casual\n"))
	res, err := a.EditVoiceProfile(ctx, cmd, VoiceEditRequest{})
	require.NoError(t, err)
	require.True(t, res.Changed)

	after, err := store.GetProfile(ctx, res.ID)
	require.NoError(t, err)
	assert.Empty(t, after.Constraints, "the constraints the edit dropped are gone")
}

// ownWorkspace gives a test its own workspace, so the operations it records
// are the only ones its project's log holds. Every project scaffolded by
// newApplyAssetProject carries one name, and one workspace would key them all
// to one log.
func ownWorkspace(t *testing.T, a *App) {
	t.Helper()
	a.SetWorkspaceRoot(t.TempDir())
}

// seedVoiceProfile gives the project a voice profile to edit, put there the
// way `kapi apply` puts one there.
func seedVoiceProfile(t *testing.T, a *App, cmd Command) {
	t.Helper()
	res := a.applyRecordedAssetEntry(cmd.Context(), cmd, changeEntry{
		Kind: kindVoice, Op: "add-rule", List: "forbidden", Term: "utilise", Replacement: "use",
	})
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)
}

// writingEditor is an editor that saves document over whatever it was given,
// which is what a person does when they rewrite the profile in front of them.
func writingEditor(t *testing.T, document string) string {
	t.Helper()
	return editorScript(t, "cat > \"$1\" <<'KAPIEOF'\n"+document+"KAPIEOF\nexit 0\n")
}

// quietEditor reads and quits, saving nothing.
func quietEditor(t *testing.T) string {
	t.Helper()
	return editorScript(t, "exit 0\n")
}

// refusingEditor exits with an error, the way a person's editor does when they
// abort.
func refusingEditor(t *testing.T) string {
	t.Helper()
	return editorScript(t, "exit 1\n")
}

// editorScript writes body as an executable shell script and returns its path.
func editorScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "editor.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755))
	return path
}

// recordedOps reads a project's context log oldest first, which is the order
// the operations were made in.
func recordedOps(t *testing.T, a *App, recipe string) []ContextOperation {
	t.Helper()
	list, err := a.ContextOperations(context.Background(), ContextLogRequest{Project: recipe})
	require.NoError(t, err)
	out := make([]ContextOperation, 0, len(list.Operations))
	for _, op := range slices.Backward(list.Operations) {
		out = append(out, op)
	}
	return out
}

// treeOf reads every file under root, keyed by its slash path relative to
// root, so a test can say which files a command wrote. The work directory is
// left out: it is gitignored machine state, rewritten by every command.
func treeOf(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	work := filepath.Join(root, project.StateDirName, project.WorkDirName)
	require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasPrefix(path, work) {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	}))
	return out
}

// sorted returns a copy of in, in order.
func sorted(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
