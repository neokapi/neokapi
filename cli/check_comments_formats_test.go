package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/host/output"
)

// commentFormatFiles are files of formats that supply their comments, each
// holding one comment with a doubled word, with the block that comment becomes
// and the line it sits on.
var commentFormatFiles = []struct {
	format, path, body, block string
	line                      int
}{
	{"androidxml", "res/values/strings.xml", "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n  <!-- Greets the the reader. -->\n  <string name=\"greeting\">Hello world</string>\n</resources>\n", "comment/resources/string[greeting]", 3},
	{"resx", "Resources.resx", "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<root>\n  <!-- Greets the the reader. -->\n  <data name=\"Greeting\" xml:space=\"preserve\">\n    <value>Hello world</value>\n  </data>\n</root>\n", "comment/root/data[Greeting]", 3},
	{"xml", "config/app.xml", "<root>\n  <!-- Greets the the reader. -->\n  <greeting>Hello world</greeting>\n</root>\n", "comment/root/greeting", 2},
	{"html", "site/index.html", "<!DOCTYPE html>\n<html>\n<body>\n  <!-- Greets the the reader. -->\n  <p id=\"greeting\">Hello world</p>\n</body>\n</html>\n", "comment/p[greeting]", 4},
	{"markdown", "docs/guide.md", "# Guide\n\n<!-- Greets the the reader. -->\n\nHello world.\n", "comment/guide", 3},
	{"mdx", "docs/page.mdx", "# Guide\n\n{/* Greets the the reader. */}\n\nHello world.\n", "comment/guide", 3},
	{"po", "locale/messages.po", "# Greets the the reader.\nmsgid \"Hello world\"\nmsgstr \"\"\n", "comment/Hello world", 1},
	{"properties", "i18n/messages.properties", "# Greets the the reader.\ngreeting.text=Hello world\n", "comment/greeting.text", 1},
}

// commentFormatsProject writes an isolated project declaring each file of
// commentFormatFiles in a source-only collection, with `comments:` set to
// comments, and returns the recipe path.
func commentFormatsProject(t *testing.T, comments bool) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("KAPI_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", filepath.Join(dir, "plugins"))

	root := filepath.Join(dir, "project")
	var recipe strings.Builder
	recipe.WriteString("version: v1\nname: comment-formats\ndefaults:\n  source_language: en\ncollections:\n")
	for _, f := range commentFormatFiles {
		path := filepath.Join(root, filepath.FromSlash(f.path))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(f.body), 0o600))
		fmt.Fprintf(&recipe, "  - name: %s\n    source_only: true\n    content:\n      - path: %q\n        format: %s\n        comments: %t\n", f.format, f.path, f.format, comments)
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe.String()), 0o600))
	return filepath.Join(root, "kapi.yaml")
}

// runCheckJSON runs `kapi check --project <recipe> --json` through the command
// tree and decodes the report it prints.
func runCheckJSON(t *testing.T, recipe string) check.Report {
	t.Helper()
	a := &App{}
	root := &cobra.Command{Use: "kapi"}
	AddCommandGroups(a, root)
	output.AddPersistentFlags(root.PersistentFlags())
	root.AddCommand(NewCheckCmd(a))
	root.SetArgs([]string{"check", "--project", recipe, "--json"})
	root.SilenceUsage, root.SilenceErrors = true, true
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	require.NoError(t, root.Execute(), "kapi check failed: %s", out.String())
	var report check.Report
	require.NoError(t, json.Unmarshal(out.Bytes(), &report), out.String())
	return report
}

// TestCheck_DeclaredCommentsInMarkupFormats is `kapi check` over a project that
// declares the comments of files in formats that supply their comments.
func TestCheck_DeclaredCommentsInMarkupFormats(t *testing.T) {
	t.Run("each format's comment is checked and located", func(t *testing.T) {
		report := runCheckJSON(t, commentFormatsProject(t, true))
		assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)

		for _, f := range commentFormatFiles {
			i := slices.IndexFunc(report.Findings, func(d check.Diagnostic) bool {
				return d.Rule == "hygiene.doubled-word" && strings.HasSuffix(filepath.ToSlash(d.Location.File), "/"+f.path)
			})
			require.GreaterOrEqual(t, i, 0, "no finding on the comment in %s: %+v", f.path, report.Findings)
			d := report.Findings[i]
			assert.Equal(t, f.block, d.Location.Block)
			require.NotNil(t, d.Location.Lines)
			assert.Equal(t, f.line, d.Location.Lines.First)
		}

		caught := map[string]check.CanaryStatus{}
		for _, run := range report.Execution.Analyzers {
			if run.Canary != nil {
				caught[run.ID] = run.Canary.Status
			}
		}
		for _, f := range commentFormatFiles {
			assert.Equal(t, check.CanaryCaught, caught["comments."+f.format], f.format)
		}
	})

	t.Run("must fail: without comments: true no comment is checked", func(t *testing.T) {
		report := runCheckJSON(t, commentFormatsProject(t, false))
		for _, d := range report.Findings {
			assert.NotEqual(t, "hygiene.doubled-word", d.Rule, "%s on %s", d.Location.File, d.Location.Block)
		}
		assert.Positive(t, report.Target.Blocks, "the readers' blocks were checked")
	})
}
