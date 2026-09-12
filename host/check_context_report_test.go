package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckContextReportDistinguishesOverrideThroughMCP(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	file := filepath.Join(root, "adult", "page.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
	require.NoError(t, os.WriteFile(file, []byte(`{"body":"A risk-free old-route."}`), 0o600))
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerCheckMCPTools(server, app)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "scope-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	reports := []check.Report{}
	for _, override := range []bool{false, true} {
		args := map[string]any{"file": file}
		if override {
			args["profile_file"] = filepath.Join(root, ".kapi", "voice.yaml")
		}
		result, callErr := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "check_file", Arguments: args})
		require.NoError(t, callErr)
		require.False(t, result.IsError)
		body, marshalErr := json.Marshal(result.StructuredContent)
		require.NoError(t, marshalErr)
		var report check.Report
		require.NoError(t, json.Unmarshal(body, &report))
		require.NotNil(t, report.Execution)
		require.Len(t, report.Execution.Contexts, 1)
		scope := report.Execution.Contexts[0]
		assert.Equal(t, file, scope.File)
		assert.Empty(t, scope.ContextPath)
		assert.True(t, scope.Voice.Applied)
		assert.Equal(t, "Service", scope.Voice.Name)
		assert.True(t, scope.TermsApplied, "voice overrides do not turn off project terms")
		if override {
			assert.Equal(t, "override", scope.Voice.Selection)
			assert.Empty(t, scope.Voice.Profile)
			assert.Empty(t, scope.Voice.Channel, "base profile must not claim adult exception coverage")
			assert.Equal(t, args["profile_file"], scope.Voice.Source)
		} else {
			assert.Equal(t, "project", scope.Voice.Selection)
			assert.Equal(t, "service", scope.Voice.Profile)
			assert.Equal(t, "adult", scope.Voice.Channel)
			assert.NotEmpty(t, scope.Voice.Source)
		}
		reports = append(reports, report)
	}
	assert.Less(t, reports[0].Summary.Findings, reports[1].Summary.Findings,
		"the scoped exception applies only in the reported adult scope")
	assert.NotEmpty(t, reports[1].Findings, "project terminology still executes under a voice override")
}

func TestCheckContextReportTracksEachCLIFileAndDraft(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	files := []string{}
	for _, channel := range []string{"child", "adult", "child"} {
		file := filepath.Join(root, channel, "page"+string(rune('a'+len(files)))+".json")
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
		require.NoError(t, os.WriteFile(file, []byte(`{"body":"Utilize the risk-free old-route."}`), 0o600))
		files = append(files, file)
	}
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
	report, err := app.ComputeCheck(cmd, files)
	require.NoError(t, err)
	require.Len(t, report.Execution.Contexts, 3)
	for i, channel := range []string{"child", "adult", "child"} {
		scope := report.Execution.Contexts[i]
		assert.Equal(t, files[i], scope.File)
		assert.Equal(t, channel, scope.Voice.Channel, "cached profiles retain their effective scope")
		_, saved, callErr := app.checkFileMCP(t.Context(), checkFileInput{File: files[i]})
		require.NoError(t, callErr)
		assert.Equal(t, scope, saved.Execution.Contexts[0], "CLI and MCP expose the same selection")
	}
	var out bytes.Buffer
	require.NoError(t, (checkReport{report}).FormatText(&out))
	assert.Contains(t, out.String(), "profile service; channel child")
	assert.Contains(t, out.String(), "profile service; channel adult")
	_, draft, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Ready.", ContextPath: "child/unwritten.json"})
	require.NoError(t, err)
	require.Len(t, draft.Execution.Contexts, 1)
	assert.Empty(t, draft.Execution.Contexts[0].File)
	assert.Equal(t, "child/unwritten.json", draft.Execution.Contexts[0].ContextPath)
	assert.Equal(t, "child", draft.Execution.Contexts[0].Voice.Channel)
}

func TestCheckContextReportUnscopedAndCLIOverride(t *testing.T) {
	isolateCheckExecution(t)
	app := &App{}
	_, report, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Ready."})
	require.NoError(t, err)
	require.Len(t, report.Execution.Contexts, 1)
	assert.Equal(t, check.CheckContext{Voice: check.VoiceContext{Selection: "none"}}, report.Execution.Contexts[0])
	file := filepath.Join(t.TempDir(), "page.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"body":"Ready."}`), 0o600))
	cmd := executionCommand(t)
	cmd.Flags().String("pack", "marketing-blog", "")
	cliReport, err := app.ComputeCheck(cmd, []string{file})
	require.NoError(t, err)
	_, mcpReport, err := app.checkFileMCP(t.Context(), checkFileInput{File: file, ProfilePack: "marketing-blog"})
	require.NoError(t, err)
	assert.Equal(t, cliReport.Execution.Contexts, mcpReport.Execution.Contexts)
	scope := cliReport.Execution.Contexts[0]
	assert.Equal(t, "override", scope.Voice.Selection)
	assert.True(t, scope.Voice.Applied)
	assert.Equal(t, "pack:marketing-blog", scope.Voice.Source)
	assert.False(t, scope.TermsApplied)
}

func TestCheckContextReportTermsWithoutVoiceAreChecked(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: terms-only
defaults:
  source_language: en
  terms_source: .kapi/terms.json
collections:
  - name: content
    content:
      - path: '*.json'
`), 0o600))
	require.NoError(t, os.Remove(filepath.Join(root, ".kapi", "voice.yaml")))
	file := filepath.Join(root, "page.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"body":"Use the old-route."}`), 0o600))
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, recipe, "")
	cliReport, err := app.ComputeCheck(cmd, []string{file})
	require.NoError(t, err)
	_, saved, err := app.checkFileMCP(t.Context(), checkFileInput{File: file})
	require.NoError(t, err)
	_, draft, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Use the old-route.", ContextPath: "draft.json"})
	require.NoError(t, err)
	for _, report := range []check.Report{cliReport, saved, draft} {
		require.Len(t, report.Execution.Contexts, 1)
		scope := report.Execution.Contexts[0]
		assert.Equal(t, "project", scope.Voice.Selection)
		assert.False(t, scope.Voice.Applied)
		assert.True(t, scope.TermsApplied)
		assert.Positive(t, ruleCounts(report)["voice.vocabulary"], "the bound deprecated term must produce a finding without a voice profile")
		found := false
		for _, run := range report.Execution.Analyzers {
			if run.ID == "voice.rules" {
				found = true
				assert.Equal(t, check.AnalyzerFindings, run.Status)
			}
		}
		assert.True(t, found)
	}
	assert.Equal(t, cliReport.Execution.Contexts, saved.Execution.Contexts)
	assert.Equal(t, cliReport.Summary, draft.Summary)
}
