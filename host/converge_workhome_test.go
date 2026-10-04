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
	assert.NotEmpty(t, hist[0].ContentHash, "with the identity history reconciles by")
	assert.NotEmpty(t, hist[0].ContextHash)
	assert.Equal(t, []string{"set_content"}, hist[0].Ops, "with the kind of operation the draft comes to")
	assert.Equal(t, "translate", hist[0].Tool, "and the tool that drafted it")
}

// TestConverge_DeletingTheCacheLosesNoDraftAndCallsNoProvider is WP8's
// acceptance: `.kapi/work/` is a cache. Deleting it and running again keeps
// every parked draft, and the edit a person made to one, and serves each
// without a provider call.
func TestConverge_DeletingTheCacheLosesNoDraftAndCallsNoProvider(t *testing.T) {
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	_, events := parkedReviewPass(t, a, cmd, recipe)
	require.Equal(t, 4, parkedProduced(t, events, "nl").ViaAI, "the first pass pays for every unit")
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "desktop"})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "site/locales/nl.json", Blocks: []string{"title"}})
	require.NoError(t, err)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(page.Blocks[0].Ref, page.Blocks[0].Rev, "Tijvenster")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	before := map[string]map[string]string{"nb": keptTexts(t, a, dir, "nb"), "nl": keptTexts(t, a, dir, "nl")}
	require.Len(t, before["nl"], 4)
	require.Equal(t, "Tijvenster", before["nl"]["title"])

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

// TestConverge_AFileAPassWritesReleasesTheKeptEdition: a recipe that stops
// withholding delivery has its next pass write each translation where the
// recipe points, and a file written that way is the edition's home, so the
// workspace home stops keeping the drafts it held.
func TestConverge_AFileAPassWritesReleasesTheKeptEdition(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	require.Len(t, keptDrafts(t, a, dir, "nl"), 4)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = project.MaterializeManual
	require.NoError(t, project.Save(recipe, proj))
	parkedReviewPass(t, a, cmd, recipe)

	for _, loc := range []string{"nb", "nl"} {
		require.FileExists(t, filepath.Join(dir, "site", "locales", loc+".json"))
		assert.Empty(t, keptDrafts(t, a, dir, loc), "%s: the file holds the edition, so the workspace keeps none of it", loc)
	}

	// The block history names what each write to the workspace home was:
	// the draft the flow's tool set, and the release that took it out.
	db, err := a.ProjectDB(context.Background(), dir)
	require.NoError(t, err)
	hist, err := db.History().Edition(context.Background(), a.documentIndexOrEmpty(context.Background(), dir).Key(parkedSource), "title", "nl", 0)
	require.NoError(t, err)
	require.Len(t, hist, 2)
	assert.Equal(t, []string{"remove_edition"}, hist[0].Ops, "the release removes the edition from the workspace home")
	assert.Equal(t, "flow:up", hist[0].Origin)
	assert.Equal(t, []string{"set_content"}, hist[1].Ops)
	assert.Equal(t, "translate", hist[1].Tool)
}

// TestStatus_ListsAKeptDraftConflict: an edit to a kept draft that another
// machine made from an older version, and that changed a block the head has
// moved since, does not land; kapi status names it.
func TestStatus_ListsAKeptDraftConflict(t *testing.T) {
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	require.Empty(t, a.statusConflicts(ctx, recipe), "one machine's own writes never conflict")

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

	conflicts := a.statusConflicts(ctx, recipe)
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
	written, err := a.materializeFromProjectStore(ctx, os.Stderr, proj, recipe, []model.LocaleID{"nl"}, false)
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	body, err := os.ReadFile(filepath.Join(dir, "site", "locales", "nl.json"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "Plan een oversteek", "the workspace home's edition is what is delivered")
	assert.Empty(t, keptDrafts(t, a, dir, "nl"), "and it is kept there no more")
	assert.Len(t, keptDrafts(t, a, dir, "nb"), 4)

	learned := false
	for _, e := range memoryEntries(t, a, recipe) {
		if e.VariantText("en") == "Plan a crossing" && e.VariantText("nl") == "Plan een oversteek" {
			learned = true
		}
	}
	assert.True(t, learned, "the delivered wording, the person's edit, reaches the content memory")
}

// TestConverge_AFileThatAppearsKeepsAPersonsEdit: a translation's file can
// appear by a path other than a delivery of what the workspace keeps: a
// person writes it, or a recipe that stops withholding delivery has its pass
// write it from the flow's drafts. The file is the edition's home from then
// on, so the workspace releases the drafts a tool made, and keeps the wording
// a person wrote that the file does not hold, which kapi status lists and
// kapi merge writes into the file.
func TestConverge_AFileThatAppearsKeepsAPersonsEdit(t *testing.T) {
	cases := []struct {
		name   string
		appear func(t *testing.T, recipe, dir string)
	}{
		{name: "a person writes the file", appear: func(t *testing.T, _, dir string) {
			path := filepath.Join(dir, "site", "locales", "nl.json")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(`{"title": "Iets anders"}`+"\n"), 0o644))
		}},
		{name: "the recipe stops withholding delivery", appear: func(t *testing.T, recipe, _ string) {
			proj, err := project.Load(recipe)
			require.NoError(t, err)
			proj.Defaults.Materialize = project.MaterializeManual
			require.NoError(t, project.Save(recipe, proj))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

			tc.appear(t, recipe, dir)
			parkedReviewPass(t, a, cmd, recipe)
			nl := filepath.Join(dir, "site", "locales", "nl.json")
			require.FileExists(t, nl)
			body, err := os.ReadFile(nl)
			require.NoError(t, err)
			require.NotContains(t, string(body), "Tijvenster")
			assert.Equal(t, map[string]string{"title": "Tijvenster"}, keptTexts(t, a, dir, "nl"),
				"the person's wording stays kept, and the tool's drafts are released")
			conflicts := a.statusConflicts(ctx, recipe)
			require.Len(t, conflicts, 1)
			assert.Equal(t, StatusConflict{Doc: parkedSource, Locale: "nl", File: "site/locales/nl.json", Blocks: []string{"title"}}, conflicts[0])
			var text strings.Builder
			require.NoError(t, StatusOutput{Conflicts: conflicts}.FormatText(&text))
			assert.Contains(t, text.String(), "kapi merge writes it into the file")

			proj, err := project.Load(recipe)
			require.NoError(t, err)
			_, err = a.materializeFromProjectStore(ctx, os.Stderr, proj, recipe, []model.LocaleID{"nl"}, false)
			require.NoError(t, err)
			body, err = os.ReadFile(nl)
			require.NoError(t, err)
			assert.Contains(t, string(body), "Tijvenster", "kapi merge writes the person's wording into the file")
			assert.Empty(t, keptTexts(t, a, dir, "nl"))
			assert.Empty(t, a.statusConflicts(ctx, recipe))
		})
	}
}

// TestConverge_ADeliveryWritesTheRunsDraftsAndTheEditsMadeToThem: a locale
// that clears its gate in the run where a source sentence changed is
// delivered with the run's fresh draft of that sentence, and with the
// wording a person gave another unit while the locale was parked, never with
// the draft the workspace kept of the old sentence.
func TestConverge_ADeliveryWritesTheRunsDraftsAndTheEditsMadeToThem(t *testing.T) {
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	require.Contains(t, keptTexts(t, a, dir, "nl")["footer"], "six")

	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "desktop"})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "site/locales/nl.json", Blocks: []string{"cta"}})
	require.NoError(t, err)
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(page.Blocks[0].Ref, page.Blocks[0].Rev, "Plan een oversteek")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	for _, key := range []string{"title", "subtitle"} {
		_, err := decideUnit(cmd.Context(), a, recipe,
			ReviewUnitRef{File: filepath.Join("site", "locales", "nl.json"), Key: key, Locale: "nl"},
			ReviewDecisionApproved, "")
		require.NoError(t, err)
	}
	src := filepath.Join(dir, "src", "en.json")
	body, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(src, []byte(strings.Replace(string(body), "every six minutes", "every ten minutes", 1)), 0o644))

	out, events := parkedReviewPass(t, a, cmd, recipe)
	require.True(t, parkedLocaleResult(t, out, "nl").Shippable)
	require.Equal(t, 1, parkedProviderCalls(events, "nl"), "the pass drafts the changed sentence")

	delivered, err := os.ReadFile(filepath.Join(dir, "site", "locales", "nl.json"))
	require.NoError(t, err)
	assert.Contains(t, string(delivered), "ten", "the run's fresh draft is delivered")
	assert.NotContains(t, string(delivered), "six", "and never the draft of the old sentence")
	assert.Contains(t, string(delivered), "Plan een oversteek", "with the person's wording")
	assert.Empty(t, keptDrafts(t, a, dir, "nl"), "and the workspace keeps none of it")
}

// TestStatus_AConflictClearsWhenItsDraftIsDelivered: a delivery that writes
// a kept edition to its file settles every write a merge left divergent on
// it, so kapi status names no conflict on an edition the workspace no longer
// keeps.
func TestStatus_AConflictClearsWhenItsDraftIsDelivered(t *testing.T) {
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
		Doc: doc, Path: parkedSource, Edition: "nl", Expect: seq, Base: "",
		Actor: change.Actor{Kind: change.ActorPerson, Name: "elsewhere"}, Origin: "desktop",
		Blocks: []workhome.CommitBlock{{Block: "title", Before: "r:0000000000000000",
			After: model.RunsRevision(model.EditionKey{Locale: "nl"}, other.Runs), Edition: &other}},
	})
	require.NoError(t, err)
	require.Len(t, a.statusConflicts(ctx, recipe), 1)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	_, err = a.materializeFromProjectStore(ctx, os.Stderr, proj, recipe, []model.LocaleID{"nl"}, false)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, "site", "locales", "nl.json"))
	assert.Empty(t, keptDrafts(t, a, dir, "nl"))
	assert.Empty(t, a.statusConflicts(ctx, recipe), "the delivery settled the conflict")
}

// TestConverge_AParkedLocaleWithAFileKeepsNoDraft: a locale delivered once
// parks again when its source gains a sentence. Its file is its home, so the
// run keeps none of its drafts in the workspace, the new sentence's included:
// the workspace never holds a second copy of an edition a file holds.
func TestConverge_AParkedLocaleWithAFileKeepsNoDraft(t *testing.T) {
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	for _, key := range []string{"title", "subtitle"} {
		_, err := decideUnit(cmd.Context(), a, recipe,
			ReviewUnitRef{File: filepath.Join("site", "locales", "nl.json"), Key: key, Locale: "nl"},
			ReviewDecisionApproved, "")
		require.NoError(t, err)
	}
	out, _ := parkedReviewPass(t, a, cmd, recipe)
	require.True(t, parkedLocaleResult(t, out, "nl").Shippable)
	require.FileExists(t, filepath.Join(dir, "site", "locales", "nl.json"))

	src := filepath.Join(dir, "src", "en.json")
	body, err := os.ReadFile(src)
	require.NoError(t, err)
	added := strings.Replace(string(body), `"footer":`, `"note": "Tide tables are estimates",
  "footer":`, 1)
	require.NoError(t, os.WriteFile(src, []byte(added), 0o644))
	keptOps := func() int {
		n := 0
		for _, op := range editOps(t, a, dir) {
			if strings.HasSuffix(op.Subject, "@nl") {
				n++
			}
		}
		return n
	}
	was := keptOps()
	out, _ = parkedReviewPass(t, a, cmd, recipe)
	require.False(t, parkedLocaleResult(t, out, "nl").Shippable, "two approvals of five units are short of the gate")
	assert.Empty(t, keptDrafts(t, a, dir, "nl"))
	assert.Equal(t, was, keptOps(), "the run records nothing in the workspace home for a locale with a file")
}

// TestMerge_AKeptDraftOfAChangedSourceStaysStale: kapi merge writes a kept
// draft with the basis it was made from, so a draft of a sentence that has
// changed since reads as stale in its file, as it did where it was kept,
// rather than as a translation of the new sentence.
func TestMerge_AKeptDraftOfAChangedSourceStaysStale(t *testing.T) {
	ctx := context.Background()
	a, cmd, recipe, dir := parkedReviewProject(t)
	parkedReviewPass(t, a, cmd, recipe)
	keptBasis := keptDrafts(t, a, dir, "nl")["footer"].Basis
	require.NotEmpty(t, keptBasis)

	src := filepath.Join(dir, "src", "en.json")
	body, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(src, []byte(strings.Replace(string(body), "every six minutes", "every ten minutes", 1)), 0o644))
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: parkedSource, Blocks: []string{"footer"}})
	require.NoError(t, err)
	require.NotEqual(t, keptBasis, page.Blocks[0].Rev, "the source moved since the draft was made")

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	_, err = a.materializeFromProjectStore(ctx, os.Stderr, proj, recipe, []model.LocaleID{"nl"}, false)
	require.NoError(t, err)
	delivered, err := os.ReadFile(filepath.Join(dir, "site", "locales", "nl.json"))
	require.NoError(t, err)
	require.Contains(t, string(delivered), "six")

	db, err := a.ProjectDB(ctx, dir)
	require.NoError(t, err)
	hist, err := db.History().Edition(ctx, a.documentIndexOrEmpty(ctx, dir).Key(parkedSource), "footer", "nl", 0)
	require.NoError(t, err)
	found := false
	for _, h := range hist {
		if h.Origin == "merge" && h.After != model.AbsentRevision {
			found = true
			assert.Equal(t, keptBasis, h.Basis, "the delivery records the basis the draft was made from")
		}
	}
	assert.True(t, found, "the delivery is recorded")
}
