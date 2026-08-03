package asciidoc_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// adocNames maps each block's source text to the name the reader gave it.
func adocNames(t *testing.T, input string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, b := range readBlocks(t, input) {
		out[b.SourceText()] = b.Name
	}
	return out
}

// Deleting a paragraph does not rename the ones after it.
//
// A heading trail is not a counter, so a block's address depends on the
// headings above it and nothing else. Under a counter every block below the
// deletion shifts, and a decision recorded about one paragraph silently becomes
// a decision about its neighbour.
func TestAsciiDocIdentity_SurvivesAnUnrelatedDeletion(t *testing.T) {
	t.Parallel()

	const v1 = "== Install\n\nAlpha\n\n== Configure\n\nBravo\n\nCharlie\n"
	const v2 = "== Install\n\n== Configure\n\nBravo\n\nCharlie\n"

	before := adocNames(t, v1)
	after := adocNames(t, v2)

	require.Contains(t, before, "Charlie")
	require.Contains(t, after, "Charlie")
	assert.Equal(t, before["Charlie"], after["Charlie"],
		"a paragraph after an unrelated deletion in another section keeps its name")
	assert.Equal(t, "Configure/p#2", before["Charlie"],
		"the address is the heading trail plus an ordinal scoped to that heading")
}

// A paragraph is addressed by the headings above it, so the same prose in two
// sections is two different blocks.
func TestAsciiDocIdentity_HeadingTrailSeparatesSections(t *testing.T) {
	t.Parallel()

	// Both paragraphs carry the same text, so the blocks are read directly
	// rather than through a text-keyed map.
	blocks := readBlocks(t, "== Install\n\nRun it.\n\n== Configure\n\nRun it.\n")
	var paras []string
	for _, b := range blocks {
		if b.SourceText() == "Run it." {
			paras = append(paras, b.Name)
		}
	}
	require.Len(t, paras, 2)
	assert.Equal(t, []string{"Install/p", "Configure/p"}, paras,
		"identical text in different sections gets distinct names")
}

// Nested headings nest in the name.
func TestAsciiDocIdentity_NestedHeadingsNest(t *testing.T) {
	t.Parallel()

	names := adocNames(t, "== Install\n\n=== Prerequisites\n\nNeeds Go.\n")

	// Content under a heading is addressed by that heading's words — readable,
	// and stable when a sibling section is added elsewhere.
	assert.Equal(t, "Install/Prerequisites/p", names["Needs Go."])

	// But the heading itself is addressed by POSITION, not by its own words.
	// See TestAsciiDocIdentity_RewordingAHeadingIsAnEdit for why that matters.
	assert.Equal(t, "Install/h3", names["Prerequisites"])
}

// The reason a heading is not named after itself.
//
// A block's name is folded into its context hash, and its text is its content
// hash. Name a heading after its own title and rewording it moves both signals
// at once, so reconcile has nothing linking the new heading to the old one: it
// grades as new and drops the translation and the approval it had. Named by
// position, a reworded heading moves content alone — which is exactly what an
// edit is.
func TestAsciiDocIdentity_RewordingAHeadingIsAnEdit(t *testing.T) {
	t.Parallel()

	const scope = "d-guide"
	v1 := readBlocks(t, "== Install\n\nNeeds Go.\n")
	v2 := readBlocks(t, "== Installation\n\nNeeds Go.\n")

	var priors []reconcile.Unit
	for i, r := range reconcile.Blocks(scope, v1, nil) {
		u := reconcile.Identify(scope, v1[i])
		u.Key = r.Key
		priors = append(priors, u)
	}

	var heading reconcile.Result
	for _, r := range reconcile.Blocks(scope, v2, priors) {
		if r.Block.SourceText() == "Installation" {
			heading = r
		}
	}

	require.NotNil(t, heading.Block, "the reworded heading must be read")
	assert.Equal(t, reconcile.Edited, heading.Kind,
		"a reworded heading is the same heading with new words, not a new heading")

	var was string
	for _, p := range priors {
		if p.ContentHash == model.ComputeContentHash("Install") {
			was = p.Key
		}
	}
	assert.Equal(t, was, heading.Key, "and it keeps the unit its approval was recorded against")
}

// An anchor is an address the author wrote down, so it IS the name — and it
// keeps the block's identity even when the block moves to another section.
func TestAsciiDocIdentity_AnchorWins(t *testing.T) {
	t.Parallel()

	before := adocNames(t, "== Install\n\n[[warning-note]]\nMind the gap.\n")
	after := adocNames(t, "== Configure\n\n=== Later\n\n[[warning-note]]\nMind the gap.\n")

	assert.Equal(t, "warning-note", before["Mind the gap."])
	assert.Equal(t, before["Mind the gap."], after["Mind the gap."],
		"an anchored block keeps its name across a move — that is what an anchor is for")
}

// The `[#id]` shorthand is the same address in different clothes.
func TestAsciiDocIdentity_IDAttributeShorthand(t *testing.T) {
	t.Parallel()

	names := adocNames(t, "== Install\n\n[#lead.summary]\nThe short version.\n")
	assert.Equal(t, "lead", names["The short version."])
}

// An anchor on a heading names the heading and everything under it.
func TestAsciiDocIdentity_AnchoredHeadingNamesItsSection(t *testing.T) {
	t.Parallel()

	names := adocNames(t, "[[setup]]\n== Installing the thing\n\nAlpha\n")
	assert.Equal(t, "setup", names["Installing the thing"])
	assert.Equal(t, "setup/p", names["Alpha"])
}

// Every block in a document gets a distinct name, including repeats of the same
// text under one heading.
func TestAsciiDocIdentity_NamesAreUnique(t *testing.T) {
	t.Parallel()

	const input = "= Guide\n\n== Install\n\nSame\n\nSame\n\n* Same\n* Same\n\n" +
		"|===\n|Same |Same\n|Same |Same\n|===\n"

	seen := map[string]bool{}
	for _, b := range readBlocks(t, input) {
		assert.False(t, seen[b.Name], "duplicate block name %q", b.Name)
		seen[b.Name] = true
	}
	assert.NotEmpty(t, seen)
}

// A cell is addressed by its table and its position in it, so a second table
// under the same heading does not renumber the first one's cells.
func TestAsciiDocIdentity_TableCellsAddressedByTable(t *testing.T) {
	t.Parallel()

	const input = "== Data\n\n|===\n|A |B\n|===\n\n|===\n|C |D\n|===\n"
	names := adocNames(t, input)
	assert.Equal(t, "Data/table/r0c0", names["A"])
	assert.Equal(t, "Data/table/r0c1", names["B"])
	assert.Equal(t, "Data/table#2/r0c0", names["C"])
	assert.Equal(t, "Data/table#2/r0c1", names["D"])
}

// Reading the same document twice produces the same names.
func TestAsciiDocIdentity_StableAcrossTwoReads(t *testing.T) {
	t.Parallel()

	const input = "= Guide\n\n== Install\n\nAlpha\n\n* One\n* Two\n\n" +
		"|===\n|H1 |H2\n\n|c1 |c2\n|===\n"
	first := adocNames(t, input)
	second := adocNames(t, input)
	assert.Equal(t, first, second, "two reads of the same bytes must name blocks identically")
}

// Naming is identity, not a join key: the skeleton round-trip is byte-exact
// through all of it.
func TestAsciiDocIdentity_RoundTripUnaffected(t *testing.T) {
	t.Parallel()

	const input = "= Guide\n\n[[setup]]\n== Install\n\nAlpha\n\n" +
		"* One\n* Two\n\n|===\n|H1 |H2\n\n|c1 |c2\n|===\n"
	assert.Equal(t, input, skelRoundtrip(t, input, ""))
}
