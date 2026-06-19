package asciidoc_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// structuralSnapshot captures, per translatable block, its structural identity
// (role + level + ordered inline-code Data) — everything a translation must
// preserve. Keyed by document order so a re-read can be compared 1:1.
type structuralSnapshot struct {
	roles  []string
	levels []int
	codes  [][]string // inline-code Data per block, in order
}

func snapshot(blocks []*model.Block) structuralSnapshot {
	s := structuralSnapshot{}
	for _, b := range blocks {
		role := b.SemanticRole()
		level := 0
		if st, ok := b.Structure(); ok && st != nil {
			level = st.Level
		}
		s.roles = append(s.roles, role)
		s.levels = append(s.levels, level)
		s.codes = append(s.codes, codeData(b.Source))
	}
	return s
}

// codeData returns the Data of every inline-code run (Ph / PcOpen / PcClose) in
// order — the placeholder skeleton a translation must keep 1:1.
func codeData(runs []model.Run) []string {
	var out []string
	for _, r := range runs {
		switch {
		case r.Ph != nil:
			out = append(out, r.Ph.Data)
		case r.PcOpen != nil:
			out = append(out, r.PcOpen.Data)
		case r.PcClose != nil:
			out = append(out, r.PcClose.Data)
		}
	}
	return out
}

// TestInvariantTranslationPreservesStructure reads the exemplar, applies a
// structure-preserving pseudo-translation to every block (upper-casing each text
// run, leaving inline-code runs untouched), writes it back, re-reads, and
// asserts the structure — roles, levels, inline-code skeleton, block count — is
// preserved exactly. Upper-casing keeps every character adjacent to a
// constrained marker intact, so the round-tripped markup re-parses identically.
func TestInvariantTranslationPreservesStructure(t *testing.T) {
	t.Parallel()
	input := mustRead(t, filepath.Join("testdata", "sample.adoc"))

	parts := readParts(t, input)
	srcBlocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, srcBlocks)
	before := snapshot(srcBlocks)

	// Translate every block into French, mutating only TextRun content and
	// preserving inline-code runs verbatim.
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		var nr []model.Run
		for _, r := range b.Source {
			if r.Text != nil {
				nr = append(nr, model.Run{Text: &model.TextRun{Text: strings.ToUpper(r.Text.Text)}})
			} else {
				nr = append(nr, r)
			}
		}
		b.SetTargetRuns(model.LocaleFrench, nr)
	}

	out := writeOriginalParts(t, input, parts, model.LocaleFrench)

	// Re-read the translated document and compare structure.
	after := snapshot(readBlocks(t, out))

	require.Len(t, after.roles, len(before.roles), "block count preserved")
	assert.Equal(t, before.roles, after.roles, "roles preserved")
	assert.Equal(t, before.levels, after.levels, "levels preserved")
	assert.Equal(t, before.codes, after.codes, "inline-code skeleton preserved 1:1")

	// The translation actually took effect (structure intact around content).
	assert.Contains(t, out, "DOCUMENT TITLE")
	assert.Contains(t, out, "== FIRST SECTION")
}

// TestInvariantSourceRoundTripIdempotent asserts read→write→read→write reaches a
// fixpoint (the untranslated projection is stable).
func TestInvariantSourceRoundTripIdempotent(t *testing.T) {
	t.Parallel()
	input := mustRead(t, filepath.Join("testdata", "sample.adoc"))
	first := skelRoundtrip(t, input, "")
	second := skelRoundtrip(t, first, "")
	assert.Equal(t, first, second, "round-trip must be idempotent")
	assert.Equal(t, input, first, "untranslated round-trip equals source")
}
