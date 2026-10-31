package host

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// placeholderUpProject writes a project whose flow redacts, runs a program
// that drops the placeholder, translates with the demo provider and restores,
// and returns the app and an `up` command bound to it.
func placeholderUpProject(t *testing.T) (*App, *EnvCommand, string) {
	t.Helper()
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
		"      - tool: translate\n        config:\n          provider: demo\n      - tool: unredact\n")
	write("redact.yaml", redactRules)
	write("docs/page.json", redactedPage)
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o700))
	recipe := filepath.Join(root, project.RecipeFileName)
	t.Chdir(root)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddUpFlags(cmd)
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	cmd.SetErr(&bytes.Buffer{})
	return a, cmd, recipe
}

// `kapi up` prints its summary, with the finding table and the line that says
// the run is not up to date, and then fails as a failed check does.
func TestExecuteUp_PrintsThePlaceholderFindingsBeforeItFails(t *testing.T) {
	a, cmd, recipe := placeholderUpProject(t)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	err := a.ExecuteUp(cmd, recipe)
	require.Error(t, err)
	assert.Equal(t, ExitGate, ExitCode(nil, err))
	out := stdout.String()
	assert.Contains(t, out, "external-command.redaction-placeholder")
	assert.Contains(t, out, "rewrite.sh dropped the redaction placeholder [REDACTED:Person] that the source holds; the block is unchanged")
	assert.Contains(t, out, "Not up to date: 1 block(s) left unchanged because a program's output changed a redaction placeholder.")
	assert.NotContains(t, out, "Up to date.")
}

// An embedder of the run (the desktop's up) gets the result with the findings
// beside the gate error.
func TestRunUp_ReturnsTheResultWithThePlaceholderFindings(t *testing.T) {
	a, _, recipe := placeholderUpProject(t)

	result, err := a.RunUp(t.Context(), recipe, "en", UpOptions{UntilGate: true, MaxPasses: 2, NoChecks: true})
	require.Error(t, err)
	assert.Equal(t, ExitGate, ExitCode(nil, err))
	require.NotNil(t, result, "the completed run's result reaches the caller")
	require.NotNil(t, result.Findings)
	assert.Equal(t, "external-command.redaction-placeholder", result.Findings.Findings[0].Rule)
}

// Under --json the stream closes with the result record, and the record holds
// the findings.
func TestExecuteUp_TheJSONResultRecordHoldsThePlaceholderFindings(t *testing.T) {
	a, cmd, recipe := placeholderUpProject(t)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	err := a.ExecuteUp(cmd, recipe)
	require.Error(t, err)
	assert.Equal(t, ExitGate, ExitCode(nil, err))

	var result struct {
		Type     string `json:"type"`
		Findings *struct {
			Findings []struct {
				Rule     string            `json:"rule"`
				Fails    bool              `json:"fails"`
				Metadata map[string]string `json:"metadata"`
			} `json:"findings"`
		} `json:"findings"`
	}
	found := false
	sc := bufio.NewScanner(&stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var line struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(sc.Bytes(), &line) == nil && line.Type == "result" {
			require.NoError(t, json.Unmarshal(sc.Bytes(), &result))
			found = true
		}
	}
	require.True(t, found, "the stream closes with a result record")
	require.NotNil(t, result.Findings)
	require.Len(t, result.Findings.Findings, 1)
	assert.Equal(t, "external-command.redaction-placeholder", result.Findings.Findings[0].Rule)
	assert.True(t, result.Findings.Findings[0].Fails)
	assert.Equal(t, "[REDACTED:Person]", result.Findings.Findings[0].Metadata["token"])
}
