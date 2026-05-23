package arb_test

import (
	"path/filepath"
	"strings"
	"testing"

	arb "github.com/neokapi/neokapi/core/formats/arb"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cldrPluralCategories is the closed set of CLDR plural keywords. ICU plural
// branches use these (plus explicit "=N" selectors, which are not keywords).
var cldrPluralCategories = map[string]bool{
	"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true,
}

// phData collects the Data string of every placeholder run in a block's source
// (for source-only ICU constructs) or the given runs.
func phData(runs []model.Run) []string {
	var out []string
	for _, r := range runs {
		if r.Ph != nil {
			out = append(out, r.Ph.Data)
		}
	}
	return out
}

// reReadBlocks parses output bytes through the ARB reader and returns blocks by
// key, asserting no parse error.
func reReadBlocks(t *testing.T, out []byte) map[string]*model.Block {
	t.Helper()
	r := arb.NewReader()
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI: "out.arb", Encoding: "UTF-8", Reader: nopReadCloser(out),
	}))
	defer r.Close()
	blocks := testutil.CollectBlocks(t, r.Read(t.Context()))
	return blocksByName(blocks)
}

// translatePreservingPlaceholders builds a target run sequence that wraps every
// text run with a marker prefix while copying placeholder runs verbatim, so a
// "translation" never drops or mutates an inline code.
func translatePreservingPlaceholders(src []model.Run, prefix string) []model.Run {
	var out []model.Run
	for _, r := range src {
		switch {
		case r.Text != nil:
			out = append(out, model.Run{Text: &model.TextRun{Text: prefix + r.Text.Text}})
		default:
			out = append(out, r)
		}
	}
	return out
}

// TestInvariantTranslationPreservesStructure translates every message in an ARB
// fixture to a synthetic "fr" target (preserving inline ICU placeholders) and
// asserts spec-shaped invariants on the rewritten output: it re-parses cleanly,
// the message-key set is preserved exactly, placeholders survive 1:1, and any
// CLDR plural categories present in the protected ICU constructs stay valid.
func TestInvariantTranslationPreservesStructure(t *testing.T) {
	for _, name := range []string{"simple_en.arb", "icu_en.arb"} {
		t.Run(name, func(t *testing.T) {
			parts, _ := readParts(t, filepath.Join("testdata", name))

			// Snapshot the source key set and per-key placeholder data.
			srcBlocks := testutil.FilterBlocks(parts)
			srcKeys := make(map[string]bool)
			srcPh := make(map[string][]string)
			for _, b := range srcBlocks {
				srcKeys[b.Name] = true
				srcPh[b.Name] = phData(b.SourceRuns())
			}
			require.NotEmpty(t, srcKeys, "fixture must have translatable messages")

			// Translate each block, preserving placeholders exactly.
			for _, p := range parts {
				if p.Type != model.PartBlock {
					continue
				}
				b := p.Resource.(*model.Block)
				b.SetTargetRuns("fr", translatePreservingPlaceholders(b.SourceRuns(), "FR:"))
			}

			out := writeParts(t, parts, "fr")

			// Invariant 1: output re-parses cleanly through the same reader.
			got := reReadBlocks(t, out)

			// Invariant 2: the message-key set is preserved (none dropped/added).
			gotKeys := make(map[string]bool, len(got))
			for k := range got {
				gotKeys[k] = true
			}
			assert.Equal(t, srcKeys, gotKeys, "translatable key set must be preserved")

			// Invariant 3 & 4: placeholders survive 1:1 and CLDR plural
			// categories inside protected ICU constructs remain valid.
			for key, want := range srcPh {
				b := got[key]
				require.NotNilf(t, b, "key %q missing after round-trip", key)
				gotPh := phData(b.SourceRuns())
				assert.Equalf(t, want, gotPh,
					"placeholders for %q must be preserved 1:1", key)
				for _, ph := range gotPh {
					assertICUPluralCategoriesValid(t, key, ph)
				}
			}
		})
	}
}

// assertICUPluralCategoriesValid checks that the bareword selectors inside an
// ICU plural construct are valid CLDR categories, and that the construct stays
// brace-balanced (no corruption). It only inspects constructs of the form
// "{arg, plural, ...}"; select/gender constructs and explicit "=N" selectors are
// not CLDR plural keywords and are out of scope.
func assertICUPluralCategoriesValid(t *testing.T, key, ph string) {
	t.Helper()
	inner := strings.TrimSpace(ph)
	if !strings.HasPrefix(inner, "{") {
		return
	}
	header, isPlural := icuPluralHeader(inner)
	if !isPlural {
		// Still confirm balance for any protected construct.
		assert.Zerof(t, braceBalance(inner), "ICU construct for %q must stay brace-balanced", key)
		return
	}

	// The plural body is everything after the "{arg, plural," header up to the
	// matching close brace. Each direct branch is "<selector>{...}".
	body := inner[header : len(inner)-1] // strip the outer "{...}"
	assert.Zerof(t, braceBalance(inner), "plural construct for %q must stay brace-balanced", key)

	for _, sel := range pluralBranchSelectors(body) {
		if strings.HasPrefix(sel, "=") {
			continue // explicit numeric selector, e.g. "=0"
		}
		assert.Truef(t, cldrPluralCategories[sel],
			"plural selector %q in %q must be a valid CLDR category", sel, key)
	}
}

// icuPluralHeader returns the byte offset just past the "{arg, plural," header
// and whether the construct is an ICU plural. It tolerates the optional
// whitespace ICU allows around the commas.
func icuPluralHeader(s string) (int, bool) {
	// s starts with '{'. Find the first comma → argument name.
	c1 := strings.IndexByte(s, ',')
	if c1 < 0 {
		return 0, false
	}
	rest := s[c1+1:]
	c2 := strings.IndexByte(rest, ',')
	if c2 < 0 {
		return 0, false
	}
	kw := strings.TrimSpace(rest[:c2])
	if kw != "plural" {
		return 0, false
	}
	return c1 + 1 + c2 + 1, true
}

// pluralBranchSelectors extracts the bareword/"=N" selectors that directly
// precede a "{...}" branch at depth 0 of the given plural body.
func pluralBranchSelectors(body string) []string {
	var sels []string
	i := 0
	depth := 0
	for i < len(body) {
		c := body[i]
		switch {
		case c == '{':
			depth++
			i++
		case c == '}':
			depth--
			i++
		case depth == 0 && (isSelectorStart(c)):
			j := i
			for j < len(body) && isSelectorChar(body[j]) {
				j++
			}
			tok := body[i:j]
			k := j
			for k < len(body) && body[k] == ' ' {
				k++
			}
			if k < len(body) && body[k] == '{' {
				sels = append(sels, tok)
			}
			i = j
		default:
			i++
		}
	}
	return sels
}

func braceBalance(s string) int {
	depth := 0
	for i := range len(s) {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
	}
	return depth
}

func isSelectorStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '='
}
func isSelectorChar(c byte) bool {
	return isSelectorStart(c) || c >= '0' && c <= '9' || c == '_'
}

// TestInvariantCorpusTranslationReParses runs the structure-preservation
// invariant over the real-world corpus: translate every message, write, and
// confirm the output re-parses with the identical key set and 1:1 placeholders.
func TestInvariantCorpusTranslationReParses(t *testing.T) {
	matches, _ := filepath.Glob(filepath.Join("testdata", "corpus", "*.arb"))
	if len(matches) == 0 {
		t.Skip("no corpus files vendored")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			parts, _ := readParts(t, path)
			srcBlocks := testutil.FilterBlocks(parts)
			srcKeys := make(map[string]bool)
			srcPh := make(map[string][]string)
			for _, b := range srcBlocks {
				srcKeys[b.Name] = true
				srcPh[b.Name] = phData(b.SourceRuns())
			}
			for _, p := range parts {
				if p.Type != model.PartBlock {
					continue
				}
				b := p.Resource.(*model.Block)
				b.SetTargetRuns("xx", translatePreservingPlaceholders(b.SourceRuns(), ""))
			}
			out := writeParts(t, parts, "xx")
			got := reReadBlocks(t, out)

			gotKeys := make(map[string]bool, len(got))
			for k := range got {
				gotKeys[k] = true
			}
			assert.Equal(t, srcKeys, gotKeys, "corpus key set must be preserved")
			for key, want := range srcPh {
				b := got[key]
				require.NotNilf(t, b, "key %q missing after round-trip", key)
				gotPh := phData(b.SourceRuns())
				assert.Equalf(t, want, gotPh,
					"placeholders for %q must be preserved 1:1", key)
				for _, ph := range gotPh {
					assertICUPluralCategoriesValid(t, key, ph)
				}
			}
		})
	}
}
