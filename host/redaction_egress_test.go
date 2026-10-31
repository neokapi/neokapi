//go:build !js

package host

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// Under a redaction policy the rest of a request is redacted as the block
// is: the instruction, the term rules, the voice profile, the neighbouring
// blocks and anything else a tool sends beside the block. Every test below
// records each request the demo provider is sent and fails if one holds the
// name the policy withholds.

// egressVoice is a voice profile whose guidance and example name the person.
func egressVoice() *coreprofile.VoiceProfile {
	return &coreprofile.VoiceProfile{
		Name: "egress",
		Tone: coreprofile.ToneProfile{Formality: "formal", Guidelines: "Greet Ada Lovelace by her full name."},
		Examples: []coreprofile.VoiceExample{{
			Before: "Hi Ada Lovelace", After: "Dear Ada Lovelace", Explanation: "Ada Lovelace is addressed formally.",
		}},
	}
}

// egressTermRules are term rules whose term or replacement holds the name.
func egressTermRules() []any {
	return []any{
		map[string]any{"term": "Write", "replacement": "Contact Ada Lovelace"},
		map[string]any{"term": "Ada Lovelace", "replacement": "Ada Lovelace", "note": "Ada Lovelace keeps her name."},
	}
}

// execRun is `kapi exec <name> <files>` in the project at recipe with config.
func execRun(a *App, name, recipe string, config map[string]any, target string, files ...string) ToolRunConfig {
	return ToolRunConfig{
		ToolName: name, Project: recipe, Files: files, TargetLang: target, DefaultLayout: target != "",
		ToolConfig: config, NewTool: demoTool(a, name, config, target),
	}
}

func TestToolRun_RedactsTheRestOfTheRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, a *App, recipe string)
	}{
		{"translate with an instruction and term rules", func(t *testing.T, a *App, recipe string) {
			root := filepath.Dir(recipe)
			config := map[string]any{
				"provider": "demo", "instruction": "Address Ada Lovelace formally.", "term_rules": egressTermRules(),
			}
			require.NoError(t, a.RunToolOnFiles(t.Context(), execRun(a, "translate", recipe, config, "fr", filepath.Join(root, "docs", "page.json"))))
			out, err := os.ReadFile(filepath.Join(root, "docs", "fr", "page.json"))
			require.NoError(t, err)
			assert.Contains(t, string(out), redactedName, "the translation carries the name again")
			assert.NotContains(t, string(out), "REDACTED", "no token is left in the translation")
		}},
		{"translate in a batch with neighbours", func(t *testing.T, a *App, recipe string) {
			root := filepath.Dir(recipe)
			page := filepath.Join(root, "docs", "page.json")
			require.NoError(t, os.WriteFile(page, []byte(`{"a": "Write to Ada Lovelace today.", "b": "Thank her twice.", "c": "Ada Lovelace replies."}`+"\n"), 0o600))
			config := map[string]any{
				"provider": "demo", "context": "neighbours", "batchSize": 2,
				"instruction": "Address Ada Lovelace formally.", "term_rules": egressTermRules(),
			}
			require.NoError(t, a.RunToolOnFiles(t.Context(), execRun(a, "translate", recipe, config, "fr", page)))
		}},
		{"review with a voice profile and term rules", func(t *testing.T, a *App, recipe string) {
			root := filepath.Dir(recipe)
			xlf := filepath.Join(root, "docs", "page.xlf")
			require.NoError(t, os.WriteFile(xlf, []byte(redactedXLIFF), 0o600))
			config := map[string]any{"provider": "demo", "profile": egressVoice(), "term_rules": egressTermRules()}
			require.NoError(t, a.RunToolOnFiles(t.Context(), execRun(a, "review", recipe, config, "fr", xlf)))
		}},
		{"voice-check with a voice profile", func(t *testing.T, a *App, recipe string) {
			root := filepath.Dir(recipe)
			config := map[string]any{"provider": "demo", "profile": egressVoice()}
			require.NoError(t, a.RunToolOnFiles(t.Context(), execRun(a, "voice-check", recipe, config, "", filepath.Join(root, "docs", "page.json"))))
		}},
		{"term-extract with a domain", func(t *testing.T, a *App, recipe string) {
			root := filepath.Dir(recipe)
			config := map[string]any{"provider": "demo", "domain": "letters to Ada Lovelace"}
			require.NoError(t, a.RunToolOnFiles(t.Context(), execRun(a, "term-extract", recipe, config, "", filepath.Join(root, "docs", "page.json"))))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateKapiEnv(t)
			onlyDemoProvider(t)
			recipe := redactProject(t, redactionYAML)
			sent := exchangesSent(t)
			a := &App{SourceLang: "en"}
			a.InitRegistries()
			tc.run(t, a, recipe)
			requireNoCallHolds(t, sent())
		})
	}
}

// The model reads one token for one value: the instruction shows the name as
// the block does, as its placeholder tag.
func TestToolRun_OneValueIsOneTokenAcrossTheRequest(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML)
	saw := providerSaw(t)
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	config := map[string]any{"provider": "demo", "instruction": "Address Ada Lovelace formally."}
	require.NoError(t, a.RunToolOnFiles(t.Context(), execRun(a, "translate", recipe, config, "fr", filepath.Join(filepath.Dir(recipe), "docs", "page.json"))))
	sent := saw()
	assert.Contains(t, sent, `Write to <x id="rdx1/"/> today.`)
	assert.Contains(t, sent, `Address <x id="rdx1/"/> formally.`)
}

// A block that withholds nothing still carries the policy, so the term rules
// and instruction sent with it are redacted too.
func TestToolRun_RedactsTheRequestOfABlockThatWithholdsNothing(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML)
	root := filepath.Dir(recipe)
	page := filepath.Join(root, "docs", "page.json")
	require.NoError(t, os.WriteFile(page, []byte(`{"greeting": "Write to her today."}`+"\n"), 0o600))
	sent := exchangesSent(t)
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	config := map[string]any{"provider": "demo", "instruction": "Address Ada Lovelace formally.", "term_rules": egressTermRules()}
	require.NoError(t, a.RunToolOnFiles(t.Context(), execRun(a, "translate", recipe, config, "fr", page)))
	calls := sent()
	requireNoCallHolds(t, calls)
	assert.Contains(t, strings.Join(calls, "\n"), "Address [REDACTED:Person] formally.")
}

// The MCP tools that run one registry tool redact what the caller passes
// beside the text.
func TestFrameworkMCPHandler_RedactsTheRestOfTheRequest(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML)
	app := &App{}
	app.InitRegistries()
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"translate", map[string]any{"instruction": "Address Ada Lovelace formally."}},
		{"review", map[string]any{"target": "Écrivez à Ada Lovelace aujourd'hui.", "term_rules": egressTermRules()}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			info := app.ToolReg.ToolInfo(registry.ToolID(tc.tool))
			require.NotNil(t, info)
			handler := app.frameworkMCPHandler(registry.ToolID(tc.tool), "", targetPolicyFor(*info))
			sent := exchangesSent(t)
			args := map[string]any{"text": "Write to Ada Lovelace today.", "target_lang": "fr", "provider": "demo", "project": recipe}
			maps.Copy(args, tc.args)
			raw, err := json.Marshal(args)
			require.NoError(t, err)
			_, err = handler(context.Background(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: raw}})
			require.NoError(t, err)
			requireNoCallHolds(t, sent())
		})
	}
}

// egressFlowProject writes a project under a redaction policy whose
// default flow redacts, translates with an instruction and term rules that
// name the person, and restores.
func egressFlowProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	recipe := `version: v1
name: egress-flow
defaults:
  source_language: en
  target_languages: [fr]
  flow: secure
  redaction:
    enabled: true
    rules: redact.yaml
    detectors: [rules]
collections:
  - name: app
    path: app/en.json
    target: "app/{lang}.json"
flows:
  secure:
    steps:
      - tool: redact
        config:
          rules:
            - term: Ada Lovelace
              category: person
      - tool: translate
        config:
          provider: demo
          instruction: Address Ada Lovelace formally.
          term_rules:
            - term: Write
              replacement: Contact Ada Lovelace
      - tool: unredact
`
	writeFixtureFile(t, root, project.RecipeFileName, recipe)
	writeFixtureFile(t, root, "redact.yaml", redactRules)
	writeFixtureFile(t, root, "app/en.json", `{"greeting": "Write to Ada Lovelace today.", "other": "Thank her."}`)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o700))
	return filepath.Join(root, project.RecipeFileName)
}

// `kapi up` runs the project's flow, and a step of it redacts the rest of
// its request as a run of one tool does.
func TestUp_AFlowStepRedactsTheRestOfTheRequest(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := egressFlowProject(t)
	sent := exchangesSent(t)
	app := &App{SourceLang: "en"}
	_, err := app.RunUp(t.Context(), recipe, "", UpOptions{MaxPasses: 1, NoChecks: true})
	require.NoError(t, err)
	requireNoCallHolds(t, sent())
}

// A built-in flow (`kapi run secure-translate`) takes its redact rules and
// translate settings from the recipe's tool presets, and its translate step
// redacts the instruction it is given.
func TestBuiltInFlow_SecureTranslateRedactsTheRestOfTheRequest(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	root := t.TempDir()
	writeFixtureFile(t, root, project.RecipeFileName, `version: v1
name: egress-builtin
defaults:
  source_language: en
  target_languages: [fr]
  redaction:
    enabled: true
    rules: redact.yaml
  tools:
    redact:
      rules:
        - term: Ada Lovelace
          category: person
    translate:
      provider: demo
      instruction: Address Ada Lovelace formally.
collections:
  - name: app
    path: app/en.json
    target: "app/{lang}.json"
`)
	writeFixtureFile(t, root, "redact.yaml", redactRules)
	writeFixtureFile(t, root, "app/en.json", `{"greeting": "Write to Ada Lovelace today."}`)
	recipe := filepath.Join(root, project.RecipeFileName)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	a := &App{SourceLang: "en"}
	a.InitRegistries()
	a.ProjectContext = project.NewProjectContext(proj, recipe)
	cmd := NewEnvCommand(t.Context(), "run")
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	input := filepath.Join(root, "app", "en.json")
	tools, cleanup, err := a.BuildFlowTools("secure-translate", input, "en", "fr", cmd)
	require.NoError(t, err)
	defer cleanup()

	sent := exchangesSent(t)
	block := model.NewBlock("greeting", "Write to Ada Lovelace today.")
	block.SourceLocale = "en"
	parts, err := runChain(t.Context(), tools, []*model.Part{{Type: model.PartBlock, Resource: block}})
	require.NoError(t, err)
	requireNoCallHolds(t, sent())
	require.NotEmpty(t, parts)
	got := parts[len(parts)-1].Resource.(*model.Block)
	assert.Contains(t, got.TargetText("fr"), redactedName, "unredact restores the name")
}

// The deeper look sends the domain a recipe preset names beside each chunk,
// and the voice-infer corpus reads each withheld value as its token.
func TestDeeperLook_RedactsTheRestOfTheRequest(t *testing.T) {
	useDemoProvider(t)
	app, _ := contextOpsApp(t)
	root := suggestProject(t, "deeperegress",
		"  redaction:\n    enabled: true\n    rules: redact.yaml\n    detectors: [rules]\n"+
			"  tools:\n    term-extract:\n      domain: Dockyard records\n    voice-infer:\n      domain: Dockyard records\n",
		deeperLookFiles)
	require.NoError(t, os.WriteFile(filepath.Join(root, "redact.yaml"),
		[]byte("version: v1\nrules:\n  - term: Dockyard\n    category: place\n"), 0o600))

	sent := exchangesSent(t)
	_, err := app.DeeperLook(context.Background(), DeeperLookRequest{Project: recipeOf(root), DryRun: true})
	require.NoError(t, err)
	calls := sent()
	require.NotEmpty(t, calls)
	for i, c := range calls {
		assert.NotContains(t, c, "Dockyard", "call %d of %d", i+1, len(calls))
	}
	all := strings.Join(calls, "\n")
	// The chunk withholds the name three times, each under a token of its
	// own, and the domain takes the first.
	assert.Contains(t, all, "Open the [REDACTED:Place]#1 to see your boats.", "the model reads a token where the value was")
	assert.Contains(t, all, "in the [REDACTED:Place]#1 records domain", "the domain shows the chunk's token")
}
