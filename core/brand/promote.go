package brand

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PromoteAndSave loads a profile, applies a correction-derived rule, and — only
// if the profile changed — bumps its version and persists it (the store archives
// the prior version, so the change is auditable and reversible). It returns the
// updated profile and whether it changed. This is the server-side step that
// turns a reviewed suggestion into an enforced, versioned check.
func PromoteAndSave(ctx context.Context, store BrandStore, profileID string, r SuggestedRule) (*VoiceProfile, bool, error) {
	p, err := store.GetProfile(ctx, profileID)
	if err != nil {
		return nil, false, err
	}
	if p == nil {
		return nil, false, fmt.Errorf("brand: profile %q not found", profileID)
	}
	if !ApplySuggestedRule(p, r) {
		return p, false, nil
	}
	p.Version++
	p.UpdatedAt = time.Now().UTC()
	p.VersionNote = fmt.Sprintf("promoted rule: %q", r.Term)
	if err := store.UpdateProfile(ctx, p); err != nil {
		return nil, false, err
	}
	return p, true, nil
}

// ApplySuggestedRule promotes a correction-derived rule into the profile's
// vocabulary: the term a team kept correcting away becomes a forbidden term
// whose replacement is what they corrected it to. This closes the loop — a
// correction made once becomes a deterministic check enforced on every future
// generation, the way fixing a bug once and adding a regression test stops it
// from coming back.
//
// It is idempotent: promoting the same term again updates the existing rule's
// replacement and provenance note instead of adding a duplicate. It reports
// whether the profile changed, so a caller knows whether to bump the version.
func ApplySuggestedRule(p *VoiceProfile, r SuggestedRule) bool {
	if p == nil || strings.TrimSpace(r.Term) == "" {
		return false
	}
	note := provenanceNote(r.CorrectionCount)
	for i := range p.Vocabulary.ForbiddenTerms {
		if strings.EqualFold(p.Vocabulary.ForbiddenTerms[i].Term, r.Term) {
			changed := false
			if r.Replacement != "" && p.Vocabulary.ForbiddenTerms[i].Replacement != r.Replacement {
				p.Vocabulary.ForbiddenTerms[i].Replacement = r.Replacement
				changed = true
			}
			if p.Vocabulary.ForbiddenTerms[i].Note != note {
				p.Vocabulary.ForbiddenTerms[i].Note = note
				changed = true
			}
			return changed
		}
	}
	p.Vocabulary.ForbiddenTerms = append(p.Vocabulary.ForbiddenTerms, TermRule{
		Term:        r.Term,
		Replacement: r.Replacement,
		Note:        note,
	})
	return true
}

// RemoveRule removes a forbidden-term rule (matched by term) from the profile's
// vocabulary. Reports whether the profile changed. The inverse of
// ApplySuggestedRule.
func RemoveRule(p *VoiceProfile, term string) bool {
	if p == nil || strings.TrimSpace(term) == "" {
		return false
	}
	kept := make([]TermRule, 0, len(p.Vocabulary.ForbiddenTerms))
	removed := false
	for _, t := range p.Vocabulary.ForbiddenTerms {
		if strings.EqualFold(t.Term, term) {
			removed = true
			continue
		}
		kept = append(kept, t)
	}
	if removed {
		p.Vocabulary.ForbiddenTerms = kept
	}
	return removed
}

// DemoteAndSave removes a previously promoted rule from a profile and bumps its
// version. The inverse of PromoteAndSave — a promoted brand rule is no longer
// append-only.
func DemoteAndSave(ctx context.Context, store BrandStore, profileID, term string) (*VoiceProfile, bool, error) {
	p, err := store.GetProfile(ctx, profileID)
	if err != nil {
		return nil, false, err
	}
	if p == nil {
		return nil, false, fmt.Errorf("brand: profile %q not found", profileID)
	}
	if !RemoveRule(p, term) {
		return p, false, nil
	}
	p.Version++
	p.UpdatedAt = time.Now().UTC()
	p.VersionNote = fmt.Sprintf("demoted rule: %q", term)
	if err := store.UpdateProfile(ctx, p); err != nil {
		return nil, false, err
	}
	return p, true, nil
}

// PromoteRules applies several suggested rules to a profile and returns how
// many of them changed it.
func PromoteRules(p *VoiceProfile, rules []SuggestedRule) int {
	n := 0
	for _, r := range rules {
		if ApplySuggestedRule(p, r) {
			n++
		}
	}
	return n
}

func provenanceNote(count int) string {
	if count == 1 {
		return "promoted from 1 correction"
	}
	return fmt.Sprintf("promoted from %d corrections", count)
}
