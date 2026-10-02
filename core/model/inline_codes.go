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
// merely reorders codes. Order is checked separately by pairedCodesKeepShape.
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

// pairedCodesNested reports whether the paired codes in one run scope nest
// properly: every PcClose closes the innermost open pair. Crossed markup such
// as <b><i></b></i> fails.
func pairedCodesNested(runs []Run) bool {
	open := []string{}
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			open = append(open, r.PcOpen.ID)
		case r.PcClose != nil:
			if len(open) == 0 || open[len(open)-1] != r.PcClose.ID {
				return false
			}
			open = open[:len(open)-1]
		}
	}
	return len(open) == 0
}

// pairedCodesBalanced reports whether the paired codes in one run scope are
// balanced: every PcClose has an earlier still-open PcOpen of the same id, and
// no code is left open at the end. Pairs may overlap.
func pairedCodesBalanced(runs []Run) bool {
	open := map[string]int{}
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			open[r.PcOpen.ID]++
		case r.PcClose != nil:
			if open[r.PcClose.ID] == 0 {
				return false
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

// pairedCodesKeepShape reports whether b's paired codes are as well formed as
// a's. When a's pairs nest (HTML, Markdown, OOXML), b's must nest too, because
// crossed pairs would be written as malformed markup. When a's own pairs
// overlap, as XLIFF 1.2 and TMX <bpt>/<ept> pairs may, b needs only every
// close to follow its open.
func pairedCodesKeepShape(a, b []Run) bool {
	if pairedCodesNested(a) {
		return pairedCodesNested(b)
	}
	return pairedCodesBalanced(b)
}

// PairedCodesKeepShape reports whether the paired codes of runs are as well
// formed as those of reference, in one run scope: nested when the reference's
// nest, balanced otherwise.
func PairedCodesKeepShape(reference, runs []Run) bool {
	return pairedCodesKeepShape(reference, runs)
}

// InlineCodesPreserved reports whether b faithfully preserves a's inline codes:
// each scope retains its code multiset, and its paired codes stay nested when
// the source's nest (balanced otherwise). Plural and select constructs retain
// their order, pivots, branch keys and each branch's codes. Invalid run unions
// are rejected. Text may change in any branch.
//
// It is the fidelity guard every structure-preserving edit producer shares
// (apply-edits gates a caller-supplied rewrite on it), so an edit that would
// corrupt inline markup or flatten a plural/select construct leaves the source
// unchanged. It checks structure, not semantic attribute changes or
// vocabulary permissions. Callers reconstruct inline-code metadata from the
// source.
func InlineCodesPreserved(a, b []Run) bool {
	for _, runs := range [][]Run{a, b} {
		for _, r := range runs {
			if !r.Valid() {
				return false
			}
		}
	}
	if !sameInlineCodes(a, b) || !pairedCodesKeepShape(a, b) {
		return false
	}
	// Branches have no IDs, so match them in document order within this scope.
	remaining := b
	for _, source := range a {
		if source.Plural == nil && source.Select == nil {
			continue
		}
		candidate, rest, ok := nextBranch(remaining)
		if !ok || !sameBranchCodes(source, candidate) {
			return false
		}
		remaining = rest
	}
	_, _, extra := nextBranch(remaining)
	return !extra
}

func nextBranch(runs []Run) (Run, []Run, bool) {
	for i, r := range runs {
		if r.Plural != nil || r.Select != nil {
			return r, runs[i+1:], true
		}
	}
	return Run{}, nil, false
}

func sameBranchCodes(a, b Run) bool {
	switch {
	case a.Plural != nil && b.Plural != nil:
		if a.Plural.Pivot != b.Plural.Pivot || len(a.Plural.Forms) != len(b.Plural.Forms) {
			return false
		}
		for key, runs := range a.Plural.Forms {
			other, ok := b.Plural.Forms[key]
			if !ok || !InlineCodesPreserved(runs, other) {
				return false
			}
		}
		return true
	case a.Select != nil && b.Select != nil:
		if a.Select.Pivot != b.Select.Pivot || len(a.Select.Cases) != len(b.Select.Cases) {
			return false
		}
		for key, runs := range a.Select.Cases {
			other, ok := b.Select.Cases[key]
			if !ok || !InlineCodesPreserved(runs, other) {
				return false
			}
		}
		return true
	}
	return false
}
