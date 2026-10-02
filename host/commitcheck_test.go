package host

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
)

// commitVoice is the project's own voice. Its pattern prohibits "simply"; the
// word rules it carries move into the project's terms when the context is read,
// "utilize" failing and "login" advisory. The required pattern holds over a
// whole document.
const commitVoice = `name: Commit Voice
style:
  prohibited_patterns:
    - regex: '(?i)\bsimply\b'
      description: Say what to do without minimising it
  required_patterns:
    - regex: 'Northsea handbook'
      description: Name the handbook
terms:
  - term: utilize
    replacement: use
  - term: login
    replacement: sign in
    advisory: true
`

// commitRecipe declares the French term rules a translation is held to: a
// missed "gadget" fails, a missed "tableau de bord" reports.
const commitRecipe = `version: v1
id: %s
name: commitcheck
defaults:
  source_language: en
  target_languages: [fr]
  flow: converge
collections:
  - name: docs
    content:
      - path: "docs/*.md"
        target: "docs/{lang}/*.md"
flows:
  converge:
    steps:
      - tool: translate
        config:
          provider: demo
          term_rules:
            - term: widget
              replacement: gadget
            - term: dashboard
              replacement: tableau de bord
              advisory: true
`

// commitFixture is a project whose voice, terms, recipe term rules and one
// suggested rule govern docs/guide.md.
type commitFixture struct {
	app    *App
	root   string
	recipe string
}

func newCommitFixture(t *testing.T) commitFixture {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(strings.Replace(commitRecipe, "%s", projectIDFor("commit"+t.Name()), 1)), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("# Guide\n\nWe use the widget every day.\n"), 0o644))
	require.NoError(t, os.WriteFile(layoutVoicePath(t, root), []byte(commitVoice), 0o644))
	readProjectContext(t, root)

	app := &App{SourceLang: "en"}
	t.Cleanup(app.Shutdown)
	// A suggested rule nobody has kept: use "use", not "leverage".
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor:   contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"},
		Project: recipe,
		Term:    "use", InsteadOf: []string{"leverage"},
		Evidence: []contextop.Evidence{{Path: "docs/guide.md", Quote: "We leverage the widget"}},
		Text:     "the handbook says use",
	})
	require.NoError(t, err)
	require.Equal(t, contextop.StatusSuggested, op.Status)
	return commitFixture{app: app, root: root, recipe: recipe}
}

// command is the command a change set arrives on, in the fixture's project.
func (f commitFixture) command(t *testing.T) *EnvCommand {
	t.Helper()
	return commitCommand(t, f.recipe)
}

// commitCommand is the command a change set arrives on, in the project of
// recipe.
func commitCommand(t *testing.T, recipe string) *EnvCommand {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "apply")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	return cmd
}

// norskVoice is a Norwegian project's voice. The word rule it carries moves
// into the project's terms in Norwegian when the context is read: "benytte"
// fails.
const norskVoice = `name: Norsk
terms:
  - term: benytte
    replacement: bruke
`

// norskRecipe is a project whose content is written in Norwegian and
// translated into English.
const norskRecipe = `version: v1
id: %s
name: norsk
defaults:
  source_language: nb
  target_languages: [en]
collections:
  - name: docs
    content:
      - path: "docs/*.md"
        target: "docs/{lang}/*.md"
`

// norskTerms holds the concept "knapp", which an English translation renders
// as "button".
const norskTerms = `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-knapp",
      "terms": [
        { "text": "knapp", "locale": "nb", "status": "preferred" },
        { "text": "button", "locale": "en", "status": "preferred" }
      ]
    }
  ]
}
`

// newNorskProject is a project written in Norwegian, with its context read
// in. It returns the recipe.
func newNorskProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(strings.Replace(norskRecipe, "%s", projectIDFor("norsk"+t.Name()), 1)), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("# Veiledning\n\nVelg knapp nummer to.\n"), 0o644))
	voice := layoutVoicePath(t, root)
	require.NoError(t, os.WriteFile(voice, []byte(norskVoice), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(voice), "terms.json"), []byte(norskTerms), 0o644))
	readProjectContext(t, root)
	return recipe
}

// norskBlock is a paragraph of the Norwegian project's guide, with an English
// translation when en is set.
func norskBlock(source, en string) *model.Block {
	b := &model.Block{ID: "p1", Translatable: true, SourceLocale: "nb",
		Source: []model.Run{{Text: &model.TextRun{Text: source}}}}
	if en != "" {
		b.SetTargetRuns("en", []model.Run{{Text: &model.TextRun{Text: en}}})
	}
	return b
}

// paragraph reads the guide's paragraph block, with its source set to source
// and, when fr is set, a French translation.
func (f commitFixture) paragraph(t *testing.T, source, fr string) *model.Block {
	t.Helper()
	f.app.InitRegistries()
	blocks, err := f.app.ReadBlocksForCheck(t.Context(), filepath.Join(f.root, "docs", "guide.md"), "", nil, "en")
	require.NoError(t, err)
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "widget") {
			b.Source = []model.Run{{Text: &model.TextRun{Text: source}}}
			if fr != "" {
				b.SetTargetRuns("fr", []model.Run{{Text: &model.TextRun{Text: fr}}})
			}
			return b
		}
	}
	t.Fatal("the guide has no paragraph")
	return nil
}

// edit applies set_content to one edition of b through change.ApplyBlock, as a
// person, and returns the change as the service hands it to the commit check.
func edit(t *testing.T, b *model.Block, edition, text string) change.EditionChange {
	t.Helper()
	key, err := model.ParseEditionKey(edition)
	require.NoError(t, err)
	before, had := b.Edition(key)
	rev := model.AbsentRevision
	var beforeRuns []model.Run
	if had {
		rev = model.EditionRevision(b, key)
		beforeRuns = slices.Clone(before.Runs)
	}
	ref := change.Ref{Doc: "docs/guide.md", Block: blockKey(b), Edition: key}
	res := change.ApplyBlock(b, []change.Op{{
		Kind: change.KindSetContent, At: ref, IfMatch: rev,
		Body: &change.SetContent{Text: &text},
	}}, change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson, Name: "asgeir"}})
	require.Equal(t, change.OpApplied, res[0].Status, "%+v", res[0].Error)
	after, _ := b.Edition(key)
	role := change.RoleDerived
	if b.IsSourceEdition(key) {
		role = change.RoleAuthoritative
	}
	return change.EditionChange{Ref: ref, Role: role, Before: beforeRuns, After: after.Runs,
		BeforeRev: rev, AfterRev: model.EditionRevision(b, key), Block: b}
}

func rules(fs []change.Finding) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	slices.Sort(out)
	return out
}

// TestCommitCheck_RefusesOnlyWhatAnEditIntroduces runs each edit through the
// commit check and the service's introduced-only rule: a failing finding the
// edit adds is introduced, one the edition already had is not, and an advisory
// or suggested rule never fails.
func TestCommitCheck_RefusesOnlyWhatAnEditIntroduces(t *testing.T) {
	f := newCommitFixture(t)
	cases := []struct {
		name string
		// source and fr are the block before the change.
		source, fr string
		// edition and text are the change.
		edition, text string
		// introduced are the rules of the failing findings the edit introduces.
		introduced []string
		// after are the rules of every finding on the edition after the edit.
		after []string
		// notFailing are rules found after the edit that must not fail.
		notFailing []string
	}{
		{
			name:   "a typo fix beside a failing term violation lands",
			source: "We utilize the widget evry day.", text: "We utilize the widget every day.",
			after: []string{"terms.vocabulary"},
		},
		{
			name:   "an edit that adds a failing term introduces it",
			source: "We use the widget every day.", text: "We utilize the widget every day.",
			introduced: []string{"terms.vocabulary"}, after: []string{"terms.vocabulary"},
		},
		{
			name:   "an edit that breaks the voice's pattern introduces it",
			source: "We use the widget every day.", text: "Simply use the widget every day.",
			introduced: []string{"voice.style"}, after: []string{"voice.style"},
		},
		{
			name:   "an advisory term reports and never fails",
			source: "We use the widget every day.", text: "Use the login every day.",
			after: []string{"terms.vocabulary"}, notFailing: []string{"terms.vocabulary"},
		},
		{
			name:   "a suggested rule reports and never fails",
			source: "We use the widget every day.", text: "We leverage the widget every day.",
			after: []string{"terms.vocabulary"}, notFailing: []string{"terms.vocabulary"},
		},
		{
			name:   "emptying the source introduces the hygiene failure",
			source: "We use the widget every day.", text: " ",
			introduced: []string{"hygiene.empty"}, after: []string{"hygiene.empty"},
		},
		{
			name:   "a translation that drops a recipe term rule introduces it",
			source: "We use the widget every day.", fr: "Nous utilisons le gadget chaque jour.",
			edition: "fr", text: "Nous utilisons le truc chaque jour.",
			introduced: []string{"terms.terminology"}, after: []string{"terms.terminology"},
		},
		{
			name:   "a typo fix in a translation that already misses the rule lands",
			source: "We use the widget every day.", fr: "Nous utilisons le truc chaque jur.",
			edition: "fr", text: "Nous utilisons le truc chaque jour.",
			after: []string{"terms.terminology"},
		},
		{
			name:    "a new translation is held to the rules from nothing",
			source:  "We use the widget every day.",
			edition: "fr", text: "Nous utilisons le truc chaque jour.",
			introduced: []string{"terms.terminology"}, after: []string{"terms.terminology"},
		},
		{
			name:   "an advisory recipe rule reports in a translation and never fails",
			source: "We use the widget on the dashboard.", fr: "Nous utilisons le gadget sur le tableau de bord.",
			edition: "fr", text: "Nous utilisons le gadget sur le panneau.",
			after: []string{"terms.terminology"}, notFailing: []string{"terms.terminology"},
		},
		{
			name:   "a source edit is never held to its lagging translation",
			source: "We use the widget every day.", fr: "Nous utilisons le truc.",
			text:  "We use the widget each day.",
			after: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := f.paragraph(t, tc.source, tc.fr)
			ch := edit(t, b, tc.edition, tc.text)
			outcomes, _, err := f.app.CommitCheck(f.command(t)).Check(t.Context(), []change.EditionChange{ch})
			require.NoError(t, err)
			require.Len(t, outcomes, 1)
			o := outcomes[0]
			introduced := tc.introduced
			if introduced == nil {
				introduced = []string{}
			}
			assert.Equal(t, introduced, rules(change.Introduced(o)), "introduced: %+v", o)
			assert.Equal(t, tc.after, rules(o.After), "after: %+v", o.After)
			for _, finding := range o.After {
				assert.Equal(t, ch.Ref, *finding.At, "a finding names the edition it was found on")
				if slices.Contains(tc.notFailing, finding.Rule) {
					assert.False(t, finding.Fails, "%s must not fail: %+v", finding.Rule, finding)
				}
			}
			if ch.Before == nil {
				assert.Empty(t, o.Before, "an edition the change created has nothing before")
			}
		})
	}
}

// Removing a derived edition leaves nothing to hold to the rules, so it
// introduces nothing, whether the edition is a translation or a channel
// edition in the source language.
func TestCommitCheck_RemovingAnEditionIntroducesNothing(t *testing.T) {
	f := newCommitFixture(t)
	for _, edition := range []string{"fr", "en;channel=short"} {
		t.Run(edition, func(t *testing.T) {
			b := f.paragraph(t, "We use the widget every day.", "Nous utilisons le truc.")
			key, err := model.ParseEditionKey(edition)
			require.NoError(t, err)
			if !key.IsZero() && key.Locale == "en" {
				b.SetEdition(key, model.Edition{Runs: []model.Run{{Text: &model.TextRun{Text: "We utilize it."}}}})
			}
			before, had := b.Edition(key)
			require.True(t, had)
			rev := model.EditionRevision(b, key)
			ref := change.Ref{Doc: "docs/guide.md", Block: blockKey(b), Edition: key}
			res := change.ApplyBlock(b, []change.Op{{Kind: change.KindRemoveEdition, At: ref, IfMatch: rev, Body: &change.RemoveEdition{}}},
				change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}})
			require.Equal(t, change.OpApplied, res[0].Status, "%+v", res[0].Error)
			ch := change.EditionChange{Ref: ref, Role: change.RoleDerived, Before: slices.Clone(before.Runs), BeforeRev: rev,
				AfterRev: model.AbsentRevision, Block: b}

			outcomes, _, err := f.app.CommitCheck(f.command(t)).Check(t.Context(), []change.EditionChange{ch})
			require.NoError(t, err)
			assert.NotEmpty(t, outcomes[0].Before, "the edition broke a rule before it was removed")
			assert.Empty(t, outcomes[0].After)
			assert.Empty(t, change.Introduced(outcomes[0]))
		})
	}
}

// A finding raised by a suggested rule says so, as it does in the check
// report.
func TestCommitCheck_MarksSuggestedFindings(t *testing.T) {
	f := newCommitFixture(t)
	ch := edit(t, f.paragraph(t, "We use the widget every day.", ""), "", "We leverage the widget every day.")
	outcomes, _, err := f.app.CommitCheck(f.command(t)).Check(t.Context(), []change.EditionChange{ch})
	require.NoError(t, err)
	require.Len(t, outcomes[0].After, 1)
	got := outcomes[0].After[0]
	assert.True(t, got.Suggested)
	assert.False(t, got.Fails)
	assert.Contains(t, got.Message, "Suggested rule")
}

// The commit check sees one edition, not its document, so the voice's
// required pattern, which holds over a whole page, is left to kapi check; the
// same blocks checked as a document report it.
func TestCommitCheck_LeavesDocumentRulesToKapiCheck(t *testing.T) {
	f := newCommitFixture(t)
	b := f.paragraph(t, "We use the widget every day.", "")
	ch := edit(t, b, "", "We use the widget each day.")
	outcomes, _, err := f.app.CommitCheck(f.command(t)).Check(t.Context(), []change.EditionChange{ch})
	require.NoError(t, err)
	for _, finding := range append(outcomes[0].Before, outcomes[0].After...) {
		assert.NotContains(t, finding.Message, "handbook", "a document rule ran at commit: %+v", finding)
	}

	in, err := f.app.resolveCommitProject(f.command(t))
	require.NoError(t, err)
	res, err := f.app.newCommitResolution(t.Context(), f.command(t), in)
	require.NoError(t, err)
	defer res.close()
	res.opts.editionsOnly = false
	diags, err := f.app.checkBlocks(t.Context(), res, filepath.Join(f.root, "docs", "guide.md"), []*model.Block{b})
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(diags, func(d check.Diagnostic) bool { return strings.Contains(d.Message, "handbook") }),
		"the document check reports the required pattern: %+v", diags)
}

// Several editions of one change set are checked together, each against its
// own analyzers, and each outcome is its own edition's.
func TestCommitCheck_ChecksEachEditionOfAChangeSet(t *testing.T) {
	f := newCommitFixture(t)
	b := f.paragraph(t, "We use the widget every day.", "Nous utilisons le gadget chaque jour.")
	source := edit(t, b, "", "We utilize the widget every day.")
	fr := edit(t, b, "fr", "Nous utilisons le truc chaque jour.")
	other := f.paragraph(t, "We use the widget each day.", "")
	other.ID, other.Name, other.Unit = "other", "other", ""
	typo := edit(t, other, "", "We use the widget each dya.")

	outcomes, _, err := f.app.CommitCheck(f.command(t)).Check(t.Context(), []change.EditionChange{source, fr, typo})
	require.NoError(t, err)
	require.Len(t, outcomes, 3)
	assert.Equal(t, []string{"terms.vocabulary"}, rules(change.Introduced(outcomes[0])))
	assert.Equal(t, []string{"terms.terminology"}, rules(change.Introduced(outcomes[1])))
	assert.Empty(t, change.Introduced(outcomes[2]))
	for i, o := range outcomes {
		for _, finding := range append(o.Before, o.After...) {
			assert.Equal(t, []change.EditionChange{source, fr, typo}[i].Ref, *finding.At)
		}
	}
}

// The fingerprint is the one the staleness gate recomputes for the document
// and the language: one edition's governance as it is, several in one
// fingerprint over theirs.
func TestCommitCheck_ReturnsTheGovernanceFingerprint(t *testing.T) {
	f := newCommitFixture(t)
	cmd := f.command(t)
	proj, err := project.Load(f.recipe)
	require.NoError(t, err)
	current, err := newContextFingerprints(f.app, cmd, proj, f.root)
	require.NoError(t, err)
	defer current.close()
	point := f.app.GovernancePointFor("", "docs/guide.md")
	wantEN, err := current.at(point, "en")
	require.NoError(t, err)
	wantFR, err := current.at(point, "fr")
	require.NoError(t, err)
	require.NotEmpty(t, wantEN.fingerprint)
	require.NotEmpty(t, wantFR.fingerprint)
	require.NotEqual(t, wantEN.fingerprint, wantFR.fingerprint)

	cc := f.app.CommitCheck(cmd)
	b := f.paragraph(t, "We use the widget every day.", "Nous utilisons le gadget chaque jour.")
	source := edit(t, b, "", "We use the widget each day.")
	fr := edit(t, b, "fr", "Nous utilisons le gadget tous les jours.")

	_, got, err := cc.Check(t.Context(), []change.EditionChange{source})
	require.NoError(t, err)
	assert.Equal(t, wantEN.fingerprint, got)
	_, got, err = cc.Check(t.Context(), []change.EditionChange{fr})
	require.NoError(t, err)
	assert.Equal(t, wantFR.fingerprint, got)

	both := []string{wantEN.fingerprint, wantFR.fingerprint}
	slices.Sort(both)
	_, got, err = cc.Check(t.Context(), []change.EditionChange{fr, source})
	require.NoError(t, err)
	assert.Equal(t, tool.OverlayConfigFingerprint(both...), got)
}

// Outside a project nothing governs the content: the edition meets hygiene
// alone, and there is no fingerprint.
func TestCommitCheck_OutsideAProjectHoldsHygieneAlone(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	t.Chdir(dir)
	b := &model.Block{ID: "p", Translatable: true, SourceLocale: "en",
		Source: []model.Run{{Text: &model.TextRun{Text: "We utilize the widget."}}}}
	ch := edit(t, b, "", " ")
	app := &App{SourceLang: "en"}
	defer app.Shutdown()
	outcomes, fingerprint, err := app.CommitCheck(NewEnvCommand(t.Context(), "apply")).Check(t.Context(), []change.EditionChange{ch})
	require.NoError(t, err)
	assert.Empty(t, fingerprint)
	assert.Equal(t, []string{"hygiene.empty"}, rules(change.Introduced(outcomes[0])))
	assert.Empty(t, outcomes[0].Before, "no terms govern the word outside a project")
}

// One App checks change sets for projects written in different languages, as
// Kapi Desktop and the MCP server do. Each project's content is read in its
// own language, whichever project the App checked before, so each is held to
// its own terms, and the English project's fingerprint is the one its
// staleness gate recomputes.
func TestCommitCheck_ReadsEachProjectInItsOwnLanguage(t *testing.T) {
	f := newCommitFixture(t)
	norsk := newNorskProject(t)

	// The fingerprint each project's staleness gate recomputes for its guide,
	// on an App that resolved that project's language.
	fingerprintOf := func(recipe string, source string) string {
		a := &App{SourceLang: source}
		a.InitRegistries()
		defer a.Shutdown()
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		current, err := newContextFingerprints(a, commitCommand(t, recipe), proj, filepath.Dir(recipe))
		require.NoError(t, err)
		defer current.close()
		g, err := current.at(a.GovernancePointFor("", "docs/guide.md"), source)
		require.NoError(t, err)
		require.NotEmpty(t, g.fingerprint)
		return g.fingerprint
	}
	want := map[string]string{f.recipe: fingerprintOf(f.recipe, "en"), norsk: fingerprintOf(norsk, "nb")}

	english := func(t *testing.T) change.EditionChange {
		return edit(t, f.paragraph(t, "We use the widget every day.", ""), "", "We utilize the widget every day.")
	}
	norwegian := func(t *testing.T) change.EditionChange {
		return edit(t, norskBlock("Vi bruker widgeten hver dag.", ""), "", "Vi vil benytte widgeten hver dag.")
	}
	translation := func(t *testing.T) change.EditionChange {
		return edit(t, norskBlock("Velg knapp nummer to.", "Choose button number two."), "en", "Choose item number two.")
	}

	app := &App{}
	app.InitRegistries()
	t.Cleanup(app.Shutdown)
	steps := []struct {
		name   string
		recipe string
		change func(*testing.T) change.EditionChange
		// introduced is the rule of the failing finding the edit introduces.
		introduced string
		// source says the edit is to a source-language edition, whose
		// fingerprint is the guide's in the project's language.
		source bool
	}{
		{"a Norwegian edit meets the Norwegian terms", norsk, norwegian, "terms.vocabulary", true},
		{"an English edit meets the English terms after a Norwegian check", f.recipe, english, "terms.vocabulary", true},
		{"a translation of Norwegian meets the rules derived from Norwegian", norsk, translation, "terms.terminology", false},
		{"an English edit meets the English terms after a Norwegian translation", f.recipe, english, "terms.vocabulary", true},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			outcomes, fingerprint, err := app.CommitCheck(commitCommand(t, step.recipe)).Check(t.Context(), []change.EditionChange{step.change(t)})
			require.NoError(t, err)
			assert.Equal(t, []string{step.introduced}, rules(change.Introduced(outcomes[0])), "%+v", outcomes[0])
			if step.source {
				assert.Equal(t, want[step.recipe], fingerprint, "the fingerprint of the project's source-language governance")
			}
		})
	}
	assert.Empty(t, app.SourceLang, "a commit check writes no source language on the App")
}

// Change sets for two projects checked at once on one App are each read in
// their own project's language. Run under -race, the calls share no state.
func TestCommitCheck_ChecksTwoProjectsAtOnce(t *testing.T) {
	f := newCommitFixture(t)
	norsk := newNorskProject(t)
	app := &App{}
	app.InitRegistries()
	t.Cleanup(app.Shutdown)

	type call struct {
		recipe string
		change change.EditionChange
	}
	var calls []call
	for range 4 {
		calls = append(calls,
			call{f.recipe, edit(t, f.paragraph(t, "We use the widget every day.", ""), "", "We utilize the widget every day.")},
			call{norsk, edit(t, norskBlock("Vi bruker widgeten hver dag.", ""), "", "Vi vil benytte widgeten hver dag.")})
	}
	introduced := make([][]string, len(calls))
	errs := make([]error, len(calls))
	var wg sync.WaitGroup
	for i, c := range calls {
		cmd := commitCommand(t, c.recipe)
		wg.Go(func() {
			outcomes, _, err := app.CommitCheck(cmd).Check(t.Context(), []change.EditionChange{c.change})
			errs[i] = err
			if err == nil {
				introduced[i] = rules(change.Introduced(outcomes[0]))
			}
		})
	}
	wg.Wait()
	for i := range calls {
		require.NoError(t, errs[i])
		assert.Equal(t, []string{"terms.vocabulary"}, introduced[i], "call %d, %s", i, calls[i].recipe)
	}
}

// Deleting a block removes its editions with it, so there is nothing after
// the change to hold to the rules, and the deletion introduces nothing, even
// of a block whose wording broke one. The service says an edition was removed
// with an AfterRev of absent; a change that carries no revisions says it with
// no block after the change.
func TestCommitCheck_DeletingABlockIntroducesNothing(t *testing.T) {
	f := newCommitFixture(t)
	b := f.paragraph(t, "We utilize the widget every day.", "Nous utilisons le truc.")
	fr := model.EditionKey{Locale: "fr"}
	srcRef := change.Ref{Doc: "docs/guide.md", Block: blockKey(b)}
	frRef := change.Ref{Doc: "docs/guide.md", Block: blockKey(b), Edition: fr}
	src, _ := b.Edition(model.EditionKey{})
	tgt, _ := b.Edition(fr)
	cases := []struct {
		name    string
		changes []change.EditionChange
	}{
		{"as the service reports it", []change.EditionChange{
			{Ref: srcRef, Role: change.RoleAuthoritative, Before: src.Runs, BeforeRev: model.EditionRevision(b, model.EditionKey{}),
				AfterRev: model.AbsentRevision, Block: b},
			{Ref: frRef, Role: change.RoleDerived, Before: tgt.Runs, BeforeRev: model.EditionRevision(b, fr),
				AfterRev: model.AbsentRevision, Block: b},
		}},
		{"with no revisions and no block after", []change.EditionChange{
			{Ref: srcRef, Role: change.RoleAuthoritative, Before: src.Runs},
			{Ref: frRef, Role: change.RoleDerived, Before: tgt.Runs},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outcomes, _, err := f.app.CommitCheck(f.command(t)).Check(t.Context(), tc.changes)
			require.NoError(t, err)
			require.Len(t, outcomes, 2)
			assert.Equal(t, []string{"terms.vocabulary"}, rules(outcomes[0].Before), "the source broke a rule before it was deleted")
			assert.Equal(t, []string{"terms.terminology"}, rules(outcomes[1].Before), "the translation broke a rule before it was deleted")
			for _, o := range outcomes {
				assert.Empty(t, o.After)
				assert.Empty(t, change.Introduced(o))
			}
		})
	}
}

// A change set that edits a source and its translation together holds the
// translation, before the change, to the source it was written against. A
// rule the new source demands and the new translation misses is introduced by
// the change set, although the old translation lacked the rendering too: the
// old source never asked for it.
func TestCommitCheck_HoldsATranslationToTheSourceItWasWrittenAgainst(t *testing.T) {
	f := newCommitFixture(t)
	b := f.paragraph(t, "We use the tool every day.", "Nous utilisons l'outil chaque jour.")
	source := edit(t, b, "", "We use the widget every day.")
	fr := edit(t, b, "fr", "Nous utilisons le truc chaque jour.")
	outcomes, _, err := f.app.CommitCheck(f.command(t)).Check(t.Context(), []change.EditionChange{source, fr})
	require.NoError(t, err)
	require.Len(t, outcomes, 2)
	assert.Empty(t, change.Introduced(outcomes[0]))
	assert.Empty(t, outcomes[1].Before, "the old translation met the rules of the old source")
	assert.Equal(t, []string{"terms.terminology"}, rules(change.Introduced(outcomes[1])))
}

// The fingerprint is taken under the governance the editions were checked
// under: the project's own voice store, whatever store the command names for
// `kapi voice`.
func TestCommitCheck_FingerprintsUnderTheProjectVoiceStore(t *testing.T) {
	f := newCommitFixture(t)
	proj, err := project.Load(f.recipe)
	require.NoError(t, err)
	current, err := newContextFingerprints(f.app, f.command(t), proj, f.root)
	require.NoError(t, err)
	want, err := current.at(f.app.GovernancePointFor("", "docs/guide.md"), "en")
	current.close()
	require.NoError(t, err)
	require.NotEmpty(t, want.profileID, "the project's voice governs the guide")

	cmd := f.command(t)
	cmd.Flags().String("file", "", "")
	require.NoError(t, cmd.Flags().Set("file", filepath.Join(t.TempDir(), "voice.db")))
	ch := edit(t, f.paragraph(t, "We use the widget every day.", ""), "", "We use the widget each day.")
	_, got, err := f.app.CommitCheck(cmd).Check(t.Context(), []change.EditionChange{ch})
	require.NoError(t, err)
	assert.Equal(t, want.fingerprint, got)
}

// channelRecipe binds the landing profile's voice to the web collection; the
// rest of the project sits at the project's own voice.
const channelRecipe = `version: v1
id: %s
name: channels
defaults:
  source_language: en
profiles:
  landing:
    channels: [web]
collections:
  - name: web
    channel: landing/web
    content:
      - path: "web/*.md"
`

// A file the project's ignore rules leave out sits at the project's default
// point, which is where the analyzers hold its edits, and its fingerprint is
// the governance there rather than at the collection that would claim it.
func TestCommitCheck_FingerprintsAnIgnoredFileAtTheDefaultPoint(t *testing.T) {
	isolateCheckExecution(t)
	root := t.TempDir()
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(strings.Replace(channelRecipe, "%s", projectIDFor("channels"+t.Name()), 1)), 0o644))
	require.NoError(t, os.WriteFile(layoutVoicePath(t, root), []byte("name: House\ntone:\n  formality: neutral\n"), 0o644))
	require.NoError(t, os.WriteFile(layoutVoicePath(t, root, "landing"), []byte(commitVoice), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapiignore"), []byte("web/drafts.md\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "web", "drafts.md"), []byte("We use the widget.\n"), 0o644))
	readProjectContext(t, root)

	app := &App{}
	app.InitRegistries()
	t.Cleanup(app.Shutdown)
	cmd := commitCommand(t, recipe)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	current, err := newContextFingerprints(app, cmd, proj, root)
	require.NoError(t, err)
	atDefault, err := current.at(app.GovernancePointFor("", ""), "en")
	require.NoError(t, err)
	atCollection, err := current.at(app.GovernancePointFor("", "web/drafts.md"), "en")
	require.NoError(t, err)
	current.close()
	require.NotEqual(t, atDefault.fingerprint, atCollection.fingerprint, "the collection's voice differs from the project's")

	b := &model.Block{ID: "p1", Translatable: true, SourceLocale: "en", Source: []model.Run{{Text: &model.TextRun{Text: "We use the widget."}}}}
	ch := edit(t, b, "", "We simply use the widget.")
	ch.Ref.Doc = "web/drafts.md"
	outcomes, got, err := app.CommitCheck(cmd).Check(t.Context(), []change.EditionChange{ch})
	require.NoError(t, err)
	assert.Equal(t, atDefault.fingerprint, got)
	assert.Empty(t, change.Introduced(outcomes[0]), "the landing voice's pattern holds at the collection, not at the default point")
}
