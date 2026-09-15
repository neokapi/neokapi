package host

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	fmtpkg "github.com/neokapi/neokapi/core/format"
)

// rewriteTS is oxfmt-clean TypeScript with a doubled word in the line comment
// on parse, on line 1, and a JSDoc block on read.
const rewriteTS = "// Parses the the input.\nexport function parse(src: string): string {\n  return src;\n}\n\n/**\n * Reads the config.\n */\nexport function read(): void {}\n"

// repoOxfmt is the oxfmt this repository formats TypeScript with, installed by
// `vp install` at the repository root.
func repoOxfmt(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "node_modules", ".bin", "oxfmt"))
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.NoError(t, err, "the TypeScript comment rewrite is held to oxfmt; run `vp install` at the repository root")
	return path
}

// runOxfmt formats src as the file at path.
func runOxfmt(t *testing.T, path string, src []byte) []byte {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), repoOxfmt(t), "--stdin-filepath="+path)
	cmd.Dir = filepath.Dir(path)
	cmd.Stdin = bytes.NewReader(src)
	out, err := cmd.Output()
	require.NoError(t, err)
	return out
}

// tsRewriteApp returns an App reading comments through the sourcecode plugin
// built from this repository, with the oxfmt its manifest declares for each
// writable language run from this repository's node_modules, and execution
// trust granted to formatters through KAPI_TRUST_EXEC. edit, when set, changes
// each language's rewrite declaration further.
func tsRewriteApp(t *testing.T, edit func(rewrite map[string]any)) *App {
	t.Helper()
	oxfmt := repoOxfmt(t)
	a := sourcecodeApp(t, func(l map[string]any) {
		rewrite, ok := l["rewrite"].(map[string]any)
		if !ok {
			return
		}
		for _, f := range rewrite["formatters"].([]any) {
			f := f.(map[string]any)
			if f["name"] == "oxfmt" {
				f["command"].([]any)[0] = oxfmt
			}
		}
		if edit != nil {
			edit(rewrite)
		}
	})
	t.Setenv(execTrustEnvVar, "1")
	return a
}

// tsProject writes rewriteTS into a project formatted by oxfmt and returns the
// file's path. files adds files beside it.
func tsProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{".oxfmtrc.json": "{}\n", "parse.ts": rewriteTS} {
		if _, set := files[name]; !set {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
		}
	}
	for name, body := range files {
		if body == "" {
			continue
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	return filepath.Join(dir, "parse.ts")
}

// pluginCommentEntry builds a comment entry guarded by the fingerprint the
// comment id has in file, as the plugin locates it.
func pluginCommentEntry(t *testing.T, a *App, file, id, text string) map[string]any {
	t.Helper()
	p, ok := a.commentProviderFor(file)
	require.True(t, ok, "no comment provider reads %s", file)
	src, err := os.ReadFile(file)
	require.NoError(t, err)
	located, err := p.Locate(file, src)
	require.NoError(t, err)
	for i, b := range located.Blocks() {
		if b.ID == id {
			return map[string]any{"kind": "comment", "file": file, "id": id, "text": text, "comment_sha256": comment.Fingerprint(src, located.Comments[i])}
		}
	}
	require.Failf(t, "no comment", "%s holds no comment %s", file, id)
	return nil
}

// applyWith runs kapi apply through a over entries written as a change-set.
func applyWith(t *testing.T, a *App, entries ...map[string]any) (applyOutput, error) {
	t.Helper()
	var lines []string
	for _, e := range entries {
		b, err := json.Marshal(e)
		require.NoError(t, err)
		lines = append(lines, string(b))
	}
	path := filepath.Join(t.TempDir(), "changeset.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	cmd := NewEnvCommand(t.Context(), "apply")
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := a.RunApply(cmd, path, false, "", true)
	var out applyOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), stdout.String()+stderr.String())
	return out, err
}

// A comment in a language the sourcecode plugin declares writable is rewritten
// through kapi apply: the host renders it in the comment's layout, the plugin
// reads the rewritten file, and oxfmt must agree with it. A rewrite whose
// formatter did not run writes nothing.
//
// The subtests named "must fail" break the write path on purpose and assert
// that the run notices.
func TestPluginCommentRewrite(t *testing.T) {
	t.Run("the fixture is oxfmt-clean", func(t *testing.T) {
		file := tsProject(t, nil)
		assert.Equal(t, rewriteTS, string(runOxfmt(t, file, []byte(rewriteTS))))
	})

	t.Run("a finding in a line comment is repaired and oxfmt agrees", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := tsProject(t, nil)
		before, err := a.ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		finding := findingOf(t, before, "hygiene.doubled-word")
		require.Equal(t, "func/parse", finding.Location.Block)
		require.Equal(t, &fmtpkg.LineRange{First: 1, Last: 1}, finding.Location.Lines)

		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, "func/parse", "Parses the input."))
		require.NoError(t, err)
		require.Len(t, out.Comments, 1)
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentWritten, edit.Status, edit.Detail)
		require.NotNil(t, out.Comments[0].Check, out.Comments[0].CheckError)
		assert.Equal(t, check.VerdictPassed, out.Comments[0].Check.Verdict, out.Comments[0].Check.DidNotRun)
		assert.NotContains(t, rulesOf(out.Comments[0].Check), "hygiene.doubled-word", "the repair cleared the finding")

		want := strings.Replace(rewriteTS, "the the", "the", 1)
		assertUnchanged(t, file, want)
		assert.Equal(t, want, string(runOxfmt(t, file, []byte(want))), "oxfmt agrees with the written file")
	})

	t.Run("a JSDoc block keeps its layout", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := tsProject(t, nil)
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, "func/read", "Reads the whole config.\n\nIt stops at the end."))
		require.NoError(t, err)
		assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status, out.Comments[0].Edits[0].Detail)
		assertUnchanged(t, file, strings.Replace(rewriteTS, "/**\n * Reads the config.\n */", "/**\n * Reads the whole config.\n *\n * It stops at the end.\n */", 1))
	})

	t.Run("text holding */ and text that makes a directive are refused and write nothing", func(t *testing.T) {
		for _, tc := range []struct{ id, text, reason string }{
			{"func/read", "Reads the config */ and returns.", string(comment.RefusedTerminator)},
			{"func/parse", "eslint-disable-next-line no-console", string(comment.RefusedDirective)},
		} {
			a := tsRewriteApp(t, nil)
			file := tsProject(t, nil)
			out, err := applyWith(t, a, pluginCommentEntry(t, a, file, tc.id, tc.text))
			assert.Equal(t, ExitGate, ExitCode(nil, err))
			edit := out.Comments[0].Edits[0]
			assert.Equal(t, commentRefused, edit.Status, tc.text)
			assert.Equal(t, tc.reason, edit.Reason, edit.Detail)
			assertUnchanged(t, file, rewriteTS)
		}
	})

	t.Run("must fail: a file no formatter is configured for did not run", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := tsProject(t, map[string]string{".oxfmtrc.json": ""})
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, "func/parse", "Parses the input."))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Equal(t, string(comment.RefusedFormatter), edit.Reason)
		assert.Contains(t, edit.Detail, "no formatter for typescript is configured")
		assertUnchanged(t, file, rewriteTS)
	})

	t.Run("must fail: a formatter set to ignore the file did not run", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := tsProject(t, map[string]string{".prettierignore": "parse.ts\n"})
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, "func/parse", "Parses the input."))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Equal(t, string(comment.RefusedFormatter), edit.Reason)
		assert.Contains(t, edit.Detail, "leaves a file at")
		assertUnchanged(t, file, rewriteTS)
	})

	// A formatter matches its ignore files against resolved paths, so the same
	// file reached through a linked directory must still be read as ignored.
	t.Run("must fail: a formatter set to ignore a file reached through a link did not run", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := tsProject(t, map[string]string{".prettierignore": "parse.ts\n"})
		link := filepath.Join(t.TempDir(), "linked")
		require.NoError(t, os.Symlink(filepath.Dir(file), link))
		linked := filepath.Join(link, "parse.ts")
		out, err := applyWith(t, a, pluginCommentEntry(t, a, linked, "func/parse", "Parses the input."))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Equal(t, string(comment.RefusedFormatter), edit.Reason)
		assert.Contains(t, edit.Detail, "leaves a file at")
		assertUnchanged(t, file, rewriteTS)
	})

	t.Run("must fail: a formatter that is not installed did not run", func(t *testing.T) {
		a := tsRewriteApp(t, func(rewrite map[string]any) {
			for _, f := range rewrite["formatters"].([]any) {
				f.(map[string]any)["command"] = []any{filepath.Join(t.TempDir(), "no-such-oxfmt")}
			}
		})
		file := tsProject(t, nil)
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, "func/parse", "Parses the input."))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Contains(t, edit.Detail, "is not installed")
		assertUnchanged(t, file, rewriteTS)
	})

	t.Run("must fail: a write canary whose refused text is prose invalidates the run", func(t *testing.T) {
		a := tsRewriteApp(t, func(rewrite map[string]any) {
			rewrite["canary"].(map[string]any)["refused"] = "Parses the input carefully."
		})
		file := tsProject(t, nil)
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, "func/parse", "Parses the input."))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Equal(t, reasonCanary, edit.Reason, edit.Detail)
		assertUnchanged(t, file, rewriteTS)
	})

	t.Run("must fail: a host renderer that writes code after the comment is refused by the plugin's reading", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := tsProject(t, nil)
		p, ok := a.commentProviderFor(file)
		require.True(t, ok)
		broken := injectingPluginRenderer{Provider: p, Rewriter: p.(comment.Rewriter)}
		_, err := comment.Rewrite(broken, file, []byte(rewriteTS), nil, comment.Target{ID: "func/parse"}, "Parses the input.", comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, comment.RefusedContainment, refusal.Reason, refusal.Detail)
	})

	t.Run("must fail: a host renderer that misindents a comment is refused by oxfmt", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := tsProject(t, nil)
		p, ok := a.commentProviderFor(file)
		require.True(t, ok)
		broken := misindentingPluginRenderer{Provider: p, Rewriter: p.(comment.Rewriter)}
		held := &formattedRewriter{Provider: broken, Rewriter: broken, commentFormatter: resolveCommentFormatter(p, file, p.(commentRewriteDeclarer).Rewrite(), newFormatterTrust(true))}
		_, err := comment.Rewrite(held, file, []byte(rewriteTS), nil, comment.Target{ID: "func/parse"}, "Parses the input.\nIt stops at the end.", comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, comment.RefusedFormatter, refusal.Reason, refusal.Detail)
	})
}

// injectingPluginRenderer renders like the plugin language's rewriter and then
// writes a declaration after the comment, inside the bytes it returns.
type injectingPluginRenderer struct {
	comment.Provider
	comment.Rewriter
}

func (p injectingPluginRenderer) Render(name string, src []byte, c comment.Comment, text string, opts comment.RenderOptions) ([]byte, error) {
	span, err := p.Rewriter.Render(name, src, c, text, opts)
	return append(span, "\nexport const injected = 1;"...), err
}

// misindentingPluginRenderer renders like the plugin language's rewriter and
// then indents every line after the first by four spaces.
type misindentingPluginRenderer struct {
	comment.Provider
	comment.Rewriter
}

func (p misindentingPluginRenderer) Render(name string, src []byte, c comment.Comment, text string, opts comment.RenderOptions) ([]byte, error) {
	span, err := p.Rewriter.Render(name, src, c, text, opts)
	return bytes.ReplaceAll(span, []byte("\n"), []byte("\n    ")), err
}
