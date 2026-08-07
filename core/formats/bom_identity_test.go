package formats

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/registry"
)

// TestBOMIdentityStability sweeps every text format that reads and writes,
// asserting a leading UTF-8 byte-order mark leaves block identity untouched and
// still round-trips byte-exactly.
func TestBOMIdentityStability(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	for _, tc := range bomSweepFormats() {
		t.Run(tc.id, func(t *testing.T) {
			probe := spectest.BOMProbe{
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
				Source: []byte(tc.source),
			}
			probe.Run(t)
		})
	}
}

type bomSweepEntry struct {
	id     string
	source string
}

func bomSweepFormats() []bomSweepEntry {
	return []bomSweepEntry{
		{id: "json", source: `{"greeting":"Hello"}`},
		{id: "yaml", source: "greeting: \"Hello\"\n"},
		{id: "properties", source: "greeting=Hello\n"},
		{id: "xml", source: "<?xml version=\"1.0\"?>\n<root><p>Hello</p></root>\n"},
		{id: "html", source: "<html><body><p>Hello</p></body></html>\n"},
		{id: "csv", source: "id,text\n1,Hello\n"},
		{id: "tsv", source: "id\ttext\n1\tHello\n"},
		{id: "markdown", source: markdownFrontMatterSource},
		{id: "plaintext", source: "Hello\n"},
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
		{id: "tmx", source: tmxSource},
		{id: "ts", source: tsSource},
		{id: "xliff", source: xliffSource},
		{id: "xliff2", source: xliff2Source},
		{id: "asciidoc", source: "= Title\n\nHello\n"},
		{id: "mdx", source: "Hello\n"},
		{id: "messageformat", source: "Hello\n"},
	}
}

// markdownFrontMatterSource carries YAML front matter, the construct a
// byte-order mark turns into body prose.
const markdownFrontMatterSource = `---
title: Greeting
---

Hello
`
