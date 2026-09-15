package host

import (
	"bytes"
	"errors"
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

// sourcecodeRewriteFile is an oxfmt-clean file in one language the sourcecode
// plugin writes comments in. A directive sits on line 1, a line comment with a
// doubled word on line 2 sits on lineID, and a JSDoc block with a {@link}, a
// @param and a @returns tag sits on docID.
type sourcecodeRewriteFile struct {
	name, src     string
	lineID, docID string
	// docSentence is the first sentence of the JSDoc block, param and link its
	// @param line and its {@link} reference.
	docSentence, param, link string
	// minFiles and minComments are floors for the language's files and comments
	// in this repository, well below what it holds.
	minFiles, minComments int
}

var sourcecodeRewriteFiles = map[string]sourcecodeRewriteFile{
	"typescript": {
		name:   "parse.ts",
		src:    "// eslint-disable-next-line no-console\n// Parses the the input.\nexport function parse(src: string): string {\n  return src;\n}\n\n/**\n * Reads the config through {@link parse}.\n *\n * @param path - the file to read\n * @returns the config\n */\nexport function read(path: string): string {\n  return parse(path);\n}\n",
		lineID: "func/parse", docID: "func/read",
		docSentence: "Reads the config through {@link parse}.", param: "@param path - the file to read", link: "{@link parse}",
		minFiles: 500, minComments: 4000,
	},
	"tsx": {
		name:   "Greeting.tsx",
		src:    "// eslint-disable-next-line no-console\n// Renders the the greeting.\nexport function Greeting() {\n  return <p>Hello</p>;\n}\n\n/**\n * Renders the farewell through {@link Greeting}.\n *\n * @param name - the person leaving\n * @returns the farewell\n */\nexport function Farewell(name: string) {\n  return <p>Bye {name}</p>;\n}\n",
		lineID: "func/Greeting", docID: "func/Farewell",
		docSentence: "Renders the farewell through {@link Greeting}.", param: "@param name - the person leaving", link: "{@link Greeting}",
		minFiles: 1000, minComments: 4000,
	},
	"javascript": {
		name:   "config.js",
		src:    "// eslint-disable-next-line no-console\n// Reads the the config.\nexport function read() {\n  return 1;\n}\n\n/**\n * Writes the config through {@link read}.\n *\n * @param value - the value to write\n * @returns the value\n */\nexport function write(value) {\n  return value;\n}\n",
		lineID: "func/read", docID: "func/write",
		docSentence: "Writes the config through {@link read}.", param: "@param value - the value to write", link: "{@link read}",
		minFiles: 50, minComments: 200,
	},
}

// docText is the JSDoc block's text with its first sentence replaced.
func (f sourcecodeRewriteFile) docText(sentence string) string {
	lines := strings.Split(f.src, "\n")
	var text []string
	in := false
	for _, l := range lines {
		switch {
		case l == "/**":
			in = true
		case l == " */":
			in = false
		case in:
			text = append(text, strings.TrimPrefix(strings.TrimPrefix(l, " *"), " "))
		}
	}
	text[0] = sentence
	return strings.Join(text, "\n")
}

// rewriteProject writes f into a project formatted by oxfmt and returns its
// path. files adds files beside it, and an empty body removes one.
func rewriteProject(t *testing.T, f sourcecodeRewriteFile, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{".oxfmtrc.json": "{}\n", f.name: f.src} {
		if _, set := files[name]; !set {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
		}
	}
	for name, body := range files {
		if body != "" {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
		}
	}
	return filepath.Join(dir, f.name)
}

// repoCommentRewrites rewrites every comment in this repository's files in
// language with its own prose, through the provider a reads the language with:
// each file is located once, and each comment is read, rendered and held to
// comment.Contain, the steps comment.Rewrite takes. It returns how many files
// were located, how many the grammar could not place the comments of, how many
// comments rewrote to their own bytes, and every failure.
func repoCommentRewrites(t *testing.T, a *App, language string) (files, unlocated, rewrote int, failures []string) {
	t.Helper()
	var exts []string
	for _, r := range a.PluginHost.CommentRoutes() {
		if r.Language.Language == language {
			exts = r.Language.Extensions
		}
	}
	require.NotEmpty(t, exts, "no plugin reads %s comments", language)
	root, err := filepath.Abs("..")
	require.NoError(t, err)
	args := []string{"-C", root, "ls-files", "-z"}
	for _, ext := range exts {
		args = append(args, "*"+ext)
	}
	listing, err := exec.CommandContext(t.Context(), "git", args...).Output()
	require.NoError(t, err)
	for rel := range strings.SplitSeq(strings.TrimRight(string(listing), "\x00"), "\x00") {
		if rel == "" || strings.HasPrefix(rel, ".claude/") || strings.Contains(rel, "/node_modules/") {
			continue
		}
		file := filepath.Join(root, rel)
		p, ok := a.commentProviderFor(file)
		if !ok || p.Language() != language {
			continue
		}
		r, ok := p.(comment.Rewriter)
		require.True(t, ok, "comments in %s are not written", language)
		src, err := os.ReadFile(file)
		require.NoError(t, err)
		located, err := comment.Locate(p, file, src, nil)
		if errors.Is(err, comment.ErrUnlocated) {
			unlocated++
			continue
		}
		if err != nil {
			failures = append(failures, rel+": "+err.Error())
			continue
		}
		files++
		for i, c := range located.Comments {
			prose, err := r.Prose(src, c)
			if err == nil {
				var span []byte
				if span, err = r.Render(file, src, c, prose, comment.RenderOptions{}); err == nil {
					after := bytes.Join([][]byte{src[:c.Start], span, src[c.End:]}, nil)
					var written *comment.Rewritten
					if written, err = comment.Contain(p, file, src, after, nil, located, i); err == nil && (written.Changed || !bytes.Equal(written.Source, src)) {
						err = errors.New("rewriting the comment with its own prose changed the file")
					}
				}
			}
			if err != nil {
				failures = append(failures, rel+": "+err.Error())
				continue
			}
			rewrote++
		}
	}
	return files, unlocated, rewrote, failures
}

// proseP3Sourcecode is the P3 rung for a language whose comments the sourcecode
// plugin reads and kapi writes: a comment is rewritten through `kapi apply`
// with every other byte of the file identical, the plugin reads the result, and
// oxfmt agrees. JSDoc tags and links stay, directives stay unaddressable, text
// holding `*/` is refused, and every comment of the language in this repository
// rewrites to its own bytes. An edit with no plugin installed, or no formatter
// configured, did not run.
//
// The subtests named "must fail" break the write path on purpose and assert
// that the run notices. Nothing in here skips.
func proseP3Sourcecode(t *testing.T, language string) {
	f := sourcecodeRewriteFiles[language]
	refusedAs := func(t *testing.T, a *App, file string, entry map[string]any, reason string) {
		t.Helper()
		out, err := applyWith(t, a, entry)
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentRefused, edit.Status, entry["text"])
		assert.Equal(t, reason, edit.Reason, edit.Detail)
		assertUnchanged(t, file, f.src)
	}

	t.Run("the fixture is oxfmt-clean", func(t *testing.T) {
		file := rewriteProject(t, f, nil)
		require.Equal(t, f.src, string(runOxfmt(t, file, []byte(f.src))))
	})

	t.Run("a finding in a line comment is repaired through kapi apply and oxfmt agrees", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := rewriteProject(t, f, nil)
		before, err := a.ComputeCheck(executionCommand(t), []string{file})
		require.NoError(t, err)
		finding := findingOf(t, before, "hygiene.doubled-word")
		require.Equal(t, f.lineID, finding.Location.Block)
		require.Equal(t, &fmtpkg.LineRange{First: 2, Last: 2}, finding.Location.Lines)

		repaired := strings.TrimPrefix(strings.Replace(strings.Split(f.src, "\n")[1], "the the", "the", 1), "// ")
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, f.lineID, repaired))
		require.NoError(t, err)
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentWritten, edit.Status, edit.Detail)
		require.NotNil(t, out.Comments[0].Check, out.Comments[0].CheckError)
		assert.Equal(t, check.VerdictPassed, out.Comments[0].Check.Verdict, out.Comments[0].Check.DidNotRun)
		assert.NotContains(t, rulesOf(out.Comments[0].Check), "hygiene.doubled-word")

		want := strings.Replace(f.src, "the the", "the", 1)
		assertUnchanged(t, file, want)
		assert.Equal(t, want, string(runOxfmt(t, file, []byte(want))), "oxfmt agrees with the written file")
	})

	t.Run("a JSDoc block keeps its layout, its tags and its link", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := rewriteProject(t, f, nil)
		sentence := strings.Replace(f.docSentence, " the ", " the whole ", 1)
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, f.docID, f.docText(sentence)))
		require.NoError(t, err)
		assert.Equal(t, commentWritten, out.Comments[0].Edits[0].Status, out.Comments[0].Edits[0].Detail)
		want := strings.Replace(f.src, f.docSentence, sentence, 1)
		assertUnchanged(t, file, want)
		assert.Equal(t, want, string(runOxfmt(t, file, []byte(want))), "oxfmt agrees with the written file")
	})

	t.Run("text that drops a tag or a link is refused as structure", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		for name, text := range map[string]string{
			"a dropped @param": strings.Replace(f.docText(f.docSentence), "\n"+f.param, "", 1),
			"a dropped link":   f.docText(strings.Replace(f.docSentence, f.link, "it", 1)),
		} {
			file := rewriteProject(t, f, nil)
			require.NotEqual(t, f.docText(f.docSentence), text, name)
			refusedAs(t, a, file, pluginCommentEntry(t, a, file, f.docID, text), string(comment.RefusedStructure))
		}
	})

	t.Run("text holding */ in the JSDoc block is refused", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := rewriteProject(t, f, nil)
		refusedAs(t, a, file, pluginCommentEntry(t, a, file, f.docID, f.docText(strings.Replace(f.docSentence, " the ", " the */ ", 1))), string(comment.RefusedTerminator))
	})

	t.Run("directives stay unaddressable", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := rewriteProject(t, f, nil)
		refusedAs(t, a, file, pluginCommentEntry(t, a, file, f.lineID, "eslint-disable-next-line no-console"), string(comment.RefusedDirective))

		byLines := rewriteProject(t, f, nil)
		refusedAs(t, a, byLines, map[string]any{
			"kind": "comment", "file": byLines, "id": "comment/eslint", "text": "Nothing to see.",
			"lines": fmtpkg.LineRange{First: 1, Last: 1}, "comment_sha256": staleFingerprint,
		}, string(comment.RefusedDirective))
	})

	t.Run("every comment in this repository rewrites to its own bytes", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		files, unlocated, rewrote, failures := repoCommentRewrites(t, a, language)
		t.Logf("rewrite-corpus %s files=%d unlocated=%d comments=%d", language, files, unlocated, rewrote)
		assert.Empty(t, failures)
		assert.GreaterOrEqual(t, files, f.minFiles, "the corpus read too few %s files", language)
		assert.GreaterOrEqual(t, rewrote, f.minComments, "the corpus rewrote too few %s comments", language)
	})

	t.Run("an edit with no plugin installed did not run", func(t *testing.T) {
		isolateCheckExecution(t)
		a := &App{SourceLang: "en"}
		a.InitRegistries()
		a.InitPluginHost()
		t.Cleanup(a.Shutdown)
		file := rewriteProject(t, f, nil)
		out, err := applyWith(t, a, map[string]any{"kind": "comment", "file": file, "id": f.lineID, "text": "Anything.", "comment_sha256": staleFingerprint})
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Equal(t, reasonNoReader, edit.Reason, edit.Detail)
		assert.Contains(t, edit.Detail, "kapi plugins install sourcecode")
		assertUnchanged(t, file, f.src)
	})

	t.Run("an edit with no formatter configured did not run", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := rewriteProject(t, f, map[string]string{".oxfmtrc.json": ""})
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, f.lineID, "Anything."))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Equal(t, string(comment.RefusedFormatter), edit.Reason, edit.Detail)
		assertUnchanged(t, file, f.src)
	})

	t.Run("must fail: an edit whose project formatter no one allowed did not run", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		t.Setenv(execTrustEnvVar, "")
		a.isTTY = func() bool { return false }
		file := rewriteProject(t, f, nil)
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, f.lineID, "Anything."))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Equal(t, string(comment.RefusedFormatter), edit.Reason, edit.Detail)
		assert.Contains(t, edit.Detail, "no one has allowed it to run")
		assertUnchanged(t, file, f.src)
	})

	t.Run("must fail: a formatter set to ignore the file did not run", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := rewriteProject(t, f, map[string]string{".prettierignore": f.name + "\n"})
		out, err := applyWith(t, a, pluginCommentEntry(t, a, file, f.lineID, "Anything."))
		assert.Equal(t, ExitGate, ExitCode(nil, err))
		edit := out.Comments[0].Edits[0]
		assert.Equal(t, commentNotRun, edit.Status)
		assert.Contains(t, edit.Detail, "leaves a file at")
		assertUnchanged(t, file, f.src)
	})

	t.Run("must fail: a host renderer that writes code after the comment is refused by the plugin's reading", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := rewriteProject(t, f, nil)
		p, ok := a.commentProviderFor(file)
		require.True(t, ok)
		broken := injectingPluginRenderer{Provider: p, Rewriter: p.(comment.Rewriter)}
		_, err := comment.Rewrite(broken, file, []byte(f.src), nil, comment.Target{ID: f.lineID}, "Anything.", comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, comment.RefusedContainment, refusal.Reason, refusal.Detail)
	})
}

func TestProseP3_typescript(t *testing.T) { proseP3Sourcecode(t, "typescript") }

func TestProseP3_tsx(t *testing.T) { proseP3Sourcecode(t, "tsx") }

func TestProseP3_javascript(t *testing.T) { proseP3Sourcecode(t, "javascript") }
