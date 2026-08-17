package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end over the real `kapi run <flow>` command: a recipe declares a
// guardrail, the guardrail fires, and before the flow findings surface was wired
// the run printed its one-line result and exited 0 — indistinguishable from a
// run where nothing fired (#1489). The check tool writes into a stand-off
// annotation no writer serializes, so the run itself is the only place that can
// say what it found.
//
// This drives the cobra command so the *wiring* is under test.

const guardXLIFF = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="nb" datatype="plaintext" original="app">
    <body>
      <trans-unit id="save">
        <source>Save now with Acme Cloud</source>
        <target>Lagre nå med Toppskyen</target>
      </trans-unit>
    </body>
  </file>
</xliff>
`

// guardProjectFixture writes a recipe whose `guard` flow is one dnt-check step,
// exactly as the issue spells it, over one XLIFF source. dntTerms are the
// do-not-translate strings the check is given.
func guardProjectFixture(t *testing.T, dntTerms []string) (recipe, root string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe = filepath.Join(real, project.RecipeFileName)
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "GuardTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []project.Collection{
			{
				Path:   "src/*.xlf",
				Format: &project.FormatSpec{Name: "xliff"},
				Target: "out/{lang}/*.xlf",
			},
		},
		Flows: map[string]*flow.StepsSpec{
			"guard": {
				Steps: []flow.FlowStep{
					{Tool: "dnt-check", Config: map[string]any{"terms": dntTerms}},
				},
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))

	srcAbs := filepath.Join(real, "src", "app.xlf")
	require.NoError(t, os.MkdirAll(filepath.Dir(srcAbs), 0o755))
	require.NoError(t, os.WriteFile(srcAbs, []byte(guardXLIFF), 0o644))
	return recipe, real
}

// TestRunFlow_ReportsWhatItsChecksFound is the defect: the finding was produced
// and discarded.
func TestRunFlow_ReportsWhatItsChecksFound(t *testing.T) {
	recipe, _ := guardProjectFixture(t, []string{"Acme Cloud"})
	out, err := runRunCmd(t, processOnlyApp(t), recipe, "guard", "--target-lang", "nb")
	require.NoError(t, err)

	assert.Contains(t, out, "CRITICAL",
		"a check inside a flow must report what it found, not exit 0 in silence")
	assert.Contains(t, out, "Acme Cloud")
	assert.Contains(t, out, "1 finding(s) (1 critical")
	assert.Contains(t, out, "dnt-check.do-not-translate",
		"the rule names the tool that produced the finding")
	assert.Contains(t, out, "app.xlf", "the finding names the file it is in")
}

// A clean run says so: "printed nothing" and "found nothing" must not look the
// same from the outside — that ambiguity is the whole defect.
func TestRunFlow_SaysWhenItsChecksFoundNothing(t *testing.T) {
	recipe, _ := guardProjectFixture(t, []string{"Nonexistent Product"})
	out, err := runRunCmd(t, processOnlyApp(t), recipe, "guard", "--target-lang", "nb")
	require.NoError(t, err)

	assert.Contains(t, out, "No findings.")
	assert.NotContains(t, out, "CRITICAL")
}

// The run reports and does not gate. `kapi check` and `kapi up` own the bar and
// their exit codes are what CI is written against; a flow run that started
// failing over a step it always ran would be a new gating rule.
func TestRunFlow_FindingsDoNotGate(t *testing.T) {
	recipe, _ := guardProjectFixture(t, []string{"Acme Cloud"})
	_, err := runRunCmd(t, processOnlyApp(t), recipe, "guard", "--target-lang", "nb")
	require.NoError(t, err, "a critical finding must not make the flow run fail")
}

// The rendering is scriptable: one run is one JSON document, the run's own
// fields plus the findings under the same `.findings[]`/`.summary` shape
// `kapi check` and `kapi exec <check>` emit.
func TestRunFlow_FindingsAreScriptable(t *testing.T) {
	recipe, _ := guardProjectFixture(t, []string{"Acme Cloud"})
	// Under a root, because --output-format is a root persistent flag: the point
	// of the case is the surface a user actually types.
	app := processOnlyApp(t)
	root := &cobra.Command{Use: "kapi"}
	AddPersistentFlags(app, root)
	root.AddCommand(NewRunCmd(app, RunCmdOptions{}))
	root.SetArgs([]string{"run", "guard", "--project", recipe, "--target-lang", "nb", "--output-format", "json"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	require.NoError(t, root.Execute())
	out := buf.String()

	var doc struct {
		FlowName string `json:"flow_name"`
		Findings *struct {
			Summary struct {
				Findings int `json:"findings"`
				Critical int `json:"critical"`
			} `json:"summary"`
			Findings []struct {
				Rule     string `json:"rule"`
				Check    string `json:"check"`
				Severity string `json:"severity"`
				Message  string `json:"message"`
				Location struct {
					File  string `json:"file"`
					Block string `json:"block"`
				} `json:"location"`
			} `json:"findings"`
			Pass *bool `json:"pass"`
			Gate any   `json:"gate"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &doc),
		"the run must stay ONE document — a second one appended would break every consumer")
	assert.Equal(t, "guard", doc.FlowName, "the run's own fields survive")
	require.NotNil(t, doc.Findings)
	assert.Equal(t, 1, doc.Findings.Summary.Findings)
	assert.Equal(t, 1, doc.Findings.Summary.Critical)
	require.Len(t, doc.Findings.Findings, 1)
	f := doc.Findings.Findings[0]
	assert.Equal(t, "dnt-check.do-not-translate", f.Rule)
	assert.Equal(t, "dnt-check", f.Check)
	assert.Equal(t, "critical", f.Severity)
	assert.NotEmpty(t, f.Message)
	assert.Contains(t, f.Location.File, "app.xlf")
	assert.NotEmpty(t, f.Location.Block)

	// No verdict: a `"pass": true` beside a critical finding is exactly the
	// reassurance this class of defect hands out, and the gate is `kapi check`'s.
	assert.Nil(t, doc.Findings.Pass)
	assert.Nil(t, doc.Findings.Gate)
}

// A flow with no check step says nothing about findings: the report belongs to
// flows that declare a check, not to every ordinary run.
func TestRunFlow_WithoutAChecksReportsNothingExtra(t *testing.T) {
	recipe, _, _ := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})
	out, err := runRunCmd(t, processOnlyApp(t), recipe, "pseudo", "--target-lang", "fr-FR")
	require.NoError(t, err)

	assert.NotContains(t, out, "No findings.")
	assert.NotContains(t, out, "finding(s)")
}
