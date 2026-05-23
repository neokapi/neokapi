package applestrings_test

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cldrPluralCategories is the closed set of CLDR plural keywords used by
// .stringsdict plural rules.
var cldrPluralCategories = map[string]bool{
	"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true,
}

// applesLeafKinds is the set of leaf locations the reader records.
var applesLeafKinds = map[string]bool{
	"value": true, "format": true, "plural": true,
}

// appleLeafKey is a structural identity for a translatable leaf.
type appleLeafKey struct {
	key, leaf, variable, category string
}

func appleLeafKeyOf(b *model.Block) appleLeafKey {
	return appleLeafKey{
		key:      b.Properties["applestrings.key"],
		leaf:     b.Properties["applestrings.leaf"],
		variable: b.Properties["applestrings.var"],
		category: b.Properties["applestrings.category"],
	}
}

func phRuns(runs []model.Run) []string {
	var out []string
	for _, r := range runs {
		if r.Ph != nil {
			out = append(out, r.Ph.Data)
		}
	}
	return out
}

// appleSnapshot captures structural identity + placeholders + source value per
// leaf of a parsed document.
type appleSnapshot struct {
	keys map[appleLeafKey]bool
	ph   map[appleLeafKey][]string
}

func snapshotApple(blocks []*model.Block) appleSnapshot {
	s := appleSnapshot{
		keys: make(map[appleLeafKey]bool),
		ph:   make(map[appleLeafKey][]string),
	}
	for _, b := range blocks {
		lk := appleLeafKeyOf(b)
		s.keys[lk] = true
		s.ph[lk] = phRuns(b.SourceRuns())
	}
	return s
}

func reReadApple(t *testing.T, uri string, out []byte) map[appleLeafKey]*model.Block {
	t.Helper()
	parts := readPartsBytes(t, uri, out)
	m := make(map[appleLeafKey]*model.Block)
	for _, b := range testutil.FilterBlocks(parts) {
		m[appleLeafKeyOf(b)] = b
	}
	return m
}

// TestInvariantTranslationPreservesStructure translates every leaf in each
// fixture to a synthetic target locale (preserving printf placeholders and the
// %#@var@ format token) and asserts the rewritten output: (1) re-parses cleanly,
// (2) preserves the exact translatable leaf set (none dropped/added), (3) keeps
// placeholders 1:1, and (4) keeps .stringsdict plural categories valid CLDR.
func TestInvariantTranslationPreservesStructure(t *testing.T) {
	type fixture struct {
		name string
		uri  string // controls kind detection on re-read
	}
	for _, fx := range []fixture{
		{"Localizable.strings", "out.strings"},
		{"Localizable.stringsdict", "out.stringsdict"},
	} {
		t.Run(fx.name, func(t *testing.T) {
			parts, _ := readParts(t, filepath.Join("testdata", fx.name))
			srcBlocks := testutil.FilterBlocks(parts)
			require.NotEmpty(t, srcBlocks, "fixture must have translatable leaves")
			before := snapshotApple(srcBlocks)

			// Translate each leaf, preserving placeholder runs verbatim.
			for _, p := range parts {
				if p.Type != model.PartBlock {
					continue
				}
				b := p.Resource.(*model.Block)
				var nr []model.Run
				for _, r := range b.SourceRuns() {
					if r.Text != nil {
						nr = append(nr, model.Run{Text: &model.TextRun{Text: "TR:" + r.Text.Text}})
					} else {
						nr = append(nr, r)
					}
				}
				b.SetTargetRuns("fr", nr)
			}

			out := writeParts(t, parts, "fr")
			got := reReadApple(t, fx.uri, out)
			assertAppleInvariants(t, before, got)
		})
	}
}

func assertAppleInvariants(t *testing.T, before appleSnapshot, got map[appleLeafKey]*model.Block) {
	t.Helper()

	// (2) Same translatable leaf set.
	gotKeys := make(map[appleLeafKey]bool, len(got))
	for lk := range got {
		gotKeys[lk] = true
	}
	assert.Equal(t, before.keys, gotKeys, "translatable leaf set must be preserved")

	for lk, b := range got {
		// leaf kind valid.
		if lk.leaf != "" {
			assert.Truef(t, applesLeafKinds[lk.leaf], "leaf kind %q invalid for %v", lk.leaf, lk)
		}
		// (4) plural category valid CLDR.
		if lk.leaf == "plural" {
			assert.Truef(t, cldrPluralCategories[lk.category],
				"plural category %q invalid for %v", lk.category, lk)
		}
		// (3) placeholders preserved 1:1. We only mutated text runs, so the
		// placeholder Data set on the value must be unchanged. After re-read the
		// value is the (translated) source for this monolingual model.
		assert.Equalf(t, before.ph[lk], phRuns(b.SourceRuns()),
			"placeholders must be preserved 1:1 for %v", lk)
	}
}

// TestInvariantCorpusTranslationReParses runs the structural invariants over the
// real-world corpus files (both .strings and .stringsdict).
func TestInvariantCorpusTranslationReParses(t *testing.T) {
	var matches []string
	for _, pat := range []string{"*.strings", "*.stringsdict"} {
		m, _ := filepath.Glob(filepath.Join("testdata", "corpus", pat))
		matches = append(matches, m...)
	}
	if len(matches) == 0 {
		t.Skip("no corpus files vendored")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			parts, _ := readParts(t, path)
			srcBlocks := testutil.FilterBlocks(parts)
			require.NotEmpty(t, srcBlocks)
			before := snapshotApple(srcBlocks)

			for _, p := range parts {
				if p.Type != model.PartBlock {
					continue
				}
				b := p.Resource.(*model.Block)
				var nr []model.Run
				for _, r := range b.SourceRuns() {
					if r.Text != nil {
						nr = append(nr, model.Run{Text: &model.TextRun{Text: "TR:" + r.Text.Text}})
					} else {
						nr = append(nr, r)
					}
				}
				b.SetTargetRuns("fr", nr)
			}

			out := writeParts(t, parts, "fr")
			// Re-read with the file's own extension to drive kind detection.
			got := reReadApple(t, filepath.Base(path), out)
			assertAppleInvariants(t, before, got)
		})
	}
}
