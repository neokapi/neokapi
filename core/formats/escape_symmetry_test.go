package formats

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/registry"
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
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	for _, tc := range escapeSweepFormats() {
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
				Source:  []byte(tc.source),
				Rejects: tc.rejects,
				Skip:    tc.skip,
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
		{id: "markdown", source: "Hello\n"},
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
		{id: "mdx", source: "Hello\n"},
		{
			id:      "messageformat",
			source:  "Hello\n",
			rejects: func(r rune) bool { return r == '{' },
			skip: map[rune]string{
				'\n': "the line break separates messages",
				'\r': "the line break separates messages",
				'\'': "the apostrophe is ICU quoting inside a pattern, not data",
				'#':  "the hash is the ICU plural placeholder inside a pattern, not data",
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
