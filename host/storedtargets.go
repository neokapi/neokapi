package host

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// A convergence pass under a gate writes two records of the same work: the
// draft files it grades itself on, discarded with the run, and the
// `targets/<locale>` overlays in the project block store, which outlive it. For
// a locale that cleared its gate the two agree, because the draft is delivered.
// For a parked locale the store record is the only one left.
//
// Every read surface goes through bilingualBlocks, which reads the delivered
// file, so the store is what it falls back to when there is none (#2356).
// Without that fallback a locale the loop had just drafted read as 0%
// `blocked: translate`, its units reached no review queue, and the `reviewed:`
// half of its ship gate was out of reach by construction.
//
// A stored target is the same translation the delivered file would hold. What a
// parked locale lacks is delivery, which is what the gate withholds and what
// stays withheld.

// storedTargetBlocks reads the unit's source blocks and overlays the project
// block store's `targets/<locale>` records onto them, returning found=false
// when the store holds nothing for this unit (no project root, no store, or no
// overlay for any of its blocks).
//
// The read binding is the unit's own, for the reason absorbStoreTargets spells
// out: the overlays are addressed by the file-local block id, so a read that
// splits the document differently pairs each block's source with another
// block's translation.
func (a *App) storedTargetBlocks(ctx context.Context, u VerifyUnit) ([]*model.Block, bool, error) {
	if u.ProjectRoot == "" || u.Locale == "" {
		return nil, false, nil
	}
	store := a.storedTargetStore(ctx, u.ProjectRoot)
	if store == nil {
		return nil, false, nil
	}
	blocks, err := a.readSource(ctx, u)
	if err != nil {
		return nil, false, fmt.Errorf("read source %s: %w", u.SourcePath, err)
	}
	sess, err := store.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("read the stored targets for %s: %w", u.Locale, err)
	}
	defer sess.Close()

	loc := model.LocaleID(u.Locale)
	found := false
	for _, b := range blocks {
		kind, key, ok := storedTargetKey(u, b)
		if !ok {
			continue
		}
		o, oerr := sess.GetOverlay(kind, key)
		if oerr != nil {
			// Absence is ordinary pending work; a store that cannot be read is
			// a fault, and conflating the two reports translations that exist
			// as translations nobody made (the shape of #1449).
			if errors.Is(oerr, blockstore.ErrNotFound) {
				continue
			}
			return nil, false, fmt.Errorf("read %s overlay for block %s of %s: %w", kind, blockKey(b), u.DisplayPath, oerr)
		}
		if len(o.Payload) == 0 {
			continue
		}
		if aerr := applyTargetOverlay(b, loc, o.Payload); aerr != nil {
			return nil, false, aerr
		}
		if !model.RunsHaveContent(b.TargetRuns(loc)) {
			continue
		}
		normalizeStoredTargetStatus(b, loc)
		found = true
	}
	if !found {
		return nil, false, nil
	}
	for _, b := range blocks {
		b.SourceLocale = model.LocaleID(a.SourceLocale())
	}
	return blocks, true, nil
}

// storedTargetKey names the overlay the store holds for one block of a unit:
// the `targets/<locale>` kind and the key the producers wrote it under, which
// is blockstore.OverlayKey as the file runner tags it (the source's project
// namespace, the block's id). ok is false for a block no producer keys a
// stored target by, so a reader skips exactly what a writer never wrote.
//
// Every read of a stored target derives its key here, so the readers cannot
// drift from each other, and none of them can drift from the writers without
// every one of them noticing.
func storedTargetKey(u VerifyUnit, b *model.Block) (kind, key string, ok bool) {
	if b == nil || !b.Translatable || b.ID == "" {
		return "", "", false
	}
	rel := blockstore.SourceNamespace(u.ProjectRoot, u.SourcePath)
	return blockstore.TargetOverlayKind(model.LocaleID(u.Locale)), blockstore.StoreKey(rel, b.ID, b.SourceText()), true
}

// normalizeStoredTargetStatus grades a stored target at the rung its delivered
// copy would read.
//
// A JSON or YAML target file carries no lifecycle metadata, so a delivered
// translation reads as `translated` (convergence.TargetState). The producers
// stamp `draft` on the overlay they write, so honoring that stamp would grade
// one record of the work a rung below the other: the locale would read
// `blocked: translate` at 0% translated while holding a complete set of
// translations, and the review queue, which lists translated units, would list
// none of them. A rung ABOVE translated is a decision that travelled with the
// overlay (a .kpz round-trip), and it is kept.
//
// core/convergence.TallyBlockStore counts the same overlays at `translated` for
// the desktop status panel, so the two block-store readers agree.
func normalizeStoredTargetStatus(b *model.Block, loc model.LocaleID) {
	t := b.Target(loc)
	if t == nil {
		return
	}
	if t.Status.Rank() < model.TargetStatusTranslated.Rank() {
		t.Status = ""
	}
}

// storedTargetStore resolves the block store to read stored targets from, or
// nil when this build or this project has none. A project with no store has
// nothing to say about a missing target, which is what the caller already
// reports.
//
// The store file has to be there already. Opening one creates it, and this is a
// read on a path every measurement takes, `up --plan` and a cold checkout
// among them: a dry run that leaves a database behind has written to the
// project it promised to only price.
func (a *App) storedTargetStore(ctx context.Context, root string) blockstore.Store {
	if a.BlocksBackend != nil {
		return a.BlocksBackend
	}
	layout, lerr := project.ResolveLayout(root)
	if lerr != nil {
		return nil
	}
	if _, serr := os.Stat(layout.StorePath()); serr != nil {
		return nil
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil || db == nil {
		return nil
	}
	// Autocommit: this is a read, and a read-only session on the transactional
	// handle would hold the write permit for its whole life, parking any
	// converge run writing beside it.
	return db.BlocksAutocommit()
}
