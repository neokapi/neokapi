//go:build !js

package host

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// exchangesSent records every call a provider is sent while the test runs,
// each as the whole exchange (messages and schema) without the provider's
// answer, which is the provider's and not what it read.
func exchangesSent(t *testing.T) func() []string {
	t.Helper()
	var (
		mu    sync.Mutex
		calls []string
	)
	remove := aiprovider.AddRecorder(func(_ context.Context, ex aiprovider.Exchange) {
		ex.Response = ""
		raw, err := json.Marshal(ex)
		if err != nil {
			raw = []byte(err.Error())
		}
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, string(raw))
	})
	t.Cleanup(remove)
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

// kpzOverlays returns every overlay a .kpz workspace's cache keeps, joined.
func kpzOverlays(t *testing.T, a *App, work string) string {
	t.Helper()
	c, err := a.ensureKpzCache(t.Context(), work)
	require.NoError(t, err)
	var all strings.Builder
	for _, src := range c.meta.Sources {
		store, err := sqlitestore.New(c.storePath(src.Key))
		require.NoError(t, err)
		sess, err := store.Begin(t.Context())
		require.NoError(t, err)
		enum, ok := sess.(blockstore.OverlayEnumerator)
		require.True(t, ok, "the workspace store enumerates its overlays")
		for ov, err := range enum.AllOverlays() {
			require.NoError(t, err)
			all.WriteString(ov.Kind + " " + string(ov.Payload) + "\n")
		}
		require.NoError(t, sess.Close())
		require.NoError(t, store.Close())
	}
	return all.String()
}

// requireNoCallHolds fails when no provider call was made, or when any call
// carries the name the policy withholds.
func requireNoCallHolds(t *testing.T, calls []string) {
	t.Helper()
	require.NotEmpty(t, calls, "the route asked the provider")
	for i, c := range calls {
		assert.NotContains(t, c, "Lovelace", "call %d of %d", i+1, len(calls))
	}
}

func demoTool(a *App, name string, config map[string]any, target string) func() (tool.Tool, error) {
	return func() (tool.Tool, error) {
		return a.ToolReg.NewToolWithConfig(registry.ToolID(name), maps.Clone(config), target)
	}
}

const redactedXLIFF = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="page" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Write to Ada Lovelace today.</source>
        <target state="translated">Écrivez à Ada Lovelace aujourd'hui.</target>
      </trans-unit>
    </body>
  </file>
</xliff>
`

// redactedARB names the person inside each plural form and in the text after
// the plural, where a carve at the wrong offset would split the name.
const redactedARB = `{
  "@@locale": "en",
  "sent": "{count, plural, one{# file for Ada Lovelace} other{# files for Ada Lovelace}} sent to Ada Lovelace today.",
  "@sent": {"placeholders": {"count": {"type": "int"}}}
}
`

const redactedMarkdown = "# Contacts\n\nWrite to Ada Lovelace today.\n\n```yaml\ncontact: Ada Lovelace\n```\n"

// Every route a run of one tool takes sends the provider the redacted blocks:
// the source, the blocks marked not translatable, and the translations.
func TestToolRun_RedactionRoutes(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, a *App, recipe string)
	}{
		{"a code fence term-extract reads", func(t *testing.T, a *App, recipe string) {
			md := filepath.Join(filepath.Dir(recipe), "docs", "contacts.md")
			require.NoError(t, os.WriteFile(md, []byte(redactedMarkdown), 0o600))
			config := map[string]any{"provider": "demo"}
			require.NoError(t, a.RunToolOnFiles(t.Context(), ToolRunConfig{
				ToolName: "term-extract", Project: recipe, Files: []string{md},
				ToolConfig: config, NewTool: demoTool(a, "term-extract", config, ""),
			}))
		}},
		{"a plural term-extract reads", func(t *testing.T, a *App, recipe string) {
			arb := filepath.Join(filepath.Dir(recipe), "docs", "app_en.arb")
			require.NoError(t, os.WriteFile(arb, []byte(redactedARB), 0o600))
			config := map[string]any{"provider": "demo"}
			require.NoError(t, a.RunToolOnFiles(t.Context(), ToolRunConfig{
				ToolName: "term-extract", Project: recipe, Files: []string{arb},
				ToolConfig: config, NewTool: demoTool(a, "term-extract", config, ""),
			}))
		}},
		{"a plural translated and written back", func(t *testing.T, a *App, recipe string) {
			root := filepath.Dir(recipe)
			arb := filepath.Join(root, "docs", "app_en.arb")
			require.NoError(t, os.WriteFile(arb, []byte(redactedARB), 0o600))
			cfg := translateRun(a, recipe, arb)
			require.NoError(t, a.RunToolOnFiles(t.Context(), cfg))
			out, err := os.ReadFile(filepath.Join(root, "docs", "app_fr.arb"))
			require.NoError(t, err)
			assert.Equal(t, 3, strings.Count(string(out), redactedName), "each form and the text after the plural carry the name again: %s", out)
		}},
		{"voice-infer's draft from the run's blocks", func(t *testing.T, a *App, recipe string) {
			md := filepath.Join(filepath.Dir(recipe), "docs", "contacts.md")
			require.NoError(t, os.WriteFile(md, []byte(redactedMarkdown), 0o600))
			config := map[string]any{"provider": "demo"}
			newTool := demoTool(a, "voice-infer", config, "")
			require.NoError(t, a.RunToolOnFiles(t.Context(), ToolRunConfig{
				ToolName: "voice-infer", Project: recipe, Files: []string{md, filepath.Join(filepath.Dir(recipe), "docs", "page.json")},
				ToolConfig: config, NewTool: newTool,
				NewCollector: NewVoiceInferCollectorFor("voice-infer", newTool), JSONOutput: true,
			}))
		}},
		{"qa in AI mode over a source and its translation", func(t *testing.T, a *App, recipe string) {
			root := filepath.Dir(recipe)
			target := filepath.Join(root, "docs", "fr.json")
			require.NoError(t, os.WriteFile(target, []byte(`{"greeting": "Écrivez à Ada Lovelace aujourd'hui."}`+"\n"), 0o600))
			config := map[string]any{"mode": "ai", "provider": "demo"}
			require.NoError(t, a.RunToolOnFiles(t.Context(), ToolRunConfig{
				ToolName: "qa", Project: recipe, Files: []string{filepath.Join(root, "docs", "page.json")},
				TargetFile: target, TargetLang: "fr",
				ToolConfig: config, NewTool: demoTool(a, "qa", config, "fr"),
			}))
		}},
		{"review over a file holding the translation", func(t *testing.T, a *App, recipe string) {
			root := filepath.Dir(recipe)
			xlf := filepath.Join(root, "docs", "page.xlf")
			require.NoError(t, os.WriteFile(xlf, []byte(redactedXLIFF), 0o600))
			config := map[string]any{"provider": "demo"}
			out := filepath.Join(root, "out", "page.xlf")
			require.NoError(t, a.RunToolOnFiles(t.Context(), ToolRunConfig{
				ToolName: "review", Project: recipe, Files: []string{xlf}, TargetLang: "fr",
				OutputTemplate: out, ToolConfig: config, NewTool: demoTool(a, "review", config, "fr"),
			}))
			written, err := os.ReadFile(out)
			require.NoError(t, err)
			assert.Contains(t, string(written), "<target state=\"translated\">Écrivez à Ada Lovelace aujourd'hui.</target>",
				"the translation is written back as it was read")
		}},
		{"a .kpz workspace transformed in place", func(t *testing.T, a *App, recipe string) {
			t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
			dir := t.TempDir()
			src := filepath.Join(dir, "messages.json")
			require.NoError(t, os.WriteFile(src, []byte(redactedPage), 0o600))
			work := filepath.Join(dir, "work.kpz")
			a.Quiet = true
			require.NoError(t, a.ExtractToKpz(t.Context(), []string{src}, work, "fr", "", false))
			config := map[string]any{"provider": "demo"}
			require.NoError(t, a.RunToolOnFiles(t.Context(), ToolRunConfig{
				ToolName: "translate", Project: recipe, Files: []string{work}, TargetLang: "fr",
				ToolConfig: config, NewTool: demoTool(a, "translate", config, "fr"),
			}))
			out := t.TempDir()
			require.NoError(t, a.MergeFromKpz(t.Context(), work, out))
			var merged []string
			require.NoError(t, filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					b, rerr := os.ReadFile(p)
					merged = append(merged, string(b))
					return rerr
				}
				return err
			}))
			require.NotEmpty(t, merged, "the merge wrote a translation")
			assert.Contains(t, strings.Join(merged, "\n"), "⟦fr⟧", "the merge wrote the translation")
			assert.Contains(t, strings.Join(merged, "\n"), redactedName, "the merged translation carries the name again")
		}},
		{"a .kpz translation review leaves as it was", func(t *testing.T, a *App, recipe string) {
			t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
			dir := t.TempDir()
			xlf := filepath.Join(dir, "page.xlf")
			require.NoError(t, os.WriteFile(xlf, []byte(strings.Replace(redactedXLIFF, `state="translated"`, `state="final"`, 1)), 0o600))
			work := filepath.Join(dir, "work.kpz")
			a.Quiet = true
			require.NoError(t, a.ExtractToKpz(t.Context(), []string{xlf}, work, "fr", "", false))
			config := map[string]any{"provider": "demo"}
			require.NoError(t, a.RunToolOnFiles(t.Context(), ToolRunConfig{
				ToolName: "review", Project: recipe, Files: []string{work}, TargetLang: "fr",
				ToolConfig: config, NewTool: demoTool(a, "review", config, "fr"),
			}))
			out := t.TempDir()
			require.NoError(t, a.MergeFromKpz(t.Context(), work, out))
			var merged []string
			require.NoError(t, filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					b, rerr := os.ReadFile(p)
					merged = append(merged, string(b))
					return rerr
				}
				return err
			}))
			all := strings.Join(merged, "\n")
			assert.Contains(t, all, `state="final"`, "the translation keeps the state it was read with: %s", all)
			assert.Contains(t, all, "Écrivez à Ada Lovelace aujourd'hui.")

			// What the workspace keeps of the translation: no draft, and no
			// edition redact claims to have produced.
			stored := kpzOverlays(t, a, work)
			require.NotEmpty(t, stored)
			assert.NotContains(t, stored, `"tool":"redact"`)
			assert.NotContains(t, stored, `"tool":"unredact"`)
			assert.NotContains(t, stored, `"draft"`)
		}},
		{"an entry inside a zip archive", func(t *testing.T, a *App, recipe string) {
			root := filepath.Dir(recipe)
			zpath := filepath.Join(root, "docs", "bundle.zip")
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			w, err := zw.Create("inner/page.json")
			require.NoError(t, err)
			_, err = w.Write([]byte(redactedPage))
			require.NoError(t, err)
			require.NoError(t, zw.Close())
			require.NoError(t, os.WriteFile(zpath, buf.Bytes(), 0o600))
			cfg := translateRun(a, recipe, zpath)
			cfg.DefaultLayout = false
			out := filepath.Join(root, "out", "bundle.fr.zip")
			cfg.OutputTemplate = out
			require.NoError(t, a.RunToolOnFiles(t.Context(), cfg))
			zr, err := zip.OpenReader(out)
			require.NoError(t, err)
			defer zr.Close()
			require.Len(t, zr.File, 1)
			rc, err := zr.File[0].Open()
			require.NoError(t, err)
			var entry bytes.Buffer
			_, err = entry.ReadFrom(rc)
			require.NoError(t, err)
			require.NoError(t, rc.Close())
			assert.Contains(t, entry.String(), redactedName, "the translated entry carries the name again")
		}},
		{"a run cancelled after its first call", func(t *testing.T, a *App, recipe string) {
			page := filepath.Join(filepath.Dir(recipe), "docs", "page.json")
			var sb strings.Builder
			sb.WriteString("{")
			for i := range 20 {
				if i > 0 {
					sb.WriteString(",")
				}
				fmt.Fprintf(&sb, `"k%d":"Block %d to Ada Lovelace."`, i, i)
			}
			sb.WriteString("}\n")
			require.NoError(t, os.WriteFile(page, []byte(sb.String()), 0o600))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var once sync.Once
			remove := aiprovider.AddRecorder(func(context.Context, aiprovider.Exchange) { once.Do(cancel) })
			defer remove()
			config := map[string]any{"provider": "demo", "batching": "single"}
			cfg := translateRun(a, recipe, page)
			cfg.NewTool = demoTool(a, "translate", config, "fr")
			_ = a.RunToolOnFiles(ctx, cfg)
			src, err := os.ReadFile(page)
			require.NoError(t, err)
			assert.Equal(t, sb.String(), string(src), "a cancelled run leaves the source as it was")
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

// The MCP tools that read a translation the caller gives send it redacted.
func TestFrameworkMCPHandler_RedactsTheTargetItIsGiven(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML)
	app := &App{}
	app.InitRegistries()
	for _, name := range []string{"review", "qa"} {
		t.Run(name, func(t *testing.T) {
			info := app.ToolReg.ToolInfo(registry.ToolID(name))
			require.NotNil(t, info)
			handler := app.frameworkMCPHandler(registry.ToolID(name), "", targetPolicyFor(*info))
			sent := exchangesSent(t)
			args, err := json.Marshal(map[string]any{
				"text": "Write to Ada Lovelace today.", "target": "Écrivez à Ada Lovelace aujourd'hui.",
				"target_lang": "fr", "provider": "demo", "mode": "ai", "project": recipe,
			})
			require.NoError(t, err)
			res, err := handler(context.Background(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}})
			require.NoError(t, err)
			requireNoCallHolds(t, sent())
			out, ok := res.StructuredContent.(*frameworkToolOutput)
			require.True(t, ok)
			assert.Contains(t, out.Targets["fr"], redactedName, "the translation comes back as the caller gave it")
			assert.NotContains(t, out.Annotations, "redaction.secret")
		})
	}
}

// A policy that detects entities needs an extraction step a run of one tool
// does not have, so the run is refused before any provider is asked.
func TestToolRun_RefusesEntityDetection(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	for name, yaml := range map[string]string{
		"entities only":    "  redaction:\n    enabled: true\n    detectors: [entities]\n",
		"rules + entities": "  redaction:\n    enabled: true\n    rules: redact.yaml\n    detectors: [rules, entities]\n",
	} {
		t.Run(name, func(t *testing.T) {
			recipe := redactProject(t, yaml)
			sent := exchangesSent(t)
			a := &App{SourceLang: "en"}
			a.InitRegistries()
			err := a.RunToolOnFiles(t.Context(), translateRun(a, recipe, filepath.Join(filepath.Dir(recipe), "docs", "page.json")))
			require.ErrorContains(t, err, "detects entities")
			assert.Empty(t, sent(), "no provider is asked")
		})
	}
	t.Run("a tool that calls no provider still runs", func(t *testing.T) {
		recipe := redactProject(t, "  redaction:\n    enabled: true\n    detectors: [entities]\n")
		a := &App{SourceLang: "en"}
		a.InitRegistries()
		cfg := upperRun(a, filepath.Join(filepath.Dir(recipe), "docs", "page.json"))
		cfg.Project = recipe
		cfg.ToolConfig = map[string]any{"mode": "upper", "applySource": true}
		require.NoError(t, a.RunToolOnFiles(t.Context(), cfg))
	})
}
