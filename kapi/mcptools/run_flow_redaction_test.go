package mcptools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// A project flow run over MCP redacts the file it runs over under the policy
// that governs that file. Here only the legal collection declares one, and
// the flow's redact step names no rules, so the provider never reads the
// name the collection withholds.
func TestHandleRunFlowRedactsUnderTheFilesCollectionPolicy(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	for _, vars := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "MISTRAL_API_KEY"} {
		t.Setenv(vars, "")
	}

	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write(project.RecipeFileName, "version: v1\nname: mcp-redact\ndefaults:\n  source_language: en\n  target_languages: [fr]\n"+
		"flows:\n  secure:\n    steps:\n      - tool: redact\n      - tool: translate\n        config:\n          provider: demo\n      - tool: unredact\n"+
		"collections:\n"+
		"  - name: legal\n    redaction:\n      enabled: true\n      rules: legal.yaml\n      detectors: [rules]\n    content:\n      - path: \"legal/*.json\"\n"+
		"  - name: docs\n    content:\n      - path: \"docs/*.json\"\n")
	write("legal.yaml", "version: v1\nrules:\n  - term: Project Nightfall\n    category: product\n")
	write("legal/memo.json", `{"line": "Ada Lovelace leads Project Nightfall."}`+"\n")

	var (
		mu   sync.Mutex
		sent strings.Builder
	)
	remove := aiprovider.AddRecorder(func(_ context.Context, ex aiprovider.Exchange) {
		mu.Lock()
		defer mu.Unlock()
		for _, m := range ex.Messages {
			sent.WriteString(m.Text())
			sent.WriteString("\n")
		}
	})
	t.Cleanup(remove)

	a := testApp()
	_, _, err := handleRunFlow(t.Context(), a, RunFlowInput{
		FlowName:   "secure",
		Project:    filepath.Join(dir, project.RecipeFileName),
		TargetLang: "fr",
		Path:       filepath.Join(dir, "legal", "memo.json"),
		OutputPath: filepath.Join(t.TempDir(), "memo.fr.json"),
	})
	require.NoError(t, err)

	mu.Lock()
	got := sent.String()
	mu.Unlock()
	require.Contains(t, got, "Ada Lovelace leads", "the run asked the provider about the block")
	assert.NotContains(t, got, "Project Nightfall", "the legal collection's policy withholds the name")
}
