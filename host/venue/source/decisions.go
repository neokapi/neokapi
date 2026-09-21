package source

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// The decisions content type, client half. A push carries the decisions that
// apply to this checkout, read from the project's ledger; a pull records the
// server's ledger into it. Records reconcile last-writer-wins by their Updated
// stamp in both directions.

// workingStore returns the project's decision ledger and this checkout's view
// of it.
//
// Not opened here and not closed by the caller: the App owns the handle, one per
// project root however many times a push or a pull asks for it. Both directions
// write through the handle the surrounding process already holds, which is the
// only way the in-process write gate can order them against whatever else that
// process is writing.
func (c *BowrainSourceConnector) workingStore(ctx context.Context) (*state.WorkStore, error) {
	if c.app == nil {
		return nil, errors.New("connector has no host app, so unit state cannot reach the project store")
	}
	return c.app.OpenProjectState(ctx, c.project.Root)
}

// bindRefsToStore ties this project's recorded positions to the store they were
// consumed into, dropping them when that store has been replaced.
//
// Best-effort: a connector with no App, a build with no file-backed store, and
// a store that will not open all leave the positions where they are. None of
// them is evidence that the store changed, and replaying a change feed on a
// guess would be a full re-pull on every contact.
func (c *BowrainSourceConnector) bindRefsToStore(ctx context.Context) {
	if c.app == nil || c.refs == nil {
		return
	}
	db, err := c.app.ProjectDB(ctx, c.project.Root)
	if err != nil {
		return
	}
	id, err := db.InstanceID(ctx)
	if err != nil {
		return
	}
	c.refs.BindStore(id)
}

// variantText renders a VariantKey in its wire text form ("nb", "fr;tone=…").
func variantText(k model.VariantKey) string {
	b, err := k.MarshalText()
	if err != nil {
		return string(k.Locale)
	}
	return string(b)
}

// projectDecisions reads the decisions that apply to this checkout and maps
// them to the wire form.
//
// It reads the ledger rather than the committed shards, so a decision reaches
// the venue as soon as it is made rather than waiting for someone to write the
// record out. What it sends is what the checkout holds: for each unit, the
// entry recorded for the source and the translation the checkout has now.
//
// The item each unit is scoped to comes from the unit's document key via the
// store's document map when one is recorded, and from the key verbatim
// otherwise. The review path records display paths as keys until the reconcile
// resolver is wired in, so the fallback is the common case today, and both
// satisfy the same rule: the document the unit was decided in, as the connector
// names items.
func (c *BowrainSourceConnector) projectDecisions(ctx context.Context) ([]venue.UnitDecision, error) {
	st, err := c.workingStore(ctx)
	if err != nil {
		return nil, fmt.Errorf("open project state: %w", err)
	}
	// The committed shards are read first, so a push carries what a `git pull`
	// brought into this checkout as well as what was decided in it.
	if err := st.Import(ctx); err != nil {
		return nil, fmt.Errorf("read the project's committed record: %w", err)
	}
	units, err := st.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the project's decisions: %w", err)
	}
	if len(units) == 0 {
		return nil, nil
	}

	docPaths := map[string]string{}
	if m, derr := st.DocumentPaths(ctx); derr == nil {
		docPaths = m
	}

	out := make([]venue.UnitDecision, 0, len(units))
	for _, u := range units {
		item := u.Scope
		if p, ok := docPaths[u.Scope]; ok && p != "" {
			item = p
		}
		out = append(out, venue.UnitDecision{
			ItemName:    item,
			Unit:        u.Unit,
			Variant:     variantText(u.Variant),
			Status:      string(u.Status),
			TargetHash:  u.TargetHash,
			ContentHash: u.ContentHash,
			ReviewState: u.Decision.ReviewState,
			DecidedBy:   u.Decision.By,
			DecidedAt:   u.Decision.At,
			Note:        u.Decision.Note,
			Parked:      u.Decision.Parked,
			Assignee:    u.Decision.Assignee,
			// The context the answer stands under travels with the decision,
			// so the venue's ledger says what the project's record says and a
			// pull brings it back to a checkout that never recorded it.
			GoverningFingerprint: u.GoverningFingerprint,
			Updated:              u.Updated,
		})
	}
	return out, nil
}

// recordPulledDecisions reconciles the server's decision ledger into the
// project's: a record newer than the local one (by Updated) is recorded; an
// older or identical one is left alone. A recorded decision is durable at once,
// and `kapi commit` writes it into the shards along with everything else the
// checkout holds.
//
// Recording is idempotent twice over. An entry is addressed by what it says, so
// a decision the ledger already holds is held once however often it arrives,
// and the venue serves its whole ledger on every pull page rather than from the
// stream position (the pull route lists it in full beside the page of changes).
// So a project whose store was deleted gets its pulled decisions back on the
// next pull, and a project that pulls twice holds them once. The position
// guards the same property from the other side: it is bound to the store that
// consumed it (refcache.Cache.BindStore), so a new store replays the feed
// rather than asking for the changes after a position it never reached.
//
// It also reports how many records it could NOT record. Skipping a decision
// whose variant does not parse is a forward-compatibility case rather than a
// corruption, so it is not fatal, but it must be counted: the reviewer's
// verdict and its attribution are then absent from this checkout with nothing
// anywhere saying so.
func (c *BowrainSourceConnector) recordPulledDecisions(ctx context.Context, pulled []venue.UnitDecision) (recorded, skipped int, err error) {
	if len(pulled) == 0 {
		return 0, 0, nil
	}
	st, err := c.workingStore(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("open project state: %w", err)
	}

	for _, d := range pulled {
		var variant model.VariantKey
		if err := variant.UnmarshalText([]byte(d.Variant)); err != nil || variant.Locale == "" {
			skipped++
			continue
		}
		k := state.Key{Scope: d.ItemName, Unit: d.Unit, Variant: variant}
		if prev, ok := st.Get(ctx, k); ok {
			if prev.Updated != "" && d.Updated != "" && d.Updated <= prev.Updated {
				continue // local record is as new or newer — leave it
			}
		}
		next := state.UnitState{
			Unit:       d.Unit,
			Variant:    variant,
			Status:     model.TargetStatus(d.Status),
			TargetHash: d.TargetHash,
			// The basis rides down with the decision. Without it a pulled
			// approval would arrive with nothing to say which source it blessed,
			// and every unit reviewed on the server would read as current here
			// however far its source had moved since.
			ContentHash: d.ContentHash,
			Decision: state.Decision{
				ReviewState: d.ReviewState,
				By:          d.DecidedBy,
				At:          d.DecidedAt,
				Note:        d.Note,
				Parked:      d.Parked,
				Assignee:    d.Assignee,
			},
			GoverningFingerprint: d.GoverningFingerprint,
			Updated:              d.Updated,
			Scope:                d.ItemName,
		}
		if err := st.RecordEntry(ctx, next, d.DecidedBy, state.OriginVenue); err != nil {
			return recorded, skipped, fmt.Errorf("record unit state %s/%s: %w", d.Unit, d.Variant, err)
		}
		recorded++
	}
	return recorded, skipped, nil
}
