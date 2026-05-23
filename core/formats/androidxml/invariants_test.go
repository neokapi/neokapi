package androidxml_test

import (
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/androidxml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file asserts in-Go spec invariants for the Android string-resources
// format using only the stdlib (encoding/xml) plus the reader/writer. Pattern:
// extract a real corpus fixture, translate EVERY entry into a pseudo-locale
// while preserving its inline codes (printf args, xliff:g spans, CDATA), write,
// then assert structural and semantic invariants on the OUTPUT bytes. No
// external tool is required, so these run in the normal `go test` pass.

// pseudoTranslateAndroid builds a target for `loc` on every translatable Block,
// prefixing the literal source text with a marker while keeping every non-text
// run (printf %s/%d Ph codes, xliff:g paired codes, CDATA codes, inline-styling
// codes) verbatim and in order — modeling a real translation where text changes
// but inline markup survives untouched.
func pseudoTranslateAndroid(t *testing.T, parts []*model.Part, loc model.LocaleID, marker string) {
	t.Helper()
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok {
			continue
		}
		src := b.SourceRuns()
		out := make([]model.Run, 0, len(src)+1)
		out = append(out, model.Run{Text: &model.TextRun{Text: marker}})
		out = append(out, src...) // keep every run (text + codes) verbatim
		b.SetTargetRuns(loc, out)
	}
}

// assertWellFormedXML parses the bytes with encoding/xml's token scanner and
// fails if any token is malformed (independent of the format's own tokenizer).
func assertWellFormedXML(t *testing.T, b []byte, label string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoErrorf(t, err, "%s must be well-formed XML", label)
	}
}

// androidEntry captures the structural shape parsed independently from a
// resources document.
type androidEntry struct {
	kind         string // "string", "string-array", "plurals"
	name         string
	product      string
	translatable bool // false when translatable="false"
	body         string
}

// parseAndroid parses a resources document with encoding/xml and returns one
// androidEntry per top-level <string>/<string-array>/<plurals> (the body is the
// concatenated raw inner text including markup approximations). Item-level
// granularity is not needed here — these invariants work at entry granularity
// plus the body-content checks below.
func parseAndroid(t *testing.T, b []byte) []androidEntry {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	dec.Strict = true
	var entries []androidEntry
	depth := 0
	var cur *androidEntry
	var body strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "string", "string-array", "plurals":
				if cur == nil {
					ent := androidEntry{kind: e.Name.Local, translatable: true}
					for _, a := range e.Attr {
						switch a.Name.Local {
						case "name":
							ent.name = a.Value
						case "product":
							ent.product = a.Value
						case "translatable":
							if a.Value == "false" {
								ent.translatable = false
							}
						}
					}
					cur = &ent
					body.Reset()
					depth = 0
					continue
				}
				depth++
			default:
				if cur != nil {
					depth++
				}
			}
		case xml.EndElement:
			if cur != nil {
				if depth == 0 {
					cur.body = body.String()
					entries = append(entries, *cur)
					cur = nil
				} else {
					depth--
				}
			}
		case xml.CharData:
			if cur != nil {
				body.Write(e)
			}
		}
	}
	return entries
}

// TestInvariantTranslatedOutput translates every entry of each real corpus
// fixture and asserts spec invariants on the output.
func TestInvariantTranslatedOutput(t *testing.T) {
	t.Parallel()

	const loc = model.LocaleID("qps-ploc")
	const marker = "«X»" // «X» — XML-safe sentinel

	fixtures, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.xml"))
	require.NoError(t, err)
	require.NotEmpty(t, fixtures)

	for _, f := range fixtures {
		t.Run(filepath.Base(f), func(t *testing.T) {
			t.Parallel()

			parts, original := readParts(t, f)
			srcBlocks := blocks(parts)
			require.NotEmpty(t, srcBlocks)

			srcNames := map[string]bool{}
			for _, b := range srcBlocks {
				srcNames[b.Name] = true
			}

			// Independently parse the ORIGINAL: learn the full entry set and the
			// translatable="false" passthrough subset.
			origEntries := parseAndroid(t, original)
			require.NotEmpty(t, origEntries)
			nonTranslatable := map[string]string{} // name -> body
			for _, e := range origEntries {
				if !e.translatable {
					nonTranslatable[e.name+"|"+e.product] = e.body
				}
			}

			pseudoTranslateAndroid(t, parts, loc, marker)
			out := writeParts(t, parts, loc)

			// 1. Output is well-formed XML.
			assertWellFormedXML(t, out, filepath.Base(f))

			// 2. Output re-parses cleanly through the Reader and the SAME set of
			//    translatable Block names survives — none dropped, none added.
			rtParts := readBytes(t, filepath.Base(f), out)
			rtNames := map[string]bool{}
			for _, b := range blocks(rtParts) {
				rtNames[b.Name] = true
			}
			assert.Equal(t, srcNames, rtNames,
				"translatable name set must be preserved across translate+write+reparse")

			// 3. The full entry set is unchanged (no entry dropped or added), and
			//    every translatable="false" entry's body passes through verbatim
			//    (never translated — no marker, body byte-identical).
			outEntries := parseAndroid(t, out)
			assertEntrySetEqual(t, origEntries, outEntries)
			for _, e := range outEntries {
				if !e.translatable {
					key := e.name + "|" + e.product
					orig, ok := nonTranslatable[key]
					require.Truef(t, ok, "unexpected non-translatable entry %q", key)
					assert.Equalf(t, orig, e.body,
						"translatable=\"false\" entry %q must pass through verbatim", key)
					assert.NotContainsf(t, e.body, marker,
						"translatable=\"false\" entry %q must not be translated", key)
				}
			}

			// 4. Placeholders preserved: every printf/xliff:g/CDATA inline code in a
			//    translated block's source re-appears in its target.
			assertPlaceholdersPreserved(t, srcBlocks, loc)

			// 5. The translation actually happened.
			assert.Contains(t, string(out), marker, "translated output must carry the marker")
		})
	}
}

// assertEntrySetEqual checks that the (kind, name, product) tuples and the
// translatable flag of the two entry lists match (order-independent).
func assertEntrySetEqual(t *testing.T, a, b []androidEntry) {
	t.Helper()
	key := func(e androidEntry) string {
		flag := "t"
		if !e.translatable {
			flag = "f"
		}
		return e.kind + "|" + e.name + "|" + e.product + "|" + flag
	}
	set := func(es []androidEntry) map[string]int {
		m := map[string]int{}
		for _, e := range es {
			m[key(e)]++
		}
		return m
	}
	assert.Equal(t, set(a), set(b),
		"the (kind,name,product,translatable) entry set must be unchanged after translation")
}

// assertPlaceholdersPreserved checks every inline-code run (printf Ph, xliff:g
// paired codes, CDATA Ph) in a Block's source re-appears in its rendered target.
func assertPlaceholdersPreserved(t *testing.T, srcBlocks []*model.Block, loc model.LocaleID) {
	t.Helper()
	for _, b := range srcBlocks {
		var codes []string
		for _, r := range b.SourceRuns() {
			switch {
			case r.Ph != nil:
				codes = append(codes, r.Ph.Data)
			case r.PcOpen != nil:
				codes = append(codes, r.PcOpen.Data)
			case r.PcClose != nil:
				codes = append(codes, r.PcClose.Data)
			}
		}
		if len(codes) == 0 {
			continue
		}
		require.Truef(t, b.HasTarget(loc), "block %q should have a target", b.Name)
		tgt := model.RenderRunsWithData(b.TargetRuns(loc))
		for _, c := range codes {
			assert.Containsf(t, tgt, c,
				"inline code %q from %q must survive in the translation", c, b.Name)
		}
	}
}

// readBytes reads Android resource bytes through the Reader and returns parts.
func readBytes(t *testing.T, uri string, data []byte) []*model.Part {
	t.Helper()
	r := androidxml.NewReader()
	require.NoError(t, r.Open(t.Context(), newDoc(uri, data)))
	defer r.Close()
	var parts []*model.Part
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		parts = append(parts, res.Part)
	}
	return parts
}
