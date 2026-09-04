package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The local run_flow action runs the flow it names over the project's
// collections, as `kapi run <flow>` does with no --input, and gates the
// triggering command on what the flow's check steps found when the rule asks
// for it (#2410). These drive runLocalAutomations, the hook every push, pull
// and up reaches, over a fixture project with a do-not-translate check flow.

const automationXLIFF = `<?xml version="1.0" encoding="UTF-8"?>
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

// isolateKapi keeps an in-repo kapi invocation off the developer's kapi
// installation and the repo's own recipe (the isolation contract in
// CLAUDE.md).
func isolateKapi(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("KAPI_CONFIG_DIR", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
}

// automationApp installs a fresh, registry-populated App as the package's
// app for the test, the way the plugin's initializer does at runtime.
func automationApp(t *testing.T) *cli.App {
	t.Helper()
	prev := app
	a := &cli.App{}
	a.InitRegistries()
	a.AssumeYes = true
	app = a
	t.Cleanup(func() { app = prev })
	return a
}

// automationFixture writes a project whose recipe declares one XLIFF
// collection, a `guard` flow of one dnt-check step over dntTerms, and the
// given rules, then loads it the way the hooks do.
func automationFixture(t *testing.T, dntTerms []string, rules ...project.AutomationSpec) *project.Project {
	t.Helper()
	isolateKapi(t)
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe := &project.Recipe{
		KapiProject: coreproj.KapiProject{
			Version: coreproj.CurrentVersion,
			Name:    "AutomationTest",
			Defaults: coreproj.Defaults{
				SourceLanguage:  "en",
				TargetLanguages: []model.LocaleID{"nb"},
			},
			Collections: []coreproj.Collection{
				{
					Path:   "src/*.xlf",
					Format: &coreproj.FormatSpec{Name: "xliff"},
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
		},
		Automations: rules,
	}
	_, err = project.InitProject(real, recipe)
	require.NoError(t, err)

	src := filepath.Join(real, "src", "app.xlf")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte(automationXLIFF), 0o644))

	proj, err := project.FindProject(real)
	require.NoError(t, err)
	return proj
}

// hookCmd is the triggering command as push, pull and up hand it over: bare
// cobra, captured streams, a live context.
func hookCmd(t *testing.T) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cmd := &cobra.Command{Use: "push"}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetContext(t.Context())
	return cmd, stdout, stderr
}

func runFlowRule(name, trigger string, config map[string]string) project.AutomationSpec {
	return project.AutomationSpec{
		Name:    name,
		Trigger: trigger,
		Actions: []project.ActionConfig{{Type: project.ActionRunFlow, Config: config}},
	}
}

// The documented check gate: findings with fail_on_error abort the push with
// the gate exit code, and the findings print in the push's output.
func TestRunFlowAction_PrePushGateBlocksOnFindings(t *testing.T) {
	automationApp(t)
	proj := automationFixture(t, []string{"Acme Cloud"},
		runFlowRule("checks-gate", project.HookPrePush, map[string]string{"flow": "guard", "fail_on_error": "true"}))
	cmd, stdout, _ := hookCmd(t)

	err := runLocalAutomations(cmd, proj, project.HookPrePush)
	require.Error(t, err)
	assert.Equal(t, cli.ExitGate, cli.ExitCode(cmd, err), "a failed gate exits like `kapi check`")
	assert.Contains(t, err.Error(), `automation "checks-gate" action "run_flow"`)
	assert.Contains(t, err.Error(), `flow "guard" found 1 finding(s) (1 critical`)

	out := stdout.String()
	assert.Contains(t, out, "Running automation: checks-gate")
	assert.Contains(t, out, "CRITICAL", "the findings table prints where the push prints")
	assert.Contains(t, out, "Acme Cloud")
	assert.NotContains(t, out, "Would run flow")
}

func TestRunFlowAction_PrePushGatePassesClean(t *testing.T) {
	automationApp(t)
	proj := automationFixture(t, []string{"Nonexistent Product"},
		runFlowRule("checks-gate", project.HookPrePush, map[string]string{"flow": "guard", "fail_on_error": "true"}))
	cmd, stdout, _ := hookCmd(t)

	require.NoError(t, runLocalAutomations(cmd, proj, project.HookPrePush))
	assert.Contains(t, stdout.String(), "No findings.")
}

// Without fail_on_error the findings are reported and the command goes on,
// which is `kapi run`'s own contract.
func TestRunFlowAction_ReportsWithoutGating(t *testing.T) {
	automationApp(t)
	proj := automationFixture(t, []string{"Acme Cloud"},
		runFlowRule("checks", project.HookPrePush, map[string]string{"flow": "guard"}))
	cmd, stdout, _ := hookCmd(t)

	require.NoError(t, runLocalAutomations(cmd, proj, project.HookPrePush))
	assert.Contains(t, stdout.String(), "CRITICAL")
	assert.Contains(t, stdout.String(), "1 finding(s)")
}

// A post-pull rule runs the flow too, and the triggering command's own
// language settings survive the run.
func TestRunFlowAction_PostPullRunsTheFlow(t *testing.T) {
	a := automationApp(t)
	a.TargetLang = "zz"
	proj := automationFixture(t, []string{"Nonexistent Product"},
		runFlowRule("after-pull", project.HookPostPull, map[string]string{"flow": "guard"}),
		runFlowRule("before-push", project.HookPrePush, map[string]string{"flow": "guard", "fail_on_error": "true"}))
	cmd, stdout, _ := hookCmd(t)

	require.NoError(t, runLocalAutomations(cmd, proj, project.HookPostPull))
	out := stdout.String()
	assert.Contains(t, out, "Running automation: after-pull")
	assert.NotContains(t, out, "before-push", "only the rules on this trigger run")
	assert.Contains(t, out, "Running flow: guard")
	assert.Contains(t, out, "No findings.")
	assert.Equal(t, "zz", a.TargetLang, "the run's flag set must not leave its defaults on the App")
}

// A built-in catalog flow resolves like a recipe flow and runs over the
// collections, process-only.
func TestRunFlowAction_BuiltInFlowRunsOverTheCollections(t *testing.T) {
	automationApp(t)
	proj := automationFixture(t, nil,
		runFlowRule("pseudo-before-push", project.HookPrePush, map[string]string{"flow": "pseudo-translate"}))
	cmd, stdout, _ := hookCmd(t)

	require.NoError(t, runLocalAutomations(cmd, proj, project.HookPrePush))
	assert.Contains(t, stdout.String(), "kapi merge", "process-only: overlays committed, no target file written")
	_, statErr := os.Stat(filepath.Join(proj.Root, "out"))
	assert.True(t, os.IsNotExist(statErr))
}

// A flow that cannot run fails the action whatever fail_on_error says: a rule
// naming a flow the project cannot see is a broken rule, as on the server.
func TestRunFlowAction_UnknownFlowFails(t *testing.T) {
	automationApp(t)
	proj := automationFixture(t, nil,
		runFlowRule("broken", project.HookPrePush, map[string]string{"flow": "no-such-flow", "fail_on_error": "false"}))
	cmd, _, _ := hookCmd(t)

	err := runLocalAutomations(cmd, proj, project.HookPrePush)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `flow "no-such-flow"`)
	assert.Equal(t, cli.ExitError, cli.ExitCode(cmd, err), "not a gate: the flow never ran")
}

func TestRunFlowAction_RejectsBadConfig(t *testing.T) {
	automationApp(t)
	tests := []struct {
		name   string
		config map[string]string
		want   string
	}{
		{"no flow", map[string]string{}, "names no flow"},
		{"bad fail_on_error", map[string]string{"flow": "guard", "fail_on_error": "maybe"}, `fail_on_error "maybe" is not a boolean`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proj := automationFixture(t, nil, runFlowRule("r", project.HookPrePush, tc.config))
			cmd, _, _ := hookCmd(t)
			err := runLocalAutomations(cmd, proj, project.HookPrePush)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// --quiet silences the flow's report and must not silence the gate.
func TestRunFlowAction_QuietStillGates(t *testing.T) {
	a := automationApp(t)
	a.Quiet = true
	proj := automationFixture(t, []string{"Acme Cloud"},
		runFlowRule("checks-gate", project.HookPrePush, map[string]string{"flow": "guard", "fail_on_error": "true"}))
	cmd, stdout, _ := hookCmd(t)

	err := runLocalAutomations(cmd, proj, project.HookPrePush)
	require.Error(t, err)
	assert.Equal(t, cli.ExitGate, cli.ExitCode(cmd, err))
	assert.NotContains(t, stdout.String(), "CRITICAL", "quiet prints no table")
}

// Under --json the command's stdout is a document; the automation narrates
// and the flow reports on stderr instead.
func TestRunFlowAction_JSONKeepsStdoutForTheDocument(t *testing.T) {
	automationApp(t)
	proj := automationFixture(t, []string{"Acme Cloud"},
		runFlowRule("checks", project.HookPrePush, map[string]string{"flow": "guard"}))
	cmd, stdout, stderr := hookCmd(t)
	cmd.Flags().Bool("json", false, "")
	require.NoError(t, cmd.Flags().Set("json", "true"))

	require.NoError(t, runLocalAutomations(cmd, proj, project.HookPrePush))
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "Running automation: checks")
	assert.Contains(t, stderr.String(), "CRITICAL")
}
