package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repairPairedFixture writes the evaluator's reference outputs into dir.
func repairPairedFixture(t *testing.T, dir string, task PairedTask) {
	t.Helper()
	prefix := "testdata/paired/references/" + task.ID
	err := fs.WalkDir(pairedFixtures, prefix, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		body, err := pairedFixtures.ReadFile(name)
		require.NoError(t, err)
		target := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(name, prefix+"/")))
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
		return os.WriteFile(target, body, 0o600)
	})
	require.NoError(t, err)
}

func pairedTaskByID(t *testing.T, id string) PairedTask {
	t.Helper()
	task, err := findPairedTask(id)
	require.NoError(t, err)
	return task
}

// pairedLanded is a transcript in which the other editor's change landed.
func pairedLanded() *PairedAgentResult {
	return &PairedAgentResult{Interference: &PairedInterferenceRecord{Triggered: true, Applied: true}}
}

// Every task fails when nothing changes and passes on its reference output.
func TestPairedIndependentValidation(t *testing.T) {
	for _, task := range pairedTasks() {
		t.Run(task.ID, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			clean := &PairedAgentResult{}
			if task.spec.Interference != nil {
				// The reference holds the other editor's change.
				clean = pairedLanded()
			}
			initial, err := validatePairedTask(dir, task, clean)
			require.NoError(t, err)
			assert.False(t, initial.ObjectivePassed, "doing nothing is not completion")
			repairPairedFixture(t, dir, task)
			repaired, err := validatePairedTask(dir, task, clean)
			require.NoError(t, err)
			assert.True(t, repaired.ObjectivePassed, "%+v", repaired.Criteria)
			for _, criterion := range repaired.Criteria {
				assert.True(t, criterion.Passed, "%s: %s", criterion.ID, criterion.Detail)
			}
			assert.True(t, repaired.HumanReviewRequired)
			assert.Equal(t, "pending", repaired.HumanReviewStatus)
			assert.NotEmpty(t, repaired.HumanReviewRubric)
		})
	}
}

// Each grader rejects the plausible wrong answers its task invites.
func TestPairedGradersRejectFaults(t *testing.T) {
	for _, tc := range []struct {
		task, name, path, find, replace string
		failing                         string
	}{
		{task: "edit-link-html-md", name: "naive URL replace reaches the FAQ link", path: "docs/help.md",
			find: "https://harbor.example/help/faq", replace: "https://harbor.example/support/contact/faq", failing: "markdown-bytes"},
		{task: "edit-link-html-md", name: "code block rewritten", path: "docs/help.md",
			find: "help_url: https://harbor.example/help\n", replace: "help_url: https://harbor.example/support/contact\n", failing: "markdown-bytes"},
		{task: "edit-plural-branch", name: "other case also changed", path: "lib/l10n/app_en.arb",
			find: "other{{count} new messages}", replace: "other{{count} unread messages}", failing: "other-branches"},
		{task: "edit-plural-branch", name: "the zero case also changed", path: "lib/l10n/app_en.arb",
			find: "=0{No new messages}", replace: "=0{No unread messages}", failing: "other-branches"},
		{task: "edit-plural-branch", name: "the plural flattened to the edited text", path: "lib/l10n/app_en.arb",
			find:    "\"{count, plural, =0{No new messages} one{{count} unread message} other{{count} new messages}}\"",
			replace: "\"{count} unread message\"", failing: "edited-branch"},
		{task: "edit-plural-branch", name: "the branch keeps the placeholder as #", path: "lib/l10n/app_en.arb",
			find: "one{{count} unread message}", replace: "one{# unread message}", failing: "edited-branch"},
		{task: "edit-plural-branch", name: "metadata rewritten", path: "lib/l10n/app_en.arb",
			find: "        \"type\": \"int\",\n        \"format\": \"compact\"\n", replace: "        \"type\": \"int\", \"format\": \"compact\"\n", failing: "metadata"},
		{task: "edit-plural-branch", name: "another plural changed", path: "lib/l10n/app_en.arb",
			find: "{count} appointment this week", replace: "{count} appointment", failing: "plural-bytes"},
		{task: "edit-po-context", name: "library entry also changed", path: "locales/nb/messages.po",
			find: "msgctxt \"library\"\nmsgid \"Book\"\nmsgstr \"Bok\"", replace: "msgctxt \"library\"\nmsgid \"Book\"\nmsgstr \"Bestill\"", failing: "po-bytes"},
		{task: "add-json-key", name: "key appended at the end of the object", path: "locales/en.json",
			find: "    \"exportData\": \"Export your data\",\n", replace: "", failing: "keys-in-order"},
		{task: "recover-stale-read", name: "the other editor's change was overwritten", path: "docs/en/upgrade.md",
			find: "keeps all your appointments", replace: "keeps your appointments", failing: "both-changes"},
		{task: "recover-gate-refusal", name: "the forbidden term", path: "docs/en/reports.md",
			find: "from the overview page.", replace: "from the portal.", failing: "sentence-added"},
		{task: "recover-gate-refusal", name: "no CSV", path: "docs/en/reports.md",
			find: "as a CSV file from", replace: "as a file from", failing: "sentence-added"},
		{task: "recover-gate-refusal", name: "the sentence as a paragraph of its own", path: "docs/en/reports.md",
			find: "page within an hour. You can", replace: "page within an hour.\n\nYou can", failing: "rest-unchanged"},
		{task: "add-edition-markup", name: "a link address translated", path: "docs/nb/welcome.md",
			find: "https://harbor.example/app", replace: "https://harbor.example/nb/app", failing: "markup-kept"},
		{task: "add-edition-markup", name: "inline code translated", path: "docs/nb/welcome.md",
			find: "`HB-2041`", replace: "`HB-2041-nb`", failing: "markup-kept"},
		{task: "add-edition-markup", name: "a block left in English", path: "docs/nb/welcome.md",
			find:    "Installer appen og logg inn med nummeret på lånekortet ditt.",
			replace: "Install the app and sign in with your library card number.", failing: "translated"},
	} {
		t.Run(tc.task+"/"+tc.name, func(t *testing.T) {
			task := pairedTaskByID(t, tc.task)
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			repairPairedFixture(t, dir, task)
			file := filepath.Join(dir, filepath.FromSlash(tc.path))
			body, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Contains(t, string(body), tc.find)
			require.NoError(t, os.WriteFile(file, []byte(strings.Replace(string(body), tc.find, tc.replace, 1)), 0o600))
			observed := &PairedAgentResult{}
			if task.spec.Interference != nil {
				observed = pairedLanded()
			}
			result, err := validatePairedTask(dir, task, observed)
			require.NoError(t, err)
			assert.False(t, result.ObjectivePassed)
			for _, criterion := range result.Criteria {
				if criterion.ID == tc.failing {
					assert.False(t, criterion.Passed, "%s should fail", tc.failing)
					assert.NotEmpty(t, criterion.Detail)
				}
			}
		})
	}
}

// The stale task is graded against what happened in the session: with the
// other editor's change when it landed, without it when it never did.
func TestPairedStaleGradeFollowsTheInterference(t *testing.T) {
	task := pairedTaskByID(t, "recover-stale-read")
	files, err := pairedTaskFiles(task)
	require.NoError(t, err)
	original := string(files["docs/en/upgrade.md"])
	agent := strings.Replace(original, "Back up your data before", "Back up your data and settings before", 1)
	both := strings.Replace(agent, "keeps your appointments", "keeps all your appointments", 1)
	other := strings.Replace(original, "keeps your appointments", "keeps all your appointments", 1)
	never := &PairedAgentResult{Interference: &PairedInterferenceRecord{}}
	landed := pairedLanded()
	afterWrite := &PairedAgentResult{Interference: &PairedInterferenceRecord{Triggered: true, Applied: true, AgentWroteFirst: true}}
	for _, tc := range []struct {
		name     string
		body     string
		observed *PairedAgentResult
		passed   bool
	}{
		{"right: the change never landed and the agent's edit is alone", agent, never, true},
		{"right: the change landed and both are kept", both, landed, true},
		{"right: the change landed after the agent wrote", both, afterWrite, true},
		{"wrong: the change landed and the agent overwrote it", agent, landed, false},
		{"wrong: the agent's edit is missing", other, landed, false},
		{"wrong: nothing changed", original, never, false},
		{"wrong: the agent made the other editor's change itself", both, never, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "en", "upgrade.md"), []byte(tc.body), 0o600))
			result, err := validatePairedTask(dir, task, tc.observed)
			require.NoError(t, err)
			assert.Equal(t, tc.passed, result.ObjectivePassed, "%+v", result.Criteria)
		})
	}
}

// Danish and Swedish share most of Bokmål's function words; the language
// check passes Bokmål and neither neighbour.
func TestPairedBokmalCheckRejectsDanishAndSwedish(t *testing.T) {
	reference, err := pairedFixtures.ReadFile("testdata/paired/references/add-edition-markup/docs/nb/welcome.md")
	require.NoError(t, err)
	reworded := strings.NewReplacer(
		"setter deg i kontakt med", "kobler deg til",
		"Du trenger en", "Du må ha en",
		"Ha timekoden klar", "Ha koden for timen klar",
	).Replace(string(reference))
	danish := "# Velkommen til Harbor Help\n\n" +
		"Harbor Help forbinder dig med en rådgiver via **videoaftale**. Du skal bruge en\n" +
		"enhed med kamera og [Harbor Help-appen](https://harbor.example/app).\n\n" +
		"## Før din første aftale\n\n" +
		"1. Installer appen, og log ind med dit lånerkortnummer.\n" +
		"2. Test dit kamera under **Indstillinger > Video**.\n" +
		"3. Hav din aftalekode klar, for eksempel `HB-2041`.\n\n" +
		"Læs mere i [Forberedelse til en videoaftale](https://harbor.example/help/prepare).\n"
	swedish := "# Välkommen till Harbor Help\n\n" +
		"Harbor Help kopplar dig till en rådgivare via **videomöte**. Du behöver en\n" +
		"enhet med kamera och [Harbor Help-appen](https://harbor.example/app).\n\n" +
		"## Före ditt första möte\n\n" +
		"1. Installera appen och logga in med ditt lånekortsnummer.\n" +
		"2. Testa kameran under **Inställningar > Video**.\n" +
		"3. Ha din mötekod redo, till exempel `HB-2041`.\n\n" +
		"Läs mer i [Förbered dig för ett videomöte](https://harbor.example/help/prepare).\n"
	task := pairedTaskByID(t, "add-edition-markup")
	for _, tc := range []struct {
		name, body string
		passed     bool
	}{
		{"right: the reference", string(reference), true},
		{"right: Bokmål worded differently", reworded, true},
		{"wrong: Danish", danish, false},
		{"wrong: Swedish", swedish, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			passed, detail, err := pairedReadsAs(tc.body, "nb")
			require.NoError(t, err)
			assert.Equal(t, tc.passed, passed, detail)
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "nb"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "nb", "welcome.md"), []byte(tc.body), 0o600))
			result, err := validatePairedTask(dir, task, &PairedAgentResult{})
			require.NoError(t, err)
			assert.Equal(t, tc.passed, result.ObjectivePassed, "%+v", result.Criteria)
		})
	}
}

// The gate task asks for the sentence without the forbidden word. Naming the
// overview page, the replacement the terms give, is reported; a sentence
// that refers back to the overview page the paragraph already names passes.
func TestPairedGateGraderAcceptsCompliantWording(t *testing.T) {
	task := pairedTaskByID(t, "recover-gate-refusal")
	files, err := pairedTaskFiles(task)
	require.NoError(t, err)
	original := string(files["docs/en/reports.md"])
	for _, tc := range []struct {
		name, sentence string
		passed, names  bool
	}{
		{"right: names the overview page", "You can download every report as a CSV file from the overview page.", true, true},
		{"right: refers back to it", "You can download every report there as a CSV file.", true, false},
		{"right: leaves the place out", "You can download every report as a CSV file.", true, false},
		{"wrong: the forbidden word", "You can download every report as a CSV file from the portal.", false, false},
		{"wrong: no CSV", "You can download every report from the overview page.", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			body := strings.Replace(original, "page within an hour.", "page within an hour. "+tc.sentence, 1)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "en", "reports.md"), []byte(body), 0o600))
			result, err := validatePairedTask(dir, task, &PairedAgentResult{})
			require.NoError(t, err)
			assert.Equal(t, tc.passed, result.ObjectivePassed, "%+v", result.Criteria)
			for _, criterion := range result.Criteria {
				if criterion.ID == "names-overview-page" {
					assert.Equal(t, tc.names, criterion.Passed)
					assert.True(t, criterion.Informational)
				}
			}
		})
	}
}

// A translation worded differently from the reference passes; the byte
// comparison is reported and decides nothing.
func TestPairedInformationalCriterionDecidesNothing(t *testing.T) {
	task := pairedTaskByID(t, "add-edition-markup")
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	repairPairedFixture(t, dir, task)
	file := filepath.Join(dir, "docs", "nb", "welcome.md")
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	reworded := strings.Replace(string(body), "Velkommen til Harbor Help", "Velkommen til tjenesten Harbor Help", 1)
	require.NoError(t, os.WriteFile(file, []byte(reworded), 0o600))
	result, err := validatePairedTask(dir, task, &PairedAgentResult{})
	require.NoError(t, err)
	assert.True(t, result.ObjectivePassed, "%+v", result.Criteria)
	for _, criterion := range result.Criteria {
		if criterion.ID == "reference-bytes" {
			assert.False(t, criterion.Passed)
			assert.True(t, criterion.Informational)
		}
	}
}

func TestPairedScopeRejectsStrayFilesAndLinks(t *testing.T) {
	task := pairedTaskByID(t, "add-json-key")
	for name, damage := range map[string]func(dir string){
		"backup beside the catalog": func(dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "locales", "en.json.bak"), []byte("{}"), 0o600))
		},
		"catalog replaced by a link": func(dir string) {
			target := filepath.Join(t.TempDir(), "en.json")
			body, err := os.ReadFile(filepath.Join(dir, "locales", "en.json"))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(target, body, 0o600))
			require.NoError(t, os.Remove(filepath.Join(dir, "locales", "en.json")))
			require.NoError(t, os.Symlink(target, filepath.Join(dir, "locales", "en.json")))
		},
		"style guide edited": func(dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "STYLE.md"), []byte("# rewritten\n"), 0o600))
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			repairPairedFixture(t, dir, task)
			damage(dir)
			result, err := validatePairedTask(dir, task, &PairedAgentResult{})
			require.NoError(t, err)
			assert.False(t, result.ObjectivePassed)
		})
	}
}

// The gate task fails an attempt that tried to land its edit over the check,
// whatever the file says, and fails without a transcript to read.
func TestPairedNoOverrideCriterion(t *testing.T) {
	task := pairedTaskByID(t, "recover-gate-refusal")
	dir := t.TempDir()
	require.NoError(t, materializePairedTask(dir, task))
	repairPairedFixture(t, dir, task)
	passed, err := validatePairedTask(dir, task, &PairedAgentResult{})
	require.NoError(t, err)
	assert.True(t, passed.ObjectivePassed)
	overridden, err := validatePairedTask(dir, task, &PairedAgentResult{OverrideAttempts: []string{"gate report (Bash)"}})
	require.NoError(t, err)
	assert.False(t, overridden.ObjectivePassed)
	unobserved, err := validatePairedTask(dir, task, nil)
	require.NoError(t, err)
	assert.False(t, unobserved.ObjectivePassed)
}

func TestPairedMarkdownBlocks(t *testing.T) {
	blocks := pairedMarkdownBlocks("# Title\n\nOne line\nwrapped.\n\n1. First\n2. Second\n   continued\n\n```yaml\na: b\n\nc: d\n```\n- bullet\n")
	require.Len(t, blocks, 6)
	assert.Equal(t, pairedMDBlock{Kind: "heading", Level: 1, Text: "Title"}, blocks[0])
	assert.Equal(t, "One line\nwrapped.", blocks[1].Text)
	assert.Equal(t, pairedMDBlock{Kind: "ordered-item", Text: "First"}, blocks[2])
	assert.Equal(t, pairedMDBlock{Kind: "ordered-item", Text: "Second\ncontinued"}, blocks[3])
	assert.Equal(t, pairedMDBlock{Kind: "code", Text: "a: b\n\nc: d"}, blocks[4])
	assert.Equal(t, "bullet-item", blocks[5].Kind)
}

func TestPairedJSONLeavesRejectDuplicates(t *testing.T) {
	leaves, err := pairedJSONLeaves([]byte(`{"a": {"b": "x", "c": [1, true]}, "d": null}`))
	require.NoError(t, err)
	assert.Equal(t, []pairedJSONLeaf{{"/a/b", `"x"`}, {"/a/c/0", "1"}, {"/a/c/1", "true"}, {"/d", "null"}}, leaves)
	// A key that holds a dot is told from a nested key.
	dotted, err := pairedJSONLeaves([]byte(`{"a.b": "x"}`))
	require.NoError(t, err)
	assert.NotEqual(t, leaves[:1], dotted)
	assert.Equal(t, "/a.b", dotted[0].Path)
	escaped, err := pairedJSONLeaves([]byte(`{"a/b~c": 1}`))
	require.NoError(t, err)
	assert.Equal(t, "/a~1b~0c", escaped[0].Path)
	_, err = pairedJSONLeaves([]byte(`{"a": "x", "a": "y"}`))
	require.ErrorContains(t, err, "duplicate key")
	_, err = pairedJSONLeaves([]byte(`{"a": "x"} {}`))
	require.Error(t, err)
}
