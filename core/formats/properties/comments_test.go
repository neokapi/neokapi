package properties

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

const pageFixture = "# Application strings\n" +
	"! Shown on the home screen.\n" +
	"greeting.text = Hello # not a comment\n" +
	"long.text = first part \\\n" +
	"  # not a comment, a continuation\n" +
	"\n" +
	"#_skip\n" +
	"internal = x\n" +
	"#\n" +
	"# Section: checkout\n" +
	"#\n" +
	"checkout.title = Checkout\n" +
	"# suppress inspection \"UnusedProperty\"\n" +
	"unused = y\n" +
	"  # Indented comment at the end.\n"

// pageLiterals hold a `#` that is content: in a value, and on a line that
// continues a value.
var pageLiterals = []string{"Hello # not a comment", "  # not a comment, a continuation"}

// directiveFixture has one line for each Okapi directive and IntelliJ's
// suppression.
const directiveFixture = "#_text\n" +
	"#_skip\n" +
	"#_btext\n" +
	"#_etext\n" +
	"#_bskip\n" +
	"#_eskip\n" +
	"# suppress inspection \"UnusedProperty\"\n" +
	"key.text = value\n"

func TestLocatePropertiesCommentsFindsEveryCommentWithItsSpan(t *testing.T) {
	src := []byte(pageFixture)
	got, err := LocateComments(src)
	require.NoError(t, err)
	assert.Equal(t, "properties", got.Language)

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
		{"comment/greeting.text", "Application strings\nShown on the home screen.", format.LineRange{First: 1, Last: 2}, "# Application strings\n! Shown on the home screen."},
		{"comment/checkout.title", "Section: checkout", format.LineRange{First: 10, Last: 10}, "# Section: checkout"},
		{"comment/document", "Indented comment at the end.", format.LineRange{First: 15, Last: 15}, "# Indented comment at the end."},
	}, comments)

	var excluded []string
	for _, e := range got.Excluded {
		excluded = append(excluded, string(e.Reason)+" "+e.Form+" "+string(src[e.Start:e.End]))
	}
	assert.Equal(t, []string{
		"directive _skip #_skip",
		"blank  #",
		"blank  #",
		"directive suppress # suppress inspection \"UnusedProperty\"",
	}, excluded)
}

func TestLocatePropertiesCommentsWithBareCarriageReturns(t *testing.T) {
	src := []byte("# One\r# Two\rkey.text = value\r")
	got, err := LocateComments(src)
	require.NoError(t, err)
	require.Len(t, got.Comments, 1)
	assert.Equal(t, "# One\r# Two", string(src[got.Comments[0].Start:got.Comments[0].End]))
	assert.Equal(t, "One\nTwo", model.RunsText(got.Comments[0].Runs))
	assert.Equal(t, "comment/key.text", got.Comments[0].Subject)
}

func TestLocatePropertiesCommentsRefusesADisagreement(t *testing.T) {
	body := []byte("# one\n# two\nkey.text = value\n")
	lines := scanCommentLines(body, 0)
	require.NoError(t, agreeWithReader(body, lines))
	err := agreeWithReader(body, lines[1:])
	require.ErrorIs(t, err, ErrCommentsUnlocated)
	require.ErrorIs(t, err, comment.ErrUnlocated, "the host treats any provider's unlocated file the same way")
}

func TestPropertiesLineText(t *testing.T) {
	for _, tc := range []struct {
		line string
		n    int
		text string
		ok   bool
	}{
		{"# note", 6, " note", true},
		{"! note  ", 6, " note", true},
		{"#", 1, "", true},
		{"key=value", 0, "", false},
		{"", 0, "", false},
	} {
		t.Run(tc.line, func(t *testing.T) {
			n, text, ok := CommentProvider{}.LineText([]byte(tc.line))
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.n, n)
			assert.Equal(t, tc.text, text)
		})
	}
}

func TestPropertiesDirectiveFixtureCoversEveryForm(t *testing.T) {
	got, err := LocateComments([]byte(directiveFixture))
	require.NoError(t, err)
	assert.Empty(t, got.Comments)
	require.Len(t, got.Excluded, 7)
	for _, e := range got.Excluded {
		assert.Equal(t, comment.ReasonDirective, e.Reason)
	}

	t.Run("must fail: a classifier missing a directive form", func(t *testing.T) {
		for i, d := range propertiesDirectives {
			t.Run(d.name, func(t *testing.T) {
				directives := slices.Delete(slices.Clone(propertiesDirectives), i, i+1)
				got, err := locateComments([]byte(directiveFixture), directives)
				require.NoError(t, err)
				assert.NotEmpty(t, got.Comments, "without %q the fixture exposes a directive as prose", d.name)
			})
		}
	})
}

func propertiesSuite(p comment.Provider) commenttest.Suite {
	return commenttest.Suite{
		Provider: p,
		Scan:     propertiesUnits,
		Fixtures: []commenttest.Fixture{
			{Name: "page.properties", Source: pageFixture, Literals: pageLiterals},
			{Name: "crlf.properties", Source: strings.ReplaceAll(pageFixture, "\n", "\r\n"), Literals: pageLiterals},
			{Name: "directives.properties", Source: directiveFixture},
		},
	}
}

// TestProseP1_properties is the P1 rung for properties comments: the
// conformance suite over fixtures, the repository's properties files accounted
// for against a line scan made without the provider, and providers broken on
// purpose that the suite must catch.
func TestProseP1_properties(t *testing.T) {
	t.Run("the shared conformance suite", func(t *testing.T) {
		commenttest.Run(t, propertiesSuite(CommentProvider{}))
	})

	t.Run("the repository's properties files account for every comment", func(t *testing.T) {
		root := propertiesRepoRoot(t)
		cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-z", "*.properties")
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
			units, err := propertiesUnits(rel, src)
			require.NoError(t, err, rel)
			assert.NoError(t, commenttest.Err(commenttest.Account(src, got, units)), rel)
			files++
			comments += len(units)
		}
		t.Logf("properties-comments files=%d comments=%d refused=%d", files, comments, refused)
		assert.Positive(t, files, "the corpus holds no properties file the provider reads")
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
			{"a # inside a value counted", countValueMarker, commenttest.PropLiteral},
		} {
			t.Run(tc.name, func(t *testing.T) {
				failures := commenttest.Verify(propertiesSuite(brokenProperties{edit: tc.edit}))
				require.NotEmpty(t, failures)
				assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
			})
		}
	})
}

var declaredDirectives = comment.Directives{"okapi-skip:", "okapi-unmapped:"}

// declaredFixture holds declared markers opening comment lines, alone and in
// the middle of a comment over several lines, and prose that only mentions a
// marker.
const declaredFixture = "# okapi-skip: MessagesTest#testEmpty\n" +
	"one.text = One\n" +
	"# The second message.\n" +
	"! okapi-unmapped: MessagesTest#testAll\n" +
	"# Its last line.\n" +
	"two.text = Two\n" +
	"# See the okapi-skip: markers; OKAPI-SKIP: in capitals is prose too.\n" +
	"three.text = Three\n"

// The conformance suite with directives declared: every comment line that
// opens with one is set aside as a directive, and every other comment is
// accounted for as it is with nothing declared.
func TestPropertiesConformanceWithDeclaredDirectives(t *testing.T) {
	s := propertiesSuite(CommentProvider{})
	s.Directives = declaredDirectives
	s.Fixtures = append(s.Fixtures,
		commenttest.Fixture{Name: "declared.properties", Source: declaredFixture},
		commenttest.Fixture{Name: "declared-crlf.properties", Source: strings.ReplaceAll(declaredFixture, "\n", "\r\n")},
	)
	commenttest.Run(t, s)

	got, err := comment.Locate(CommentProvider{}, "declared.properties", []byte(declaredFixture), declaredDirectives)
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

// propertiesUnits is the conformance suite's scan for properties files, written
// without the provider: outside a value's continuation, a line whose first
// character other than a blank is `#` or `!` is a comment, and consecutive
// comment lines share a group unless a tool reads one.
func propertiesUnits(_ string, src []byte) ([]commenttest.Unit, error) {
	var units []commenttest.Unit
	group, prevComment := 0, -2
	continued := false
	body := bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	base := len(src) - len(body)
	for n, start := 0, base; start < len(src); n++ {
		end, next := len(src), len(src)
		if i := bytes.IndexByte(src[start:], '\n'); i >= 0 {
			end, next = start+i, start+i+1
		}
		line := bytes.TrimSuffix(src[start:end], []byte("\r"))
		trimmed := bytes.TrimLeft(line, " \t")
		backslashes := len(trimmed) - len(bytes.TrimRight(trimmed, "\\"))
		switch {
		case continued:
			continued = backslashes%2 == 1
		case len(trimmed) > 0 && (trimmed[0] == '#' || trimmed[0] == '!'):
			at := start + len(line) - len(trimmed)
			u := commenttest.Unit{Start: at, End: at + len(bytes.TrimRight(trimmed, " \t")), Open: 1}
			for _, d := range propertiesDirectives {
				if _, ok := d.match(trimmed[0], strings.TrimSpace(string(trimmed[1:]))); ok {
					u.Directive = true
				}
			}
			if u.Directive || prevComment != n-1 {
				group++
			}
			prevComment = n
			if u.Directive {
				prevComment = -2
			}
			u.Group = group
			units = append(units, u)
		case len(bytes.TrimSpace(line)) > 0:
			continued = backslashes%2 == 1
		}
		start = next
	}
	return units, nil
}

// brokenProperties damages what the provider locates in every file with a
// comment, except the canary.
type brokenProperties struct {
	CommentProvider
	edit func([]byte, *comment.File)
}

func (b brokenProperties) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.CommentProvider.Locate(name, src)
	if err == nil && name != "canary" && len(f.Comments) > 0 {
		b.edit(src, f)
	}
	return f, err
}

// countValueMarker reports the `#` inside a value, to the end of its line, as a
// comment.
func countValueMarker(src []byte, f *comment.File) {
	at := bytes.Index(src, []byte("# not a comment\n"))
	if at < 0 {
		at = bytes.Index(src, []byte("# not a comment\r"))
	}
	if at < 0 {
		return
	}
	end := at + len("# not a comment")
	f.Comments = append(f.Comments, comment.Comment{
		Start: at, End: end, Lines: format.NewLineIndex(src).Range(at, end),
		Style: comment.StyleLine, Subject: "comment/document", Runs: []model.Run{model.TextR("not a comment")},
	})
	slices.SortFunc(f.Comments, func(a, b comment.Comment) int { return a.Start - b.Start })
}

func propertiesRepoRoot(t *testing.T) string {
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
