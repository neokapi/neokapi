package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workhome"
)

// A gated run keeps what it withholds in the workspace home, outside the
// checkout: the drafts of a parked locale are logged, readable and editable
// where they live, delivered by the next delivery, and a checkout whose
// `.kapi/work/` is deleted loses none of them and pays no provider to serve
// them again.

// parkedSource is the document the parked-review fixture translates.
const parkedSource = "src/en.json"

// keptDrafts reads what the workspace home keeps of a locale's edition of
// the fixture's document, by block key.
func keptDrafts(t *testing.T, a *App, dir, locale string) map[string]workhome.Row {
	t.Helper()
	h, err := a.keptEditions(dir).open(context.Background(), false)
	require.NoError(t, err)
	if h == nil {
		return nil
	}
	rows, _, err := h.Rows(context.Background(), parkedSource, model.EditionKey{Locale: model.LocaleID(locale)})
	require.NoError(t, err)
	return rows
}

// keptTexts reads the text of each block a locale's kept edition holds.
func keptTexts(t *testing.T, a *App, dir, locale string) map[string]string {
	t.Helper()
	h, err := a.keptEditions(dir).open(context.Background(), false)
	require.NoError(t, err)
	if h == nil {
		return nil
	}
	_, eds, err := h.Rows(context.Background(), parkedSource, model.EditionKey{Locale: model.LocaleID(locale)})
	require.NoError(t, err)
	out := map[string]string{}
	for key, ed := range eds {
		out[key] = model.RunsText(ed.Runs)
	}
	return out
}

// freshApp is a second process over the same project and workspace: a new
// App and command, as `kapi up` run again opens them.
func freshApp(t *testing.T, recipe string) (*App, *EnvCommand) {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	t.Cleanup(a.Shutdown)
	return a, cmd
}

// TestConverge_KeepsParkedDraftsInTheWorkspaceHome: the drafts a gated run
// withholds are kept in the workspace home, as the flow's write, with the
// status and origin its producer stamped, the basis each was made from and
// the stamp the producer serves it again by.
func TestConverge_KeepsParkedDraftsInTheWorkspaceHome(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	out, _ := parkedReviewPass(t, a, cmd, recipe)
	require.False(t, out.Converged)

	for _, loc := range []string{"nb", "nl"} {
		rows := keptDrafts(t, a, dir, loc)
		require.Len(t, rows, 4, "every %s draft is kept", loc)
		for key, r := range rows {
			assert.Equal(t, model.Status(model.TargetStatusDraft), r.Status, "%s %s: a tool's draft", loc, key)
			assert.Equal(t, model.OriginAI, r.Origin.Kind, "%s %s: with the producer's origin", loc, key)
			assert.NotEmpty(t, r.Basis, "%s %s: and the source it was made from", loc, key)
			var st draftStamp
			require.NoError(t, json.Unmarshal(r.Stamp, &st), "%s %s: and the stamp its producer serves it by", loc, key)
			assert.NotEmpty(t, st.Config)
		}
		_, err := os.Stat(filepath.Join(dir, "site", "locales", loc+".json"))
		assert.True(t, os.IsNotExist(err), "%s is parked, so it reaches no file", loc)
	}

	db, err := a.ProjectDB(context.Background(), dir)
	require.NoError(t, err)
	hist, err := db.History().Edition(context.Background(), a.documentIndexOrEmpty(context.Background(), dir).Key(parkedSource), "title", "nl", 0)
	require.NoError(t, err)
	require.NotEmpty(t, hist, "keeping a draft is recorded")
	assert.Equal(t, "flow:translate", hist[0].Origin)
	assert.Equal(t, string(change.ActorTool), hist[0].Actor)
}

// TestConverge_DeletingTheCacheLosesNoDraftAndCallsNoProvider is WP8's
// acceptance: `.kapi/work/` is a cache. Deleting it and running again keeps
// every parked draft and serves each one without a provider call.
func TestConverge_DeletingTheCacheLosesNoDraftAndCallsNoProvider(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	_, events := parkedReviewPass(t, a, cmd, recipe)
	require.Equal(t, 4, parkedProduced(t, events, "nl").ViaAI, "the first pass pays for every unit")
	before := map[string]map[string]string{"nb": keptTexts(t, a, dir, "nb"), "nl": keptTexts(t, a, dir, "nl")}
	require.Len(t, before["nl"], 4)

	a.Shutdown()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(layout.WorkDir()))

	a2, cmd2 := freshApp(t, recipe)
	out, events2 := parkedReviewPass(t, a2, cmd2, recipe)
	require.False(t, out.Converged, "nothing was reviewed, so both locales stay parked")
	for _, loc := range []string{"nb", "nl"} {
		ev := parkedProduced(t, events2, loc)
		assert.Zero(t, ev.ViaAI, "%s: no provider call", loc)
		assert.Equal(t, 4, ev.ViaDraft, "%s: every unit served from its kept draft", loc)
		assert.Equal(t, before[loc], keptTexts(t, a2, dir, loc), "%s: no draft lost", loc)
		cov := parkedCoverage(t, a2, cmd2, recipe, dir, loc)
		assert.Equal(t, 100, cov.Pct["translated"], "%s: the drafts still measure", loc)
	}
}

// TestConverge_AParkedDraftIsEditedInTheWorkspaceHome: a person edits a
// parked draft through the change service, as Kapi Desktop's review pane and
// kapi apply do. The edit lands in the workspace home and in no file, the
// review queue reads it, and the next run keeps the person's wording.
func TestConverge_AParkedDraftIsEditedInTheWorkspaceHome(t *testing.T) {
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)

	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "desktop"})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "site/locales/nl.json", Blocks: []string{"title"}})
	require.NoError(t, err)
	assert.Equal(t, workhome.Name, page.Home, "the parked translation is read from the workspace home")
	require.Len(t, page.Blocks, 1)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(page.Blocks[0].Ref, page.Blocks[0].Rev, "Tijvenster")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	_, err = os.Stat(filepath.Join(dir, "site", "locales", "nl.json"))
	assert.True(t, os.IsNotExist(err), "editing a parked draft delivers nothing")
	row := keptDrafts(t, a, dir, "nl")["title"]
	assert.Equal(t, model.Status(model.TargetStatusTranslated), row.Status)
	assert.Equal(t, model.OriginHuman, row.Origin.Kind)
	assert.Equal(t, "Tijvenster", keptTexts(t, a, dir, "nl")["title"])

	q, err := a.ReviewQueue(ctx, recipe, "en", ReviewQueueOptions{Languages: []string{"nl"}})
	require.NoError(t, err)
	found := false
	for _, it := range q.Pending {
		if it.Locale == "nl" && it.Key == "title" {
			found = true
			assert.Equal(t, "Tijvenster", it.Target, "the review queue reads the edit")
		}
	}
	assert.True(t, found, "the edited unit is in the review queue")

	parkedReviewPass(t, a, cmd, recipe)
	assert.Equal(t, "Tijvenster", keptTexts(t, a, dir, "nl")["title"], "the next run keeps the person's wording")
}

// TestChangeService_DecidesOnAParkedDraft: a person approves a parked draft
// through the change service, with the revision the workspace home holds, and
// the decision counts where the gate reads it.
func TestChangeService_DecidesOnAParkedDraft(t *testing.T) {
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)

	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "desktop"})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "site/locales/nl.json"})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 4)
	var ops []change.Op
	for _, b := range page.Blocks[:2] {
		require.NotEqual(t, model.AbsentRevision, b.Rev, "a parked draft has a revision in its home")
		ops = append(ops, change.Op{Kind: change.KindDecide, At: b.Ref, IfMatch: b.Rev, Body: &change.Decide{Outcome: change.OutcomeEstablish}})
	}
	res, err := svc.Apply(ctx, change.Set{Ops: ops}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, 50, parkedCoverage(t, a, cmd, recipe, dir, "nl").Pct["established"])
}

// TestChangeService_RefusesADecisionOnNoContent: a translation neither its
// file nor the workspace home holds has nothing a decision could bind to.
func TestChangeService_RefusesADecisionOnNoContent(t *testing.T) {
	a, recipe := withheldProject(t, map[string]string{"docs/a.json": `{"greeting": "Hello there"}` + "\n"})
	svc := changeService(t, a, recipe)
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{{Kind: change.KindDecide,
		At: change.Ref{Doc: "docs/a.json", Block: "greeting", Edition: model.EditionKey{Locale: "de"}}, IfMatch: model.AbsentRevision,
		Body: &change.Decide{Outcome: change.OutcomeEstablish}}}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeNotFound, res.Ops[0].Error.Code)
}

// TestConverge_DeliveryMovesTheDraftsToTheirFile: a locale that clears its
// gate is delivered, and the workspace home stops keeping it, so no edition
// is kept twice; a locale still parked stays kept.
func TestConverge_DeliveryMovesTheDraftsToTheirFile(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	keys := parkedQueueKeys(t, a, recipe, "nl")
	require.Len(t, keys, 4)
	for _, key := range keys[:2] {
		_, err := decideUnit(cmd.Context(), a, recipe,
			ReviewUnitRef{File: filepath.Join("site", "locales", "nl.json"), Key: key, Locale: "nl"},
			ReviewDecisionApproved, "")
		require.NoError(t, err)
	}
	out, _ := parkedReviewPass(t, a, cmd, recipe)
	require.True(t, parkedLocaleResult(t, out, "nl").Shippable)

	require.FileExists(t, filepath.Join(dir, "site", "locales", "nl.json"))
	assert.Empty(t, keptDrafts(t, a, dir, "nl"), "the delivered edition's home is its file")
	assert.Len(t, keptDrafts(t, a, dir, "nb"), 4, "the parked edition stays kept")
}

// TestStatus_ListsAKeptDraftConflict: an edit to a kept draft that another
// machine made from an older version, and that changed a block the head has
// moved since, does not land; kapi status names it.
func TestStatus_ListsAKeptDraftConflict(t *testing.T) {
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	require.Empty(t, a.statusConflicts(ctx, dir), "one machine's own writes never conflict")

	// Another machine's edit of the title, made before this machine's last
	// write to the edition: its base is not the head, and the block it
	// started from has moved since.
	p, err := a.Projector(ctx, dir)
	require.NoError(t, err)
	doc := a.documentIndexOrEmpty(ctx, dir).Key(parkedSource)
	seq, err := p.SubjectHead(ctx, doc, "nl")
	require.NoError(t, err)
	other := model.Edition{Runs: []model.Run{model.TextR("Getijdenvenster")}, Status: model.Status(model.TargetStatusTranslated)}
	_, err = p.CommitWorkspace(ctx, workhome.Commit{
		Doc: doc, Path: parkedSource, Edition: "nl", Expect: seq, Base: "",
		Actor: change.Actor{Kind: change.ActorPerson, Name: "elsewhere"}, Origin: "desktop",
		Blocks: []workhome.CommitBlock{{Block: "title", Before: model.AbsentRevision,
			After: model.RunsRevision(model.EditionKey{Locale: "nl"}, other.Runs), Edition: &other}},
	})
	require.NoError(t, err)

	conflicts := a.statusConflicts(ctx, dir)
	require.Len(t, conflicts, 1)
	assert.Equal(t, parkedSource, conflicts[0].Doc)
	assert.Equal(t, "nl", conflicts[0].Locale)
	assert.Equal(t, []string{"title"}, conflicts[0].Blocks)
	assert.NotEqual(t, "Getijdenvenster", keptTexts(t, a, dir, "nl")["title"], "the edit that sorts first holds the block")

	var text strings.Builder
	require.NoError(t, StatusOutput{Conflicts: conflicts}.FormatText(&text))
	assert.Contains(t, text.String(), "Conflict: the nl draft of src/en.json holds another edit to title")
}

// TestMerge_MaterializeDeliversTheKeptEdition: kapi merge writes a parked
// locale from the workspace home, a person's edit included, and the workspace
// home stops keeping it.
func TestMerge_MaterializeDeliversTheKeptEdition(t *testing.T) {
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)

	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "desktop"})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "site/locales/nl.json", Blocks: []string{"cta"}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(page.Blocks[0].Ref, page.Blocks[0].Rev, "Plan een oversteek")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	written, err := a.materializeFromProjectStore(ctx, os.Stderr, proj, recipe, []model.LocaleID{"nl"}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	body, err := os.ReadFile(filepath.Join(dir, "site", "locales", "nl.json"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "Plan een oversteek", "the workspace home's edition is what is delivered")
	assert.Empty(t, keptDrafts(t, a, dir, "nl"), "and it is kept there no more")
	assert.Len(t, keptDrafts(t, a, dir, "nb"), 4)
}
