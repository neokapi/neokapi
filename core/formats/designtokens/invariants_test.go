package designtokens_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The invariants here translate a DTCG fixture's $description prose to a target
// locale, then RE-PARSE the writer's output through the Reader and assert the
// DTCG-specific spec property: ONLY $description values may change; every token
// $value, $type, $extensions, $deprecated and the entire group structure must
// be byte-identical to the source. They use only the stdlib + testify and the
// format's own public API, so they always run.

// stripDescriptions recursively replaces every "$description" value in a parsed
// JSON tree with the empty string, leaving all other keys and values intact.
// Comparing the source and translated-output trees AFTER this normalization
// proves that the ONLY thing the translation changed was $description prose.
func stripDescriptions(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if k == "$description" {
				out[k] = ""
				continue
			}
			out[k] = stripDescriptions(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = stripDescriptions(e)
		}
		return out
	default:
		return v
	}
}

// descriptionValues recursively collects every "$description" string value in a
// parsed JSON tree, sorted, for set comparison.
func descriptionValues(v any) []string {
	var acc []string
	var walk func(any)
	walk = func(n any) {
		switch t := n.(type) {
		case map[string]any:
			for k, val := range t {
				if k == "$description" {
					if s, ok := val.(string); ok {
						acc = append(acc, s)
					}
					continue
				}
				walk(val)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	sort.Strings(acc)
	return acc
}

func mustParseJSON(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal(b, &v), "output is not valid JSON")
	return v
}

// TestInvariantOnlyDescriptionChanges translates several $description values in
// the rich DTCG fixture to fr-FR, writes for that locale, and asserts:
//
//   - the output re-parses cleanly through the Reader;
//   - with all $description values normalized away, the source and translated
//     output JSON trees are IDENTICAL — i.e. every $value, $type, $extensions,
//     $deprecated, and the structure are byte-for-byte preserved;
//   - the translated $description values are present in the output and the
//     untranslated ones keep their source text.
func TestInvariantOnlyDescriptionChanges(t *testing.T) {
	const fr = model.LocaleID("fr-FR")
	srcBytes, err := os.ReadFile(filepath.Join("testdata", "tokens.tokens.json"))
	require.NoError(t, err)

	parts, _ := readParts(t, filepath.Join("testdata", "tokens.tokens.json"))

	translations := map[string]string{
		"/color/primary/$description":     "Couleur de marque principale pour les appels à l'action.",
		"/color/secondary/$description":   "Couleur secondaire discrète pour les éléments de support.",
		"/spacing/small/$description":     "Espacement serré pour les mises en page compactes.",
		"/button/background/$description": "La surface du bouton, alias de la couleur de marque principale.",
	}
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok {
			continue
		}
		if tr, ok := translations[b.Name]; ok {
			b.SetTargetText(fr, tr)
		}
	}

	out := writeParts(t, parts, fr)

	// Invariant 1: the output re-parses cleanly through the reader.
	rrParts := readBytesParts(t, out)
	require.NotEmpty(t, collectBlocks(rrParts), "translated output must re-parse into $description blocks")

	srcTree := mustParseJSON(t, srcBytes)
	outTree := mustParseJSON(t, out)

	// Invariant 2: with $description normalized away, the trees are identical —
	// proving nothing but $description prose changed.
	assert.Equal(t, stripDescriptions(srcTree), stripDescriptions(outTree),
		"only $description values may change; all token values/types/extensions/structure must be preserved")

	// Invariant 3: the translated descriptions are present; the untranslated one
	// keeps its source text.
	outDescs := descriptionValues(outTree)
	for _, tr := range translations {
		assert.Contains(t, outDescs, tr, "translated $description present in output")
	}
	assert.Contains(t, outDescs, "Default reading typeface for body copy.",
		"untranslated $description keeps its source text")

	// Invariant 4: the set of $description KEYS (paths) is unchanged — no
	// description added or dropped by the translation round.
	srcDescBlocks := collectBlocks(parts)
	assert.Equal(t, descriptionKeySet(srcDescBlocks), descriptionKeySet(collectBlocks(rrParts)),
		"the set of $description blocks must be preserved across translate→write→re-parse")
}

// TestInvariantNoDescriptionRoundTrip verifies that a real corpus DTCG file
// with NO $description (the style-dictionary demo) yields no translatable
// surface and re-parses to the same structure after an (empty) write.
func TestInvariantNoDescriptionRoundTrip(t *testing.T) {
	for _, path := range corpusFiles(t) {
		parts, _ := readParts(t, path)
		if len(collectBlocks(parts)) != 0 {
			continue // only the description-free corpus files are exercised here
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			out := writeParts(t, parts, "")
			rr := readBytesParts(t, out)
			assert.Empty(t, collectBlocks(rr), "a description-free token file stays description-free on re-parse")

			srcBytes, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, stripDescriptions(mustParseJSON(t, srcBytes)), stripDescriptions(mustParseJSON(t, out)),
				"structure preserved for description-free corpus file")
		})
	}
}

// descriptionKeySet returns the sorted set of block names (key paths).
func descriptionKeySet(blocks map[string]*model.Block) []string {
	names := make([]string, 0, len(blocks))
	for n := range blocks {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// readBytesParts parses an in-memory byte slice through the design-tokens
// reader (via a temp file) and returns its parts, so the writer's output can be
// fed straight back through the reader.
func readBytesParts(t *testing.T, data []byte) []*model.Part {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "reparse.tokens.json")
	require.NoError(t, os.WriteFile(tmp, data, 0o644))
	parts, _ := readParts(t, tmp)
	return parts
}
