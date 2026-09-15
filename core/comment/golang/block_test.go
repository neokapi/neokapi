package golang

import (
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	fmtpkg "github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockFixture is gofmt-clean Go with a delimited comment in each layout Go
// code writes one in. The package doc has bare lines and a list, and Parse's doc
// has a line of asterisks. Parse's body holds a comment alone on its line, one
// after code, one inside a line of code without spaces, and one whose text
// starts on the opener's line and ends on the closer's. A deprecated
// declaration's doc and a field's doc indented under its opener follow, with
// one line comment beside them.
const blockFixture = `/*
Package demo is a fixture for rewriting delimited comments.

It covers:
  - one layout
  - another layout
*/
package demo

import "fmt"

/*
 * Parse reads the input.
 *
 * It stops at the end.
 */
func Parse() {
	/* A comment inside the body. */
	fmt.Println("x") /* a trailing comment */
	fmt.Println( /*inline*/ "y")
	/* Split starts on the opener's line
	   and ends on the closer's. */
	_ = 1
}

/* Close releases the reader. */
func Close() {}

/*
Old parses the old way.

Deprecated: use [Parse].
*/
func Old() {}

// Block holds one unit.
type Block struct {
	/*
	   ID identifies the block,
	   and is never empty.
	*/
	ID string
}
`

// blockFixtureComments is how many delimited comments blockFixture holds.
const blockFixtureComments = 9

// blockRewrites rewrites every comment in blockFixture and gives the bytes each
// rewrite must write.
var blockRewrites = []struct{ id, text, want string }{
	// gofmt writes a doc comment's list items two spaces in, so the text's one
	// space is written as two.
	{"package", "Package demo is a fixture.\n\nIt covers, over two lines:\n - one layout\n - another layout",
		"/*\nPackage demo is a fixture.\n\nIt covers, over two lines:\n  - one layout\n  - another layout\n*/"},
	{"func/Parse", "Parse reads the whole input.\n\nIt stops at the end,\nor at the first error.",
		"/*\n * Parse reads the whole input.\n *\n * It stops at the end,\n * or at the first error.\n */"},
	{"func/Parse/comment", "A rewritten comment.", "/* A rewritten comment. */"},
	{"func/Parse/comment#2", "a rewritten trailing comment", "/* a rewritten trailing comment */"},
	{"func/Parse/comment#3", "rewritten", "/*rewritten*/"},
	{"func/Parse/comment#4", "Split starts here,\ncontinues,\nand ends here.",
		"/* Split starts here,\n\t   continues,\n\t   and ends here. */"},
	{"func/Close", "Close releases the reader and returns.", "/* Close releases the reader and returns. */"},
	{"func/Old", "Old parses in the old way.\n\nDeprecated: use [Parse].", "/*\nOld parses in the old way.\n\nDeprecated: use [Parse].\n*/"},
	{"type/Block", "Block holds exactly one unit.", "// Block holds exactly one unit."},
	{"type/Block/ID", "ID names the block.", "/*\n\t   ID names the block.\n\t*/"},
}

// blockReadsBack is the text a comment in blockRewrites reads back as once
// rewritten, where gofmt's printer wrote the text differently.
var blockReadsBack = map[string]string{
	"package": "Package demo is a fixture.\n\nIt covers, over two lines:\n  - one layout\n  - another layout",
}

// Floors for the Go source tree, well below what it holds, so that a scan over
// the wrong tree or almost nothing fails.
const (
	gorootMinBlocks    = 4000
	gorootMinMultiline = 200
)

// TestProseP4_go is the P4 rung for Go comments: every comment form Go code
// uses is rewritten, delimited comments included, in the layout the comment was
// written in. Text holding `*/` is refused and never written, every byte outside
// the comment stays identical, the file parses and gofmt agrees with the whole
// file. Every delimited comment in the fixtures, the repository and the Go
// source tree rewrites to its own bytes.
//
// The subtests named "must fail" corrupt a rewrite on purpose and assert that
// the containment check refuses it. Nothing in here skips.
func TestProseP4_go(t *testing.T) {
	src := []byte(blockFixture)
	formatted, err := format.Source(src)
	require.NoError(t, err)
	require.Equal(t, blockFixture, string(formatted), "the fixture is gofmt-clean")
	located, err := Provider{}.Locate("demo.go", src)
	require.NoError(t, err)

	t.Run("every layout is kept when a comment's text changes", func(t *testing.T) {
		rewritten := map[string]bool{}
		for _, tc := range blockRewrites {
			t.Run(tc.id, func(t *testing.T) {
				i := indexOf(t, located, tc.id)
				c := located.Comments[i]
				rewritten[tc.id] = true
				r, err := comment.Rewrite(Provider{}, "demo.go", src, nil, comment.Target{ID: tc.id, Fingerprint: comment.Fingerprint(src, c)}, tc.text, comment.RenderOptions{})
				require.NoError(t, err)
				after := r.Source

				// The assertions below read the bytes themselves rather than
				// trusting what Contain decided.
				end := c.End + len(after) - len(src)
				assert.Equal(t, string(src[:c.Start]), string(after[:c.Start]), "the bytes before the comment")
				assert.Equal(t, string(src[c.End:]), string(after[end:]), "the bytes after the comment")
				assert.Equal(t, tc.want, string(after[c.Start:end]))
				_, err = parser.ParseFile(token.NewFileSet(), "demo.go", after, parser.ParseComments)
				require.NoError(t, err, "the result parses")
				again, err := format.Source(after)
				require.NoError(t, err)
				assert.Equal(t, string(after), string(again), "gofmt agrees with the whole file")

				relocated, err := Provider{}.Locate("demo.go", after)
				require.NoError(t, err)
				require.Len(t, relocated.Comments, len(located.Comments), "the comment count is unchanged")
				for j, before := range located.Comments {
					if j != i {
						now := relocated.Comments[j]
						assert.Equal(t, string(src[before.Start:before.End]), string(after[now.Start:now.End]), "comment %d is untouched", j)
					}
				}
				assert.Equal(t, c.Style, relocated.Comments[i].Style)
				prose, err := Provider{}.Prose(after, relocated.Comments[i])
				require.NoError(t, err)
				reads := tc.text
				if r, ok := blockReadsBack[tc.id]; ok {
					reads = r
				}
				assert.Equal(t, reads, prose, "the rewritten comment reads back as the text")
			})
		}
		for i, b := range located.Blocks() {
			assert.True(t, rewritten[b.ID], "the fixture's %s comment %s has no rewrite", located.Comments[i].Style, b.ID)
		}
	})

	t.Run("every delimited comment in the fixtures, the repository and the Go source tree rewrites to itself", func(t *testing.T) {
		dir := t.TempDir()
		fixtures := []string{filepath.Join(dir, "demo.go"), filepath.Join(dir, "crlf.go")}
		require.NoError(t, os.WriteFile(fixtures[0], src, 0o600))
		require.NoError(t, os.WriteFile(fixtures[1], []byte(strings.ReplaceAll(blockFixture, "\n", "\r\n")), 0o600))
		got := rewriteBlocks(fixtures)
		require.Empty(t, got.failures)
		assert.Equal(t, 2*blockFixtureComments, got.rewrote, "every delimited comment in the fixtures rewrites")

		repo := rewriteBlocks(goFilesUnder(t, repoRoot(t)))
		t.Logf("block-corpus repository files=%d comments=%d rewrote=%d multiline=%d refused=%v", repo.files, repo.comments, repo.rewrote, repo.multiline, repo.refused)
		assert.Empty(t, repo.failures)

		tree := rewriteBlocks(goFilesUnder(t, goroot(t)))
		t.Logf("block-corpus goroot files=%d comments=%d rewrote=%d multiline=%d refused=%v", tree.files, tree.comments, tree.rewrote, tree.multiline, tree.refused)
		assert.Empty(t, tree.failures)
		assert.GreaterOrEqual(t, tree.rewrote, gorootMinBlocks, "the Go source tree rewrote too few delimited comments")
		assert.GreaterOrEqual(t, tree.multiline, gorootMinMultiline, "the Go source tree rewrote too few delimited comments over several lines")
	})

	t.Run("text holding */ is refused in every layout", func(t *testing.T) {
		for i, c := range located.Comments {
			if c.Style != comment.StyleBlock {
				continue
			}
			id := located.Blocks()[i].ID
			for _, text := range []string{"Ends */ here.", "*/", "Ends here. */", "One.\n*/\nTwo."} {
				_, err := comment.Rewrite(Provider{}, "demo.go", src, nil, comment.Target{ID: id}, text, comment.RenderOptions{})
				refusal, ok := comment.AsRefusal(err)
				require.True(t, ok, "%s: %q: %v", id, text, err)
				assert.Equal(t, comment.RefusedTerminator, refusal.Reason, "%s: %q: %s", id, text, refusal.Detail)
			}
		}
	})

	t.Run("a comment made of several comments is refused as layout", func(t *testing.T) {
		for _, file := range []string{
			"package p\n\n/* One. */ /* Two. */\nfunc F() {}\n",
			"package p\n\n// One.\n/* Two. */\nfunc F() {}\n",
			"package p\n\n/* One. */\n// Two.\nfunc F() {}\n",
		} {
			_, err := comment.Rewrite(Provider{}, "p.go", []byte(file), nil, comment.Target{ID: "func/F"}, "One.", comment.RenderOptions{})
			refusal, ok := comment.AsRefusal(err)
			require.True(t, ok, "%q: %v", file, err)
			assert.Equal(t, comment.RefusedLayout, refusal.Reason, "%q: %s", file, refusal.Detail)
		}
	})

	t.Run("directives, the cgo preamble, generated files, the deprecation marker and references are kept", func(t *testing.T) {
		for _, tc := range []struct {
			name, file, src string
			target          comment.Target
			text            string
			reason          comment.RefusalReason
		}{
			{"text that makes a comment a line directive", "demo.go", blockFixture,
				comment.Target{ID: "func/Parse/comment#3"}, "line demo.go:10", comment.RefusedDirective},
			{"the cgo preamble", "c.go", "package p\n\n/*\n#include <stdio.h>\n*/\nimport \"C\"\n",
				comment.Target{ID: "import/C", Lines: &fmtpkg.LineRange{First: 3, Last: 5}}, "Nothing.", comment.RefusedCgoPreamble},
			{"a generated file", "g.go", "// Code generated by gen. DO NOT EDIT.\n\npackage p\n\n/* Parse parses. */\nfunc Parse() {}\n",
				comment.Target{ID: "func/Parse"}, "Parse reads.", comment.RefusedGenerated},
			{"a dropped deprecation marker", "demo.go", blockFixture,
				comment.Target{ID: "func/Old"}, "Old parses the old way, see [Parse].", comment.RefusedDeprecation},
			{"a dropped reference", "demo.go", blockFixture,
				comment.Target{ID: "func/Old"}, "Old parses the old way.\n\nDeprecated: use Parse.", comment.RefusedStructure},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := comment.Rewrite(Provider{}, tc.file, []byte(tc.src), nil, tc.target, tc.text, comment.RenderOptions{})
				refusal, ok := comment.AsRefusal(err)
				require.True(t, ok, "%v", err)
				assert.Equal(t, tc.reason, refusal.Reason, refusal.Detail)
			})
		}
	})

	t.Run("CRLF line endings are kept", func(t *testing.T) {
		crlf := []byte(strings.ReplaceAll(blockFixture, "\n", "\r\n"))
		r, err := comment.Rewrite(Provider{}, "crlf.go", crlf, nil, comment.Target{ID: "func/Parse"}, "Parse reads.\n\nIt stops.", comment.RenderOptions{})
		require.NoError(t, err)
		assert.Contains(t, string(r.Source), "import \"fmt\"\r\n\r\n/*\r\n * Parse reads.\r\n *\r\n * It stops.\r\n */\r\nfunc Parse() {\r\n")
	})

	t.Run("a paragraph wider than the width is reflowed under the line of asterisks", func(t *testing.T) {
		long := "Parse reads the input " + strings.Repeat("and keeps reading ", 8) + "until the end."
		i := indexOf(t, located, "func/Parse")
		c := located.Comments[i]
		r, err := comment.Rewrite(Provider{}, "demo.go", src, nil, comment.Target{ID: "func/Parse"}, long, comment.RenderOptions{Width: 60})
		require.NoError(t, err)
		lines := strings.Split(string(r.Source[c.Start:r.After.End]), "\n")
		require.Greater(t, len(lines), 4, "the paragraph is laid out over several lines")
		assert.Equal(t, "/*", lines[0])
		assert.Equal(t, " */", lines[len(lines)-1])
		for _, line := range lines[1 : len(lines)-1] {
			assert.True(t, strings.HasPrefix(line, " * "), "%q keeps the line of asterisks", line)
			assert.LessOrEqual(t, len(line), 60, "%q", line)
		}
	})

	t.Run("must fail: a one-byte corruption at a delimited comment's edge is refused", func(t *testing.T) {
		index := indexOf(t, located, "func/Parse")
		c := located.Comments[index]
		span, err := Provider{}.Render("demo.go", src, c, "Parse reads the whole input.", comment.RenderOptions{})
		require.NoError(t, err)
		splice := func(start, end int) []byte {
			return bytes.Join([][]byte{src[:start], span, src[end:]}, nil)
		}
		good := splice(c.Start, c.End)
		_, err = comment.Contain(Provider{}, "demo.go", src, good, nil, located, index)
		require.NoError(t, err, "the uncorrupted rewrite is contained")

		flip := func(b []byte, at int) []byte {
			out := bytes.Clone(b)
			out[at] ^= 0x20
			return out
		}
		end := c.Start + len(span)
		for _, tc := range []struct {
			name   string
			after  []byte
			reason comment.RefusalReason
		}{
			{"a byte before the span flipped", flip(good, c.Start-2), comment.RefusedContainment},
			{"a byte after the span flipped", flip(good, end+2), comment.RefusedContainment},
			{"the span starts one byte early", splice(c.Start-1, c.End), comment.RefusedContainment},
			{"the span ends one byte late", splice(c.Start, c.End+1), comment.RefusedContainment},
			{"the closer's slash dropped", append(bytes.Clone(good[:end-1]), good[end:]...), comment.RefusedParse},
		} {
			_, err := comment.Contain(Provider{}, "demo.go", src, tc.after, nil, located, index)
			refusal, ok := comment.AsRefusal(err)
			require.True(t, ok, "%s: the corruption was not refused: %v", tc.name, err)
			assert.Equal(t, tc.reason, refusal.Reason, "%s: %s", tc.name, refusal.Detail)
		}
	})

	t.Run("must fail: a renderer that writes */ as it is is refused by containment", func(t *testing.T) {
		for _, tc := range []struct {
			id, text string
			reason   comment.RefusalReason
		}{
			// The comment ends early and the rest of the text parses as code
			// holding a second comment.
			{"func/Parse/comment#3", `rewritten*/ "z", /*again`, comment.RefusedContainment},
			// The comment ends early and the rest of the text is not Go.
			{"func/Parse", "Parse reads */ the input.", comment.RefusedParse},
		} {
			_, err := comment.Rewrite(rawTerminator{}, "demo.go", src, nil, comment.Target{ID: tc.id}, tc.text, comment.RenderOptions{})
			refusal, ok := comment.AsRefusal(err)
			require.True(t, ok, "%s: %v", tc.id, err)
			assert.Equal(t, tc.reason, refusal.Reason, "%s: %s", tc.id, refusal.Detail)
		}
	})

	t.Run("must fail: a rewrite that changes how gofmt aligns the code around it is refused", func(t *testing.T) {
		const aligned = "package p\n\nfunc A(x int /* the x */) {}\nfunc B()                  {}\n"
		formatted, err := format.Source([]byte(aligned))
		require.NoError(t, err)
		require.Equal(t, aligned, string(formatted), "the fixture is gofmt-clean")

		_, err = comment.Rewrite(Provider{}, "a.go", []byte(aligned), nil, comment.Target{ID: "func/A/comment"}, "the width of x", comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, comment.RefusedFormatter, refusal.Reason, refusal.Detail)

		r, err := comment.Rewrite(Provider{}, "a.go", []byte(aligned), nil, comment.Target{ID: "func/A/comment"}, "the y", comment.RenderOptions{})
		require.NoError(t, err, "text of the same width leaves the alignment as it is")
		assert.Equal(t, "package p\n\nfunc A(x int /* the y */) {}\nfunc B()                  {}\n", string(r.Source))
	})

	t.Run("must fail: a renderer that moves the line of asterisks is refused by gofmt", func(t *testing.T) {
		_, err := comment.Rewrite(shiftedStars{}, "demo.go", src, nil, comment.Target{ID: "func/Parse"}, "Parse reads the input.\n\nIt stops at the end.", comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, comment.RefusedFormatter, refusal.Reason, refusal.Detail)
	})
}

// shiftedStars renders like the Go provider, then indents every line after the
// opener by one more space.
type shiftedStars struct{ Provider }

func (p shiftedStars) Render(name string, src []byte, c comment.Comment, text string, opts comment.RenderOptions) ([]byte, error) {
	span, err := p.Provider.Render(name, src, c, text, opts)
	return bytes.ReplaceAll(span, []byte("\n "), []byte("\n  ")), err
}

// FuzzRewriteBlock rewrites a delimited comment in each layout with arbitrary
// text. Each rewrite is either refused with a reason or leaves every byte
// outside the comment identical, in a file gofmt agrees with, with the comment
// closing only at its end. Rendering the rewritten comment's own prose changes
// nothing.
func FuzzRewriteBlock(f *testing.F) {
	for _, seed := range []string{
		"Parse ends at */ and resumes.",
		"Parse handles /* nested */ text.",
		"Parse handles // inside a delimited comment.",
		"Before " + strings.Repeat("x", 200) + " after.",
		"Combining marks: e\u0301 a\u0308 \u0301alone.",
		"\u05E9\u05DC\u05D5\u05DD \u05E2\u05D5\u05DC\u05DD and \u0645\u0631\u062D\u0628\u0627 \u0628\u0627\u0644\u0639\u0627\u0644\u0645 in one line.",
		"A lone\rcarriage return.",
		"Paragraph.\n\n\tcode := 1\n\nAfter.",
		"Cases:\n  - one\n  - two",
		"/",
		"*",
		"ends with *\n/starts with a slash",
		"line demo.go:10",
		"Deprecated: use New.",
		"[io.Reader] reads.",
		"\u202Eoverride",
		"",
	} {
		f.Add(seed)
	}
	src := []byte(blockFixture)
	located, err := Provider{}.Locate("demo.go", src)
	if err != nil {
		f.Fatal(err)
	}
	index := map[string]int{}
	for i, b := range located.Blocks() {
		index[b.ID] = i
	}
	targets := []string{"package", "func/Parse", "func/Parse/comment", "func/Parse/comment#2", "func/Parse/comment#3", "func/Parse/comment#4", "func/Close", "func/Old", "type/Block/ID"}
	for _, id := range targets {
		if i, ok := index[id]; !ok || located.Comments[i].Style != comment.StyleBlock {
			f.Fatalf("the fixture has no delimited comment %s", id)
		}
	}

	f.Fuzz(func(t *testing.T, text string) {
		for _, id := range targets {
			c := located.Comments[index[id]]
			r, err := comment.Rewrite(Provider{}, "demo.go", src, nil, comment.Target{ID: id}, text, comment.RenderOptions{})
			if err != nil {
				if _, ok := comment.AsRefusal(err); !ok {
					t.Fatalf("%s: %q: an error that is not a refusal: %v", id, text, err)
				}
				continue
			}
			after := r.Source
			end := c.End + len(after) - len(src)
			if end < c.Start || !bytes.Equal(src[:c.Start], after[:c.Start]) || !bytes.Equal(src[c.End:], after[end:]) {
				t.Fatalf("%s: %q: a byte outside the comment changed", id, text)
			}
			span := after[c.Start:end]
			if !bytes.HasPrefix(span, []byte("/*")) || bytes.Index(span[2:], []byte("*/")) != len(span)-4 {
				t.Fatalf("%s: %q: the rewritten comment does not close exactly at its end: %q", id, text, span)
			}
			formatted, err := format.Source(after)
			if err != nil || !bytes.Equal(formatted, after) {
				t.Fatalf("%s: %q: gofmt disagrees with the rewrite: %v", id, text, err)
			}
			prose, err := Provider{}.Prose(after, r.After)
			if err != nil {
				t.Fatalf("%s: %q: %v", id, text, err)
			}
			again, err := comment.Rewrite(Provider{}, "demo.go", after, nil, comment.Target{ID: id}, prose, comment.RenderOptions{})
			if err != nil || again.Changed {
				t.Fatalf("%s: %q: rendering the rewritten comment's own prose is not a no-op: %v", id, text, err)
			}
		}
	})
}
