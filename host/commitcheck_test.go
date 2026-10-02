package host

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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
func (f commitFixture) command(t *testing.T) Command {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "apply")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", f.recipe))
	return cmd
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

	res, err := f.app.newCommitResolution(t.Context(), f.command(t))
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
