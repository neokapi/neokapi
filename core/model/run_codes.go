package model

import "sort"

// RunsHaveInlineCodes reports whether a run sequence carries anything other
// than plain text — a placeholder, a paired open/close, a subflow reference, a
// plural or a select.
//
// It is the cheap gate in front of the expensive path: a block of pure text can
// be handed to a translator, a writer, or an editor as a string, while one
// holding codes has to travel as runs so the codes survive. The test is the
// absence of Text rather than the presence of any particular code, so a run
// kind added later counts as a code by default — which is the safe direction:
// a new kind is treated as structure to preserve, not as text to flatten.
func RunsHaveInlineCodes(runs []Run) bool {
	for i := range runs {
		if runs[i].Text == nil {
			return true
		}
	}
	return false
}

// RunCodeSignature returns a comparable identity for an inline-code run — the
// unit a translation has to preserve — or ok=false for runs that carry no code
// of their own (text, plural, select).
//
// Paired codes match by ID, placeholders by Equiv (falling back to ID, since a
// reader that mints no equivalence text still numbers its codes), subblock
// references by ID.
func RunCodeSignature(r Run) (string, bool) {
	switch {
	case r.PcOpen != nil:
		return "pc-open:" + r.PcOpen.ID, true
	case r.PcClose != nil:
		return "pc-close:" + r.PcClose.ID, true
	case r.Ph != nil:
		if r.Ph.Equiv != "" {
			return "ph:" + r.Ph.Equiv, true
		}
		return "ph:" + r.Ph.ID, true
	case r.Sub != nil:
		return "sub:" + r.Sub.ID, true
	}
	return "", false
}

// RunCodeCounts returns the inline-code multiset of a Run sequence, keyed by
// RunCodeSignature.
//
// Plural / Select constructs are folded in by taking, per signature, the
// highest count any single branch uses: a placeholder that appears in one
// plural form is required of a translation, but a target whose branch set
// differs from the source's — languages disagree on plural categories — is not
// penalised for the difference.
func RunCodeCounts(runs []Run) map[string]int {
	counts := map[string]int{}
	addRunCodeCounts(counts, runs)
	return counts
}

func addRunCodeCounts(counts map[string]int, runs []Run) {
	for _, r := range runs {
		if sig, ok := RunCodeSignature(r); ok {
			counts[sig]++
			continue
		}
		switch {
		case r.Plural != nil:
			branches := make([][]Run, 0, len(r.Plural.Forms))
			for _, form := range r.Plural.Forms {
				branches = append(branches, form)
			}
			mergeBranchCodeCounts(counts, branches)
		case r.Select != nil:
			branches := make([][]Run, 0, len(r.Select.Cases))
			for _, c := range r.Select.Cases {
				branches = append(branches, c)
			}
			mergeBranchCodeCounts(counts, branches)
		}
	}
}

func mergeBranchCodeCounts(counts map[string]int, branches [][]Run) {
	best := map[string]int{}
	for _, branch := range branches {
		sub := map[string]int{}
		addRunCodeCounts(sub, branch)
		for sig, n := range sub {
			if n > best[sig] {
				best[sig] = n
			}
		}
	}
	for sig, n := range best {
		counts[sig] += n
	}
}

// RunCodeDiff reports how a candidate target's inline codes differ from the
// source's. Both directions are defects, for different reasons:
//
//   - Missing codes are content the reader will never see. A target that
//     dropped `{count}` renders a sentence with a hole where a number belongs
//     — worse than an untranslated string, because nothing signals the loss.
//   - Extra codes are foreign markup: splicing the target in would introduce
//     a tag or placeholder the source never had, which the writer then has to
//     serialize into a document that has no slot for it.
type RunCodeDiff struct {
	// Missing counts, per signature, how many occurrences of a source code the
	// target fails to carry.
	Missing map[string]int
	// Extra counts, per signature, how many occurrences of a target code the
	// source does not have.
	Extra map[string]int
}

// Balanced reports whether the candidate carries exactly the source's inline
// codes — nothing dropped, nothing invented.
func (d RunCodeDiff) Balanced() bool { return len(d.Missing) == 0 && len(d.Extra) == 0 }

// Lossy reports whether the candidate dropped at least one source code.
func (d RunCodeDiff) Lossy() bool { return len(d.Missing) > 0 }

// MissingCodes returns the dropped code signatures in sorted order, for stable
// diagnostics.
func (d RunCodeDiff) MissingCodes() []string { return sortedCodeKeys(d.Missing) }

// ExtraCodes returns the invented code signatures in sorted order, for stable
// diagnostics.
func (d RunCodeDiff) ExtraCodes() []string { return sortedCodeKeys(d.Extra) }

func sortedCodeKeys(m map[string]int) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DiffRunCodes compares the inline-code multisets of a source Run sequence and
// a candidate target Run sequence. It is the shared placeholder-integrity
// predicate for every stage that commits a machine-selected target: TM
// leverage, diff leverage, and the placeholder check all key off it, so
// "a target carries its source's codes" means one thing across the pipeline.
func DiffRunCodes(source, target []Run) RunCodeDiff {
	src := RunCodeCounts(source)
	tgt := RunCodeCounts(target)

	var d RunCodeDiff
	for sig, want := range src {
		if got := tgt[sig]; got < want {
			if d.Missing == nil {
				d.Missing = make(map[string]int)
			}
			d.Missing[sig] = want - got
		}
	}
	for sig, got := range tgt {
		if want := src[sig]; got > want {
			if d.Extra == nil {
				d.Extra = make(map[string]int)
			}
			d.Extra[sig] = got - want
		}
	}
	return d
}
