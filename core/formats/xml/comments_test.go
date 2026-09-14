package xml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
	"github.com/neokapi/neokapi/core/comment/markup"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stringsFixture = `<?xml version="1.0" encoding="utf-8"?>
<!-- Strings for the start screen. -->
<resources xmlns:tools="http://schemas.android.com/tools">
    <!-- The greeting on the start screen. -->
    <string name="greeting">Hello</string>
    <string name="farewell">Goodbye</string> <!-- Shown on exit. -->
    <!--
        Spans several lines,
        and ends here.
    -->
    <string name="multi">Multi</string>
    <!-- <string name="old">Retired</string> -->
    <!--suppress UnusedResources -->
    <string name="unused">Unused</string>
    <!---->
    <string name="cdata"><![CDATA[<!-- not a comment -->]]></string>
    <string name="attr" tools:note="--> is text">Text --> too</string>
    <?tool <!-- not a comment either -->?>
    <!-- Closes the resources. -->
</resources>
<!-- After the root. -->
`

// stringsLiterals hold comment markers that are content.
var stringsLiterals = []string{
	"<![CDATA[<!-- not a comment -->]]>",
	"<?tool <!-- not a comment either -->?>",
	`"--> is text"`,
	"Text --> too",
}

// directiveFixture has one line for each directive form, matched by that form
// alone, so dropping any form lets a directive through as prose.
const directiveFixture = `<root>
  <!-- prettier-ignore -->
  <!-- @formatter:off -->
  <!--suppress UnusedResources -->
  <!-- ReSharper disable MarkupTextTypo -->
</root>
`

func TestLocateXMLCommentsFindsEveryCommentWithItsSpan(t *testing.T) {
	src := []byte(stringsFixture)
	got, err := LocateComments("androidxml", src)
	require.NoError(t, err)
	assert.Equal(t, "androidxml", got.Language)

	type located struct {
		subject string
		text    string
		lines   format.LineRange
		bytes   string
	}
	var comments []located
	for _, c := range got.Comments {
		assert.Equal(t, comment.StyleBlock, c.Style)
		comments = append(comments, located{c.Subject, model.RunsText(c.Runs), c.Lines, string(src[c.Start:c.End])})
	}
	assert.Equal(t, []located{
		{"comment/resources", "Strings for the start screen.", format.LineRange{First: 2, Last: 2}, "<!-- Strings for the start screen. -->"},
		{"comment/resources/string[greeting]", "The greeting on the start screen.", format.LineRange{First: 4, Last: 4}, "<!-- The greeting on the start screen. -->"},
		{"comment/resources/string[farewell]", "Shown on exit.", format.LineRange{First: 6, Last: 6}, "<!-- Shown on exit. -->"},
		{"comment/resources/string[multi]", "Spans several lines,\nand ends here.", format.LineRange{First: 7, Last: 10}, "<!--\n        Spans several lines,\n        and ends here.\n    -->"},
		{"comment/resources/string[unused]", "", format.LineRange{First: 12, Last: 12}, `<!-- <string name="old">Retired</string> -->`},
		{"comment/resources", "Closes the resources.", format.LineRange{First: 19, Last: 19}, "<!-- Closes the resources. -->"},
		{"comment/document", "After the root.", format.LineRange{First: 21, Last: 21}, "<!-- After the root. -->"},
	}, comments)

	retired := got.Comments[4].Runs
	require.Len(t, retired, 1)
	require.NotNil(t, retired[0].Ph, "commented-out markup is a placeholder, never prose")
	assert.Equal(t, markup.SubMarkup, retired[0].Ph.SubType)

	var excluded []string
	for _, e := range got.Excluded {
		excluded = append(excluded, string(e.Reason)+" "+e.Form+" "+string(src[e.Start:e.End]))
	}
	assert.Equal(t, []string{
		"directive suppress <!--suppress UnusedResources -->",
		"blank  <!---->",
	}, excluded)
}

func TestLocateXMLCommentsRefusesWhatItCannotPlace(t *testing.T) {
	for name, src := range map[string]string{
		"-- inside a comment":               "<r><!-- a -- b --></r>",
		"a comment in the DOCTYPE":          "<!DOCTYPE r [ <!-- in the subset --> ]>\n<r/>",
		"an unclosed comment":               "<r><!-- a",
		"an unclosed CDATA section":         "<r><![CDATA[ <!-- a --> </r>",
		"a document the parser rejects":     "<r a=\"<b>\"><!-- a --></r>",
		"an encoding the parser can't read": "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><r><!-- a --></r>",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LocateComments("xml", []byte(src))
			require.ErrorIs(t, err, ErrCommentsUnlocated)
			require.ErrorIs(t, err, comment.ErrUnlocated, "the host treats any provider's unlocated file the same way")
		})
	}

	t.Run("a scan and a parser that disagree", func(t *testing.T) {
		parsed := []parsedComment{{start: 3, end: 13}}
		require.NoError(t, agree([]span{{3, 13}}, parsed))
		for _, scanned := range [][]span{nil, {{3, 12}}, {{3, 13}, {20, 30}}} {
			assert.ErrorIs(t, agree(scanned, parsed), ErrCommentsUnlocated)
		}
	})
}

func TestXMLLineText(t *testing.T) {
	for _, tc := range []struct {
		line string
		n    int
		text string
		ok   bool
	}{
		{"<!-- Greets the reader. -->", 27, " Greets the reader. ", true},
		{"<!-- a --> <string name=\"b\">B</string>", 10, " a ", true},
		{"<!---->", 7, "", true},
		{"<!--suppress UnusedResources -->", 32, "suppress UnusedResources ", true},
		{"<!--", 0, "", false},
		{"<!-- spans on to the next line", 0, "", false},
		{"<!-- a -- b -->", 0, "", false},
		{"<string/> <!-- a -->", 0, "", false},
	} {
		t.Run(tc.line, func(t *testing.T) {
			n, text, ok := CommentProvider{}.LineText([]byte(tc.line))
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.n, n)
			assert.Equal(t, tc.text, text)
		})
	}

	t.Run("each located one-line comment occupies its span on its line", func(t *testing.T) {
		src := []byte(stringsFixture)
		got, err := LocateComments("xml", src)
		require.NoError(t, err)
		oneLine := 0
		for _, c := range got.Comments {
			eol := bytes.IndexByte(src[c.Start:], '\n')
			n, text, ok := CommentProvider{}.LineText(src[c.Start : c.Start+eol])
			if c.Lines.First != c.Lines.Last {
				assert.False(t, ok, "the comment at %d runs past its first line", c.Start)
				continue
			}
			oneLine++
			require.True(t, ok, "comment at %d", c.Start)
			assert.Equal(t, c.End-c.Start, n)
			assert.Equal(t, string(src[c.Start+len(markup.Open):c.End-len(markup.Close)]), text)
		}
		assert.Positive(t, oneLine)
	})
}

func TestXMLDirectiveFixtureCoversEveryForm(t *testing.T) {
	got, err := LocateComments("xml", []byte(directiveFixture))
	require.NoError(t, err)
	assert.Empty(t, got.Comments)
	require.Len(t, got.Excluded, len(directiveForms))
	for _, f := range directiveForms {
		matched := slices.ContainsFunc(got.Excluded, func(e comment.Excluded) bool { return e.Form == f.Name })
		assert.True(t, matched, "no line in the directive fixture exercises %q", f.Name)
	}

	t.Run("must fail: a classifier missing a directive form", func(t *testing.T) {
		for i, f := range directiveForms {
			t.Run(f.Name, func(t *testing.T) {
				forms := slices.Delete(slices.Clone(directiveForms), i, i+1)
				got, err := locateComments("xml", []byte(directiveFixture), forms)
				require.NoError(t, err)
				assert.NotEmpty(t, got.Comments, "without %q the fixture exposes a directive as prose", f.Name)
			})
		}
	})
}

func TestProseP1_xml(t *testing.T) {
	proseP1(t, "xml", []commenttest.Fixture{
		{Name: "strings.xml", Source: stringsFixture, Literals: stringsLiterals},
		{Name: "crlf.xml", Source: strings.ReplaceAll(stringsFixture, "\n", "\r\n"), Literals: stringsLiterals},
		{Name: "directives.xml", Source: directiveFixture},
	}, corpus{patterns: []string{"*.xml"}})
}

func TestProseP1_androidxml(t *testing.T) {
	proseP1(t, "androidxml", []commenttest.Fixture{
		{Name: "strings.xml", Source: stringsFixture, Literals: stringsLiterals},
	}, corpus{patterns: []string{"*.xml"}, contains: "<resources"})
}

func TestProseP1_resx(t *testing.T) {
	proseP1(t, "resx", []commenttest.Fixture{{Name: "Resources.resx", Source: `<?xml version="1.0" encoding="utf-8"?>
<root>
  <!-- Resources for the start screen. -->
  <data name="Greeting" xml:space="preserve">
    <value>Hello</value>
    <!-- The comment element below is what translators read. -->
    <comment>Shown on the start screen.</comment>
  </data>
  <data name="Markup"><value><![CDATA[<!-- not a comment -->]]></value></data>
</root>
`, Literals: []string{"<![CDATA[<!-- not a comment -->]]>"}}}, corpus{patterns: []string{"*.resx", "*.resw"}})
}

func TestProseP1_tmx(t *testing.T) {
	proseP1(t, "tmx", []commenttest.Fixture{{Name: "memory.tmx", Source: `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header srclang="en" datatype="plaintext" segtype="sentence" adminlang="en" o-tmf="none" creationtool="kapi" creationtoolversion="1"/>
  <body>
    <!-- Entries reviewed for the spring release. -->
    <tu tuid="greeting">
      <tuv xml:lang="en"><seg>Hello</seg></tuv>
      <tuv xml:lang="nb"><seg><![CDATA[<!-- not a comment -->]]></seg></tuv>
    </tu>
  </body>
</tmx>
`, Literals: []string{"<![CDATA[<!-- not a comment -->]]>"}}}, corpus{patterns: []string{"*.tmx"}})
}

func TestProseP1_doclang(t *testing.T) {
	proseP1(t, "doclang", []commenttest.Fixture{{Name: "report.dclg.xml", Source: `<?xml version="1.0" encoding="utf-8"?>
<doclang version="0.6">
  <!-- Converted from the spring report. -->
  <text>Hello</text>
  <code><![CDATA[<!-- not a comment -->]]></code>
</doclang>
`, Literals: []string{"<![CDATA[<!-- not a comment -->]]>"}}}, corpus{patterns: []string{"*.dclg.xml"}})
}

// The XLIFF formats and Qt Linguist read their files with the same provider.
// The repository holds none of their files with a comment, so no corpus backs
// a P1 rung for them, and the suite runs over fixtures alone.
func TestCommentsInFormatsWithoutACommentedCorpus(t *testing.T) {
	for _, tc := range []struct{ format, source string }{
		{"ts", `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="nb">
<context>
    <name>MainWindow</name>
    <!-- Strings on the main window. -->
    <message>
        <source>Open</source>
        <translation><![CDATA[<!-- not a comment -->]]></translation>
    </message>
</context>
</TS>
`},
		{"xliff", `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="nb" datatype="plaintext" original="app">
    <body>
      <!-- Strings for the start screen. -->
      <trans-unit id="greeting">
        <source>Hello</source>
        <target><![CDATA[<!-- not a comment -->]]></target>
      </trans-unit>
    </body>
  </file>
</xliff>
`},
		{"xliff2", `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0" srcLang="en" trgLang="nb">
  <file id="f1">
    <!-- Strings for the start screen. -->
    <unit id="greeting">
      <segment>
        <source>Hello</source>
        <target><![CDATA[<!-- not a comment -->]]></target>
      </segment>
    </unit>
  </file>
</xliff>
`},
	} {
		t.Run(tc.format, func(t *testing.T) {
			commenttest.Run(t, commenttest.Suite{
				Provider: CommentProvider{Format: tc.format},
				Scan:     xmlUnits,
				Fixtures: []commenttest.Fixture{{Name: "app.xlf", Source: tc.source, Literals: []string{"<![CDATA[<!-- not a comment -->]]>"}}},
			})
			got, err := CommentProvider{Format: tc.format}.Locate("app.xlf", []byte(tc.source))
			require.NoError(t, err)
			require.Len(t, got.Comments, 1)
			assert.Equal(t, tc.format, got.Language)
		})
	}
}

// corpus selects the repository's files of one format: tracked files matching
// a pattern, holding contains when it is set.
type corpus struct {
	patterns []string
	contains string
}

// proseP1 is the P1 rung for one format this provider serves: the conformance
// suite over fixtures in the format, the repository's files of the format
// accounted for against the XML parser, and providers broken on purpose that
// the suite must catch. The corpus must hold a comment, or it proves nothing
// about losing one.
func proseP1(t *testing.T, formatName string, fixtures []commenttest.Fixture, c corpus) {
	t.Helper()
	p := CommentProvider{Format: formatName}

	t.Run("the shared conformance suite", func(t *testing.T) {
		commenttest.Run(t, commenttest.Suite{Provider: p, Scan: xmlUnits, Fixtures: fixtures})
	})

	t.Run("the repository's files account for every comment", func(t *testing.T) {
		root := xmlRepoRoot(t)
		files, comments, refused := 0, 0, 0
		for _, rel := range trackedFiles(t, root, c) {
			src, err := os.ReadFile(filepath.Join(root, rel))
			require.NoError(t, err)
			got, err := p.Locate(rel, src)
			if errors.Is(err, ErrCommentsUnlocated) {
				refused++
				t.Logf("%s: %v", rel, err)
				continue
			}
			require.NoError(t, err, rel)
			units, err := xmlUnits(rel, src)
			require.NoError(t, err, rel)
			assert.NoError(t, commenttest.Err(commenttest.Account(src, got, units)), rel)
			files++
			comments += len(units)
		}
		t.Logf("%s-comments files=%d comments=%d refused=%d", formatName, files, comments, refused)
		assert.Positive(t, files, "the corpus holds no %s file the provider reads", formatName)
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
			{"a marker inside a CDATA section counted", countCDATAMarker, commenttest.PropLiteral},
		} {
			t.Run(tc.name, func(t *testing.T) {
				failures := commenttest.Verify(commenttest.Suite{Provider: brokenXML{p, tc.edit}, Scan: xmlUnits, Fixtures: fixtures})
				require.NotEmpty(t, failures)
				assert.Contains(t, commenttest.Properties(failures), tc.want, "%v", commenttest.Err(failures))
			})
		}
	})
}

// xmlUnits is the conformance suite's scan for XML: every comment the XML
// parser reports, at the offsets it reports.
func xmlUnits(_ string, src []byte) ([]commenttest.Unit, error) {
	d := xml.NewDecoder(bytes.NewReader(src))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	var units []commenttest.Unit
	prev := 0
	for {
		tok, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			return units, nil
		}
		if err != nil {
			return nil, err
		}
		end := int(d.InputOffset())
		if c, ok := tok.(xml.Comment); ok {
			_, directive := markup.Classify(string(c), directiveForms)
			units = append(units, commenttest.Unit{Start: prev, End: end, Open: len(markup.Open), Close: len(markup.Close), Group: len(units), Directive: directive})
		}
		prev = end
	}
}

// brokenXML damages what the provider locates in every file with a comment,
// except the canary.
type brokenXML struct {
	CommentProvider
	edit func([]byte, *comment.File)
}

func (b brokenXML) Locate(name string, src []byte) (*comment.File, error) {
	f, err := b.CommentProvider.Locate(name, src)
	if err == nil && name != "canary" && len(f.Comments) > 0 {
		b.edit(src, f)
	}
	return f, err
}

// countCDATAMarker reports the comment marker inside a CDATA section as a
// comment.
func countCDATAMarker(src []byte, f *comment.File) {
	const marker = "<!-- not a comment -->"
	at := bytes.Index(src, []byte("<![CDATA["+marker))
	if at < 0 {
		return
	}
	at += len("<![CDATA[")
	f.Comments = append(f.Comments, comment.Comment{
		Start: at, End: at + len(marker), Lines: format.NewLineIndex(src).Range(at, at+len(marker)),
		Style: comment.StyleBlock, Subject: "comment/document", Runs: []model.Run{model.TextR("not a comment")},
	})
	slices.SortFunc(f.Comments, func(a, b comment.Comment) int { return a.Start - b.Start })
}

func trackedFiles(t *testing.T, root string, c corpus) []string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"ls-files", "-z"}, c.patterns...)...)
	cmd.Dir = root
	listing, err := cmd.Output()
	require.NoError(t, err)
	var out []string
	for rel := range strings.SplitSeq(strings.TrimRight(string(listing), "\x00"), "\x00") {
		if rel == "" || strings.Contains(rel, "node_modules/") {
			continue
		}
		if c.contains != "" {
			src, err := os.ReadFile(filepath.Join(root, rel))
			require.NoError(t, err)
			if !bytes.Contains(src, []byte(c.contains)) {
				continue
			}
		}
		out = append(out, rel)
	}
	return out
}

func xmlRepoRoot(t *testing.T) string {
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
