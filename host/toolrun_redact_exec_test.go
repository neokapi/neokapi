package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
)

// echoProgram writes a script that records the text it reads on stdin to
// saw and prints it back with "Write" changed to "Mail", and returns the
// external-command config that runs it over the source.
func echoProgram(t *testing.T) (config map[string]any, saw string) {
	t.Helper()
	dir := t.TempDir()
	saw = filepath.Join(dir, "saw.txt")
	script := filepath.Join(dir, "echo.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ntee \"$1\" | sed 's/Write/Mail/'\n"), 0o700))
	return map[string]any{
		"command":     script,
		"args":        []any{saw},
		"applySource": true,
		"applyTarget": false,
	}, saw
}

// rewriteProgram writes a script that runs sed with expr over the text it reads
// on stdin, and returns the external-command config that runs it over the
// source.
func rewriteProgram(t *testing.T, expr string) map[string]any {
	t.Helper()
	script := filepath.Join(t.TempDir(), "rewrite.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nsed '"+expr+"'\n"), 0o700))
	return map[string]any{"command": script, "applySource": true, "applyTarget": false}
}

// A program that drops the placeholder it read leaves the block as it was:
// the run prints the finding, writes the file with the block unchanged, and
// exits with the code a failed check uses.
func TestToolRun_ExternalCommandThatDropsAPlaceholderFailsTheRun(t *testing.T) {
	isolateKapiEnv(t)
	recipe := redactProject(t, redactionYAML)
	page := filepath.Join(filepath.Dir(recipe), "docs", "page.json")
	config := rewriteProgram(t, `s/Write/Mail/; s/\[REDACTED:Person\]/someone/`)
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	stdout, err := captureStdout(t, func() error {
		return a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, page, config))
	})
	require.Error(t, err)
	assert.Equal(t, ExitGate, ExitCode(nil, err), "the run exits as a failed check does")
	assert.Equal(t, "1 block(s): the program's output changed its redaction placeholders, so kapi left each such block as it was", err.Error())
	assert.Contains(t, stdout, "external-command.redaction-placeholder")
	assert.Contains(t, stdout, "page.json:greeting")
	assert.Contains(t, stdout, "rewrite.sh dropped the redaction placeholder [REDACTED:Person] that the source holds; the block is unchanged")
	assert.NotContains(t, stdout, redactedName, "the report names the placeholder, never the value")

	out, rerr := os.ReadFile(page)
	require.NoError(t, rerr)
	assert.Equal(t, redactedPage, string(out), "the block keeps its content")
}

// The same run with --json prints the finding in the kapi.check/v2 shape.
func TestToolRun_ExternalCommandPlaceholderFindingJSON(t *testing.T) {
	isolateKapiEnv(t)
	recipe := redactProject(t, redactionYAML)
	page := filepath.Join(filepath.Dir(recipe), "docs", "page.json")
	config := rewriteProgram(t, `s/\[REDACTED:Person\]/&, &/`)
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	run := externalCommandRun(a, recipe, page, config)
	run.JSONOutput = true

	stdout, err := captureStdout(t, func() error { return a.RunToolOnFiles(t.Context(), run) })
	assert.Equal(t, ExitGate, ExitCode(nil, err))
	var report struct {
		Summary  check.Summary      `json:"summary"`
		Findings []check.Diagnostic `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &report), stdout)
	require.Len(t, report.Findings, 1)
	f := report.Findings[0]
	assert.Equal(t, "external-command.redaction-placeholder", f.Rule)
	assert.True(t, f.Fails)
	assert.Equal(t, "greeting", f.Location.Block)
	assert.Equal(t, "duplicated", f.Metadata["kind"])
	assert.Equal(t, "[REDACTED:Person]", f.Metadata["token"])
	assert.Equal(t, "1", f.Metadata["expected"])
	assert.Equal(t, "2", f.Metadata["found"])
}

// A program that keeps the placeholder gets its output written with the
// original back in place, and the run reports no finding and exits 0.
func TestToolRun_ExternalCommandThatKeepsThePlaceholderPasses(t *testing.T) {
	isolateKapiEnv(t)
	recipe := redactProject(t, redactionYAML)
	page := filepath.Join(filepath.Dir(recipe), "docs", "page.json")
	config := rewriteProgram(t, `s/Write/Mail/`)
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	stdout, err := captureStdout(t, func() error {
		return a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, page, config))
	})
	require.NoError(t, err)
	assert.Contains(t, stdout, "No findings.")
	out, rerr := os.ReadFile(page)
	require.NoError(t, rerr)
	assert.Contains(t, string(out), "Mail to "+redactedName+" today.")
}

// A run over a package reports the finding and fails as a run over a file
// does.
func TestToolRun_ExternalCommandOverAPackageFailsOnADroppedPlaceholder(t *testing.T) {
	isolateKapiEnv(t)
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	recipe := redactProject(t, redactionYAML)
	root := filepath.Dir(recipe)
	a := &App{SourceLang: "en", Quiet: true}
	a.InitRegistries()
	pkg := filepath.Join(root, "work.kpz")
	require.NoError(t, a.ExtractToKpz(t.Context(), []string{filepath.Join(root, "docs", "page.json")}, pkg, "fr", "", false))

	config := rewriteProgram(t, `s/\[REDACTED:Person\]/someone/`)
	stdout, err := captureStdout(t, func() error {
		return a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, pkg, config))
	})
	require.Error(t, err)
	assert.Equal(t, ExitGate, ExitCode(nil, err))
	assert.Contains(t, stdout, "rewrite.sh dropped the redaction placeholder [REDACTED:Person] that the source holds; the block is unchanged")

	// The same package and a program that keeps the placeholder: no report.
	keep := rewriteProgram(t, `s/Write/Mail/`)
	stdout, err = captureStdout(t, func() error {
		return a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, pkg, keep))
	})
	require.NoError(t, err)
	assert.Empty(t, stdout)
}

// With no policy, nothing is redacted and the run reports nothing.
func TestToolRun_ExternalCommandWithNoPolicyPrintsNoReport(t *testing.T) {
	isolateKapiEnv(t)
	recipe := redactProject(t, "")
	page := filepath.Join(filepath.Dir(recipe), "docs", "page.json")
	config := rewriteProgram(t, `s/Write/Mail/`)
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	stdout, err := captureStdout(t, func() error {
		return a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, page, config))
	})
	require.NoError(t, err)
	assert.Empty(t, stdout)
}

// externalCommandRun is `kapi exec external-command <file>` with config in
// the project at recipe ("" for none).
func externalCommandRun(a *App, recipe, file string, config map[string]any) ToolRunConfig {
	return ToolRunConfig{
		ToolName:      "external-command",
		Project:       recipe,
		Files:         []string{file},
		DefaultLayout: true,
		ToolConfig:    config,
		NewTool: func() (tool.Tool, error) {
			return a.ToolReg.NewToolWithConfig(registry.ToolID("external-command"), config, "")
		},
	}
}

func TestToolRun_ExternalCommandReadsTheRedactedBlocksAndRestoresAfter(t *testing.T) {
	isolateKapiEnv(t)
	recipe := redactProject(t, redactionYAML)
	page := filepath.Join(filepath.Dir(recipe), "docs", "page.json")
	config, saw := echoProgram(t)
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	require.NoError(t, a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, page, config)))

	read, err := os.ReadFile(saw)
	require.NoError(t, err)
	assert.NotContains(t, string(read), redactedName, "the program never reads the name the policy withholds")
	assert.Equal(t, "Write to [REDACTED:Person] today.", string(read), "the program reads where the name was withheld")

	out, err := os.ReadFile(page)
	require.NoError(t, err)
	assert.Contains(t, string(out), "Mail to "+redactedName+" today.", "the written output carries the name again")
	assert.NotContains(t, string(out), "REDACTED")
}

func TestToolRun_ExternalCommandWithNoPolicyReadsTheBlockAsItIs(t *testing.T) {
	isolateKapiEnv(t)
	recipe := redactProject(t, "")
	page := filepath.Join(filepath.Dir(recipe), "docs", "page.json")
	config, saw := echoProgram(t)
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	require.NoError(t, a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, page, config)))

	read, err := os.ReadFile(saw)
	require.NoError(t, err)
	assert.Equal(t, "Write to "+redactedName+" today.", string(read))
	out, err := os.ReadFile(page)
	require.NoError(t, err)
	assert.Contains(t, string(out), "Mail to "+redactedName+" today.")
}

// A flow the project composes is held to the same rule: under a policy, a
// flow that runs external-command with no redact step fails the check.
func TestFlowPlacement_ExternalCommandNeedsARedactStepUnderAPolicy(t *testing.T) {
	isolateKapiEnv(t)
	config, _ := echoProgram(t)
	def := &flow.FlowDefinition{ID: "pipe", Name: "pipe", Nodes: []flow.FlowNode{
		{ID: "x", Type: flow.NodeTool, Name: "external-command", Config: config},
	}}

	for name, defaults := range map[string]string{"a policy": redactionYAML, "no policy": ""} {
		t.Run(name, func(t *testing.T) {
			recipe := redactProject(t, defaults)
			proj, err := project.Load(recipe)
			require.NoError(t, err)
			a := &App{SourceLang: "en", ProjectContext: project.NewProjectContext(proj, recipe)}
			a.InitRegistries()
			err = a.checkFlowPlacement(def, a.flowRequiresRedaction(""))
			if defaults == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, `"external-command" hands source to a program outside kapi and the flow has no redact step`)
		})
	}
}

// In a flow the step records the finding on the block, the run reports it
// beside the findings of any other check, and the block reaches the output
// unchanged.
func TestFlowRun_ExternalCommandThatDropsAPlaceholderReportsTheFinding(t *testing.T) {
	isolateKapiEnv(t)
	t.Setenv("KAPI_TRUST_EXEC", "1")
	dir := t.TempDir()
	saw := filepath.Join(dir, "saw.txt")
	script := filepath.Join(dir, "rewrite.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ntee \""+saw+"\" | sed 's/Write/Mail/; s/\\[REDACTED:Person\\]/someone/'\n"), 0o700))
	// The redact step names no rules, so it redacts under the project's policy.
	recipe := redactProject(t, redactionYAML+"flows:\n  rewrite:\n    steps:\n      - tool: redact\n"+
		"      - tool: external-command\n        config:\n          command: "+script+
		"\n          applySource: true\n          applyTarget: false\n      - tool: unredact\n")
	stdout, got, err := runRecipeFlow(t, recipe, "rewrite")
	require.NoError(t, err, "a flow reports its findings and leaves the gate to kapi check")

	read, rerr := os.ReadFile(saw)
	require.NoError(t, rerr)
	assert.Equal(t, "Write to [REDACTED:Person] today.", string(read), "the program reads the placeholder, never the name")

	require.Len(t, got, 1)
	require.Len(t, got[0].Findings, 1)
	f := got[0].Findings[0]
	assert.Equal(t, "external-command.redaction-placeholder", f.Rule)
	assert.True(t, f.Fails)
	assert.Equal(t, "greeting", f.Location.Block)
	assert.Equal(t, "rewrite.sh dropped the redaction placeholder [REDACTED:Person] that the source holds; the block is unchanged", f.Message)
	assert.Contains(t, stdout, "external-command.redaction-placeholder")

	out, rerr := os.ReadFile(filepath.Join(filepath.Dir(recipe), "out", "page.json"))
	require.NoError(t, rerr)
	assert.Contains(t, string(out), "Write to "+redactedName+" today.", "the block reaches the output unchanged")
}

// runRecipeFlow is `kapi run <flow> -p <recipe> -o out/{name}.{ext}`, and
// returns what it printed and the findings it handed to its sink.
func runRecipeFlow(t *testing.T, recipe, flowName string, extra ...string) (string, []FlowFindings, error) {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	cmd := flowRunCmd(a, "run")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	require.NoError(t, cmd.Flags().Set("output", filepath.Join(filepath.Dir(recipe), "out", "{name}.{ext}")))
	for i := 0; i+1 < len(extra); i += 2 {
		require.NoError(t, cmd.Flags().Set(extra[i], extra[i+1]))
	}
	var got []FlowFindings
	stdout, err := captureStdout(t, func() error {
		cmd.SetOut(os.Stdout)
		return a.RunFromProject(cmd, flowName, recipe, RunCmdOptions{OnFindings: func(f FlowFindings) { got = append(got, f) }})
	})
	return stdout, got, err
}

// A recipe flow that redacts ahead of a provider sends the provider no value
// the project's policy withholds, though its redact step names no rules.
func TestFlowRun_ARedactStepWithNoRulesTakesTheProjectsPolicy(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML+"flows:\n  secure:\n    steps:\n      - tool: redact\n"+
		"      - tool: translate\n        config:\n          provider: demo\n      - tool: unredact\n")
	saw := providerSaw(t)
	_, _, err := runRecipeFlow(t, recipe, "secure", "target-lang", "fr")
	require.NoError(t, err)
	sent := saw()
	require.Contains(t, sent, "today", "the run asked the provider about the block")
	assert.NotContains(t, sent, redactedName, "the provider never reads the name the policy withholds")
}

// The redact step redacts each file under the policy that governs that file:
// the legal collection's rules for its file, the defaults for the docs file.
func TestFlowRun_ARedactStepRedactsEachFileUnderItsOwnPolicy(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := collectionRedactProject(t, redactionYAML+"flows:\n  secure:\n    steps:\n      - tool: redact\n"+
		"      - tool: translate\n        config:\n          provider: demo\n      - tool: unredact\n")
	saw := providerSaw(t)
	_, _, err := runRecipeFlow(t, recipe, "secure", "target-lang", "fr")
	require.NoError(t, err)
	sent := saw()
	assert.Contains(t, sent, redactedName+" leads", "the legal file reads under its collection's rules")
	assert.Contains(t, sent, "leads "+legalName, "the docs file reads under the defaults")
	assert.NotContains(t, sent, redactedName+" leads "+legalName, "no file reaches the provider whole")
}

// A redact step that names rules of its own keeps them, and the project's
// policy does not replace them.
func TestFlowRun_ARedactStepWithItsOwnRulesKeepsThem(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	rules := filepath.Join(t.TempDir(), "rules.yaml")
	require.NoError(t, os.WriteFile(rules, []byte("version: v1\nrules:\n  - term: today\n    category: date\n"), 0o600))
	recipe := redactProject(t, redactionYAML+"flows:\n  secure:\n    steps:\n"+
		"      - tool: redact\n        config:\n          rulesPath: "+rules+"\n"+
		"      - tool: translate\n        config:\n          provider: demo\n      - tool: unredact\n")
	saw := providerSaw(t)
	_, _, err := runRecipeFlow(t, recipe, "secure", "target-lang", "fr")
	require.NoError(t, err)
	sent := saw()
	require.Contains(t, sent, "Write to", "the run asked the provider about the block")
	assert.NotContains(t, sent, "today", "the step's own rule applies")
	assert.Contains(t, sent, redactedName, "the project's rule does not replace the step's own")
}

// The plan builds a unit's chain under the policy that governs the unit's
// file, as the run does. The legal collection's policy names no rules file,
// so a run refuses its redact step there: the plan cannot build that chain
// either, and builds the docs file's chain under the defaults.
func TestPlanReuse_BuildsEachUnitsChainUnderItsFilesPolicy(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write(project.RecipeFileName, "version: v1\nname: plan-redact\ndefaults:\n  source_language: en\n  target_languages: [fr]\n"+redactionYAML+
		"collections:\n"+
		"  - name: legal\n    redaction:\n      enabled: true\n      detectors: [rules]\n    content:\n      - path: \"legal/*.json\"\n"+
		"  - name: docs\n    content:\n      - path: \"docs/*.json\"\n")
	write("redact.yaml", redactRules)
	write("docs/page.json", redactedPage)
	write("legal/memo.json", redactedPage)
	recipe := filepath.Join(root, project.RecipeFileName)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	a := &App{SourceLang: "en"}
	a.InitRegistries()
	cmd, err := a.unitCommand(t.Context(), recipe)
	require.NoError(t, err)
	spec := &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "redact"},
		{Tool: "translate", Config: map[string]any{"provider": "demo"}},
		{Tool: "unredact"},
	}}
	p := &planReuse{
		app:        a,
		cmd:        cmd,
		pctx:       project.NewProjectContext(proj, recipe),
		projectDir: root,
		fl:         upPlanFlow{Name: "secure", Spec: spec, Drafts: true},
		bindings:   a.newLocaleBindings(cmd, proj, recipe),
		producers:  map[string][]tool.StoredTargetReuser{},
	}
	defer p.close()

	assert.NotEmpty(t, p.producersFor(VerifyUnit{SourcePath: filepath.Join(root, "docs", "page.json"), Locale: "fr"}),
		"the docs file's chain builds under the defaults")
	assert.Empty(t, p.producersFor(VerifyUnit{SourcePath: filepath.Join(root, "legal", "memo.json"), Locale: "fr"}),
		"the legal file's chain takes its collection's policy, which a run refuses")
}

// `kapi up` over a flow whose program drops a placeholder reports the finding
// and fails as a failed check does, rather than reporting the project up to
// date.
func TestUp_AProgramThatDropsAPlaceholderFailsTheRun(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	t.Setenv("KAPI_TRUST_EXEC", "1")
	script := filepath.Join(t.TempDir(), "rewrite.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nsed 's/Write/Mail/; s/\\[REDACTED:Person\\]/someone/'\n"), 0o700))
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write(project.RecipeFileName, "version: v1\nname: up-redact\ndefaults:\n  source_language: en\n  flow: rewrite\n  target_languages: [fr]\n"+
		"  translate_after: none\n"+redactionYAML+
		"collections:\n  - name: docs\n    content:\n      - path: \"docs/*.json\"\n        target: \"out/{lang}/{name}.{ext}\"\n"+
		"flows:\n  rewrite:\n    steps:\n      - tool: redact\n"+
		"      - tool: external-command\n        config:\n          command: "+script+"\n          applySource: true\n          applyTarget: false\n"+
		"      - tool: unredact\n      - tool: translate\n        config:\n          provider: demo\n")
	write("redact.yaml", redactRules)
	write("docs/page.json", redactedPage)
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o700))
	recipe := filepath.Join(root, project.RecipeFileName)
	t.Chdir(root)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(t.Context(), "up")
	a.AddFlowRunFlags(cmd)
	var out ConvergeOutput
	err = a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{UntilGate: true, MaxPasses: 2, noChecks: true, capture: &out})
	require.Error(t, err)
	assert.Equal(t, ExitGate, ExitCode(nil, err))
	assert.Equal(t, "1 block(s): the program's output changed its redaction placeholders, so kapi left each such block as it was", err.Error())
	require.NotNil(t, out.Findings, "the run's output carries the finding")
	require.NotEmpty(t, out.Findings.Findings)
	f := out.Findings.Findings[0]
	assert.Equal(t, "external-command.redaction-placeholder", f.Rule)
	assert.Equal(t, "greeting", f.Location.Block)

	var text strings.Builder
	require.NoError(t, out.FormatText(&text))
	assert.Contains(t, text.String(), "rewrite.sh dropped the redaction placeholder [REDACTED:Person] that the source holds; the block is unchanged")
	assert.Contains(t, text.String(), "Not up to date: 1 block(s) left unchanged because a program's output changed a redaction placeholder.")
	assert.NotContains(t, text.String(), "Up to date.")
}
