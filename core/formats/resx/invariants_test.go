package resx_test

import (
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/resx"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file asserts in-Go spec invariants for the .NET RESX format using only
// the stdlib (encoding/xml) plus the reader/writer. The pattern is: extract a
// real corpus fixture, translate EVERY string entry into a pseudo-locale while
// preserving its inline placeholder runs, write the document, and then assert
// structural and semantic invariants on the OUTPUT bytes. No external tool is
// required, so these run in the normal `go test` pass.

// pseudoTranslateRESX builds a target for `loc` on every translatable Block,
// prefixing the source text with a marker while keeping every non-text run (the
// .NET composite-format placeholder Ph runs) verbatim in order. This models a
// real translation: literal text changes; placeholders survive.
func pseudoTranslateRESX(t *testing.T, parts []*model.Part, loc model.LocaleID, marker string) {
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
		for _, r := range src {
			if r.Text != nil {
				// Reverse the literal text so it is visibly "translated" yet still
				// XML-significant characters are exercised on the encode path.
				out = append(out, model.Run{Text: &model.TextRun{Text: reverse(r.Text.Text)}})
			} else {
				out = append(out, r) // keep placeholders/codes verbatim
			}
		}
		b.SetTargetRuns(loc, out)
	}
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// assertWellFormedXML parses the bytes with encoding/xml's token scanner and
// fails if any token is malformed. This is an independent well-formedness check
// (it does not go through the format's own lossless tokenizer).
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

// dataNames returns the set of <data>/@name attributes present in a RESX
// document, parsed independently with encoding/xml. translatableValue maps each
// string <data> name to the raw text of its <value> (typed/binary entries carry
// type=/mimetype= and are reported via the typed set instead).
func parseRESX(t *testing.T, b []byte) (allNames map[string]bool, typedNames map[string]bool, resheaders map[string]string) {
	t.Helper()
	allNames = map[string]bool{}
	typedNames = map[string]bool{}
	resheaders = map[string]string{}

	dec := xml.NewDecoder(strings.NewReader(string(b)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "data":
			var name string
			var typed bool
			for _, a := range se.Attr {
				switch a.Name.Local {
				case "name":
					name = a.Value
				case "type", "mimetype":
					typed = true
				}
			}
			if name != "" {
				allNames[name] = true
				if typed {
					typedNames[name] = true
				}
			}
		case "resheader":
			for _, a := range se.Attr {
				if a.Name.Local == "name" {
					resheaders[a.Value] = a.Value
				}
			}
		}
	}
	return allNames, typedNames, resheaders
}

// TestInvariantTranslatedOutput translates every string entry of each real
// corpus fixture and asserts a battery of spec invariants on the output.
func TestInvariantTranslatedOutput(t *testing.T) {
	t.Parallel()

	const loc = model.LocaleID("qps-ploc")
	const marker = "[x]"

	fixtures := corpusFiles(t)
	require.NotEmpty(t, fixtures)

	for _, f := range fixtures {
		t.Run(baseName(f), func(t *testing.T) {
			t.Parallel()

			parts, original := readParts(t, f)
			srcBlocks := blocks(parts)
			require.NotEmpty(t, srcBlocks)

			// Names extracted by the reader (the translatable set).
			srcNames := map[string]bool{}
			for _, b := range srcBlocks {
				srcNames[b.Name] = true
			}

			// Independently parse the ORIGINAL to learn the full <data> name set,
			// the typed/binary subset, and the resheaders that must pass through.
			origAll, origTyped, origHeaders := parseRESX(t, original)
			require.NotEmpty(t, origHeaders, "every RESX has resheaders")

			pseudoTranslateRESX(t, parts, loc, marker)
			out := writeParts(t, parts, loc)

			// 1. Output is well-formed XML (independent of the lossless tokenizer).
			assertWellFormedXML(t, out, baseName(f))

			// 2. Output re-parses cleanly through the Reader, and the SAME set of
			//    translatable names survives — none dropped, none added.
			rtParts := readBytes(t, baseName(f), out)
			rtNames := map[string]bool{}
			for _, b := range blocks(rtParts) {
				rtNames[b.Name] = true
			}
			assert.Equal(t, srcNames, rtNames,
				"translatable name set must be preserved across translate+write+reparse")

			// 3. Non-translatable passthrough intact: every <data> name in the
			//    original is still present, the typed/binary subset is unchanged,
			//    and the resheaders are byte-present.
			outAll, outTyped, outHeaders := parseRESX(t, out)
			assert.Equal(t, origAll, outAll, "the full <data> name set must be unchanged")
			assert.Equal(t, origTyped, outTyped, "the typed/binary <data> set must be unchanged")
			assert.Equal(t, origHeaders, outHeaders, "resheaders must pass through unchanged")

			// 4. Typed/binary <data> bodies pass through verbatim (never translated):
			//    none carries the translation marker.
			for name := range origTyped {
				assert.NotContains(t, extractDataBody(t, out, name), marker,
					"typed/binary <data name=%q> must not be translated", name)
			}

			// 5. Placeholders preserved: every {0}/{1:t}-style composite-format token
			//    present in a translated string's source re-appears in its target.
			assertPlaceholdersPreserved(t, srcBlocks, loc)

			outStr := string(out)
			// 6. The translation actually happened (marker is present on output).
			assert.Contains(t, outStr, marker, "translated output must carry the marker")
		})
	}
}

// assertPlaceholdersPreserved checks that every composite-format placeholder run
// in a Block's source also appears in its target for loc, in the rendered output.
func assertPlaceholdersPreserved(t *testing.T, srcBlocks []*model.Block, loc model.LocaleID) {
	t.Helper()
	for _, b := range srcBlocks {
		var phs []string
		for _, r := range b.SourceRuns() {
			if r.Ph != nil {
				phs = append(phs, r.Ph.Data)
			}
		}
		if len(phs) == 0 {
			continue
		}
		require.True(t, b.HasTarget(loc), "block %q should have a target", b.Name)
		tgt := model.RenderRunsWithData(b.TargetRuns(loc))
		for _, ph := range phs {
			assert.Containsf(t, tgt, ph,
				"placeholder %q from %q must survive in the translation", ph, b.Name)
		}
	}
}

// extractDataBody returns the inner text of a <data name=...> element parsed
// independently with encoding/xml (CharData concatenated).
func extractDataBody(t *testing.T, b []byte, name string) string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	depth := 0
	in := false
	var body strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "data" && !in {
				for _, a := range e.Attr {
					if a.Name.Local == "name" && a.Value == name {
						in = true
						depth = 0
					}
				}
			} else if in {
				depth++
			}
		case xml.EndElement:
			if in {
				if depth == 0 {
					in = false
				} else {
					depth--
				}
			}
		case xml.CharData:
			if in {
				body.Write(e)
			}
		}
	}
	return body.String()
}

// corpusFiles globs the vendored real-world corpus.
func corpusFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, pat := range []string{"testdata/corpus/*.resx", "testdata/corpus/*.resw"} {
		m, err := filepath.Glob(pat)
		require.NoError(t, err)
		files = append(files, m...)
	}
	return files
}

func baseName(p string) string { return filepath.Base(p) }

// readBytes reads RESX bytes through the Reader and returns the parts.
func readBytes(t *testing.T, uri string, data []byte) []*model.Part {
	t.Helper()
	r := resx.NewReader()
	require.NoError(t, r.Open(t.Context(), newDoc(uri, data)))
	defer r.Close()
	var parts []*model.Part
	for res := range r.Read(t.Context()) {
		require.NoError(t, res.Error)
		parts = append(parts, res.Part)
	}
	return parts
}
