package host

import (
	"context"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
)

// A producer (the AI and MT translate tools) keeps what it drafted as a
// `targets/<locale>` overlay in the project block store, a cache under
// `.kapi/work/`, and serves a block from it rather than calling a provider
// while the source and its configuration stand (tool.StoredTargetReuser).
// `kapi up --plan` asks the same question of the same store, through the
// helpers here. The drafts themselves live in their homes: a delivered file,
// or the workspace home for a locale whose files are withheld (workhome.go).

// storedTargetKey names the overlay the store holds for one block of a unit:
// the `targets/<locale>` kind and the key the producers wrote it under, which
// is blockstore.OverlayKey as the file runner tags it (the source's project
// namespace, the block's id). ok is false for a block no producer keys a
// stored target by, so a reader skips exactly what a writer never wrote.
func storedTargetKey(u VerifyUnit, b *model.Block) (kind, key string, ok bool) {
	if b == nil || !b.Translatable || b.ID == "" {
		return "", "", false
	}
	rel := blockstore.SourceNamespace(u.ProjectRoot, u.SourcePath)
	return blockstore.TargetOverlayKind(model.LocaleID(u.Locale)), blockstore.StoreKey(rel, b.ID, b.SourceText()), true
}

// storedTargetStore resolves the block store to read stored targets from, or
// nil when this build or this project has none.
//
// The store file has to be there already. Opening one creates it, and this is a
// read on a path every measurement takes, `up --plan` and a cold checkout
// among them: a dry run that leaves a database behind has written to the
// project it promised to only price.
func (a *App) storedTargetStore(ctx context.Context, root string) blockstore.Store {
	layout, lerr := project.ResolveLayout(root)
	if lerr != nil {
		return nil
	}
	if held, _ := storage.Exists(layout.StorePath()); !held {
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
