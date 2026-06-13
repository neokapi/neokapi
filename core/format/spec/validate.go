package spec

import (
	"fmt"
	"regexp"
)

// validate.go is the meta-schema gate (format-spec-cases.md §8): the
// structural case-shape checks the design's CI gate requires, beyond the
// legacy semantic checks in load.go's Validate. It is invoked by Validate()
// so a malformed case is rejected at Load() time — the precondition for
// trusting generated cases. Every check keys off the new optional case
// fields, so the ~41 existing specs (no id/class/cite/expected) pass
// unchanged.

// caseIDPattern is the stable case-id contract: 4–6 alphanumeric chars
// (yaml-test-suite IDs, §2).
var caseIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{4,6}$`)

// validRoundtripModes is the closed mode enum for expected.roundtrip.
var validRoundtripModes = map[string]bool{
	RoundtripByteExact:  true,
	RoundtripIdempotent: true,
	RoundtripNormalized: true,
}

// validateCases enforces the case-level meta-schema across every example in
// the spec: id format + uniqueness, class enum, citation completeness, the
// one-fault-per-invalid-case rule, and view/class coherence.
func (s *Spec) validateCases() error {
	seenID := map[string]string{} // id -> "feature/example" for a friendly dup message
	for _, f := range s.Features {
		for _, ex := range f.Examples {
			where := fmt.Sprintf("feature %q example %q", f.ID, ex.Name)
			if err := validateCaseID(ex, where, seenID, f.ID); err != nil {
				return err
			}
			if err := validateClass(ex, where); err != nil {
				return err
			}
			if err := validateCite(ex, where); err != nil {
				return err
			}
			if err := validateExpected(ex, where); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCaseID(ex Example, where string, seen map[string]string, featureID string) error {
	if ex.ID == "" {
		return nil // legacy examples address by name; id is optional
	}
	if !caseIDPattern.MatchString(ex.ID) {
		return fmt.Errorf("%s: case id %q must be 4–6 alphanumeric chars", where, ex.ID)
	}
	loc := featureID + "/" + ex.Name
	if prev, dup := seen[ex.ID]; dup {
		return fmt.Errorf("case id %q declared twice (%s and %s)", ex.ID, prev, loc)
	}
	seen[ex.ID] = loc
	return nil
}

func validateClass(ex Example, where string) error {
	switch ex.Class {
	case "", ClassValid, ClassInvalid, ClassOperation:
		return nil
	default:
		return fmt.Errorf("%s: class %q must be %q, %q, or %q", where, ex.Class, ClassValid, ClassInvalid, ClassOperation)
	}
}

func validateCite(ex Example, where string) error {
	if ex.Cite == nil {
		return nil
	}
	if ex.Cite.Spec == "" {
		return fmt.Errorf("%s: cite present but cite.spec is empty", where)
	}
	if ex.Cite.URL == "" {
		return fmt.Errorf("%s: cite present but cite.url is empty", where)
	}
	return nil
}

func validateExpected(ex Example, where string) error {
	class := ex.CaseClass()
	exp := ex.Expected

	if class == ClassInvalid {
		// One fault per invalid case: exactly expected.error, nothing else.
		if exp == nil || exp.Error == nil {
			return fmt.Errorf("%s: class: invalid requires expected.error", where)
		}
		if exp.Error.Category == "" {
			return fmt.Errorf("%s: expected.error.category is required", where)
		}
		if exp.Blocks != "" || exp.Roundtrip != nil || exp.Extracted != nil || exp.ValidBy != "" {
			return fmt.Errorf("%s: class: invalid case carries a non-error view (one fault per case)", where)
		}
		return nil
	}

	// Non-invalid cases must not declare an error expectation.
	if exp != nil && exp.Error != nil {
		return fmt.Errorf("%s: expected.error is only valid on class: invalid cases", where)
	}
	if exp != nil && exp.Roundtrip != nil {
		mode := exp.Roundtrip.Mode
		if mode == "" {
			return fmt.Errorf("%s: expected.roundtrip.mode is required", where)
		}
		if !validRoundtripModes[mode] {
			return fmt.Errorf("%s: expected.roundtrip.mode %q must be %q, %q, or %q", where, mode, RoundtripByteExact, RoundtripIdempotent, RoundtripNormalized)
		}
		if mode == RoundtripNormalized && exp.Roundtrip.OutputFile == "" {
			return fmt.Errorf("%s: expected.roundtrip mode normalized requires output_file", where)
		}
	}
	return nil
}
