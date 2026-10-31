package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The redacted name in every fixture below, and the rules file that withholds
// it.
const (
	redactedName  = "Ada Lovelace"
	redactRules   = "version: v1\nrules:\n  - term: Ada Lovelace\n    category: person\n"
	redactedPage  = `{"greeting": "Write to Ada Lovelace today."}` + "\n"
	redactionYAML = "  redaction:\n    enabled: true\n    rules: redact.yaml\n    detectors: [rules]\n"
)

// redactProject writes a project over docs/*.json holding docs/page.json,
// with the extra recipe block given under defaults, and returns its recipe.
func redactProject(t *testing.T, defaults string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write(project.RecipeFileName, "version: v1\nname: exec-redact\ndefaults:\n  source_language: en\n"+defaults+
		"collections:\n  - name: docs\n    content:\n      - path: \"docs/*.json\"\n")
	write("redact.yaml", redactRules)
	write("docs/page.json", redactedPage)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o700))
	return filepath.Join(root, project.RecipeFileName)
}

// onlyDemoProvider clears every provider key the environment could hold, so
// the demo provider a run names is the only one it can reach.
func onlyDemoProvider(t *testing.T) {
	t.Helper()
	for _, vars := range providerenv.Vars {
		for _, v := range vars {
			t.Setenv(v, "")
		}
	}
	t.Setenv("KAPI_AI_PROVIDER", "")
	t.Setenv("KAPI_AI_MODEL", "")
	aiprovider.SetDemoNoticeWriter(nopDemoNotice{})
}

type nopDemoNotice struct{}

func (nopDemoNotice) Write(p []byte) (int, error) { return len(p), nil }

// providerSaw records every message a provider is sent while the test runs.
func providerSaw(t *testing.T) func() string {
	t.Helper()
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
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		return sent.String()
	}
}

// translateRun is `kapi exec translate docs/page.json --target-lang fr
// --provider demo` in the project at recipe ("" for none).
func translateRun(a *App, recipe, file string) ToolRunConfig {
	config := map[string]any{"provider": "demo"}
	return ToolRunConfig{
		ToolName:      "translate",
		Project:       recipe,
		Files:         []string{file},
		DefaultLayout: true,
		TargetLang:    "fr",
		ToolConfig:    config,
		NewTool: func() (tool.Tool, error) {
			return a.ToolReg.NewToolWithConfig(registry.ToolID("translate"), map[string]any{"provider": "demo"}, "fr")
		},
	}
}

func TestToolRun_RedactsBeforeTheProviderReadsAndRestoresAfter(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML)
	root := filepath.Dir(recipe)
	saw := providerSaw(t)
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	page := filepath.Join(root, "docs", "page.json")
	require.NoError(t, a.RunToolOnFiles(t.Context(), translateRun(a, recipe, page)))

	sent := saw()
	require.NotEmpty(t, sent, "the run asked the provider")
	assert.NotContains(t, sent, redactedName, "the provider never reads the name the policy withholds")
	assert.Contains(t, sent, "Write to", "the provider reads the rest of the block")

	out, err := os.ReadFile(filepath.Join(root, "docs", "fr", "page.json"))
	require.NoError(t, err)
	assert.Contains(t, string(out), redactedName, "the translation carries the name again")
	src, err := os.ReadFile(page)
	require.NoError(t, err)
	assert.Equal(t, redactedPage, string(src), "the source file is left as it was")
}

func TestToolRun_TermExtractReadsTheRedactedBlocks(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML)
	saw := providerSaw(t)
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	config := map[string]any{"provider": "demo"}
	require.NoError(t, a.RunToolOnFiles(t.Context(), ToolRunConfig{
		ToolName:   "term-extract",
		Project:    recipe,
		Files:      []string{filepath.Join(filepath.Dir(recipe), "docs", "page.json")},
		ToolConfig: config,
		NewTool: func() (tool.Tool, error) {
			return a.ToolReg.NewToolWithConfig(registry.ToolID("term-extract"), map[string]any{"provider": "demo"}, "")
		},
	}))
	sent := saw()
	require.Contains(t, sent, "Write to", "the run asked the provider about the block")
	assert.NotContains(t, sent, redactedName)
}

func TestToolRun_LeavesAToolThatCallsNoProviderAsItWas(t *testing.T) {
	// case-transform calls no provider, so it reads the name and upper-cases
	// it; under redaction it would have read a placeholder, and the restored
	// name would come back as written.
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML)
	root := filepath.Dir(recipe)
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	page := filepath.Join(root, "docs", "page.json")
	cfg := upperRun(a, page)
	cfg.Project = recipe
	cfg.ToolConfig = map[string]any{"mode": "upper", "applySource": true}
	require.NoError(t, a.RunToolOnFiles(t.Context(), cfg))
	out, err := os.ReadFile(page)
	require.NoError(t, err)
	assert.Contains(t, string(out), strings.ToUpper(redactedName))
}

func TestToolRun_WithNoPolicySendsTheBlockAsItIs(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	for name, recipe := range map[string]func(t *testing.T) string{
		"a project with no policy": func(t *testing.T) string { return redactProject(t, "") },
		"a policy switched off": func(t *testing.T) string {
			return redactProject(t, "  redaction:\n    enabled: false\n    rules: redact.yaml\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := recipe(t)
			saw := providerSaw(t)
			a := &App{SourceLang: "en"}
			a.InitRegistries()
			require.NoError(t, a.RunToolOnFiles(t.Context(), translateRun(a, r, filepath.Join(filepath.Dir(r), "docs", "page.json"))))
			assert.Contains(t, saw(), redactedName)
		})
	}

	t.Run("outside a project", func(t *testing.T) {
		dir := t.TempDir()
		page := filepath.Join(dir, "page.json")
		require.NoError(t, os.WriteFile(page, []byte(redactedPage), 0o600))
		saw := providerSaw(t)
		a := &App{SourceLang: "en"}
		a.InitRegistries()
		require.NoError(t, a.RunToolOnFiles(t.Context(), translateRun(a, "", page)))
		assert.Contains(t, saw(), redactedName)
	})
}

func TestToolRun_RefusesAPolicyWithNoRules(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, "  redaction:\n    enabled: true\n")
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	err := a.RunToolOnFiles(t.Context(), translateRun(a, recipe, filepath.Join(filepath.Dir(recipe), "docs", "page.json")))
	require.ErrorContains(t, err, "no rules file is set")
}

// The chain a run applies is held to the rule a flow is held to: under a
// declared policy, a tool that sends source to a remote provider never runs
// without the redact step.
func TestToolRun_ChainCoverage(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	remote := map[string]any{"provider": "anthropic"}
	translate := &tool.BaseTool{ToolName: "translate"}

	alone := toolChain{main: translate}
	require.ErrorContains(t, a.checkChainCoverage("translate", remote, alone, true), "no redact step ahead of it",
		"a remote provider with no redact step under a policy fails as a flow does")
	require.NoError(t, a.checkChainCoverage("translate", remote, alone, false), "no policy, nothing to hold it to")
	require.Error(t, a.checkChainCoverage("translate", map[string]any{}, alone, true),
		"a provider the command leaves unset counts as remote")

	recipe := redactProject(t, redactionYAML)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	pol, err := a.toolRunPolicyFor(recipe, proj, "", "translate", remote)
	require.NoError(t, err)
	require.NotNil(t, pol.redaction, "translate calls a provider")
	c, err := a.chainFor(pol, "translate", remote, translate)
	require.NoError(t, err, "the chain a run builds passes the check")
	require.Len(t, c.before, 1)
	require.Len(t, c.after, 1)
	assert.Equal(t, "redact", c.before[0].Name())
	assert.Equal(t, "unredact", c.after[0].Name())

	pol, err = a.toolRunPolicyFor(recipe, proj, "", "whitespace-correct", nil)
	require.NoError(t, err)
	assert.True(t, pol.declared)
	assert.Nil(t, pol.redaction, "a tool that calls no provider runs alone")
}

// qa calls a provider in AI mode only, and the registry fills in the
// configured default provider only for a contract that needs credentials, so
// a configured provider leaves rules mode as it is.
func TestToolRun_QaToolCallsAProviderInAIModeOnly(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	t.Setenv("KAPI_AI_PROVIDER", "demo")
	recipe := redactProject(t, redactionYAML)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	a := &App{SourceLang: "en"}
	a.InitRegistries()

	for config, redacts := range map[string]bool{"": false, "rules": false, "ai": true} {
		cfg := map[string]any{}
		if config != "" {
			cfg["mode"] = config
		}
		pol, err := a.toolRunPolicyFor(recipe, proj, "", "qa", cfg)
		require.NoError(t, err)
		assert.Equal(t, redacts, pol.redaction != nil, "mode %q", config)

		built, err := a.ToolReg.NewToolWithConfig(registry.ToolID("qa"), cfg, "fr")
		require.NoError(t, err)
		_, judges := built.(interface{ TotalUsage() aiprovider.TokenUsage })
		assert.Equal(t, redacts, judges, "the tool the registry builds for mode %q", config)
	}
}
