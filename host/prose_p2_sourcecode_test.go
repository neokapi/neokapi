package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
)

// sourcecodePlugin builds the sourcecode plugin from this repository once per
// test process, into one fixed directory that later runs overwrite. A build that
// fails fails every test that needs the plugin.
var sourcecodePlugin = sync.OnceValues(func() (string, error) {
	src, err := filepath.Abs(filepath.Join("..", "plugins", "sourcecode"))
	if err != nil {
		return "", err
	}
	bin := filepath.Join(os.TempDir(), "kapi-host-test-plugins", "kapi-sourcecode")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", bin, "./cmd/kapi-sourcecode")
	build.Dir = src
	build.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=1")
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build the sourcecode plugin: %w\n%s", err, out)
	}
	// A new binary's first run can be slow while the platform assesses it. Pay
	// that here, outside a daemon's startup budget.
	if out, err := exec.CommandContext(context.Background(), bin, "version").CombinedOutput(); err != nil {
		return "", fmt.Errorf("run the sourcecode plugin: %w\n%s", err, out)
	}
	return bin, nil
})

// sourcecodeApp returns an App that discovers the sourcecode plugin built from
// this repository and nothing else. edit, when set, changes each comment
// language the staged manifest declares.
func sourcecodeApp(t *testing.T, edit func(language map[string]any)) *App {
	t.Helper()
	isolateCheckExecution(t)
	bin, err := sourcecodePlugin()
	require.NoError(t, err)
	dir := filepath.Join(os.Getenv("KAPI_PLUGINS_DIR"), "sourcecode")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "formats", "sourcecode"), 0o755))
	if err := os.Link(bin, filepath.Join(dir, "kapi-sourcecode")); err != nil {
		data, rerr := os.ReadFile(bin)
		require.NoError(t, rerr)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi-sourcecode"), data, 0o755))
	}
	plugin := filepath.Join("..", "plugins", "sourcecode")
	data, err := os.ReadFile(filepath.Join(plugin, "manifest.json"))
	require.NoError(t, err)
	if edit != nil {
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		for _, l := range m["capabilities"].(map[string]any)["comments"].([]any) {
			edit(l.(map[string]any))
		}
		data, err = json.Marshal(m)
		require.NoError(t, err)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644))
	schema, err := os.ReadFile(filepath.Join(plugin, "formats", "sourcecode", "schema.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "formats", "sourcecode", "schema.json"), schema, 0o644))

	a := &App{SourceLang: "en"}
	a.InitRegistries()
	a.InitPluginHost()
	t.Cleanup(a.Shutdown)
	require.NotNil(t, a.PluginHost)
	require.NotEmpty(t, a.PluginHost.CommentRoutes(), "the staged plugin declares no comment language")
	return a
}

// sourcecodeFile is a commented file in one language the plugin reads.
type sourcecodeFile struct {
	// path is where the file sits in the project, and ext its extension.
	path, ext string
	// governed holds the prohibited word in a doc comment on line, for block,
	// and again in code, where it is not content.
	governed string
	block    string
	line     int
	// clean holds one doc comment with nothing to find, and doubled one with a
	// doubled word, on line 1.
	clean, doubled string
}

var sourcecodeFiles = map[string]sourcecodeFile{
	"typescript": {
		path: "src/parse.ts", ext: ".ts",
		governed: "// eslint-disable-next-line no-console\n/** Parse helps you utilize the input. */\nexport function parse(src: string): string {\n  return \"utilize\" + src;\n}\n",
		block:    "func/parse", line: 2,
		clean:   "/** Parses the input. */\nexport function parse(src: string): string {\n  return src;\n}\n",
		doubled: "/** Parses the the input. */\nexport function parse(src: string): string {\n  return src;\n}\n",
	},
	"tsx": {
		path: "ui/Greeting.tsx", ext: ".tsx",
		governed: "/** Greeting helps you utilize the page. */\nexport function Greeting() {\n  return <p>utilize {/* A comment inside JSX. */}</p>;\n}\n",
		block:    "func/Greeting", line: 1,
		clean:   "/** Renders the greeting. */\nexport function Greeting() {\n  return <p>Hello</p>;\n}\n",
		doubled: "/** Renders the the greeting. */\nexport function Greeting() {\n  return <p>Hello</p>;\n}\n",
	},
	"javascript": {
		path: "scripts/read.mjs", ext: ".mjs",
		governed: "#!/usr/bin/env node\n/** Read helps you utilize the config. */\nexport function read() {\n  return `utilize`;\n}\n",
		block:    "func/read", line: 2,
		clean:   "/** Reads the config. */\nexport function read() {\n  return 1;\n}\n",
		doubled: "/** Reads the the config. */\nexport function read() {\n  return 1;\n}\n",
	},
}

// sourcecodeProject is a project declaring the comments of one file in each
// language the plugin reads, under a voice that prohibits "utilize".
func sourcecodeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	var recipe strings.Builder
	recipe.WriteString("version: v1\nname: source-comments\ndefaults:\n  source_language: en\n  voice:\n    profile_file: .kapi/voice.yaml\ncollections:\n  - name: code\n    source_only: true\n    content:\n")
	for _, f := range sourcecodeFiles {
		fmt.Fprintf(&recipe, "      - path: %q\n        comments: true\n", f.path)
		write(f.path, f.governed)
	}
	write("kapi.yaml", recipe.String())
	write(".kapi/voice.yaml", `id: source-comments
name: Service
constraints:
  - id: service/plain-words
    version: 1
    source: service-guide.md
    statement: Say use rather than utilize.
    kind: prohibited_pattern
    regex: '(?i)\butilize\b'
`)
	return root
}

// namedSourceCheck checks one file in a directory of its own with a.
func namedSourceCheck(t *testing.T, a *App, name, src string) check.Report {
	t.Helper()
	file := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(file, []byte(src), 0o600))
	report, err := a.ComputeCheck(executionCommand(t), []string{file})
	require.NoError(t, err)
	return report
}

// proseP2Sourcecode is the P2 rung for a language whose comments the sourcecode
// plugin reads, through the plugin built from this repository: comments take
// part in `kapi check` at their file's point with their own lines, the plugin
// catches its canary on every run, a file with nothing to check or that cannot
// be read did not run, and a plugin that answers wrongly on its canary leaves
// the run invalid.
func proseP2Sourcecode(t *testing.T, language string) {
	f := sourcecodeFiles[language]
	analyzer := "comments." + language

	t.Run("a comment is held to the governance of its file's point", func(t *testing.T) {
		a := sourcecodeApp(t, nil)
		root := sourcecodeProject(t)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		report, err := a.ComputeCheck(cmd, nil)
		require.NoError(t, err)
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		assert.Positive(t, report.Target.Blocks)

		var voice []check.Diagnostic
		for _, d := range findingsOf(report, "voice") {
			if strings.HasSuffix(filepath.ToSlash(d.Location.File), f.path) {
				voice = append(voice, d)
			}
		}
		require.Len(t, voice, 1, "the doc comment is held to the rule, and the word in code is not content")
		assert.Equal(t, f.block, voice[0].Location.Block)
		require.NotNil(t, voice[0].Location.Lines)
		assert.Equal(t, format.LineRange{First: f.line, Last: f.line}, *voice[0].Location.Lines)

		run := analyzerRun(t, report, analyzer)
		assert.Equal(t, check.AnalyzerPassed, run.Status)
		require.NotNil(t, run.Canary)
		assert.Equal(t, check.CanaryCaught, run.Canary.Status)
	})

	t.Run("a named file's comments are read through the plugin, beside its canary", func(t *testing.T) {
		a := sourcecodeApp(t, nil)
		report := namedSourceCheck(t, a, "clean"+f.ext, f.clean)
		assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)
		assert.Equal(t, 1, report.Target.Blocks)
		run := analyzerRun(t, report, analyzer)
		assert.Equal(t, check.AnalyzerPassed, run.Status)
		require.NotNil(t, run.Canary)
		assert.Equal(t, check.CanaryCaught, run.Canary.Status)
		assert.Equal(t, check.AnalyzerUnsupported, analyzerRun(t, report, formatterCheck).Status)

		doubled := namedSourceCheck(t, a, "doubled"+f.ext, f.doubled)
		require.Len(t, doubled.Findings, 1)
		assert.Equal(t, "hygiene.doubled-word", doubled.Findings[0].Rule)
		assert.Equal(t, f.block, doubled.Findings[0].Location.Block)
		require.NotNil(t, doubled.Findings[0].Location.Lines)
		assert.Equal(t, format.LineRange{First: 1, Last: 1}, *doubled.Findings[0].Location.Lines)
	})

	t.Run("a file with no comment prose did not run", func(t *testing.T) {
		a := sourcecodeApp(t, nil)
		report := namedSourceCheck(t, a, "directives"+f.ext, "// eslint-disable-next-line no-console\nexport const answer = 42;\n")
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict, "zero comments checked is never a pass")
		assert.False(t, report.Pass)
	})

	t.Run("a file that does not parse did not run", func(t *testing.T) {
		a := sourcecodeApp(t, nil)
		report := namedSourceCheck(t, a, "broken"+f.ext, "/** Parses the input. */\nexport function (\n")
		run := analyzerRun(t, report, analyzer)
		assert.Equal(t, check.AnalyzerDidNotRun, run.Status)
		assert.Contains(t, run.Reason, "does not parse")
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("must fail: a plugin that misses its canary's comment invalidates the run", func(t *testing.T) {
		a := sourcecodeApp(t, func(l map[string]any) { l["canary"].(map[string]any)["block"] = "func/elsewhere" })
		report := namedSourceCheck(t, a, "clean"+f.ext, f.clean)
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, analyzer).Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, check.CauseCheckerInvalid, report.DidNotRunCause)
	})

	t.Run("must fail: a canary with no doubled word invalidates the run", func(t *testing.T) {
		a := sourcecodeApp(t, func(l map[string]any) {
			c := l["canary"].(map[string]any)
			c["source"] = strings.Replace(c["source"].(string), "the the", "the", 1)
		})
		report := namedSourceCheck(t, a, "clean"+f.ext, f.clean)
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, analyzer).Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("a diff-scoped check records the analyzers a whole-file check does", func(t *testing.T) {
		a := sourcecodeApp(t, nil)
		dir := t.TempDir()
		name := "doubled" + f.ext
		writeCheckInput(t, dir, name, f.doubled)
		t.Chdir(dir)
		firstLine := f.doubled[:strings.IndexByte(f.doubled, '\n')]
		patch := "--- a/" + name + "\n+++ b/" + name + "\n@@ -1 +1 @@\n-/** Before. */\n+" + firstLine + "\n"
		cmd := diffCommand(t)
		require.NoError(t, cmd.Flags().Set("diff-file", StdinName))
		cmd.SetIn(bytes.NewBufferString(patch))
		diff, err := a.ComputeCheck(cmd, nil)
		require.NoError(t, err)
		whole, err := a.ComputeCheck(executionCommand(t), []string{name})
		require.NoError(t, err)

		type analyzerOutcome struct {
			ID     string
			Status check.AnalyzerStatus
			Canary check.CanaryStatus
		}
		outcomes := func(report check.Report) []analyzerOutcome {
			require.NotNil(t, report.Execution)
			var out []analyzerOutcome
			for _, run := range report.Execution.Analyzers {
				if run.File != name || run.ID == "reader.validation" {
					continue
				}
				o := analyzerOutcome{ID: run.ID, Status: run.Status}
				if run.Canary != nil {
					o.Canary = run.Canary.Status
				}
				out = append(out, o)
			}
			return out
		}
		scoped := outcomes(diff)
		var ids []string
		for _, o := range scoped {
			ids = append(ids, o.ID)
		}
		assert.Subset(t, ids, []string{analyzer, formatterCheck, "hygiene"})
		assert.Equal(t, outcomes(whole), scoped)
		require.Len(t, findingsOf(diff, "hygiene"), 1, "the touched comment's doubled word is found")
	})
}

func TestProseP2_typescript(t *testing.T) {
	proseP2Sourcecode(t, "typescript")

	// The marker line holds the prohibited word, and the lines around it do not.
	const skip = "// Parse reads the input.\n// okapi-skip: utilize this later\n// It stops at the end.\nexport function parse(): void {}\n"
	declaredProject := func(t *testing.T, item string) string {
		t.Helper()
		root := t.TempDir()
		recipe := "version: v1\nname: declared\ndefaults:\n  source_language: en\n  voice:\n    profile_file: .kapi/voice.yaml\ncollections:\n  - name: code\n    source_only: true\n    content:\n" + item
		for name, body := range map[string]string{
			"kapi.yaml":        recipe,
			".kapi/voice.yaml": "id: declared\nname: Service\nconstraints:\n  - id: service/plain-words\n    version: 1\n    source: service-guide.md\n    statement: Say use rather than utilize.\n    kind: prohibited_pattern\n    regex: '(?i)\\butilize\\b'\n",
			"src/skip.ts":      skip,
		} {
			path := filepath.Join(root, filepath.FromSlash(name))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		}
		return root
	}
	checkProject := func(t *testing.T, root string) check.Report {
		t.Helper()
		a := sourcecodeApp(t, nil)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		report, err := a.ComputeCheck(cmd, nil)
		require.NoError(t, err)
		return report
	}

	t.Run("a directive the recipe declares is set aside through the plugin", func(t *testing.T) {
		report := checkProject(t, declaredProject(t, "      - path: \"src/*.ts\"\n        comments:\n          directives: [\"okapi-skip:\"]\n"))
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		assert.Empty(t, findingsOf(report, "voice"), "the declared marker line is not prose")
		assert.Equal(t, 2, report.Target.Blocks, "the comment splits around the marker")
		assert.Equal(t, check.AnalyzerPassed, analyzerRun(t, report, "comments.typescript").Status)
	})

	t.Run("must fail: undeclared, the marker line is prose", func(t *testing.T) {
		report := checkProject(t, declaredProject(t, "      - path: \"src/*.ts\"\n        comments: true\n"))
		require.Len(t, findingsOf(report, "voice"), 1)
		assert.Equal(t, 1, report.Target.Blocks)
	})
}

func TestProseP2_tsx(t *testing.T) { proseP2Sourcecode(t, "tsx") }

func TestProseP2_javascript(t *testing.T) { proseP2Sourcecode(t, "javascript") }
