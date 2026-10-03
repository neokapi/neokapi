package formats

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/xmlesc"
)

// TestEscapeSymmetryOnModify sweeps every text format that reads and writes,
// driving read → modify → write → reparse over the escape corpus.
//
// A format passes when a translated value carrying any corpus character comes
// back out of its own reader intact, or when the writer refuses a character the
// format genuinely cannot represent. Emitting a document that will not parse is
// the failure this sweep exists to catch, and skeleton replay hides it: an
// untouched value is written back as its original bytes, so the escape path is
// never exercised until a tool changes the text.
func TestEscapeSymmetryOnModify(t *testing.T) {
	runEscapeSweep(t, false)
}

// TestEscapeSymmetryOnSourceEdit runs the same sweep over the monolingual
// edit path: the source is rewritten and the writer is given no locale, which
// is how `kapi apply`, `ksed` and the MCP apply_edits tool write a document
// back. An edit's text is text, so a `<` or `&` in it must come back out of
// the reader as the character it was, never as markup.
func TestEscapeSymmetryOnSourceEdit(t *testing.T) {
	runEscapeSweep(t, true)
}

func runEscapeSweep(t *testing.T, editSource bool) {
	t.Helper()
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	for _, tc := range append(escapeSweepFormats(), containerSweepFormats(t)...) {
		t.Run(tc.id, func(t *testing.T) {
			probe := spectest.ModifyProbe{
				Format: tc.id,
				NewReader: func() format.DataFormatReader {
					r, err := reg.NewReader(registry.FormatID(tc.id))
					if err != nil {
						t.Fatalf("new reader: %v", err)
					}
					return r
				},
				NewWriter: func() format.DataFormatWriter {
					w, err := reg.NewWriter(registry.FormatID(tc.id))
					if err != nil {
						t.Fatalf("new writer: %v", err)
					}
					return w
				},
				Source:     []byte(tc.source),
				Rejects:    tc.rejects,
				Skip:       tc.skip,
				EditSource: editSource,
				TextOf:     tc.textOf,
			}
			probe.Run(t)
		})
	}
}

// escapeSweepEntry is one format's participation in the sweep: a minimal
// document with a translatable value, plus the characters that format cannot
// carry in that position.
type escapeSweepEntry struct {
	id      string
	source  string
	rejects func(rune) bool
	skip    map[rune]string
	// textOf reads a block back as the text a reader of the document sees;
	// nil reads the runs as they render.
	textOf func([]model.Run) string
}

func escapeSweepFormats() []escapeSweepEntry {
	// A line-oriented format gives the line break the role of a record
	// separator: a value carrying one becomes two records, which is the format
	// working as specified rather than a writer failing to escape.
	lineOriented := map[rune]string{
		'\n': "the line break separates records",
		'\r': "the line break separates records",
	}
	return []escapeSweepEntry{
		{id: "json", source: `{"greeting":"Hello"}`},
		{id: "yaml", source: "greeting: \"Hello\"\n"},
		{id: "properties", source: "greeting=Hello\n"},
		{
			id:      "xml",
			source:  "<?xml version=\"1.0\"?>\n<root><p>Hello</p></root>\n",
			rejects: notRepresentableInXML,
			skip: map[rune]string{
				'\t': "whitespace collapses inside a non-preserve container",
				'\n': "whitespace collapses inside a non-preserve container",
				'\r': "whitespace collapses inside a non-preserve container",
			},
		},
		{
			id:     "html",
			source: "<html><body><p>Hello</p></body></html>\n",
			skip: map[rune]string{
				'\t': "HTML collapses whitespace in normal flow",
				'\n': "HTML collapses whitespace in normal flow",
				'\f': "HTML collapses whitespace in normal flow",
				'\r': "HTML collapses whitespace in normal flow",
				// A parser discards a NUL character token (HTML5 §13.2.6.4.7)
				// and `&#0;` is a parse error, so the character is not
				// representable. Refusing the write is not yet the right
				// response: until the declared encoding selects a codec
				// (#1714) a UTF-16 document reaches the writer as text full of
				// NUL bytes, and rejecting it would fail a file that reads
				// today.
				0: "a parser discards a NUL character token; see #1714",
			},
		},
		{id: "csv", source: "id,text\n1,Hello\n"},
		{id: "tsv", source: "id\ttext\n1\tHello\n"},
		{id: "markdown", source: "Hello\n", textOf: commonMarkText},
		{id: "plaintext", source: "Hello\n", skip: lineOriented},
		{id: "po", source: "msgid \"Hello\"\nmsgstr \"\"\n"},
		{id: "androidxml", source: "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n    <string name=\"greeting\">Hello</string>\n</resources>\n"},
		{id: "applestrings", source: "\"greeting\" = \"Hello\";\n"},
		{id: "arb", source: "{\n  \"greeting\": \"Hello\"\n}\n"},
		{id: "i18next", source: "{\"greeting\":\"Hello\"}"},
		{id: "designtokens", source: "{\"color\":{\"$type\":\"color\",\"accent\":{\"$value\":\"#ff5722\",\"$description\":\"Hello\"}}}"},
		{id: "xcstrings", source: xcstringsSource},
		{id: "resx", source: resxSource},
		{id: "srt", source: "1\n00:00:01,000 --> 00:00:02,000\nHello\n\n"},
		{id: "vtt", source: "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello\n"},
		{id: "tmx", source: tmxSource, rejects: notRepresentableInXML},
		{id: "ts", source: tsSource, rejects: notRepresentableInXML},
		{id: "xliff", source: xliffSource, rejects: notRepresentableInXML},
		{id: "xliff2", source: xliff2Source, rejects: notRepresentableInXML},
		{id: "asciidoc", source: "Hello\n"},
		{id: "mdx", source: "Hello\n", textOf: commonMarkText},
		{
			// The writer quotes ICU syntax in text, so a brace, an apostrophe
			// and a hash read back as the characters written.
			id:     "messageformat",
			source: "Hello\n",
			skip: map[rune]string{
				'\n': "the line break separates messages",
				'\r': "the line break separates messages",
			},
		},
	}
}

// notRepresentableInXML reports the characters XML 1.0 §2.2 excludes, for which
// a writer is required to fail rather than emit a file that will not reopen.
func notRepresentableInXML(r rune) bool { return !xmlesc.ValidChar(r) }

const xcstringsSource = `{
  "sourceLanguage" : "en",
  "strings" : {
    "greeting" : {
      "localizations" : {
        "en" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Hello"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`

const resxSource = `<?xml version="1.0" encoding="utf-8"?>
<root>
  <data name="greeting" xml:space="preserve">
    <value>Hello</value>
  </data>
</root>
`

const tmxSource = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="kapi" creationtoolversion="1" segtype="sentence" o-tmf="tmx" adminlang="en" srclang="en" datatype="plaintext"/>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Hello</seg></tuv>
    </tu>
  </body>
</tmx>
`

// tsSource carries the prologue the ts writer normalizes to, so a round-trip
// assertion measures the content path rather than the prologue rewrite.
const tsSource = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE TS []>
<TS version="2.1" language="fr">
<context>
    <name>Main</name>
    <message>
        <source>Hello</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>
`

const xliffSource = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="a.txt" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
      </trans-unit>
    </body>
  </file>
</xliff>
`

const xliff2Source = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1">
      <segment>
        <source>Hello</source>
        <target>Bonjour</target>
      </segment>
    </unit>
  </file>
</xliff>
`

// commonMarkText reads Markdown text runs as CommonMark shows them: a
// backslash before ASCII punctuation is the punctuation, and a character
// reference is its character.
func commonMarkText(runs []model.Run) string {
	src := model.RenderRunsWithData(runs)
	var b strings.Builder
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\\' && i+1 < len(src) && strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", src[i+1]) >= 0:
			b.WriteByte(src[i+1])
			i++
		case c == '&':
			if m := charRefRE.FindString(src[i:]); m != "" {
				b.WriteString(html.UnescapeString(m))
				i += len(m) - 1
				continue
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

var charRefRE = regexp.MustCompile(`^&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]{1,7}|#[xX][0-9A-Fa-f]{1,6});`)

// asciidocText reads AsciiDoc text runs back with the character references the
// writer escapes markup with decoded. A reference an author or an edit
// spells is AsciiDoc source the processor passes to the page as is, so it
// stays as written.
func asciidocText(runs []model.Run) string {
	return asciidocEscapes.Replace(model.RenderRunsWithData(runs))
}

var asciidocEscapes = strings.NewReplacer("&#43;", "+", "&#91;", "[", "&#123;", "{", "&#60;", "<")

// TestEditedWordingReadsBackAsText edits a block of each format whose text
// can spell markup with wording that spells some, and reads the written
// document back. The wording must come back as text: no inline code beyond a
// character reference, and the characters typed. A writer that left the
// markup live hands the reader a tag, a link or a macro, which it reads as an
// inline code. Markup a reader does not model (an MDX expression, an
// AsciiDoc passthrough) reads back as text either way; the format's own
// writer tests check how it is written.
func TestEditedWordingReadsBackAsText(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	payloads := []string{
		"Hello <script>alert(1)</script> & goodbye",
		"<div onmouseover=alert(1) x",
		"a\n<div onclick=alert(1)",
		"<!-- hidden",
		"[x]: javascript:alert(1)\nclick [x]",
		"click [a [b] c](javascript:alert(1))",
		"see <https://evil.example/> and ![i](javascript:alert(1))",
		"x {process.exit(1)} <Danger/>",
		"x +++<b>raw</b>+++ y pass:[<img src=x onerror=alert(1)>]",
		"go link:javascript:alert(1)[here] and <<sec,there>>",
		"Fish &amp; chips &lt;3 and a \"quote\" 'too'",
	}
	formats := []struct {
		id     string
		source string
		textOf func([]model.Run) string
	}{
		{id: "html", source: "<html><body><p>Hello</p></body></html>\n", textOf: model.RunsEditText},
		{id: "markdown", source: "Hello\n", textOf: commonMarkText},
		{id: "mdx", source: "Hello\n", textOf: commonMarkText},
		{id: "asciidoc", source: "Hello\n", textOf: asciidocText},
	}
	space := regexp.MustCompile(`\s+`)
	for _, f := range formats {
		for _, payload := range payloads {
			t.Run(f.id+"/"+payload, func(t *testing.T) {
				reader, err := reg.NewReader(registry.FormatID(f.id))
				require.NoError(t, err)
				writer, err := reg.NewWriter(registry.FormatID(f.id))
				require.NoError(t, err)
				store, err := format.NewWiredSkeleton(reader, writer)
				require.NoError(t, err)
				if store != nil {
					defer store.Close()
				}
				parts, err := spec.ReadParts(reader, []byte(f.source))
				require.NoError(t, err)
				for _, p := range parts {
					if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
						b.EditSourceRuns(model.ParseRunsEditText(payload, b.SourceRuns()))
					}
				}
				out, err := spec.WriteParts(writer, parts, []byte(f.source))
				require.NoError(t, err)

				newReader, err := reg.NewReader(registry.FormatID(f.id))
				require.NoError(t, err)
				reparsed, err := spec.ReadParts(newReader, out)
				require.NoError(t, err)
				var texts []string
				for _, p := range reparsed {
					b, ok := p.Resource.(*model.Block)
					if !ok || !b.Translatable {
						continue
					}
					for _, r := range b.SourceRuns() {
						if r.Text != nil {
							continue
						}
						_, isRef := model.CharacterReference(r.Ph)
						assert.True(t, isRef, "the edit came back as markup %+v\noutput: %q", r, out)
					}
					texts = append(texts, space.ReplaceAllString(f.textOf(b.SourceRuns()), " "))
				}
				assert.Contains(t, texts, space.ReplaceAllString(payload, " "), "output: %q", out)
			})
		}
	}
}
