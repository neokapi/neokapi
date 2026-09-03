package source

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// The producer's side of the venue's review governance.
//
// A venue is authoritative about what has been approved in it. A push can carry
// an approval the venue declines to accept, because the pusher holds no review
// permission for that language or the workspace refuses a verdict on work its
// author wrote. The venue then stores the translation without the verdict.
//
// The project's own record has to follow, or the two disagree forever: the
// decisions component of the freshness ref would keep differing, so every push
// from then on would send the same refused approvals again, be refused again,
// and report it again. So a refused verdict is retired here the way the venue
// retired it, leaving the basis it carried and nothing more.
//
// It is not staged. Staging is where a person's own pending decisions wait for
// a commit, and this is not the person's decision: it is the venue's answer
// about a decision they already published.

// retireRefusedVerdicts brings the project's committed record into line with
// what the venue accepted, and reports how many records it retired.
//
// Two rules, because the venue reports refusals two ways. A refusal for want of
// review permission is about the language: the pusher holds none, so every
// verdict this push carried for that language was refused, whether or not the
// venue's bounded per-unit list happens to name it. Every other refusal is
// about the unit, and applies to the units the venue named.
func (c *BowrainSourceConnector) retireRefusedVerdicts(ctx context.Context, report *venue.PushGovernance) (int, error) {
	if report == nil || report.Empty() {
		return 0, nil
	}
	blockedLocales := map[string]bool{}
	for _, r := range report.Refusals {
		if r.Reason == venue.RefusedNoReviewPermission {
			blockedLocales[r.Locale] = true
		}
	}
	units := map[unitKey]bool{}
	for _, u := range report.Units {
		units[unitKey{item: u.ItemName, unit: u.Unit, variant: u.Variant}] = true
	}

	st, err := c.workingStore(ctx)
	if err != nil {
		return 0, fmt.Errorf("open project state: %w", err)
	}
	docPaths := map[string]string{}
	if m, derr := st.DocumentPaths(ctx); derr == nil {
		docPaths = m
	}
	all, err := st.All(ctx)
	if err != nil {
		return 0, fmt.Errorf("read project state: %w", err)
	}
	// A staged decision is one nobody has published: the push sent the
	// committed record, so the venue has not seen it and has not refused it.
	// Retiring it would delete a person's pending work on the strength of an
	// answer about somebody else's.
	staged, err := st.Staged(ctx)
	if err != nil {
		return 0, fmt.Errorf("read staged decisions: %w", err)
	}
	pending := make(map[state.Key]bool, len(staged))
	for _, u := range staged {
		pending[u.Key()] = true
	}

	retired := 0
	for _, u := range all {
		if !carriesVerdict(u) || pending[u.Key()] {
			continue
		}
		item := u.Scope
		if p, ok := docPaths[u.Scope]; ok && p != "" {
			item = p
		}
		variant := variantText(u.Variant)
		if !blockedLocales[string(u.Variant.Locale)] &&
			!units[unitKey{item: item, unit: u.Unit, variant: variant}] {
			continue
		}
		if err := st.Record(ctx, withoutVerdict(u)); err != nil {
			return retired, fmt.Errorf("retire unit state %s/%s: %w", u.Unit, variant, err)
		}
		retired++
	}
	if retired == 0 {
		return 0, nil
	}
	if err := st.PersistRecords(ctx); err != nil {
		return retired, fmt.Errorf("write the project's committed record: %w", err)
	}
	return retired, nil
}

// unitKey is the venue's name for one record: the item that scopes the unit's
// identity, the unit, and the variant.
type unitKey struct{ item, unit, variant string }

// carriesVerdict reports whether a unit's state claims something only a
// reviewer may claim: a review state, or a rung above translated.
func carriesVerdict(u state.UnitState) bool {
	switch u.Decision.ReviewState {
	case venue.ReviewStateApproved, venue.ReviewStateSignedOff:
		return true
	}
	return u.Status.Rank() > model.TargetStatusTranslated.Rank()
}

// withoutVerdict is the record the venue kept: the basis, without the verdict.
// It mirrors venue.UnitDecision.AsBasis exactly, so the project's record and
// the venue's ledger fold to the same component and the next push has nothing
// to send.
func withoutVerdict(u state.UnitState) state.UnitState {
	u.Status = model.TargetStatusTranslated
	u.Decision.ReviewState = ""
	u.Decision.By = ""
	u.Decision.At = ""
	return u
}
