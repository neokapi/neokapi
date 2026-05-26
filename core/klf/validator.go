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
//  1. Every run carries exactly one discriminator (malformed-runs).
//  2. Every pcOpen has a matching pcClose in the same runs scope.
//  3. No duplicate pcOpen ids in a single scope.
//  4. Every reference made by a source run (a ph / pcOpen / sub
//     `equiv`, or a plural / select `pivot`) is declared in the
//     block's Placeholders list (unknown-placeholder).
//
// The structural checks (1–3) run over the source runs and over every
// target run sequence (including nested plural forms and select
// cases). The declaration check (4) runs over the source runs only —
// targets are checked against the source by
// ValidateTargetAgainstSource, which permits a target to re-wrap or
// restructure markup so long as it preserves required placeholders.
//
// Returns an empty slice if the block is valid.
func ValidateBlock(b *Block) []ValidationError {
	var errs []ValidationError
	errs = append(errs, validateRunShape(b.ID, b.Source)...)
	errs = append(errs, validateRunScope(b.ID, b.Source)...)
	// Scope-walk plural/select forms too.
	walkFormScopes(b.ID, b.Source, &errs)
	// Every reference a source run makes must be declared.
	errs = append(errs, validateDeclaredPlaceholders(b)...)
	// Target scopes get the same structural checks.
	for loc, runs := range b.Targets {
		errs = append(errs, validateRunShape(b.ID+":"+loc, runs)...)
		errs = append(errs, validateRunScope(b.ID+":"+loc, runs)...)
		walkFormScopes(b.ID+":"+loc, runs, &errs)
	}
	return errs
}

// validateRunShape walks a run sequence (recursing into plural forms
// and select cases) and flags every run that is not a well-formed
// discriminated union: a run with no discriminator set, or a run with
// more than one. The JSON decoder (model.Run.UnmarshalJSON) rejects
// these on the wire, but a Block assembled in memory by a tool can
// still carry a malformed run, so ValidateBlock guards the
// programmatic path.
func validateRunShape(blockID string, runs []Run) []ValidationError {
	var errs []ValidationError
	for i := range runs {
		r := runs[i]
		if n := discriminatorCount(r); n != 1 {
			detail := "no discriminator set"
			if n > 1 {
				detail = fmt.Sprintf("%d discriminators set", n)
			}
			errs = append(errs, ValidationError{
				BlockID: blockID, Kind: ErrMalformedRuns, RunID: r.RunID(),
				Message: fmt.Sprintf("block %q: run at index %d is malformed (%s)", blockID, i, detail),
			})
		}
		// Recurse into plural/select children regardless — a malformed
		// outer run with extra discriminators may still carry forms.
		if r.Plural != nil {
			for form, formRuns := range r.Plural.Forms {
				errs = append(errs, validateRunShape(fmt.Sprintf("%s[plural:%s]", blockID, form), formRuns)...)
			}
		}
		if r.Select != nil {
			for key, caseRuns := range r.Select.Cases {
				errs = append(errs, validateRunShape(fmt.Sprintf("%s[select:%s]", blockID, key), caseRuns)...)
			}
		}
	}
	return errs
}

// discriminatorCount returns how many discriminator fields a run has
// set. Exactly one is well-formed; zero or more than one is
// malformed-runs.
func discriminatorCount(r Run) int {
	n := 0
	if r.Text != nil {
		n++
	}
	if r.Ph != nil {
		n++
	}
	if r.PcOpen != nil {
		n++
	}
	if r.PcClose != nil {
		n++
	}
	if r.Sub != nil {
		n++
	}
	if r.Plural != nil {
		n++
	}
	if r.Select != nil {
		n++
	}
	return n
}

// validateDeclaredPlaceholders flags every reference a source run
// makes (ph / pcOpen / sub `equiv`, or plural / select `pivot`) that
// is not declared in the block's Placeholders list. pcClose runs
// repeat their partner's equiv for locality and so are not checked
// independently. Returns one unknown-placeholder error per offending
// reference name (deduplicated).
func validateDeclaredPlaceholders(b *Block) []ValidationError {
	declared := make(map[string]struct{}, len(b.Placeholders))
	for _, p := range b.Placeholders {
		declared[p.Name] = struct{}{}
	}

	var errs []ValidationError
	reported := make(map[string]struct{})
	report := func(name string) {
		if name == "" {
			return
		}
		if _, ok := declared[name]; ok {
			return
		}
		if _, dup := reported[name]; dup {
			return
		}
		reported[name] = struct{}{}
		errs = append(errs, ValidationError{
			BlockID:     b.ID,
			Kind:        ErrUnknownPlaceholder,
			Placeholder: name,
			Message:     fmt.Sprintf("block %q: run references placeholder %q not declared in placeholders", b.ID, name),
		})
	}

	var visit func(rs []Run)
	visit = func(rs []Run) {
		for _, r := range rs {
			switch {
			case r.Ph != nil:
				report(r.Ph.Equiv)
			case r.PcOpen != nil:
				report(r.PcOpen.Equiv)
			case r.Sub != nil:
				report(r.Sub.Equiv)
			case r.Plural != nil:
				report(r.Plural.Pivot)
				for _, formRuns := range r.Plural.Forms {
					visit(formRuns)
				}
			case r.Select != nil:
				report(r.Select.Pivot)
				for _, caseRuns := range r.Select.Cases {
					visit(caseRuns)
				}
			}
		}
	}
	visit(b.Source)
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

// ValidateTargetAgainstSource asserts that a target run sequence is
// placeholder-faithful to its source:
//
//   - It preserves every required source placeholder. A target may
//     add content, restructure text, or rewrap paired codes, but it
//     cannot drop a required placeholder (missing-placeholder).
//     Optional placeholders (conditional JSX nodes) may be dropped.
//   - It does not introduce a placeholder the source never declared
//     (extra-placeholder) — symmetric to the missing-placeholder
//     check. The set of legitimate target references is the union of
//     the source's declared Placeholders and the references its own
//     source runs make, so a target that reuses any source
//     placeholder is accepted while a freshly invented one is flagged.
//
// Returns an empty slice if the target is valid.
//
// Mirrors validateTargetAgainstSource from
// packages/kapi-format/src/preview.ts.
func ValidateTargetAgainstSource(src *Block, target []Run) []ValidationError {
	var errs []ValidationError
	targetNames := collectRunEquivs(target)

	for _, p := range src.Placeholders {
		if p.Optional {
			continue
		}
		if _, ok := targetNames[p.Name]; !ok {
			errs = append(errs, ValidationError{
				BlockID:     src.ID,
				Kind:        ErrMissingPlaceholder,
				Placeholder: p.Name,
				Message:     fmt.Sprintf("target is missing required placeholder %q", p.Name),
			})
		}
	}

	// The allowed set is every placeholder the source declares plus
	// every reference the source runs make (the two usually coincide,
	// but a source run may legitimately reference a name not yet in
	// the Placeholders list — that's caught by ValidateBlock's
	// unknown-placeholder check, not here).
	allowed := make(map[string]struct{}, len(src.Placeholders))
	for _, p := range src.Placeholders {
		allowed[p.Name] = struct{}{}
	}
	for name := range collectRunEquivs(src.Source) {
		allowed[name] = struct{}{}
	}
	for name := range targetNames {
		if name == "" {
			continue
		}
		if _, ok := allowed[name]; !ok {
			errs = append(errs, ValidationError{
				BlockID:     src.ID,
				Kind:        ErrExtraPlaceholder,
				Placeholder: name,
				Message:     fmt.Sprintf("target introduces placeholder %q not present in the source", name),
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
