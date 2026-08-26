package profile_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func terms(pairs ...string) []profile.TermRule {
	out := make([]profile.TermRule, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, profile.TermRule{Term: pairs[i], Replacement: pairs[i+1]})
	}
	return out
}

func termsOf(rules []profile.TermRule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.Term
	}
	return out
}

func TestScopeTermRules(t *testing.T) {
	t.Parallel()

	rules := terms(
		"cart", "basket",
		"sign-in", "log in",
		"workspace", "arbeidsområde",
		"plan", "subscription",
	)

	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "only the rules the text can use",
			text: "Add this to your cart",
			want: []string{"cart"},
		},
		{
			name: "an inflected form keeps its rule",
			text: "Empty your carts before checkout",
			want: []string{"cart"},
		},
		{
			// The reason this matches by word rather than by substring: the
			// substring version sends a rule about shopping to a sentence about
			// printers, and does it silently.
			name: "a longer word is a different word",
			text: "Replace the cartridge",
			want: nil,
		},
		{
			name: "several rules, several matches",
			text: "Open your workspace and choose a plan",
			want: []string{"workspace", "plan"},
		},
		{
			name: "a hyphenated term matches the unhyphenated text",
			text: "Please sign in to continue",
			want: []string{"sign-in"},
		},
		{
			name: "nothing applicable sends nothing",
			text: "Hello there",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profile.ScopeTermRules(rules, tt.text)
			assert.ElementsMatch(t, tt.want, termsOf(got))
		})
	}
}

// TestScopeTermRulesOverABatch: a call sends the union of what its segments can
// use, which is the grain that matters — a batch of twenty carries the rules
// those twenty need, not the rules the collection is governed by.
func TestScopeTermRulesOverABatch(t *testing.T) {
	t.Parallel()

	rules := terms("cart", "basket", "workspace", "arbeidsområde", "invoice", "faktura")
	got := profile.ScopeTermRules(rules,
		"Add this to your cart",
		"Nothing relevant here",
		"Open your workspace settings",
	)

	assert.ElementsMatch(t, []string{"cart", "workspace"}, termsOf(got),
		"the union of the segments, and no more")
}

// TestScopingNeverDropsWhatItCannotJudge: a rule with no term matches nothing by
// construction, and dropping it here would make this a second place that decides
// which rules are real.
func TestScopingNeverDropsWhatItCannotJudge(t *testing.T) {
	t.Parallel()

	rules := append(terms("cart", "basket"), profile.TermRule{Replacement: "orphan"})
	got := profile.ScopeTermRules(rules, "Nothing here at all")

	require.Len(t, got, 1)
	assert.Equal(t, "orphan", got[0].Replacement)
}

// TestNoTextsMeansNoScoping: a caller with no text in hand gets everything,
// because it has nothing to scope against and silently sending nothing would be
// the worst of the three options.
func TestNoTextsMeansNoScoping(t *testing.T) {
	t.Parallel()

	rules := terms("cart", "basket", "workspace", "arbeidsområde")
	assert.Len(t, profile.ScopeTermRules(rules), 2)
}

// TestScopingDoesNotMoveTheFingerprint is the asymmetry, asserted.
//
// What is SENT is scoped to the text; what is HASHED is every rule at the
// coordinate. They answer different questions. The fingerprint is a staleness
// detector, and a rule added about words a block does not contain should still
// re-check that block — the block's wording may have been chosen under the old
// set. Scoping the fingerprint would make a governance change invisible to
// exactly the content it was meant to reach.
//
// If someone "fixes" this asymmetry, this test is what says it was deliberate.
func TestScopingDoesNotMoveTheFingerprint(t *testing.T) {
	t.Parallel()

	all := terms("cart", "basket", "workspace", "arbeidsområde", "invoice", "faktura")
	scoped := profile.ScopeTermRules(all, "Add this to your cart")
	require.Len(t, scoped, 1, "the prompt would carry one rule")

	_, _, fpAll := profile.GovernanceContext(nil, all)
	_, _, fpScoped := profile.GovernanceContext(nil, scoped)

	assert.NotEqual(t, fpAll, fpScoped,
		"the fingerprint must be computed over every rule, never over the scoped subset")
}
