package host

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// answerAt resolves the by-location answer for one file of a project, the
// answer `kapi context <path>` prints.
func answerAt(t *testing.T, app *App, root, path string) *ContextAnswer {
	t.Helper()
	req := ContextPointRequest{Path: path}
	src, done := app.ContextSourcesAt(bindingsCmd(t, recipeOf(root)), req)
	defer done()
	res, err := ResolveContextAt(t.Context(), src, req)
	require.NoError(t, err)
	return res
}

// TestKeptNoteAnswersWhereItHolds: a note in prose that a person keeps is
// stated by the answer at the point its evidence was seen, as an established
// note with the person's keep behind it, and by no answer at a point it does not
// cover. Keeping it writes no store, and the keep says where it lives.
func TestKeptNoteAnswersWhereItHolds(t *testing.T) {
	ctx := t.Context()
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-keptnote")
	const text = "The teen pages address the reader as you."

	noted, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agentIn("s-keptnote"), Project: recipeOf(root), Text: text,
		Evidence: []contextop.Evidence{{Path: "teens/a.md"}},
	})
	require.NoError(t, err)

	before := answerAt(t, app, root, "teens/a.md")
	require.Len(t, before.Suggestions, 1, "the note is a suggestion until a person keeps it")
	assert.Equal(t, text, before.Suggestions[0].Text)
	assert.Empty(t, before.EstablishedNotes)

	kept, err := app.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: noted.ID})
	require.NoError(t, err)
	assert.Equal(t, landedNote, kept.Landed, "a kept note is written to no store")
	assert.NotContains(t, kept.Landed, "terms store")

	at := answerAt(t, app, root, "teens/a.md")
	assert.Empty(t, at.Suggestions, "a kept note is no longer a suggestion")
	require.Len(t, at.EstablishedNotes, 1, "the kept note answers at its evidence file")
	got := at.EstablishedNotes[0]
	assert.Equal(t, noted.ID, got.Operation)
	assert.Equal(t, string(contextop.SubjectNote), got.Kind)
	assert.Equal(t, string(contextop.StatusEstablished), got.Status)
	assert.Equal(t, text, got.Text)
	assert.NotEqual(t, CoverageEmpty, at.Coverage, "a kept note is context the answer stands on")

	var out bytes.Buffer
	require.NoError(t, at.FormatText(&out))
	assert.Contains(t, out.String(), "Established notes:")
	assert.Contains(t, out.String(), "- "+text)
	assert.NotContains(t, out.String(), "Suggested, not yet established")

	digest, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, digest.Established, 1)
	assert.Equal(t, []string{"age"}, digest.Established[0].WidenTo,
		"a kept note widens past its axes and never to the workspace")

	elsewhere := answerAt(t, app, root, "adults/a.md")
	assert.Empty(t, elsewhere.EstablishedNotes, "a note seen in the teen pages holds only there")

	// Dropping it takes it out of every answer.
	_, err = app.DropContextOperation(ctx, ContextDropRequest{Actor: person, Project: recipeOf(root), ID: noted.ID})
	require.NoError(t, err)
	dropped := answerAt(t, app, root, "teens/a.md")
	assert.Empty(t, dropped.EstablishedNotes)
	assert.Empty(t, dropped.Suggestions)
}

// TestKeptNoteHoldsWithinItsProject: the workspace holds term rules, and a note
// states none, so keeping a note across the workspace is refused before
// anything is recorded, and the note stays a suggestion a person can keep.
func TestKeptNoteHoldsWithinItsProject(t *testing.T) {
	ctx := t.Context()
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-keptnotews")

	noted, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agentIn("s-keptnotews"), Project: recipeOf(root), Text: "The teen pages address the reader as you.",
		Evidence: []contextop.Evidence{{Path: "teens/a.md"}},
	})
	require.NoError(t, err)

	_, err = app.KeepContextOperation(ctx, ContextKeepRequest{
		Actor: person, Project: recipeOf(root), ID: noted.ID, WidenTo: WidenToWorkspace,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a note holds within its project")

	log, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	for _, op := range log.Operations {
		assert.NotEqual(t, contextop.KindKeep, op.Kind, "a refused keep records nothing")
	}
	assert.Len(t, answerAt(t, app, root, "teens/a.md").Suggestions, 1)
}
