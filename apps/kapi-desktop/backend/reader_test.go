package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
)

// readerFinding is the first finding of the read whose label is label.
func readerFinding(t *testing.T, r *ReaderFileDTO, label string) ReaderFindingDTO {
	t.Helper()
	for _, b := range r.Blocks {
		for _, f := range b.Findings {
			if f.Label == label {
				return f
			}
		}
	}
	require.FailNowf(t, "no finding", "the Reader holds no finding labelled %q", label)
	return ReaderFindingDTO{}
}

// cachedBlocks are the block cache's translatable blocks for one file, by id,
// as the Map counts them.
func cachedBlocks(t *testing.T, app *App, tabID, rel string) map[string]string {
	t.Helper()
	db, ok := app.existingProjectStore(app.getOpenProject(tabID))
	require.True(t, ok, "the store sync wrote the block cache")
	sess, err := db.BlocksAutocommit().Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()
	translatable := true
	out := map[string]string{}
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Translatable: &translatable}) {
		require.NoError(t, berr)
		if filepath.ToSlash(b.Properties.File) == rel {
			out[b.ID] = model.RunsText(b.SourceRuns())
		}
	}
	return out
}

// TestReaderFile_KapiMart: the Reader of a sample file answers what the other
// surfaces answer for it. Its blocks are the block cache's for the file, its
// findings a checks run over that file alone, and its rules the context
// answer for it, in that answer's order.
func TestReaderFile_KapiMart(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	app := NewApp()
	t.Cleanup(func() { app.shutdownEngine() })
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))
	tab, err := app.OpenProject(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	_, err = app.SyncProjectStore(tab.ID)
	require.NoError(t, err)

	summary, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	res, err := app.RunChecks(tab.ID, ProjectFilter{Languages: summary.TargetLanguages})
	require.NoError(t, err)
	byPath := map[string]int{}
	for _, f := range res.Files {
		byPath[f.Path] += len(f.Findings)
	}
	// The file with the most findings, so the comparison has findings to make.
	summary, err = app.MapSummary(tab.ID)
	require.NoError(t, err)
	require.NotEmpty(t, summary.FileRows)
	row := summary.FileRows[0]
	require.Positive(t, row.Findings, "the sample holds findings")

	got, err := app.ReaderFile(tab.ID, row.Relative)
	require.NoError(t, err)
	assert.Equal(t, row.Relative, got.Path)
	assert.Equal(t, row.Path, got.Abs)

	// Blocks: the block cache's for the file, in the order the file holds them.
	cache := cachedBlocks(t, app, tab.ID, row.Relative)
	require.NotEmpty(t, cache)
	read := map[string]string{}
	for _, b := range got.Blocks {
		read[b.ID] = model.RunsText(b.Runs)
	}
	assert.Equal(t, cache, read)

	// Findings: a checks run over this one file, in every target language,
	// which is what the whole-project run reports for it too.
	var one []*model.Block
	single, err := app.runChecks(tab.ID, ProjectFilter{Languages: summary.TargetLanguages}, checkScope{file: row.Relative, blocks: &one})
	require.NoError(t, err)
	require.Len(t, single.Files, 1)
	want := single.Files[0].Findings
	assert.Equal(t, len(want), got.Findings)
	assert.Equal(t, byPath[row.Path], got.Findings, "the Map's count for the file")
	placed := len(got.Document)
	fixable := 0
	type key struct{ block, label string }
	wantKeys := map[key]int{}
	for _, f := range want {
		wantKeys[key{f.BlockID, readerLabel(f)}]++
		if f.Fix != "" {
			fixable++
		}
	}
	gotKeys := map[key]int{}
	for _, b := range got.Blocks {
		placed += len(b.Findings)
		for _, f := range b.Findings {
			assert.Equal(t, b.ID, f.BlockID)
			gotKeys[key{f.BlockID, f.Label}]++
		}
	}
	for _, f := range got.Document {
		gotKeys[key{f.BlockID, f.Label}]++
	}
	assert.Equal(t, got.Findings, placed, "every finding is drawn once")
	assert.Equal(t, wantKeys, gotKeys)
	assert.Equal(t, fixable, got.Fixable)

	// Rules: the context answer for the file, row for row in its order.
	answer, err := app.ContextAt(tab.ID, row.Relative, 0)
	require.NoError(t, err)
	rules, total := readerRules(answer)
	assert.Equal(t, rules, got.Rules)
	assert.Equal(t, total, got.RulesTotal)
	require.NotEmpty(t, got.Rules, "the sample's context holds rules")
	i := 0
	if answer.VoiceBrief != "" {
		assert.Equal(t, "tone", got.Rules[0].Kind)
		assert.Equal(t, strings.TrimSpace(answer.VoiceBrief), got.Rules[0].Rule)
		i = 1
	}
	for _, r := range answer.Rules {
		require.Less(t, i, len(got.Rules))
		assert.Equal(t, strings.Join(r.From, ", "), got.Rules[i].From)
		if r.Say != "" {
			assert.True(t, strings.HasPrefix(got.Rules[i].Rule, r.Say), "%q states %q", got.Rules[i].Rule, r.Say)
		}
		i++
	}

	// A narrowed run is never the one the Map counts.
	after, err := app.MapSummary(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, *summary.Findings, *after.Findings)
}

// TestFixFinding_TermRule: a finding on a term rule with a replacement is
// fixed through the change service. The words change and every other byte of
// the file stays as it was, and the finding is gone from the file read again.
func TestFixFinding_TermRule(t *testing.T) {
	app := NewApp()
	source := "{\n    \"greeting\":   \"Please utilize the dashboard\",\n  \"other\": \"Keep  this\"\n}\n"
	tabID, srcPath := setupCheckProject(t, app, source)

	before, err := app.ReaderFile(tabID, "locales/en.json")
	require.NoError(t, err)
	assert.Equal(t, "failed", before.Verdict)
	assert.Equal(t, 1, before.Fixable)
	f := readerFinding(t, before, "use, not utilize")
	assert.True(t, f.Fixable)
	assert.True(t, f.Fails)
	assert.Equal(t, "word", f.Kind)
	require.NotNil(t, f.Span)
	assert.Equal(t, ReaderSpan{Start: 7, End: 14}, *f.Span, "the span covers the word")

	again, err := app.ReaderFile(tabID, "locales/en.json")
	require.NoError(t, err)
	assert.Equal(t, f.ID, readerFinding(t, again, "use, not utilize").ID, "a finding keeps its id between reads")
	byAbs, err := app.ReaderFile(tabID, srcPath)
	require.NoError(t, err, "the Checks page names the file by its absolute path")
	assert.Equal(t, "locales/en.json", byAbs.Path)

	after, err := app.FixFinding(tabID, "locales/en.json", f.ID)
	require.NoError(t, err)
	assert.Empty(t, after.Refused)
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, strings.Replace(source, "utilize", "use", 1), string(data))
	assert.Zero(t, after.Findings, "the check read again finds nothing")
	assert.Equal(t, "passed", after.Verdict)
	require.NotEmpty(t, after.Blocks)
	var texts []string
	for _, b := range after.Blocks {
		texts = append(texts, model.RunsText(b.Runs))
	}
	assert.Contains(t, texts, "Please use the dashboard", "the block is drawn from the file as it now stands")

	gone, err := app.FixFinding(tabID, "locales/en.json", f.ID)
	require.NoError(t, err)
	require.Len(t, gone.Refused, 1)
	assert.Equal(t, f.ID, gone.Refused[0].FindingID)
}

// refusingVoiceYAML asks for a word the voice's own pattern forbids, so the
// fix the term rule carries is one the commit check refuses.
const refusingVoiceYAML = `id: house
name: House Style
style:
  prohibited_patterns:
    - regex: '(?i)\bleverage\b'
      description: Write use
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: leverage
`

// TestFixFinding_RefusedIsReported: a fix the change service refuses is
// reported with the service's reason and changes nothing.
func TestFixFinding_RefusedIsReported(t *testing.T) {
	// One workspace for the context the project reads in and for the
	// desktop's engine, so the commit check holds the edit to the same rules.
	wsRoot := t.TempDir()
	app := NewApp()
	app.hostEngine().SetWorkspaceRoot(wsRoot)
	source := `{"greeting":"Please utilize the dashboard"}`
	tabID, srcPath := gatedProject(t, app, wsRoot, source, refusingVoiceYAML)

	before, err := app.ReaderFile(tabID, "locales/en.json")
	require.NoError(t, err)
	f := readerFinding(t, before, "leverage, not utilize")
	require.True(t, f.Fixable)
	after, err := app.FixFinding(tabID, "locales/en.json", f.ID)
	require.NoError(t, err)
	require.Len(t, after.Refused, 1)
	assert.Equal(t, f.ID, after.Refused[0].FindingID)
	assert.Contains(t, after.Refused[0].Reason, "introduces 1 failing finding", "the service's own reason")
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, source, string(data), "a refused fix writes nothing")
	assert.Equal(t, f.ID, readerFinding(t, after, "leverage, not utilize").ID, "the finding still stands")

	file, err := app.FixFile(tabID, "locales/en.json")
	require.NoError(t, err)
	require.Len(t, file.Refused, 1)
	assert.Equal(t, f.ID, file.Refused[0].FindingID)
	data, err = os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, source, string(data))
}

// TestFixFile_AppliesEveryFix: Fix in this file applies every fix as one
// change set, and the file read again holds none of the findings.
func TestFixFile_AppliesEveryFix(t *testing.T) {
	app := NewApp()
	source := `{"a":"Please utilize the dashboard","b":"We utilize it daily"}`
	tabID, srcPath := setupCheckProject(t, app, source)

	before, err := app.ReaderFile(tabID, "locales/en.json")
	require.NoError(t, err)
	assert.Equal(t, 2, before.Fixable)

	after, err := app.FixFile(tabID, "locales/en.json")
	require.NoError(t, err)
	assert.Empty(t, after.Refused)
	assert.Zero(t, after.Findings)
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, `{"a":"Please use the dashboard","b":"We use it daily"}`, string(data))
}

// TestFixFile_UpdatesTheMapsRun: a fix replaces the file's entry in the last
// whole-project run, so the Map's counts and the Checks page show what the
// file holds after it, without a second run over every file.
func TestFixFile_UpdatesTheMapsRun(t *testing.T) {
	wsRoot := t.TempDir()
	app := NewApp()
	app.hostEngine().SetWorkspaceRoot(wsRoot)
	tabID, srcPath := gatedProject(t, app, wsRoot, `{"a":"Please utilize the dashboard","b":"We utilize it daily"}`, houseVoiceYAML)

	whole, err := app.RunChecks(tabID, ProjectFilter{})
	require.NoError(t, err)
	assert.Equal(t, "failed", whole.Verdict)
	before, err := app.MapSummary(tabID)
	require.NoError(t, err)
	require.NotNil(t, before.Findings)
	assert.Equal(t, 2, *before.Findings)

	_, err = app.FixFile(tabID, "locales/en.json")
	require.NoError(t, err)

	after, err := app.MapSummary(tabID)
	require.NoError(t, err)
	require.NotNil(t, after.Findings)
	assert.Zero(t, *after.Findings)
	require.Len(t, after.FileRows, 1)
	assert.Zero(t, after.FileRows[0].Findings)
	last, err := app.LastProjectChecks(tabID)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, "passed", last.Verdict)
	assert.True(t, last.Pass)
	assert.Equal(t, 100, last.Score)
	require.Len(t, last.Files, 1)
	assert.Equal(t, srcPath, last.Files[0].Path)
	assert.Empty(t, last.Files[0].Findings)
}

// partialVoiceYAML asks for two replacements, one of which its own pattern
// forbids, so a Fix all holds one fix the commit check refuses and one it
// lets through.
const partialVoiceYAML = `id: house
name: House Style
style:
  prohibited_patterns:
    - regex: '(?i)\bleverage\b'
      description: Write use
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: leverage
    - term: whilst
      replacement: while
`

// TestFixFile_PartialRefusal: one fix the service refuses is set aside with
// its reason, and the rest are sent again without it and land.
func TestFixFile_PartialRefusal(t *testing.T) {
	wsRoot := t.TempDir()
	app := NewApp()
	app.hostEngine().SetWorkspaceRoot(wsRoot)
	tabID, srcPath := gatedProject(t, app, wsRoot, `{"a":"Please utilize the dashboard","b":"Read it whilst you wait"}`, partialVoiceYAML)

	before, err := app.ReaderFile(tabID, "locales/en.json")
	require.NoError(t, err)
	require.Equal(t, 2, before.Fixable)
	refusedID := readerFinding(t, before, "leverage, not utilize").ID

	after, err := app.FixFile(tabID, "locales/en.json")
	require.NoError(t, err)
	require.Len(t, after.Refused, 1)
	assert.Equal(t, refusedID, after.Refused[0].FindingID)
	assert.Contains(t, after.Refused[0].Reason, "introduces 1 failing finding")
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, `{"a":"Please utilize the dashboard","b":"Read it while you wait"}`, string(data))
	assert.Equal(t, refusedID, readerFinding(t, after, "leverage, not utilize").ID, "the refused finding still stands")
	assert.Equal(t, 1, after.Findings)
}

// TestReaderFile_RefusesACommentsOnlyFile: a file read for its comments alone
// holds no block the Reader draws, so the Reader refuses it and the Checks
// page shows it in the preview.
func TestReaderFile_RefusesACommentsOnlyFile(t *testing.T) {
	isolateCheckPlugins(t)
	app := NewApp()
	tabID, _ := commentsOnlyTab(t, app, "{only: true}")
	_, err := app.ReaderFile(tabID, "config/app.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "comments alone")
	_, err = app.ReaderFile(tabID, "locales/en.json")
	require.NoError(t, err, "the project's other files read")
}

// gatedProject opens a project with one JSON file under voiceYAML, its context
// read into the workspace at wsRoot the app's engine reads, so the commit
// check holds an edit to the same rules the Reader reports.
func gatedProject(t *testing.T, app *App, wsRoot, source, voiceYAML string) (tabID, srcPath string) {
	t.Helper()
	dir := t.TempDir()
	srcPath = filepath.Join(dir, "locales", "en.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(srcPath), 0o755))
	require.NoError(t, os.WriteFile(srcPath, []byte(source), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, project.RelStatePath("voice.yaml")), []byte(voiceYAML), 0o644))
	recipe := filepath.Join(dir, "proj.kapi")
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version:     project.CurrentVersion,
		Name:        "Reader",
		Defaults:    project.Defaults{SourceLanguage: "en"},
		Collections: []project.Collection{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
	}))
	importer := &host.App{}
	importer.InitRegistries()
	importer.SetWorkspaceRoot(wsRoot)
	_, err := importer.ImportProjectContext(t.Context(), recipe, host.ContextImportRequest{})
	require.NoError(t, err)
	importer.Shutdown()
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID, srcPath
}

// TestReaderFile_RefusesATranslation: the Reader reads and fixes the source.
// A translation the recipe keeps is refused by name, before anything runs.
func TestReaderFile_RefusesATranslation(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "locales"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "locales", "en.json"), []byte(`{"greeting":"Please utilize the dashboard"}`), 0o644))
	fr := filepath.Join(dir, "locales", "fr.json")
	frSource := `{"greeting":"Veuillez utilize le tableau"}`
	require.NoError(t, os.WriteFile(fr, []byte(frSource), 0o644))
	recipe := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version:     project.CurrentVersion,
		Defaults:    project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []project.Collection{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
	}))
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	tabID := tab.ID
	_, err = app.ReaderFile(tabID, "locales/en.json")
	require.NoError(t, err, "the source reads")

	_, err = app.ReaderFile(tabID, "locales/fr.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "translation of locales/en.json")

	_, err = app.FixFile(tabID, "locales/fr.json")
	require.Error(t, err)
	_, err = app.FixFinding(tabID, "locales/fr.json", "any")
	require.Error(t, err)
	data, err := os.ReadFile(fr)
	require.NoError(t, err)
	assert.Equal(t, frSource, string(data), "the translation is untouched")

	_, err = app.ReaderFile(tabID, "../outside.json")
	require.Error(t, err)
	_, err = app.ReaderFile(tabID, "locales/missing.json")
	require.Error(t, err)
}

// TestReaderRules_Order: the margin keeps the order `kapi context <path>`
// prints the answer in, and states only what the answer carries.
func TestReaderRules_Order(t *testing.T) {
	rules, total := readerRules(nil)
	assert.Empty(t, rules)
	assert.Zero(t, total)

	answer := &host.ContextAnswer{
		VoiceBrief: "Plain and direct.\n",
		Voice:      &host.ContextVoice{Name: "house"},
		Rules: []host.ContextRule{
			{Say: "Sign in", Not: []string{"Login"}, From: []string{"terms"}},
			{Say: "KapiMart", Not: []string{"Kapi Mart"}, From: []string{"terms", "pack technical-docs"}},
			{Not: []string{"utilize"}, From: []string{"workspace"}},
		},
		RulesTotal: 4,
		Elsewhere:  []host.ContextElsewhere{{Keep: []string{"Basket"}, Instead: "Cart", HeldIn: []string{"help/"}}},
		EstablishedNotes: []host.ContextSuggestion{
			{Kind: "note", Status: "established", Text: "Address the reader as you.", SuggestedBy: "ada", At: "2026-10-20T09:00:00Z"},
		},
		Suggestions: []host.ContextSuggestion{
			// Stated already as a rule: left out, as the text answer leaves it out.
			{Kind: "term", Status: "suggested", Term: "Login", Replacement: "Sign in", SuggestedBy: "claude-code"},
			{Kind: "term", Status: "contested", Term: "e-mail", Replacement: "email", SuggestedBy: "codex", At: "2026-10-21T10:00:00Z"},
		},
	}
	rules, total = readerRules(answer)
	assert.Equal(t, []ReaderRuleDTO{
		{Kind: "tone", Rule: "Plain and direct.", Status: "established", From: "voice house"},
		{Kind: "word", Rule: "Sign in, not Login", Status: "established", From: "terms"},
		{Kind: "name", Rule: "KapiMart, not Kapi Mart", Status: "established", From: "terms, pack technical-docs"},
		{Kind: "word", Rule: "Avoid utilize", Status: "established", From: "workspace"},
		{Kind: "keep", Rule: "Basket, not Cart", Status: "established", From: "help/"},
		{Kind: "note", Rule: "Address the reader as you.", Status: "established", By: "ada", At: "2026-10-20T09:00:00Z"},
		{Kind: "name", Rule: "email, not e-mail", Status: "contested", By: "codex", At: "2026-10-21T10:00:00Z"},
	}, rules)
	assert.Equal(t, 8, total, "the rules the answer capped count toward the total")
}
