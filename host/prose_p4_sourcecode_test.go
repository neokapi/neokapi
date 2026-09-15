package host

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
)

// sourcecodeFormRewrite rewrites one comment of a sourcecodeFormsFile and gives
// the bytes the rewritten comment must hold.
type sourcecodeFormRewrite struct {
	id, text, want string
	// delimited reports a /* */ comment, where text holding */ is refused.
	delimited bool
}

// sourcecodeFormsFile is an oxfmt-clean file holding every comment form a
// language's code in this repository writes, with a rewrite of each.
type sourcecodeFormsFile struct {
	name, src string
	rewrites  []sourcecodeFormRewrite
	// jsdoc names a multi-line JSDoc block, which the must-fail renderers break.
	jsdoc string
}

var sourcecodeFormsFiles = map[string]sourcecodeFormsFile{
	"typescript": {
		name: "forms.ts",
		src: "// Parses the input.\n// Stops at the end.\nexport function parse(src: string): string {\n  return src; // returns the input\n}\n\n" +
			"/* A plain comment. */\nexport const answer = 42;\n\n" +
			"/*\n * A starred comment\n * over two lines.\n */\nexport const limit = 10;\n\n" +
			"/** Reads the config. */\nexport function read(): void {}\n\n" +
			"/**\n * Writes the config.\n *\n * @param value - the value to write\n */\nexport function write(value: string): void {\n  console.log(value);\n}\n\n" +
			"/** Closes the reader\n * and releases it. */\nexport function close(): void {}\n",
		rewrites: []sourcecodeFormRewrite{
			{id: "func/parse", text: "Parses the whole input.\nStops at the end.", want: "// Parses the whole input.\n// Stops at the end."},
			{id: "func/parse/comment", text: "returns the same input", want: "// returns the same input"},
			{id: "const/answer", text: "A plainer comment.", want: "/* A plainer comment. */", delimited: true},
			{id: "const/limit", text: "A starred comment\nover three\nlines.", want: "/*\n * A starred comment\n * over three\n * lines.\n */", delimited: true},
			{id: "func/read", text: "Reads the whole config.", want: "/** Reads the whole config. */", delimited: true},
			{id: "func/write", text: "Writes the whole config.\n\n@param value - the value to write", want: "/**\n * Writes the whole config.\n *\n * @param value - the value to write\n */", delimited: true},
			{id: "func/close", text: "Closes the reader\nand releases it now.", want: "/** Closes the reader\n * and releases it now. */", delimited: true},
		},
		jsdoc: "func/write",
	},
	"tsx": {
		name: "forms.tsx",
		src: "// Renders the greeting.\nexport function Greeting() {\n  return (\n    <div>\n      {/* A comment inside JSX. */}\n      <p>Hello</p>\n      {/* A longer comment\n          inside JSX. */}\n    </div>\n  );\n}\n\n" +
			"/** Renders the farewell. */\nexport function Farewell() {\n  return <p>Bye</p>; // the last word\n}\n\n" +
			"/**\n * Renders the frame.\n *\n * @param title - the frame's title\n */\nexport function Frame(title: string) {\n  return <section>{title}</section>;\n}\n\n" +
			"/* A plain comment. */\nexport const size = 3;\n",
		rewrites: []sourcecodeFormRewrite{
			{id: "func/Greeting", text: "Renders the whole greeting.", want: "// Renders the whole greeting."},
			{id: "func/Greeting/comment", text: "A comment still inside JSX.", want: "/* A comment still inside JSX. */", delimited: true},
			{id: "func/Greeting/comment#2", text: "A longer comment\ninside JSX now.", want: "/* A longer comment\n          inside JSX now. */", delimited: true},
			{id: "func/Farewell", text: "Renders the last farewell.", want: "/** Renders the last farewell. */", delimited: true},
			{id: "func/Farewell/comment", text: "the very last word", want: "// the very last word"},
			{id: "func/Frame", text: "Renders the whole frame.\n\n@param title - the frame's title", want: "/**\n * Renders the whole frame.\n *\n * @param title - the frame's title\n */", delimited: true},
			{id: "const/size", text: "A plainer comment.", want: "/* A plainer comment. */", delimited: true},
		},
		jsdoc: "func/Frame",
	},
	"javascript": {
		name: "forms.js",
		src: "// Reads the config.\n// Stops at the end.\nexport function read() {\n  return 1; // always one\n}\n\n" +
			"/* A plain comment. */\nexport const size = 3;\n\n" +
			"/** Writes the config. */\nexport function write() {}\n\n" +
			"/**\n * Closes the reader.\n *\n * @param reader - the reader to close\n */\nexport function close(reader) {\n  return reader;\n}\n",
		rewrites: []sourcecodeFormRewrite{
			{id: "func/read", text: "Reads the whole config.\nStops at the end.", want: "// Reads the whole config.\n// Stops at the end."},
			{id: "func/read/comment", text: "always exactly one", want: "// always exactly one"},
			{id: "const/size", text: "A plainer comment.", want: "/* A plainer comment. */", delimited: true},
			{id: "func/write", text: "Writes the whole config.", want: "/** Writes the whole config. */", delimited: true},
			{id: "func/close", text: "Closes the reader now.\n\n@param reader - the reader to close", want: "/**\n * Closes the reader now.\n *\n * @param reader - the reader to close\n */", delimited: true},
		},
		jsdoc: "func/close",
	},
}

// formsProject writes f into a project formatted by oxfmt and returns its path.
func formsProject(t *testing.T, f sourcecodeFormsFile) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".oxfmtrc.json"), []byte("{}\n"), 0o644))
	file := filepath.Join(dir, f.name)
	require.NoError(t, os.WriteFile(file, []byte(f.src), 0o644))
	return file
}

// heldRewriter returns the provider a reads file with, held to the formatter
// the file's project uses, as kapi apply holds it.
func heldRewriter(t *testing.T, a *App, file string) (comment.Provider, *formattedRewriter) {
	t.Helper()
	p, ok := a.commentProviderFor(file)
	require.True(t, ok, "no comment provider reads %s", file)
	held, ok := withCommentFormatter(p, file, newFormatterTrust(true)).(*formattedRewriter)
	require.True(t, ok, "comments in %s are not held to a formatter", p.Language())
	require.Empty(t, held.notRun)
	return p, held
}

// indexOfBlock returns the position in f.Comments of the comment id names.
func indexOfBlock(t *testing.T, f *comment.File, id string) int {
	t.Helper()
	for i, b := range f.Blocks() {
		if b.ID == id {
			return i
		}
	}
	require.Failf(t, "no comment", "the file holds no comment %s", id)
	return -1
}

// proseP4Sourcecode is the P4 rung for a language whose comments the
// sourcecode plugin reads and kapi writes: every comment form the language's
// code uses is rewritten in the layout it is written in, with exact bytes, the
// plugin reading the result and oxfmt agreeing with the whole file. Text
// holding */ is refused in every delimited form and written in a line comment,
// where it closes nothing.
//
// The subtests named "must fail" break the renderer on purpose and assert that
// the plugin's reading or the formatter refuses the rewrite. Nothing in here
// skips.
func proseP4Sourcecode(t *testing.T, language string) {
	f := sourcecodeFormsFiles[language]

	t.Run("the fixture is oxfmt-clean", func(t *testing.T) {
		file := formsProject(t, f)
		require.Equal(t, f.src, string(runOxfmt(t, file, []byte(f.src))))
	})

	t.Run("every comment form keeps its layout when its text changes", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := formsProject(t, f)
		src := []byte(f.src)
		p, held := heldRewriter(t, a, file)
		located, err := comment.Locate(p, file, src, nil)
		require.NoError(t, err)

		rewritten := map[string]bool{}
		for _, tc := range f.rewrites {
			t.Run(tc.id, func(t *testing.T) {
				rewritten[tc.id] = true
				i := indexOfBlock(t, located, tc.id)
				c := located.Comments[i]
				r, err := comment.Rewrite(held, file, src, nil, comment.Target{ID: tc.id, Fingerprint: comment.Fingerprint(src, c)}, tc.text, comment.RenderOptions{})
				require.NoError(t, err)
				after := r.Source

				end := c.End + len(after) - len(src)
				assert.Equal(t, string(src[:c.Start]), string(after[:c.Start]), "the bytes before the comment")
				assert.Equal(t, string(src[c.End:]), string(after[end:]), "the bytes after the comment")
				assert.Equal(t, tc.want, string(after[c.Start:end]))
				assert.Equal(t, string(after), string(runOxfmt(t, file, after)), "oxfmt agrees with the whole file")

				relocated, err := comment.Locate(p, file, after, nil)
				require.NoError(t, err)
				require.Len(t, relocated.Comments, len(located.Comments), "the comment count is unchanged")
				for j, before := range located.Comments {
					if j != i {
						now := relocated.Comments[j]
						assert.Equal(t, string(src[before.Start:before.End]), string(after[now.Start:now.End]), "comment %d is untouched", j)
					}
				}
				prose, err := held.Prose(after, relocated.Comments[i])
				require.NoError(t, err)
				assert.Equal(t, tc.text, prose, "the rewritten comment reads back as the text")
			})
		}
		for i, b := range located.Blocks() {
			assert.True(t, rewritten[b.ID], "the fixture's %s comment %s has no rewrite", located.Comments[i].Style, b.ID)
		}
	})

	t.Run("text holding */ is refused in every delimited form and written in a line comment", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := formsProject(t, f)
		src := []byte(f.src)
		_, held := heldRewriter(t, a, file)
		for _, tc := range f.rewrites {
			text := "Holds */ inside."
			r, err := comment.Rewrite(held, file, src, nil, comment.Target{ID: tc.id}, text, comment.RenderOptions{})
			if tc.delimited {
				refusal, ok := comment.AsRefusal(err)
				require.True(t, ok, "%s: %v", tc.id, err)
				assert.Equal(t, comment.RefusedTerminator, refusal.Reason, "%s: %s", tc.id, refusal.Detail)
				continue
			}
			require.NoError(t, err, tc.id)
			assert.Contains(t, string(r.Source), "// Holds */ inside.", tc.id)
		}
	})

	if language == "javascript" {
		t.Run("a legacy HTML-like comment has no layout to keep and is refused", func(t *testing.T) {
			a := tsRewriteApp(t, nil)
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".oxfmtrc.json"), []byte("{}\n"), 0o644))
			file := filepath.Join(dir, "legacy.js")
			const legacy = "<!-- A legacy comment.\nvar x = 1;\n"
			require.NoError(t, os.WriteFile(file, []byte(legacy), 0o644))
			p, ok := a.commentProviderFor(file)
			require.True(t, ok)
			located, err := comment.Locate(p, file, []byte(legacy), nil)
			require.NoError(t, err)
			require.Len(t, located.Comments, 1, "the plugin locates the comment")
			_, err = comment.Rewrite(p, file, []byte(legacy), nil, comment.Target{ID: located.Blocks()[0].ID}, "A newer comment.", comment.RenderOptions{})
			refusal, refused := comment.AsRefusal(err)
			require.True(t, refused, "%v", err)
			assert.Equal(t, comment.RefusedLayout, refusal.Reason, refusal.Detail)
			assertUnchanged(t, file, legacy)
		})
	}

	t.Run("must fail: a renderer that writes */ as it is into a JSDoc block is refused by the plugin's reading", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := formsProject(t, f)
		p, ok := a.commentProviderFor(file)
		require.True(t, ok)
		broken := rawTerminatorPluginRenderer{Provider: p, Rewriter: p.(comment.Rewriter)}
		_, err := comment.Rewrite(broken, file, []byte(f.src), nil, comment.Target{ID: f.jsdoc}, "Holds */ inside.", comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Contains(t, []comment.RefusalReason{comment.RefusedContainment, comment.RefusedParse}, refusal.Reason, refusal.Detail)
	})

	t.Run("must fail: a renderer that moves a JSDoc block's asterisks is refused by oxfmt", func(t *testing.T) {
		a := tsRewriteApp(t, nil)
		file := formsProject(t, f)
		p, held := heldRewriter(t, a, file)
		broken := shiftedStarsPluginRenderer{Provider: p, Rewriter: p.(comment.Rewriter)}
		misaligned := &formattedRewriter{Provider: broken, Rewriter: broken, commentFormatter: held.commentFormatter}
		var text string
		for _, tc := range f.rewrites {
			if tc.id == f.jsdoc {
				text = tc.text
			}
		}
		_, err := comment.Rewrite(misaligned, file, []byte(f.src), nil, comment.Target{ID: f.jsdoc}, text, comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, comment.RefusedFormatter, refusal.Reason, refusal.Detail)
	})
}

// rawTerminatorPluginRenderer renders like the plugin language's rewriter, then
// writes each */ the text holds into the comment as it is.
type rawTerminatorPluginRenderer struct {
	comment.Provider
	comment.Rewriter
}

func (p rawTerminatorPluginRenderer) Render(name string, src []byte, c comment.Comment, text string, opts comment.RenderOptions) ([]byte, error) {
	const standIn = "zzTERMINATORzz"
	span, err := p.Rewriter.Render(name, src, c, strings.ReplaceAll(text, "*/", standIn), opts)
	return bytes.ReplaceAll(span, []byte(standIn), []byte("*/")), err
}

// shiftedStarsPluginRenderer renders like the plugin language's rewriter, then
// indents every line after the opener by one more space.
type shiftedStarsPluginRenderer struct {
	comment.Provider
	comment.Rewriter
}

func (p shiftedStarsPluginRenderer) Render(name string, src []byte, c comment.Comment, text string, opts comment.RenderOptions) ([]byte, error) {
	span, err := p.Rewriter.Render(name, src, c, text, opts)
	return bytes.ReplaceAll(span, []byte("\n "), []byte("\n  ")), err
}

func TestProseP4_typescript(t *testing.T) { proseP4Sourcecode(t, "typescript") }

func TestProseP4_tsx(t *testing.T) { proseP4Sourcecode(t, "tsx") }

func TestProseP4_javascript(t *testing.T) { proseP4Sourcecode(t, "javascript") }
