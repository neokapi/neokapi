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
// author wrote. The venue then stores the translation without the verdict. A
// push can also take back a sign-off the venue holds, and the venue declines
// that too for a pusher without review permission: the sign-off stands, and
// the venue sends back the record it kept.
//
// The project's own record has to follow, or the two disagree forever: the
// decisions component of the freshness ref would keep differing, so every push
// from then on would send the same refused claims again, be refused again, and
// report it again. So a refused verdict is retired here the way the venue
// retired it, leaving the basis it carried and nothing more, and a refused
// withdrawal is restored to the record the venue kept.
//
// Both are recorded with the venue as their origin, because the venue is who
// reached them: they are its answer about a decision this project sent it.

// retireRefusedVerdicts brings the project's committed record into line with
// what the venue accepted, and reports how many records it changed.
//
// Four rules, because the venue reports refusals four ways. A refusal for
// want of review permission is about the language: the pusher holds none, so
// every verdict this push carried for that language was refused, whether or not
// the venue's bounded per-unit list happens to name it. A refused withdrawal of
// a sign-off is about the unit and carries the record the venue kept, which is
// written back as it is. A refused rejection of a translation the venue has
// since replaced is about the unit as well: the project takes the record the
// venue sends back, or keeps the basis without the rejection when the venue
// holds none. Every other refusal is about the unit, and applies to the units
// the venue named.
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
	held := map[unitKey]venue.UnitDecision{}
	kept := map[unitKey]venue.UnitDecision{}
	staleRejections := map[unitKey]bool{}
	for _, u := range report.Units {
		key := unitKey{item: u.ItemName, unit: u.Unit, variant: u.Variant}
		if u.Reason == venue.RefusedSignOffWithdrawal {
			// A withdrawal the venue refused without saying what it holds
			// leaves nothing to write: the local record cannot invent the
			// sign-off's decider, and a pull settles it.
			if u.Held != nil {
				held[key] = *u.Held
			}
			continue
		}
		if u.Reason == venue.RefusedStaleRejection {
			if u.Held != nil {
				kept[key] = *u.Held
			} else {
				staleRejections[key] = true
			}
			continue
		}
		units[key] = true
	}

	st, err := c.workingStore(ctx)
	if err != nil {
		return 0, fmt.Errorf("open project state: %w", err)
	}
	docPaths := map[string]string{}
	if m, derr := st.DocumentPaths(ctx); derr == nil {
		docPaths = m
	}
	// The push sent every decision this checkout holds, so the venue's report
	// covers all of them.
	all, err := st.All(ctx)
	if err != nil {
		return 0, fmt.Errorf("read project state: %w", err)
	}

	retired := 0
	for _, u := range all {
		item := u.Scope
		if p, ok := docPaths[u.Scope]; ok && p != "" {
			item = p
		}
		variant := variantText(u.Variant)
		key := unitKey{item: item, unit: u.Unit, variant: variant}
		if h, ok := held[key]; ok {
			if err := st.RecordEntry(ctx, withHeld(u, h), h.DecidedBy, state.OriginVenue); err != nil {
				return retired, fmt.Errorf("restore unit state %s/%s: %w", u.Unit, variant, err)
			}
			retired++
			continue
		}
		if h, ok := kept[key]; ok {
			if err := st.RecordEntry(ctx, asVenueRecord(u, h), h.DecidedBy, state.OriginVenue); err != nil {
				return retired, fmt.Errorf("take the venue's record for %s/%s: %w", u.Unit, variant, err)
			}
			retired++
			continue
		}
		if staleRejections[key] && u.Decision.ReviewState == venue.ReviewStateRejected {
			if err := st.RecordEntry(ctx, withoutVerdict(u), "", state.OriginVenue); err != nil {
				return retired, fmt.Errorf("retire unit state %s/%s: %w", u.Unit, variant, err)
			}
			retired++
			continue
		}
		if !carriesVerdict(u) {
			continue
		}
		if !blockedLocales[string(u.Variant.Locale)] && !units[key] {
			continue
		}
		if err := st.RecordEntry(ctx, withoutVerdict(u), "", state.OriginVenue); err != nil {
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
	u.GoverningFingerprint = ""
	return u
}

// withHeld is the record the venue kept when it refused to take back a
// sign-off: the local record with the venue's verdict on it again, and the
// venue's record time, since the record is now the venue's. The hashes stay
// the local record's own; a withdrawal is refused only over the pairing the
// sign-off blessed, so they already agree.
func withHeld(u state.UnitState, h venue.UnitDecision) state.UnitState {
	u.Status = model.TargetStatus(h.Status)
	u.Decision.ReviewState = h.ReviewState
	u.Decision.By = h.DecidedBy
	u.Decision.At = h.DecidedAt
	u.Decision.Note = h.Note
	u.Decision.Parked = h.Parked
	u.Decision.Assignee = h.Assignee
	u.GoverningFingerprint = h.GoverningFingerprint
	if h.Updated != "" {
		u.Updated = h.Updated
	}
	return u
}

// asVenueRecord is the record the venue kept when it refused a rejection of a
// translation it has since replaced: the venue's record whole, hashes included,
// because the local record names a translation the venue no longer holds.
func asVenueRecord(u state.UnitState, h venue.UnitDecision) state.UnitState {
	u = withHeld(u, h)
	u.TargetHash = h.TargetHash
	u.ContentHash = h.ContentHash
	return u
}
