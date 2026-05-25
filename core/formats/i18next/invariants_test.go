package i18next_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The invariants here translate a fixture to a target locale, then RE-PARSE
// the writer's output through the Reader and assert spec-level properties on
// that re-parsed stream. They use only the stdlib + testify and the format's
// own public API — no external tooling — so they always run.

// keyPathSet returns the sorted set of top-level block key paths (the slash
// path names the JSON reader assigns), so two reads can be compared for an
// identical key set.
func keyPathSet(blocks []*model.Block) []string {
	names := make([]string, 0, len(blocks))
	for _, b := range blocks {
		names = append(names, b.Name)
	}
	sort.Strings(names)
	return names
}

// placeholderData returns the protected inline-code data strings of a block's
// source content, in order.
func placeholderData(b *model.Block) []string {
	var ph []string
	for _, run := range b.Source {
		if run.Ph != nil {
			ph = append(ph, run.Ph.Data)
		}
	}
	return ph
}

// TestInvariantTranslatedOutputReparses translates several keys in the rich
// plurals fixture to a target locale, writes for that locale, then asserts the
// OUTPUT re-parses cleanly and preserves every spec invariant:
//
//   - the full key set is preserved, including plural sibling keys;
//   - interpolation ({{count}}) is preserved 1:1 as protected inline codes;
//   - plural annotations (base key, category, group) survive on the re-read;
//   - nesting / namespace structure is intact.
func TestInvariantTranslatedOutputReparses(t *testing.T) {
	resolver := newResolver(t)
	const fr = model.LocaleID("fr-FR")

	srcParts, _ := readParts(t, filepath.Join("testdata", "plurals_v4_en.json"), resolver)
	srcBlocks := collectBlocks(srcParts)
	srcKeys := keyPathSet(srcBlocks)

	// Translate every plural sibling key, keeping the {{count}} placeholder.
	translations := map[string]string{
		"/key_one":          "{{count}} article",
		"/key_other":        "{{count}} articles",
		"/cart/items_zero":  "Votre panier est vide",
		"/cart/items_one":   "{{count}} article dans votre panier",
		"/cart/items_two":   "{{count}} articles dans votre panier",
		"/cart/items_few":   "{{count}} articles dans votre panier",
		"/cart/items_many":  "{{count}} articles dans votre panier",
		"/cart/items_other": "{{count}} articles dans votre panier",
	}
	for _, b := range srcBlocks {
		if tr, ok := translations[b.Name]; ok {
			b.SetTargetText(fr, tr)
		}
	}

	out := writeParts(t, srcParts, fr, resolver)

	// Re-parse the OUTPUT through the reader.
	rrParts := readBytesParts(t, out, resolver)
	rrBlocks := collectBlocks(rrParts)

	// Invariant 1: the key set is preserved 1:1 (no plural sibling dropped or
	// merged, no key added).
	assert.Equal(t, srcKeys, keyPathSet(rrBlocks),
		"translated output must preserve the exact i18next key set, including plural siblings")

	byName := blocksByName(rrBlocks)

	// Invariant 2: {{count}} is preserved 1:1 as a protected inline code on the
	// translated source values, and never leaks into translatable text.
	for _, name := range []string{
		"/key_one", "/key_other",
		"/cart/items_one", "/cart/items_two", "/cart/items_few",
		"/cart/items_many", "/cart/items_other",
	} {
		b := byName[name]
		require.NotNil(t, b, "missing re-parsed block %s", name)
		assert.Equal(t, []string{"{{count}}"}, placeholderData(b),
			"{{count}} must survive as a single protected inline code in %s", name)
		assert.NotContains(t, b.SourceText(), "{{count}}",
			"{{count}} must not appear in the translatable text of %s", name)
	}
	// The empty-cart value carries no interpolation.
	require.NotNil(t, byName["/cart/items_zero"])
	assert.Empty(t, placeholderData(byName["/cart/items_zero"]),
		"items_zero has no interpolation")

	// Invariant 3: plural annotations survive the re-read for the sibling set.
	one := byName["/key_one"]
	other := byName["/key_other"]
	require.NotNil(t, one)
	require.NotNil(t, other)
	assert.Equal(t, "key", one.Properties["i18next.baseKey"])
	assert.Equal(t, "one", one.Properties["i18next.pluralCategory"])
	assert.Equal(t, "other", other.Properties["i18next.pluralCategory"])
	assert.Equal(t, one.Properties["i18next.pluralGroup"], other.Properties["i18next.pluralGroup"],
		"plural siblings must share a group id after re-parse")

	// Invariant 4: the nested namespace (cart.*) is intact — every items_*
	// sibling is scoped under the same cart.items group.
	cats := []string{"zero", "one", "two", "few", "many", "other"}
	var groupID string
	for _, cat := range cats {
		b := byName["/cart/items_"+cat]
		require.NotNil(t, b, "missing nested re-parsed block cart.items_%s", cat)
		assert.Equal(t, "items", b.Properties["i18next.baseKey"])
		assert.Equal(t, "cart.items", b.Properties["i18next.pluralGroup"])
		if groupID == "" {
			groupID = b.Properties["i18next.pluralGroup"]
		}
		assert.Equal(t, groupID, b.Properties["i18next.pluralGroup"])
	}
}

// TestInvariantContextAndNestingPreserved translates the context fixture and a
// nested namespace fixture and verifies that context keys, combined
// context+plural keys, and namespace nesting all survive a translate→write→
// re-parse round.
func TestInvariantContextAndNestingPreserved(t *testing.T) {
	resolver := newResolver(t)
	const de = model.LocaleID("de-DE")

	// Context fixture: translate the context siblings.
	ctxParts, _ := readParts(t, filepath.Join("testdata", "context_en.json"), resolver)
	ctxSrcKeys := keyPathSet(collectBlocks(ctxParts))
	for _, b := range collectBlocks(ctxParts) {
		switch b.Name {
		case "/friend_male":
			b.SetTargetText(de, "Ein Freund")
		case "/friend_female":
			b.SetTargetText(de, "Eine Freundin")
		case "/invite_male_one":
			b.SetTargetText(de, "Lade ihn ein")
		}
	}
	ctxOut := writeParts(t, ctxParts, de, resolver)
	ctxRR := blocksByName(collectBlocks(readBytesParts(t, ctxOut, resolver)))

	// Key set preserved.
	assert.Equal(t, ctxSrcKeys, keyPathSet(collectBlocksMap(ctxRR)),
		"context fixture key set preserved across translate→write→re-parse")

	// Context annotations survive.
	require.NotNil(t, ctxRR["/friend_male"])
	assert.Equal(t, "friend", ctxRR["/friend_male"].Properties["i18next.baseKey"])
	assert.Equal(t, "male", ctxRR["/friend_male"].Properties["i18next.context"])
	assert.Equal(t, "female", ctxRR["/friend_female"].Properties["i18next.context"])

	// Combined context + plural survives.
	combo := ctxRR["/invite_male_one"]
	require.NotNil(t, combo)
	assert.Equal(t, "invite", combo.Properties["i18next.baseKey"])
	assert.Equal(t, "male", combo.Properties["i18next.context"])
	assert.Equal(t, "one", combo.Properties["i18next.pluralCategory"])

	// Namespace fixture: translate a deeply nested value and confirm the nesting
	// path survives re-parse and {{appName}} interpolation is preserved.
	nsParts, _ := readParts(t, filepath.Join("testdata", "namespaces_en.json"), resolver)
	nsSrcKeys := keyPathSet(collectBlocks(nsParts))
	for _, b := range collectBlocks(nsParts) {
		switch b.Name {
		case "/common/actions/save":
			b.SetTargetText(de, "Speichern")
		case "/home/title":
			b.SetTargetText(de, "Willkommen bei {{appName}}")
		}
	}
	nsOut := writeParts(t, nsParts, de, resolver)
	nsRR := blocksByName(collectBlocks(readBytesParts(t, nsOut, resolver)))

	assert.Equal(t, nsSrcKeys, keyPathSet(collectBlocksMap(nsRR)),
		"namespace nesting key set preserved across re-parse")
	require.Contains(t, nsRR, "/common/actions/save", "deep nesting path preserved")
	require.Contains(t, nsRR, "/common/actions/cancel", "sibling nesting path preserved")

	title := nsRR["/home/title"]
	require.NotNil(t, title)
	assert.Equal(t, []string{"{{appName}}"}, placeholderData(title),
		"{{appName}} interpolation preserved 1:1 in the translated title")
}

// collectBlocksMap converts a name→block map back into a slice for keyPathSet.
func collectBlocksMap(m map[string]*model.Block) []*model.Block {
	out := make([]*model.Block, 0, len(m))
	for _, b := range m {
		out = append(out, b)
	}
	return out
}

// readBytesParts parses an in-memory byte slice through the i18next reader and
// returns all parts (mirrors readParts but from bytes via a temp file rather
// than a fixture path), so the writer's output can be fed straight back through
// the reader.
func readBytesParts(t *testing.T, data []byte, resolver format.SubfilterResolver) []*model.Part {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "reparse.json")
	require.NoError(t, os.WriteFile(tmp, data, 0o644))
	parts, _ := readParts(t, tmp, resolver)
	return parts
}
