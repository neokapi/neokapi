package klf

import "fmt"

// ValidationErrorKind discriminates ValidationError reasons.
type ValidationErrorKind string

const (
	ErrMissingPlaceholder  ValidationErrorKind = "missing-placeholder"
	ErrExtraPlaceholder    ValidationErrorKind = "extra-placeholder"
	ErrMalformedRuns       ValidationErrorKind = "malformed-runs"
	ErrUnknownPlaceholder  ValidationErrorKind = "unknown-placeholder"
	ErrUnclosedPairedCode  ValidationErrorKind = "unclosed-paired-code"
	ErrUnmatchedCloseCode  ValidationErrorKind = "unmatched-close-code"
	ErrDuplicatePairedCode ValidationErrorKind = "duplicate-paired-code"
)

// ValidationError describes one problem found by ValidateBlock /
// ValidateTargetAgainstSource.
type ValidationError struct {
	BlockID     string
	Kind        ValidationErrorKind
	Placeholder string
	RunID       string
	Message     string
}

func (e ValidationError) Error() string { return e.Message }

// ValidateBlock checks that a Block's source runs are well-formed
// and self-consistent:
//  1. Every pcOpen has a matching pcClose in the same runs scope.
//  2. No duplicate pcOpen ids in a single scope.
//  3. Every run referenced by `equiv` is declared in Placeholders
//     (or is a plural/select pivot, which is allowed to be declared
//     with kind 'icu-pivot').
//
// Returns an empty slice if the block is valid.
func ValidateBlock(b *Block) []ValidationError {
	var errs []ValidationError
	errs = append(errs, validateRunScope(b.ID, b.Source)...)
	// Scope-walk plural/select forms too.
	walkFormScopes(b.ID, b.Source, &errs)
	// Target scopes get the same structural check.
	for loc, runs := range b.Targets {
		errs = append(errs, validateRunScope(b.ID+":"+loc, runs)...)
		walkFormScopes(b.ID+":"+loc, runs, &errs)
	}
	return errs
}

func validateRunScope(blockID string, runs []Run) []ValidationError {
	var errs []ValidationError
	opens := make(map[string]int) // id -> index of the matching pcOpen
	for i, r := range runs {
		switch {
		case r.PcOpen != nil:
			if _, dup := opens[r.PcOpen.ID]; dup {
				errs = append(errs, ValidationError{
					BlockID: blockID, Kind: ErrDuplicatePairedCode, RunID: r.PcOpen.ID,
					Message: fmt.Sprintf("block %q: duplicate pcOpen id %q in the same runs scope", blockID, r.PcOpen.ID),
				})
			}
			opens[r.PcOpen.ID] = i
		case r.PcClose != nil:
			if _, ok := opens[r.PcClose.ID]; !ok {
				errs = append(errs, ValidationError{
					BlockID: blockID, Kind: ErrUnmatchedCloseCode, RunID: r.PcClose.ID,
					Message: fmt.Sprintf("block %q: pcClose id %q has no matching pcOpen", blockID, r.PcClose.ID),
				})
				continue
			}
			delete(opens, r.PcClose.ID)
		}
	}
	for id := range opens {
		errs = append(errs, ValidationError{
			BlockID: blockID, Kind: ErrUnclosedPairedCode, RunID: id,
			Message: fmt.Sprintf("block %q: pcOpen id %q has no matching pcClose", blockID, id),
		})
	}
	return errs
}

func walkFormScopes(blockID string, runs []Run, errs *[]ValidationError) {
	for _, r := range runs {
		if r.Plural != nil {
			for form, formRuns := range r.Plural.Forms {
				tag := fmt.Sprintf("%s[plural:%s]", blockID, form)
				*errs = append(*errs, validateRunScope(tag, formRuns)...)
				walkFormScopes(tag, formRuns, errs)
			}
		}
		if r.Select != nil {
			for key, caseRuns := range r.Select.Cases {
				tag := fmt.Sprintf("%s[select:%s]", blockID, key)
				*errs = append(*errs, validateRunScope(tag, caseRuns)...)
				walkFormScopes(tag, caseRuns, errs)
			}
		}
	}
}

// ValidateTargetAgainstSource asserts that a target run sequence
// preserves every required source placeholder. A target may add
// content, restructure text, or rewrap paired codes, but it cannot
// drop a required placeholder. Optional placeholders (conditional
// JSX nodes) may be dropped. Returns an empty slice if the target
// is valid.
//
// Mirrors validateTargetAgainstSource from
// packages/format/src/preview.ts.
func ValidateTargetAgainstSource(src *Block, target []Run) []ValidationError {
	var errs []ValidationError
	names := collectRunEquivs(target)

	for _, p := range src.Placeholders {
		if p.Optional {
			continue
		}
		if _, ok := names[p.Name]; !ok {
			errs = append(errs, ValidationError{
				BlockID:     src.ID,
				Kind:        ErrMissingPlaceholder,
				Placeholder: p.Name,
				Message:     fmt.Sprintf("target is missing required placeholder %q", p.Name),
			})
		}
	}
	return errs
}

// collectRunEquivs walks a run sequence (including nested plural /
// select forms) and collects every reference that counts toward
// placeholder preservation: `equiv` of ph / pcOpen / sub runs, plus
// the `pivot` of any plural / select construct encountered.
func collectRunEquivs(runs []Run) map[string]struct{} {
	names := make(map[string]struct{})
	var visit func(rs []Run)
	visit = func(rs []Run) {
		for _, r := range rs {
			switch {
			case r.Ph != nil:
				names[r.Ph.Equiv] = struct{}{}
			case r.PcOpen != nil:
				names[r.PcOpen.Equiv] = struct{}{}
			case r.Sub != nil:
				names[r.Sub.Equiv] = struct{}{}
			case r.Plural != nil:
				names[r.Plural.Pivot] = struct{}{}
				for _, formRuns := range r.Plural.Forms {
					visit(formRuns)
				}
			case r.Select != nil:
				names[r.Select.Pivot] = struct{}{}
				for _, caseRuns := range r.Select.Cases {
					visit(caseRuns)
				}
			}
		}
	}
	visit(runs)
	return names
}
