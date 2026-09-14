package po

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

const pageFixture = "# Translator notes for the app.\n" +
	"# A second line of the same note.\n" +
	"#, fuzzy\n" +
	"msgid \"\"\n" +
	"msgstr \"\"\n" +
	"\"Content-Type: text/plain; charset=UTF-8\\n\"\n" +
	"\n" +
	"#.  Extracted from the source.\n" +
	"#: src/greeting.js:10 src/other.js:2\n" +
	"# The greeting on the start screen.\n" +
	"#| msgid \"Hello\"\n" +
	"msgid \"Hello # world\"\n" +
	"msgstr \"Bonjour # le monde\"\n" +
	"\n" +
	"#\n" +
	"# Farewell note.\n" +
	"#\n" +
	"msgctxt \"menu\"\n" +
	"msgid \"Goodbye\"\n" +
	"msgstr \"\"\n" +
	"\n" +
	"#~ msgid \"Obsolete # entry\"\n" +
	"#~ msgstr \"Ancien\"\n"

// pageLiterals hold a `#` that is content: in a msgid, a msgstr and an obsolete
// entry.
var pageLiterals = []string{"\"Hello # world\"", "\"Bonjour # le monde\"", "#~ msgid \"Obsolete # entry\""}

func TestLocatePOCommentsFindsEveryCommentWithItsSpan(t *testing.T) {
	src := []byte(pageFixture)
	got, err := LocateComments(src)
	require.NoError(t, err)
	assert.Equal(t, "po", got.Language)

	type located struct {
		subject string
		text    string
		lines   format.LineRange
		bytes   string
	}
	var comments []located
	for _, c := range got.Comments {
		assert.Equal(t, comment.StyleLine, c.Style)
		comments = append(comments, located{c.Subject, model.RunsText(c.Runs), c.Lines, string(src[c.Start:c.End])})
	}
	assert.Equal(t, []located{
		{"comment/header", "Translator notes for the app.\nA second line of the same note.", format.LineRange{First: 1, Last: 2}, "# Translator notes for the app.\n# A second line of the same note."},
		{"comment/Hello # world", "The greeting on the start screen.", format.LineRange{First: 10, Last: 10}, "# The greeting on the start screen."},
		{"comment/menu/Goodbye", "Farewell note.", format.LineRange{First: 16, Last: 16}, "# Farewell note."},
	}, comments)

	var excluded []string
	for _, e := range got.Excluded {
		excluded = append(excluded, string(e.Reason)+" "+e.Form+" "+string(src[e.Start:e.End]))
	}
	assert.Equal(t, []string{
		"directive flags #, fuzzy",
		"generated  #.  Extracted from the source.",
		"directive reference #: src/greeting.js:10 src/other.js:2",
		"directive previous #| msgid \"Hello\"",
		"blank  #",
		"blank  #",
	}, excluded)
}

func TestLocatePOCommentsRefusesWhatItCannotPlace(t *testing.T) {
	for name, src := range map[string]string{
		"a file in UTF-16": "\xff\xfe#\x00 \x00a\x00\n\x00",
		"a header declaring another charset over non-ASCII bytes": "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=ISO-8859-1\\n\"\n\n# Café\nmsgid \"x\"\nmsgstr \"\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LocateComments([]byte(src))
			require.ErrorIs(t, err, ErrCommentsUnlocated)
			require.ErrorIs(t, err, comment.ErrUnlocated, "the host treats any provider's unlocated file the same way")
		})
	}

	t.Run("a scan and a reader that disagree", func(t *testing.T) {
		body := []byte("# one\n#: a.js:1\nmsgid \"x\"\nmsgstr \"\"\n")
		lines := scanCommentLines(body, 0)
		require.NoError(t, agreeWithReader(body, lines))
		assert.ErrorIs(t, agreeWithReader(body, lines[1:]), ErrCommentsUnlocated)
	})
}

func TestPOLineText(t *testing.T) {
	for _, tc := range []struct {
		line string
		n    int
		text string
		ok   bool
	}{
		{"# a note", 8, " a note", true},
		{"#. extracted", 12, " extracted", true},
		{"#: src/a.js:1  ", 13, " src/a.js:1", true},
		{"#", 1, "", true},
		{"#~ msgid \"x\"", 0, "", false},
		{"msgid \"x\"", 0, "", false},
	} {
		t.Run(tc.line, func(t *testing.T) {
			n, text, ok := CommentProvider{}.LineText([]byte(tc.line))
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.n, n)
			assert.Equal(t, tc.text, text)
		})
	}
}

func poSuite(p comment.Provider) commenttest.Suite {
	return commenttest.Suite{
		Provider: p,
		Scan:     poUnits,
		Fixtures: []commenttest.Fixture{
			{Name: "page.po", Source: pageFixture, Literals: pageLiterals},
			{Name: "crlf.po", Source: strings.ReplaceAll(pageFixture, "\n", "\r\n"), Literals: pageLiterals},
		},
	}
}

// TestProseP1_po is the P1 rung for PO comments: the conformance suite over
// fixtures, the repository's PO files accounted for against a line scan made
// without the provider, and providers broken on purpose that the suite must
// catch.
func TestProseP1_po(t *testing.T) {
	t.Run("the shared conformance suite", func(t *testing.T) {
		commenttest.Run(t, poSuite(CommentProvider{}))
	})

	t.Run("the repository's PO files account for every comment", func(t *testing.T) {
		root := poRepoRoot(t)
		cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z", "*.po", "*.pot")
		cmd.Dir = root
		listing, err := cmd.Output()
		require.NoError(t, err)
		files, comments, refused := 0, 0, 0
		for rel := range strings.SplitSeq(strings.TrimRight(string(listing), "\x00"), "\x00") {
			if rel == "" || strings.Contains(rel, "node_modules/") {
				continue
			}
			src, err := os.ReadFile(filepath.Join(root, rel))
			require.NoError(t, err)
			got, err := LocateComments(src)
			if errors.Is(err, ErrCommentsUnlocated) {
				refused++
				t.Logf("%s: %v", rel, err)
				continue
			}
			require.NoError(t, err, rel)
			units, err := poUnits(rel, src)
			require.NoError(t, err, rel)
			assert.NoError(t, commenttest.Err(commenttest.Account(src, got, units)), rel)
			files++
			comments += len(units)
		}
		t.Logf("po-comments files=%d comments=%d refused=%d", files, comments, refused)
		assert.Positive(t, files, "the corpus holds no PO file the provider reads")
		assert.Positive(t, comments, "the corpus holds no comment, so it proves nothing about losing one")
	})

	t.Run("must fail: the conformance suite catches a broken provider", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			edit func([]byte, *comment.File)
			want commenttest.Property
		}{
			{"a span one byte short", func(_ []byte, f *comment.File) { f.Comments[0].End-- }, commenttest.PropSpan},
			{"a comment dropped", func(_ []byte, f *comment.File) { f.Comments = f.Comments[1:] }, commenttest.PropAccount},
			{"a # inside a msgstr counted", countMsgstrMarker, commenttest.PropLiteral},
		} {
			t.Run(tc.name, func(t *testing.T) {
				failures := commenttest.Verify(poSuite(brokenPO{edit: tc.edit}))
				require.NotEmpty(t, failures)
				assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
			})
		}
	})
}

var declaredDirectives = comment.Directives{"okapi-skip:", "okapi-unmapped:"}

// declaredFixture holds declared markers opening translator comment lines,
// alone and inside a comment over several lines, and prose that only mentions
// a marker.
const declaredFixture = "# okapi-skip: MessagesTest#testEmpty\n" +
	"msgid \"One\"\n" +
	"msgstr \"\"\n" +
	"\n" +
	"# The second message.\n" +
	"# okapi-unmapped: MessagesTest#testAll\n" +
	"# Its last line.\n" +
	"msgid \"Two\"\n" +
	"msgstr \"\"\n" +
	"\n" +
	"# See the okapi-skip: markers; OKAPI-SKIP: in capitals is prose too.\n" +
	"msgid \"Three\"\n" +
	"msgstr \"\"\n"

// The conformance suite with directives declared: every translator comment
// line that opens with one is set aside as a directive, and every other
// comment is accounted for as it is with nothing declared.
func TestPOConformanceWithDeclaredDirectives(t *testing.T) {
	s := poSuite(CommentProvider{})
	s.Directives = declaredDirectives
	s.Fixtures = append(s.Fixtures,
		commenttest.Fixture{Name: "declared.po", Source: declaredFixture},
		commenttest.Fixture{Name: "declared-crlf.po", Source: strings.ReplaceAll(declaredFixture, "\n", "\r\n")},
	)
	commenttest.Run(t, s)

	got, err := comment.Locate(CommentProvider{}, "declared.po", []byte(declaredFixture), declaredDirectives)
	require.NoError(t, err)
	forms := 0
	for _, e := range got.Excluded {
		if e.Reason == comment.ReasonDirective {
			forms++
		}
	}
	assert.Equal(t, 2, forms, "the suite ran over the declared markers")
	assert.Len(t, got.Comments, 3, "the lines on each side of a marker, and the line that mentions one, stay prose")
}

// poUnits is the conformance suite's scan for PO, written without the
// provider: a line that opens with `#` and not `#~` is a comment, and
// consecutive translator comment lines share a group.
func poUnits(_ string, src []byte) ([]commenttest.Unit, error) {
	var units []commenttest.Unit
	group, prevTranslator := 0, -2
	body := bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	base := len(src) - len(body)
	for n, start := 0, base; start < len(src); n++ {
		end, next := len(src), len(src)
		if i := bytes.IndexByte(src[start:], '\n'); i >= 0 {
			end, next = start+i, start+i+1
		}
		line := bytes.TrimRight(src[start:end], " \t\r")
		if len(line) > 0 && line[0] == '#' && !bytes.HasPrefix(line, []byte("#~")) {
			u := commenttest.Unit{Start: start, End: start + len(line), Open: 1}
			kind := byte(' ')
			if len(line) > 1 && bytes.IndexByte([]byte(".:,|"), line[1]) >= 0 {
				kind, u.Open = line[1], 2
			}
			if kind != ' ' || prevTranslator != n-1 {
				group++
			}
			if kind == ' ' {
				prevTranslator = n
			}
			u.Group = group
			u.Directive = kind == ':' || kind == ',' || kind == '|'
			units = append(units, u)
		}
		start = next
	}
	return units, nil
}

// brokenPO damages what the provider locates in every file with a comment,
// except the canary.
type brokenPO struct {
	CommentProvider
	edit func([]byte, *comment.File)
}

func (b brokenPO) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.CommentProvider.Locate(name, src)
	if err == nil && name != "canary" && len(f.Comments) > 0 {
		b.edit(src, f)
	}
	return f, err
}

// countMsgstrMarker reports the `#` inside a msgstr, to the end of its line, as
// a comment.
func countMsgstrMarker(src []byte, f *comment.File) {
	at := bytes.Index(src, []byte("# le monde"))
	if at < 0 {
		return
	}
	end := at + len("# le monde\"")
	f.Comments = append(f.Comments, comment.Comment{
		Start: at, End: end, Lines: format.NewLineIndex(src).Range(at, end),
		Style: comment.StyleLine, Subject: "comment/document", Runs: []model.Run{model.TextR("le monde\"")},
	})
	slices.SortFunc(f.Comments, func(a, b comment.Comment) int { return a.Start - b.Start })
}

func poRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "no go.work above the package")
		dir = parent
	}
}
