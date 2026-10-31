package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			err = a.checkFlowPlacement(def)
			if defaults == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, `"external-command" hands source to a program outside kapi and the flow has no redact step`)
		})
	}
}
