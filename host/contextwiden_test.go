package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
)

// pointedProject is a project with two profiles: acme governs docs/ and
// help/ through two channels, relaunch governs landing/. A rule seen in docs/
// sits at acme/docs, and acme/help is the sibling point in the same product.
func pointedProject(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write("kapi.yaml", `version: v1
id: `+projectIDFor(name)+`
name: `+name+`
defaults:
  source_language: en
profiles:
  acme:
    channels: [docs, help]
  relaunch:
    channels: [landing]
collections:
  - name: acme-docs
    channel: acme/docs
    source_only: true
    content:
      - path: "docs/*.md"
  - name: acme-help
    channel: acme/help
    source_only: true
    content:
      - path: "help/*.md"
  - name: relaunch-landing
    channel: relaunch/landing
    source_only: true
    content:
      - path: "landing/*.md"
`)
	write("docs/a.md", "We utilise the widget.\n")
	write("help/b.md", "We utilise the widget.\n")
	write("landing/c.md", "We utilise the widget.\n")
	return root
}

// keptAtDocs records the rule against docs/a.md and keeps it, so it holds at
// acme/docs.
func keptAtDocs(t *testing.T, app *App, root string) ContextOperation {
	t.Helper()
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor:    person,
		Project:  recipeOf(root),
		Term:     "use", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "docs/a.md", Unit: "p", Quote: "We utilise the widget"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: op.ID})
	require.NoError(t, err)
	return op
}

// pointRefs lists the refs of a preview's points.
func pointRefs(points []ContextWidenPoint) []string {
	out := make([]string, 0, len(points))
	for _, p := range points {
		out = append(out, p.Ref)
	}
	return out
}

// gapReasons maps a preview's unexamined projects to the reason given.
func gapReasons(coverage ContextWidenCoverage) map[workspace.ProjectKey]string {
	out := map[workspace.ProjectKey]string{}
	for _, g := range coverage.NotExamined {
		out[g.Project] = g.Reason
	}
	return out
}

// TestWideningPreviewCoversTheSiblingPoints: widening a rule from acme/docs
// past the channel axis reaches acme/help, the other point of the same
// product, and nothing of another product. The scope shown is the one the
// widening records.
func TestWideningPreviewCoversTheSiblingPoints(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := pointedProject(t, "ctxwiden-sibling")
	kept := keptAtDocs(t, app, root)

	preview, err := app.PreviewContextWidening(t.Context(), ContextWidenPreviewRequest{
		Project: recipeOf(root), ID: kept.ID, To: "channel",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"product": "acme", "channel": "docs"}, preview.From.Coordinates)
	assert.Equal(t, map[string]string{"product": "acme"}, preview.Scope.Coordinates)
	assert.Equal(t, contextop.LevelProject, preview.Scope.Level, "dropping an axis keeps the rule in its project")
	assert.Equal(t, []string{"acme/help"}, pointRefs(preview.Points), "the sibling point is new; acme/docs is already covered and relaunch is another product")
	assert.Equal(t, []string{"acme-help"}, preview.Points[0].Collections)
	assert.Empty(t, preview.Projects, "widening past an axis stays in the project")
	assert.Equal(t, "utilise", preview.Rule.Term.Term)

	widened, err := app.WidenContextOperation(t.Context(), ContextWidenRequest{
		Actor: person, Project: recipeOf(root), ID: kept.ID, To: "channel",
	})
	require.NoError(t, err)
	assert.Equal(t, preview.Scope, widened.Scope, "the preview and the widening share one scope arithmetic")

	_, err = app.PreviewContextWidening(t.Context(), ContextWidenPreviewRequest{
		Project: recipeOf(root), ID: kept.ID, To: "mode",
	})
	assert.Error(t, err, "an axis the rule is not specific about is refused, as the widening refuses it")
}

// TestWideningPreviewListsAProjectWithNoCheckoutAsNotExamined: a project
// registered in the workspace from another machine is named as newly covered
// by a workspace-wide rule, and listed as not examined because none of its
// files are here. The rule's own project, checked out but never extracted,
// is listed as not examined for want of a projection.
func TestWideningPreviewListsAProjectWithNoCheckoutAsNotExamined(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := pointedProject(t, "ctxwiden-reach")
	kept := keptAtDocs(t, app, root)
	own := workspace.ProjectKey(projectIDFor("ctxwiden-reach"))

	ws, err := app.Workspace(t.Context())
	require.NoError(t, err)
	archive := workspace.ProjectKey(projectIDFor("ctxwiden-archive"))
	_, err = ws.Register(t.Context(), archive, "Archive", "")
	require.NoError(t, err)

	preview, err := app.PreviewContextWidening(t.Context(), ContextWidenPreviewRequest{
		Project: recipeOf(root), ID: kept.ID, To: WidenToWorkspace,
	})
	require.NoError(t, err)
	assert.Equal(t, contextop.LevelWorkspace, preview.Scope.Level)
	require.Len(t, preview.Projects, 1)
	assert.Equal(t, ContextWidenProject{Key: archive, Name: "Archive", CheckedOut: false}, preview.Projects[0])
	assert.ElementsMatch(t, []string{"", "acme/help", "relaunch/landing"}, pointRefs(preview.Points),
		"a workspace rule drops the profile's axes, so every other point of its own project is new")

	gaps := gapReasons(preview.Coverage)
	assert.Equal(t, ContextWidenNoCheckout, gaps[archive])
	assert.Equal(t, ContextWidenNoProjection, gaps[own])
	assert.Empty(t, preview.Coverage.Examined)
	assert.Empty(t, preview.Units)
}

// TestWideningPreviewReportsMatchedUnitsFromABuiltProjection: with a
// projection built, the units at the newly covered points whose text holds
// the avoided form are listed, and the units the rule already covers or
// never will are not. Coverage carries the counts.
func TestWideningPreviewReportsMatchedUnitsFromABuiltProjection(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := pointedProject(t, "ctxwiden-units")
	kept := keptAtDocs(t, app, root)

	db, err := app.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	sess, err := db.Blocks().Begin(t.Context())
	require.NoError(t, err)
	put := func(file, id, text string) {
		b := &blockstore.Block{Hash: file + "#" + id, ID: id, Translatable: true,
			Editions: kbf.SourceEditions([]model.Run{model.TextR(text)})}
		b.Properties.File = file
		require.NoError(t, sess.PutBlock("", b))
	}
	put("docs/a.md", "p1", "We utilise the widget.")
	put("help/b.md", "p1", "Please utilise the widget, then utilise it again.")
	put("help/b.md", "p2", "Use the widget.")
	put("landing/c.md", "p1", "We utilise the widget.")
	require.NoError(t, sess.Commit())

	preview, err := app.PreviewContextWidening(t.Context(), ContextWidenPreviewRequest{
		Project: recipeOf(root), ID: kept.ID, To: "channel",
	})
	require.NoError(t, err)
	require.Len(t, preview.Units, 1, "docs/ is covered already and landing/ is another product")
	unit := preview.Units[0]
	assert.Equal(t, "help/b.md", unit.Document)
	assert.Equal(t, "p1", unit.Unit)
	assert.Equal(t, 2, unit.Matches)
	assert.Equal(t, "Please utilise the widget, then utilise it again.", unit.Text)

	require.Len(t, preview.Coverage.Examined, 1)
	assert.Equal(t, 4, preview.Coverage.Examined[0].Units)
	assert.Equal(t, 1, preview.Coverage.Examined[0].Matched)
	assert.Empty(t, preview.Coverage.NotExamined)
	assert.False(t, preview.Coverage.Truncated)
}

// TestDecidingOnAProjectWithNoCheckout: a project registered in the
// workspace with no checkout on this machine is read and decided on by its
// key. The policy is the same one: an agent is refused, a person keeps, and
// the kept rule lands in the terms store the workspace holds for the project.
func TestDecidingOnAProjectWithNoCheckout(t *testing.T) {
	isolateCheckExecution(t)
	t.Setenv(EnvDataDir, t.TempDir())
	app := &App{SourceLang: "en"}
	t.Cleanup(app.Shutdown)
	ctx := t.Context()

	ws, err := app.Workspace(ctx)
	require.NoError(t, err)
	key := workspace.ProjectKey(projectIDFor("ctxops-archive"))
	_, err = ws.Register(ctx, key, "Archive", "")
	require.NoError(t, err)
	rule := contextop.ObservedRule("use", []string{"utilise"})
	suggested, err := contextop.NewLedger(ws, contextop.PersonDecides).Append(ctx, contextop.Record{
		Actor:    agentIn("s1"),
		Kind:     contextop.KindObserve,
		Project:  key,
		Subject:  contextop.Subject{Kind: contextop.SubjectTerm, Term: &rule},
		Evidence: []contextop.Evidence{{Path: "docs/a.md", Unit: "p1"}},
		Scope:    contextop.Scope{Level: contextop.LevelProject},
	})
	require.NoError(t, err)

	read, err := app.ContextOperations(ctx, ContextLogRequest{Project: string(key)})
	require.NoError(t, err)
	require.Len(t, read.Operations, 1)
	assert.Equal(t, string(key), read.Project)
	assert.Equal(t, contextop.StatusSuggested, read.Operations[0].Status)

	whole, err := app.ContextOperations(ctx, ContextLogRequest{AllProjects: true})
	require.NoError(t, err)
	require.Len(t, whole.Operations, 1, "the whole-workspace read holds it too")
	assert.Equal(t, key, whole.Operations[0].Project)

	_, err = app.KeepContextOperation(ctx, ContextKeepRequest{Actor: agentIn("s1"), Project: string(key), ID: suggested.ID})
	require.ErrorIs(t, err, contextop.ErrRefused, "the policy is unchanged: an agent does not keep")

	kept, err := app.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: string(key), ID: suggested.ID})
	require.NoError(t, err)
	assert.Equal(t, landedTerms, kept.Landed)

	w, err := app.ProjectorFor(ctx, key)
	require.NoError(t, err)
	concepts, err := w.Terms().Concepts(ctx)
	require.NoError(t, err)
	var stored []string
	for _, c := range concepts {
		for _, term := range c.Terms {
			stored = append(stored, term.Text)
		}
	}
	assert.Contains(t, stored, "utilise", "the kept rule lands in the terms store the workspace holds for the project")

	preview, err := app.PreviewContextWidening(ctx, ContextWidenPreviewRequest{Project: string(key), ID: suggested.ID, To: WidenToWorkspace})
	require.NoError(t, err)
	assert.Equal(t, ContextWidenNoCheckout, gapReasons(preview.Coverage)[key], "its files are not here to read")

	dropped, err := app.DropContextOperation(ctx, ContextDropRequest{Actor: person, Project: string(key), ID: suggested.ID})
	require.NoError(t, err)
	assert.Contains(t, dropped.Landed, "taken back out of")
	concepts, err = w.Terms().Concepts(ctx)
	require.NoError(t, err)
	stored = stored[:0]
	for _, c := range concepts {
		for _, term := range c.Terms {
			stored = append(stored, term.Text)
		}
	}
	assert.NotContains(t, stored, "utilise", "dropping takes the rule back out of the store")

	_, err = app.ContextOperations(ctx, ContextLogRequest{Project: projectIDFor("ctxops-nowhere")})
	assert.Error(t, err, "a key the workspace does not hold is still an error")
}
