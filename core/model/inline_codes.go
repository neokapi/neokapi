package model

// inlineCodeKey identifies one inline-code run for the fidelity guard.
type inlineCodeKey struct{ kind, id string }

// inlineCodeMultiset counts the inline-code runs in a flat run sequence by
// kind+id. Text runs are ignored — only the non-text codes (placeholders,
// paired open/close, sub-flows) must survive a faithful rewrite.
func inlineCodeMultiset(runs []Run) map[inlineCodeKey]int {
	m := map[inlineCodeKey]int{}
	for _, r := range runs {
		switch {
		case r.Ph != nil:
			m[inlineCodeKey{"ph", r.Ph.ID}]++
		case r.PcOpen != nil:
			m[inlineCodeKey{"pcOpen", r.PcOpen.ID}]++
		case r.PcClose != nil:
			m[inlineCodeKey{"pcClose", r.PcClose.ID}]++
		case r.Sub != nil:
			m[inlineCodeKey{"sub", r.Sub.ID}]++
		}
	}
	return m
}

// sameInlineCodes reports whether two run sequences carry exactly the same
// multiset of inline codes (each code present the same number of times). It
// catches an edit that drops, invents, or duplicates a code, but not one that
// merely reorders codes — order is checked separately by pairedCodesBalanced.
func sameInlineCodes(a, b []Run) bool {
	ma, mb := inlineCodeMultiset(a), inlineCodeMultiset(b)
	if len(ma) != len(mb) {
		return false
	}
	for k, v := range ma {
		if mb[k] != v {
			return false
		}
	}
	return true
}

// pairedCodesBalanced reports whether the paired open/close codes in a run
// sequence are balanced: every PcClose has an earlier still-open PcOpen of the
// same id, and no code is left open at the end. An edit that reorders a close
// ahead of its open (same multiset, unbalanced markup) is rejected here. The
// source side comes from a parser and is already balanced; this validates the
// reconstructed sequence.
func pairedCodesBalanced(runs []Run) bool {
	open := map[string]int{}
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			open[r.PcOpen.ID]++
		case r.PcClose != nil:
			if open[r.PcClose.ID] == 0 {
				return false // a close with no matching open before it
			}
			open[r.PcClose.ID]--
		}
	}
	for _, n := range open {
		if n != 0 {
			return false
		}
	}
	return true
}

// InlineCodesPreserved reports whether b faithfully preserves a's inline codes:
// the same multiset of codes AND well-balanced paired open/close codes. It is
// the condition under which an edit cannot unbalance the document's inline
// markup by dropping, inventing, duplicating, or reordering a code.
//
// This is the fidelity guard shared by every faithful, structure-preserving
// edit producer: the AI rewrite tool (provider-driven) and the apply-edits tool
// (caller-supplied) both gate a block's rewrite on it, so a rewrite that would
// corrupt inline markup leaves the source unchanged rather than write malformed
// structure. It lives in core/model so the caller-supplied path needs no
// dependency on providers/ai.
func InlineCodesPreserved(a, b []Run) bool {
	return sameInlineCodes(a, b) && pairedCodesBalanced(b)
}
