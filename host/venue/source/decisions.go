package source

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// The decisions content type, client half. Push carries the project's
// COMMITTED decision record (the git-tracked shards `kapi commit` writes — not
// the staged working set, which is deliberately unpublished work); pull stages
// the server's ledger into the working store, where `kapi commit` remains the
// only door into the tracked record. Records reconcile last-writer-wins by
// their Updated stamp in both directions.

// workingStore returns the project's staging area for decisions — the
// working-set schema of its one store.
//
// Not opened here and not closed by the caller: the App owns the handle, one per
// project root however many times a push or a pull asks for it. The pull's
// staging loop and the push's document-path lookup used to open and close a
// working store each; both now write through the handle the surrounding process
// already holds, which is the only way the in-process write gate can order them
// against whatever else that process is writing.
func (c *BowrainSourceConnector) workingStore(ctx context.Context) (*state.WorkStore, error) {
	if c.app == nil {
		return nil, errors.New("connector has no host app, so unit state cannot reach the project store")
	}
	return c.app.OpenProjectState(ctx, c.project.Root)
}

// variantText renders a VariantKey in its wire text form ("nb", "fr;tone=…").
func variantText(k model.VariantKey) string {
	b, err := k.MarshalText()
	if err != nil {
		return string(k.Locale)
	}
	return string(b)
}

// committedDecisions loads the project's committed decision record and maps it
// to the wire form. The item each unit is scoped to comes from the unit's
// document key via the working store's document map when one is recorded, and
// from the key verbatim otherwise — the review path records display paths as
// keys until the reconcile resolver is wired in, so the fallback IS the common
// case today, and both eras satisfy the same rule: "the document the unit was
// decided in, as the connector names items".
func (c *BowrainSourceConnector) committedDecisions(ctx context.Context) ([]venue.UnitDecision, error) {
	units, err := state.ReadCommitted(c.project.Layout.UnitStateDir())
	if err != nil {
		return nil, fmt.Errorf("read committed unit state: %w", err)
	}
	if len(units) == 0 {
		return nil, nil
	}

	docPaths := map[string]string{}
	if st, err := c.workingStore(ctx); err == nil {
		if m, derr := st.DocumentPaths(ctx); derr == nil {
			docPaths = m
		}
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
			Updated:     u.Updated,
		})
	}
	return out, nil
}

// stagePulledDecisions reconciles the server's decision ledger into the
// project's working store: a record newer than the local one (by Updated)
// replaces it; an older or identical one is left alone. Staged, not committed —
// publishing to the tracked record stays a deliberate act.
// It also reports how many records it could NOT stage. Skipping a decision
// whose variant does not parse is not fatal — a variant spelling this client
// does not understand is a forward-compatibility case, not a corruption — but
// it must be counted, because the pull advances a forward-only stream cursor
// and the server never offers that record again. An uncounted skip is a review
// approval, and its attribution, gone with nothing anywhere saying so.
func (c *BowrainSourceConnector) stagePulledDecisions(ctx context.Context, pulled []venue.UnitDecision) (staged, skipped int, err error) {
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
			Updated: d.Updated,
			Scope:   d.ItemName,
		}
		if err := st.Put(ctx, next); err != nil {
			return staged, skipped, fmt.Errorf("stage unit state %s/%s: %w", d.Unit, d.Variant, err)
		}
		staged++
	}
	return staged, skipped, nil
}
