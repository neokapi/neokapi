package xcstrings_test

import (
	"path/filepath"
	"testing"

	xcstrings "github.com/neokapi/neokapi/core/formats/xcstrings"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validXCStringsStates is the closed set of stringUnit "state" values Apple's
// String Catalog tooling emits.
var validXCStringsStates = map[string]bool{
	"new": true, "needs_review": true, "translated": true, "stale": true,
}

// validXCStringsKinds is the set of leaf-location kinds the reader records.
var validXCStringsKinds = map[string]bool{
	"stringUnit": true, "plural": true, "device": true,
	"subPlural": true, "subDevice": true,
}

// cldrPluralCategories is the closed set of CLDR plural keywords used by
// xcstrings plural variations.
var cldrPluralCategories = map[string]bool{
	"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true,
}

// xcDeviceClasses is the set of device-variation classes Xcode emits.
var xcDeviceClasses = map[string]bool{
	"iphone": true, "ipad": true, "mac": true, "applewatch": true,
	"appletv": true, "applevision": true, "other": true,
}

// leafKey is a structural identity for a translatable leaf, independent of its
// human-readable Block.Name.
type leafKey struct {
	key, lang, kind, sub, category string
}

func leafKeyOf(b *model.Block) leafKey {
	return leafKey{
		key:      b.Properties["xcstrings.key"],
		lang:     b.Properties["xcstrings.lang"],
		kind:     b.Properties["xcstrings.kind"],
		sub:      b.Properties["xcstrings.sub"],
		category: b.Properties["xcstrings.category"],
	}
}

// phOf collects placeholder Data from a run sequence.
func phOf(runs []model.Run) []string {
	var out []string
	for _, r := range runs {
		if r.Ph != nil {
			out = append(out, r.Ph.Data)
		}
	}
	return out
}

// leafValueRuns returns the runs that carry the leaf's own value: the target
// runs for a non-source leaf, otherwise the source runs.
func leafValueRuns(b *model.Block) []model.Run {
	lang := model.LocaleID(b.Properties["xcstrings.lang"])
	if b.HasTarget(lang) {
		return b.TargetRuns(lang)
	}
	return b.SourceRuns()
}

// reReadXC parses output bytes and returns the leaf blocks keyed by structural
// identity, asserting no parse error.
func reReadXC(t *testing.T, out []byte) map[leafKey]*model.Block {
	t.Helper()
	r := xcstrings.NewReader()
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI: "out.xcstrings", Encoding: "UTF-8", Reader: ioNopCloser(out),
	}))
	defer r.Close()
	blocks := testutil.CollectBlocks(t, r.Read(t.Context()))
	m := make(map[leafKey]*model.Block, len(blocks))
	for _, b := range blocks {
		m[leafKeyOf(b)] = b
	}
	return m
}

// snapshotXC captures the structural identity, placeholder set, and value runs
// of every leaf block in a parsed catalog.
type xcSnapshot struct {
	keys  map[leafKey]bool
	ph    map[leafKey][]string
	state map[leafKey]string
}

func snapshotXC(blocks []*model.Block) xcSnapshot {
	s := xcSnapshot{
		keys:  make(map[leafKey]bool),
		ph:    make(map[leafKey][]string),
		state: make(map[leafKey]string),
	}
	for _, b := range blocks {
		lk := leafKeyOf(b)
		s.keys[lk] = true
		s.ph[lk] = phOf(leafValueRuns(b))
		s.state[lk] = b.Properties["state"]
	}
	return s
}

// targetLocalesOf returns the distinct non-source localization languages present
// in the parsed blocks.
func targetLocalesOf(blocks []*model.Block, srcLang string) map[string]bool {
	out := make(map[string]bool)
	for _, b := range blocks {
		lang := b.Properties["xcstrings.lang"]
		if lang != "" && lang != srcLang {
			out[lang] = true
		}
	}
	return out
}

// TestInvariantTranslationPreservesStructure translates the values of one
// existing target locale in each fixture (preserving inline placeholders) and
// asserts the rewritten catalog: (1) re-parses cleanly, (2) preserves the exact
// set of translatable leaves (per-locale stringUnit / variations structure
// intact — none dropped or added), (3) keeps placeholders 1:1, (4) keeps every
// "state" valid, and (5) keeps plural categories valid CLDR / device classes
// valid.
func TestInvariantTranslationPreservesStructure(t *testing.T) {
	for _, name := range fixtures() {
		t.Run(name, func(t *testing.T) {
			parts, _ := readParts(t, filepath.Join("testdata", name))
			srcBlocks := testutil.FilterBlocks(parts)
			if len(srcBlocks) == 0 {
				t.Skip("fixture has no translatable leaves")
			}
			before := snapshotXC(srcBlocks)

			// Determine the source language from the layer, then pick one target
			// locale that actually exists in this file to translate.
			srcLang := ""
			for _, p := range parts {
				if p.Type == model.PartLayerStart {
					srcLang = p.Resource.(*model.Layer).Properties["xcstrings.sourceLanguage"]
				}
			}
			targets := targetLocalesOf(srcBlocks, srcLang)
			if len(targets) == 0 {
				t.Skip("fixture has no non-source target locales to translate")
			}
			var tgt string
			for l := range targets {
				tgt = l
				break
			}

			// Translate every leaf whose language is the chosen target, mutating
			// only its text runs and preserving placeholder runs verbatim.
			for _, p := range parts {
				if p.Type != model.PartBlock {
					continue
				}
				b := p.Resource.(*model.Block)
				if b.Properties["xcstrings.lang"] != tgt {
					continue
				}
				lang := model.LocaleID(tgt)
				var nr []model.Run
				for _, r := range b.TargetRuns(lang) {
					if r.Text != nil {
						nr = append(nr, model.Run{Text: &model.TextRun{Text: "TR:" + r.Text.Text}})
					} else {
						nr = append(nr, r)
					}
				}
				b.SetTargetRuns(lang, nr)
				b.Properties["state"] = "translated"
			}

			out := writeParts(t, parts, model.LocaleID(tgt))
			got := reReadXC(t, out)
			assertXCInvariants(t, before, got)
		})
	}
}

// assertXCInvariants checks structure, placeholder, state, and category
// invariants on a re-parsed catalog against the pre-translation snapshot.
func assertXCInvariants(t *testing.T, before xcSnapshot, got map[leafKey]*model.Block) {
	t.Helper()

	// (2) Same set of translatable leaves.
	gotKeys := make(map[leafKey]bool, len(got))
	for lk := range got {
		gotKeys[lk] = true
	}
	assert.Equal(t, before.keys, gotKeys, "translatable leaf set must be preserved")

	for lk, b := range got {
		// (5a) kind is valid.
		assert.Truef(t, validXCStringsKinds[lk.kind], "leaf kind %q invalid for %v", lk.kind, lk)

		// (4) state is valid CLDR-tooling state where present.
		if st := b.Properties["state"]; st != "" {
			assert.Truef(t, validXCStringsStates[st], "state %q invalid for %v", st, lk)
		}

		// (5b) plural categories valid CLDR; device classes valid.
		switch lk.kind {
		case "plural", "subPlural":
			assert.Truef(t, cldrPluralCategories[lk.category],
				"plural category %q invalid for %v", lk.category, lk)
		case "device", "subDevice":
			assert.Truef(t, xcDeviceClasses[lk.category],
				"device class %q invalid for %v", lk.category, lk)
		}

		// (3) placeholders preserved 1:1. For the translated locale we prepended
		// "TR:" to text runs only, so the placeholder Data set is unchanged.
		assert.Equalf(t, before.ph[lk], phOf(leafValueRuns(b)),
			"placeholders must be preserved 1:1 for %v", lk)
	}
}

// TestInvariantCorpusTranslationReParses runs the structural invariants over the
// real-world corpus catalogs.
func TestInvariantCorpusTranslationReParses(t *testing.T) {
	matches, _ := filepath.Glob(filepath.Join("testdata", "corpus", "*.xcstrings"))
	if len(matches) == 0 {
		t.Skip("no corpus files vendored")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			parts, _ := readParts(t, path)
			srcBlocks := testutil.FilterBlocks(parts)
			if len(srcBlocks) == 0 {
				t.Skip("no translatable leaves")
			}
			before := snapshotXC(srcBlocks)

			srcLang := ""
			for _, p := range parts {
				if p.Type == model.PartLayerStart {
					srcLang = p.Resource.(*model.Layer).Properties["xcstrings.sourceLanguage"]
				}
			}
			targets := targetLocalesOf(srcBlocks, srcLang)
			if len(targets) == 0 {
				t.Skip("no non-source target locales")
			}
			var tgt string
			for l := range targets {
				tgt = l
				break
			}

			for _, p := range parts {
				if p.Type != model.PartBlock {
					continue
				}
				b := p.Resource.(*model.Block)
				if b.Properties["xcstrings.lang"] != tgt {
					continue
				}
				lang := model.LocaleID(tgt)
				var nr []model.Run
				for _, r := range b.TargetRuns(lang) {
					if r.Text != nil {
						nr = append(nr, model.Run{Text: &model.TextRun{Text: "TR:" + r.Text.Text}})
					} else {
						nr = append(nr, r)
					}
				}
				b.SetTargetRuns(lang, nr)
			}

			out := writeParts(t, parts, model.LocaleID(tgt))
			got := reReadXC(t, out)
			assertXCInvariants(t, before, got)
		})
	}
}
