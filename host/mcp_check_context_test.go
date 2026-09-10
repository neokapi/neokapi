package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scopedTextCheckFixture(t *testing.T) (*App, string) {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		file := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
		require.NoError(t, os.WriteFile(file, []byte(body), 0o600))
	}
	write("kapi.yaml", `version: v1
name: scoped-drafts
defaults:
  source_language: en
  terms_source: .kapi/terms.json
  voice:
    profile_file: .kapi/voice.yaml
profiles:
  service:
    channels: [child, adult]
    voice: .kapi/voice.yaml
collections:
  - name: children
    channel: service/child
    content:
      - path: child/*.json
  - name: adults
    channel: service/adult
    content:
      - path: adult/*.json
`)
	write(".kapi/voice.yaml", `name: Service
constraints:
  - id: service/assurance
    version: 1
    source: service-facts.md
    statement: Do not promise a risk-free service.
    kind: prohibited_pattern
    regex: '(?i)\brisk-free\b'
    exceptions:
      - scope:
          channel: adult
        reason: Reviewed policy example.
        approved_by: fixture
        approval_ref: service-facts.md#exception
  - id: service/facts
    version: 1
    source: service-facts.md
    statement: Preserve the service facts.
    kind: guidance
channels:
  child:
    vocabulary:
      forbidden_terms:
        - term: utilize
          replacement: use
          severity: major
  adult:
    tone:
      formality: neutral
`)
	write(".kapi/terms.json", `{
  "schemaVersion":"1.0", "kind":"kapi-terms", "concepts":[{
    "id":"service-route", "definition":"The support route", "terms":[
      {"text":"support route", "locale":"en", "status":"preferred"},
      {"text":"old-route", "locale":"en", "status":"deprecated"}
    ], "created_at":"2026-01-01T00:00:00Z", "updated_at":"2026-01-01T00:00:00Z"
  }]
}`)
	app := &App{}
	cmd := NewEnvCommand(t.Context(), "mcp")
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
	require.NoError(t, app.ResolveMCPProject(cmd))
	return app, root
}

func TestCheckTextMCPDestinationMatchesFileContext(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	text := "Utilize the risk-free old-route."
	// The server's cwd does not determine the draft's project-relative scope.
	t.Chdir(t.TempDir())
	for _, channel := range []string{"child", "adult"} {
		t.Run(channel, func(t *testing.T) {
			relative := channel + "/new-page.json"
			file := filepath.Join(root, filepath.FromSlash(relative))
			_, err := os.Stat(file)
			require.ErrorIs(t, err, os.ErrNotExist)
			_, draft, err := app.checkTextMCP(t.Context(), checkTextInput{Text: text, ContextPath: relative})
			require.NoError(t, err, "unwritten drafts still resolve collection and channel context")
			assert.Equal(t, "text", draft.Target.Kind)
			assert.Equal(t, relative, draft.Target.ContextPath)
			assert.Empty(t, draft.Target.File)
			assert.Positive(t, ruleCounts(draft)["voice.vocabulary"], "project terms must apply")
			if channel == "child" {
				assert.Positive(t, draft.Summary.Critical, "shared constraints survive the child presentation override")
			} else {
				assert.Zero(t, draft.Summary.Critical, "the adult scope's approved exception must apply")
			}
			for _, finding := range draft.Findings {
				assert.Empty(t, finding.Location.File, "a draft finding must not imply disk extraction")
			}
			for _, analyzer := range draft.Execution.Analyzers {
				assert.Empty(t, analyzer.File)
			}
			require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
			body, err := json.Marshal(map[string]string{"body": text})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(file, body, 0o600))
			_, saved, err := app.checkFileMCP(t.Context(), checkFileInput{File: file})
			require.NoError(t, err)
			assert.Equal(t, ruleCounts(saved), ruleCounts(draft))
			assert.Equal(t, saved.Summary, draft.Summary)
			assert.Equal(t, saved.Gate, draft.Gate)
			assert.Empty(t, saved.Target.ContextPath)
			assert.Equal(t, scopedFindingMeaning(saved), scopedFindingMeaning(draft))
		})
	}
}

// Locations differ between an in-memory block and an extracted JSON block;
// compare the rules, severities and messages rather than masking them entirely.
func scopedFindingMeaning(report check.Report) []check.Diagnostic {
	findings := append([]check.Diagnostic{}, report.Findings...)
	for i := range findings {
		findings[i].Location = check.Location{}
	}
	return findings
}

func TestCheckTextMCPUnscopedAndExplicitProfileRemainAvailable(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	_, report, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Utilize the risk-free old-route."})
	require.NoError(t, err)
	assert.Empty(t, report.Target.ContextPath)
	assert.Empty(t, report.Findings, "unscoped snippets do not implicitly select project guidance")
	override := filepath.Join(root, "override.yaml")
	profileBody := "name: Explicit\nvocabulary:\n  forbidden_terms:\n" +
		"    - term: override-only\n      severity: critical\n"
	require.NoError(t, os.WriteFile(override, []byte(profileBody), 0o600))
	_, report, err = app.checkTextMCP(t.Context(), checkTextInput{Text: "An override-only phrase.", ProfileFile: override})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Summary.Critical)
	_, report, err = app.checkTextMCP(t.Context(), checkTextInput{Text: "Hello.", ProfilePack: "marketing-blog"})
	require.NoError(t, err)
	assert.Equal(t, "kapi.check/v1", report.Schema)
}

func TestCheckTextMCPDestinationValidationPrecedesOverrides(t *testing.T) {
	isolateCheckExecution(t)
	for _, input := range []checkTextInput{
		{ContextPath: "child/new.json", ProfileFile: "missing.yaml"},
		{ContextPath: "child/new.json", ProfilePack: "missing-pack"},
	} {
		_, report, err := (&App{}).checkTextMCP(t.Context(), input)
		require.ErrorContains(t, err, "cannot be combined")
		assert.Empty(t, report.Schema)
	}
	for _, path := range []string{"/absolute.json", "../outside.json", "child/../adult/page.json", ".", `child\page.json`, "child/\x00.json"} {
		t.Run(path, func(t *testing.T) {
			_, report, err := (&App{}).checkTextMCP(t.Context(), checkTextInput{ContextPath: path})
			require.ErrorContains(t, err, "project-relative")
			assert.Empty(t, report.Schema)
		})
	}
	_, report, err := (&App{}).checkTextMCP(t.Context(), checkTextInput{ContextPath: "child/new.json"})
	require.ErrorContains(t, err, "requires a project")
	assert.Empty(t, report.Schema)
}

func TestCheckTextMCPDestinationFailsClosed(t *testing.T) {
	for _, unavailable := range []string{"kapi.yaml", ".kapi/voice.yaml", ".kapi/terms.json"} {
		t.Run(unavailable, func(t *testing.T) {
			app, root := scopedTextCheckFixture(t)
			require.NoError(t, os.WriteFile(filepath.Join(root, unavailable), []byte("invalid: ["), 0o600))
			_, report, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Ready.", ContextPath: "child/new.json"})
			require.Error(t, err, "bound context faults cannot silently omit a gate")
			assert.Empty(t, report.Schema)
			assert.False(t, report.Pass)
		})
	}
}

func TestCheckTextMCPDestinationThroughProtocol(t *testing.T) {
	app, _ := scopedTextCheckFixture(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerCheckMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "draft-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "check_text", Arguments: map[string]any{
			"text": "A risk-free appointment.", "context_path": "child/draft.json",
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	body, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	var report check.Report
	require.NoError(t, json.Unmarshal(body, &report))
	assert.Equal(t, "child/draft.json", report.Target.ContextPath)
	assert.Equal(t, "text", report.Target.Kind)
	assert.Empty(t, report.Target.File)
	assert.Positive(t, report.Summary.Critical)
}
