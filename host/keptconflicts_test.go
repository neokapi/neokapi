package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workhome"
)

// editConflict is a parked project whose Dutch title holds a conflict: an
// edit another machine made from an older head, which did not land.
func editConflict(t *testing.T) (*App, string, string) {
	t.Helper()
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	p, err := a.Projector(ctx, dir)
	require.NoError(t, err)
	doc := a.documentIndexOrEmpty(ctx, dir).Key(parkedSource)
	seq, err := p.SubjectHead(ctx, doc, "nl")
	require.NoError(t, err)
	other := model.Edition{Runs: []model.Run{model.TextR("Getijdenvenster")}, Status: model.Status(model.TargetStatusTranslated)}
	_, err = p.CommitWorkspace(ctx, workhome.Commit{
		Doc: doc, Path: parkedSource, Edition: "nl", Expect: seq,
		Actor: change.Actor{Kind: change.ActorPerson, Name: "elsewhere"}, Origin: "desktop",
		Blocks: []workhome.CommitBlock{{Block: "title", Before: model.AbsentRevision,
			After: model.RunsRevision(model.EditionKey{Locale: "nl"}, other.Runs), Edition: &other}},
	})
	require.NoError(t, err)
	return a, recipe, dir
}

// decide sends the Dutch of block as text, guarded by rev, as a person does
// from a surface that shows the conflict.
func decide(t *testing.T, a *App, recipe, doc, block, rev, text string) *change.Result {
	t.Helper()
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: recipe, Origin: "desktop", TargetLocale: "nl"})
	require.NoError(t, err)
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		setTo(change.Ref{Doc: doc, Block: block, Edition: model.EditionKey{Locale: "nl"}}, rev, text)}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	return res
}

// TestKeptConflicts_AnEditThatDidNotLandShowsBothWordings: the conflict a
// surface reads names the block with the wording the workspace holds, the
// revision an operation names, and the other machine's wording.
func TestKeptConflicts_AnEditThatDidNotLandShowsBothWordings(t *testing.T) {
	a, recipe, dir := editConflict(t)
	conflicts, err := a.KeptConflicts(context.Background(), recipe)
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	c := conflicts[0]
	assert.Equal(t, ConflictEdit, c.Kind)
	assert.Equal(t, parkedSource, c.Doc)
	assert.Equal(t, "nl", c.Locale)
	assert.NotEmpty(t, c.Edit)
	require.Len(t, c.Blocks, 1)
	b := c.Blocks[0]
	assert.Equal(t, "title", b.Block)
	assert.NotEmpty(t, b.Source)
	assert.Equal(t, keptTexts(t, a, dir, "nl")["title"], b.Held.Text)
	assert.Regexp(t, `^r:`, b.Held.Rev)
	assert.Equal(t, "Getijdenvenster", b.Other.Text)
}

// TestKeptConflicts_EachChoiceSettlesAnEdit: keeping the held wording,
// taking the other machine's, or writing a third each settles the conflict
// through the change service.
func TestKeptConflicts_EachChoiceSettlesAnEdit(t *testing.T) {
	for _, choice := range []string{"held", "other", "edit"} {
		t.Run(choice, func(t *testing.T) {
			a, recipe, dir := editConflict(t)
			conflicts, err := a.KeptConflicts(context.Background(), recipe)
			require.NoError(t, err)
			require.Len(t, conflicts, 1)
			b := conflicts[0].Blocks[0]
			text := map[string]string{"held": b.Held.Text, "other": b.Other.Text, "edit": "Getijvenster"}[choice]
			res := decide(t, a, recipe, conflicts[0].Doc, b.Block, b.Held.Rev, text)
			require.NotNil(t, res.Record, "the decision is recorded")
			assert.Equal(t, text, keptTexts(t, a, dir, "nl")["title"])
			after, err := a.KeptConflicts(context.Background(), recipe)
			require.NoError(t, err)
			assert.Empty(t, after)
			assert.Empty(t, a.statusConflicts(context.Background(), recipe))
		})
	}
}

// fileConflict is a parked project where a person wrote the Dutch title into
// the workspace, and a Dutch file appeared since without it.
func fileConflict(t *testing.T) (*App, string, string) {
	t.Helper()
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "desktop"})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "site/locales/nl.json", Blocks: []string{"title"}})
	require.NoError(t, err)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(page.Blocks[0].Ref, page.Blocks[0].Rev, "Tijvenster")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	path := filepath.Join(dir, "site", "locales", "nl.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`{"title": "Iets anders"}`+"\n"), 0o644))
	parkedReviewPass(t, a, cmd, recipe)
	return a, recipe, dir
}

// TestKeptConflicts_WordingTheFileDoesNotHold: the file is the edition's
// home, so the held wording is the file's; the workspace's is the other.
// Keeping the file's wording releases the workspace's copy; taking the
// workspace's writes it into the file through the change service first.
func TestKeptConflicts_WordingTheFileDoesNotHold(t *testing.T) {
	for _, choice := range []string{"file", "workspace"} {
		t.Run(choice, func(t *testing.T) {
			ctx := context.Background()
			a, recipe, dir := fileConflict(t)
			conflicts, err := a.KeptConflicts(ctx, recipe)
			require.NoError(t, err)
			require.Len(t, conflicts, 1)
			c := conflicts[0]
			assert.Equal(t, ConflictFile, c.Kind)
			assert.Equal(t, "site/locales/nl.json", c.File)
			require.Len(t, c.Blocks, 1)
			b := c.Blocks[0]
			assert.Equal(t, "Iets anders", b.Held.Text)
			assert.Equal(t, "Tijvenster", b.Other.Text)

			want := b.Held.Text
			if choice == "workspace" {
				decide(t, a, recipe, c.Doc, b.Block, b.Held.Rev, b.Other.Text)
				want = b.Other.Text
			}
			require.NoError(t, a.ReleaseKeptWording(ctx, recipe, c.Doc, model.LocaleID(c.Locale), []string{b.Block}, "desktop"))
			body, err := os.ReadFile(filepath.Join(dir, "site", "locales", "nl.json"))
			require.NoError(t, err)
			assert.Contains(t, string(body), want)
			assert.Empty(t, keptTexts(t, a, dir, "nl"), "the workspace keeps no copy beside the file")
			after, err := a.KeptConflicts(ctx, recipe)
			require.NoError(t, err)
			assert.Empty(t, after)
		})
	}
}
