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
// A venue may decline an approval a push carries (no review permission, or
// separation of duties) and store the translation without it, or decline to
// lower an established unit and send back the record it kept.
//
// The project's record follows, or every later push resends the refused
// claims. A refused verdict is withdrawn and the rest of the record stays; a
// refused withdrawal takes the venue's record. A decision refused for its
// note or a reason stays for the person to record again. Each is recorded
// with the venue as its origin.

// retireRefusedVerdicts brings the project's committed record into line with
// what the venue accepted, and reports how many records it changed.
//
// Four rules, because the venue reports refusals four ways. A refusal for
// want of review permission is about the language: the pusher holds none, so
// every verdict this push carried for that language was refused, whether or not
// the venue's bounded per-unit list happens to name it. A refused withdrawal of
// an approval is about the unit and carries the record the venue kept, which is
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
		if u.Reason == venue.RefusedEstablishedWithdrawal {
			// A withdrawal the venue refused without saying what it holds
			// leaves nothing to write: the local record cannot invent the
			// approval's decider, and a pull settles it.
			if u.Held != nil {
				held[key] = *u.Held
			}
			continue
		}
		if u.Reason == venue.RefusedNoteTooLong || u.Reason == venue.RefusedUnknownReason {
			// The decision stays as the person made it; they record it
			// again within the bounds. Retiring it to a basis would drop
			// the verdict without anyone deciding that.
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
	scopes := map[string]string{}
	for _, u := range all {
		item := u.Scope
		if p, ok := docPaths[u.Scope]; ok && p != "" {
			item = p
		}
		scopes[item] = u.Scope
		variant := variantText(u.Variant)
		key := unitKey{item: item, unit: u.Unit, variant: variant}
		if h, ok := held[key]; ok {
			delete(held, key)
			if err := recordVenueAnswer(ctx, st, u, withHeld(u, h), h.DecidedBy); err != nil {
				return retired, fmt.Errorf("restore block state %s/%s: %w", u.Unit, variant, err)
			}
			retired++
			continue
		}
		if h, ok := kept[key]; ok {
			if err := recordVenueAnswer(ctx, st, u, asVenueRecord(u, h), h.DecidedBy); err != nil {
				return retired, fmt.Errorf("take the venue's record for %s/%s: %w", u.Unit, variant, err)
			}
			retired++
			continue
		}
		if staleRejections[key] && u.Decision.ReviewState == venue.ReviewStateRejected {
			if err := recordVenueAnswer(ctx, st, u, withoutVerdict(u), ""); err != nil {
				return retired, fmt.Errorf("retire block state %s/%s: %w", u.Unit, variant, err)
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
		if err := recordVenueAnswer(ctx, st, u, withoutVerdict(u), ""); err != nil {
			return retired, fmt.Errorf("retire block state %s/%s: %w", u.Unit, variant, err)
		}
		retired++
	}
	// An approval the project withdrew holds no record here (the ledger keeps
	// decisions only, and a withdrawal leaves the unit with none), so the
	// approval the venue kept is recorded back from what the venue sent.
	for key, h := range held {
		var variant model.EditionKey
		if err := variant.UnmarshalText([]byte(h.Variant)); err != nil || variant.Locale == "" {
			continue
		}
		scope := key.item
		if s, ok := scopes[key.item]; ok {
			scope = s
		} else {
			for s, p := range docPaths {
				if p == key.item {
					scope = s
					break
				}
			}
		}
		u := withHeld(state.UnitState{Scope: scope, Unit: h.Unit, Variant: variant,
			Revision: h.Revision, Basis: h.Basis}, h)
		if !u.Decides() {
			continue
		}
		if err := st.RecordEntry(ctx, u, h.DecidedBy, state.OriginVenue); err != nil {
			return retired, fmt.Errorf("restore block state %s/%s: %w", h.Unit, h.Variant, err)
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

// recordVenueAnswer records next, the venue's answer about the local record
// was, with the venue as its origin. An answer that decides nothing (the
// basis a refused verdict leaves) withdraws was instead: the ledger holds
// decisions only, and the unit is left with none, as the venue leaves it.
func recordVenueAnswer(ctx context.Context, st *state.WorkStore, was, next state.UnitState, by string) error {
	if next.Decides() {
		return st.RecordEntry(ctx, next, by, state.OriginVenue)
	}
	return st.Delete(ctx, was.Key())
}

// unitKey is the venue's name for one record: the item that scopes the unit's
// identity, the unit, and the variant.
type unitKey struct{ item, unit, variant string }

// carriesVerdict reports whether a unit's state claims something only a
// reviewer may claim: a review state, or a rung above translated.
func carriesVerdict(u state.UnitState) bool {
	if u.Decision.ReviewState == venue.ReviewStateApproved {
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

// withHeld is the record the venue kept when it refused to take back an
// approval: the local record with the venue's verdict on it again, and the
// venue's record time, since the record is now the venue's. The pairing stays
// the local record's own; a withdrawal is refused only over the pairing the
// approval blessed, so the two already agree.
func withHeld(u state.UnitState, h venue.UnitDecision) state.UnitState {
	h = venue.SplitNote(h)
	u.Status = model.TargetStatus(h.Status)
	u.Decision.ReviewState = h.ReviewState
	u.Decision.By = h.DecidedBy
	u.Decision.At = h.DecidedAt
	u.Decision.Note = h.Note
	u.Decision.AgentHost = h.AgentHost
	u.Decision.Parked = h.Parked
	u.Decision.Assignee = h.Assignee
	u.GoverningFingerprint = h.GoverningFingerprint
	if h.Updated != "" {
		u.Updated = h.Updated
	}
	return u
}

// asVenueRecord is the record the venue kept when it refused a rejection of a
// translation it has since replaced: the venue's record whole, its pairing
// included, because the local record names a translation the venue no longer
// holds.
func asVenueRecord(u state.UnitState, h venue.UnitDecision) state.UnitState {
	u = withHeld(u, h)
	u.Revision = h.Revision
	u.Basis = h.Basis
	return u
}
