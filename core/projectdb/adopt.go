package projectdb

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
)

// Adoption is the first open of a project that was living in the embedded
// layout, where the context tables sat beside the projection in
// `.kapi/work/store.db`.
//
// Almost everything in those tables is a projection of a committed source: the
// terms bundle, the memory bundles, the voice profiles and the decision record
// under `.kapi/`. The next command derives them again into the context store,
// so adoption carries none of it.
//
// What it does carry is every decision the committed shards do not hold.
// Between a decision being recorded and `kapi commit` writing it to
// `.kapi/state/`, the ledger holds its only copy, and that ledger is in the
// projection. Those rows are recorded in the context store before anything is
// dropped, and nothing is dropped if that fails.

// adoptEmbeddedContext carries a project out of the embedded layout: the
// decisions the record does not hold move into the context store, and the
// context tables then leave the projection.
//
// Best-effort. A project that cannot be adopted keeps both copies and works
// from the context store, which the committed sources re-seed; refusing to open
// the project would be the worse answer.
func adoptEmbeddedContext(ctx context.Context, db *DB) {
	if db.projection == nil || db.context == nil || db.work == nil {
		return
	}
	if has, err := hasUserTables(ctx, db.projection); err != nil || !has {
		return
	}
	if !carryDecisionsFromProjection(ctx, db) {
		return
	}
	// Recomputed after the carry: opening the old store creates the ledger's
	// own tables where a project predates them, and those leave with the rest.
	residue, err := embeddedContextTables(ctx, db.projection)
	if err != nil || len(residue) == 0 {
		return
	}
	dropTables(ctx, db.projection, residue)
}

// carryDecisionsFromProjection records in the context store every decision the
// projection's ledger holds that this checkout's committed shards do not. It
// reports whether every one of them landed.
//
// Both sides are opened against the same committed record, which is what
// identifies a checkout (core/state.checkoutID), so a decision arrives in the
// context store under the view it was made in. Opening the old store also runs
// core/state's own migration, so a project that predates the ledger has its
// rows carried into one before they are read.
//
// core/state.WorkStore.Staged is the reading: what this checkout holds that its
// shards do not. Everything the shards do supply comes back on the next import
// in the context store, so it is neither read here nor needed.
//
// Nothing is deleted from the projection here; the rows go out of scope when
// their tables do.
func carryDecisionsFromProjection(ctx context.Context, db *DB) bool {
	old, err := state.OpenWorkFromDB(ctx, db.projection, db.layout.UnitStateDir())
	if err != nil {
		return false
	}
	unrecorded, err := old.Staged(ctx)
	if err != nil {
		return false
	}
	for _, u := range unrecorded {
		if err := db.work.Put(ctx, u); err != nil {
			return false
		}
	}
	return true
}

// hasUserTables reports whether a database holds any table of its own.
func hasUserTables(ctx context.Context, db *storage.DB) (bool, error) {
	empty, err := isEmptyDatabase(ctx, db)
	return !empty, err
}

// isEmptyDatabase reports whether a database holds no table of its own, which
// is what a context store created by this open looks like.
func isEmptyDatabase(ctx context.Context, db *storage.DB) (bool, error) {
	if db == nil {
		return false, nil
	}
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("projectdb: read the context store's schema: %w", err)
	}
	return count == 0, nil
}

// embeddedContextTables reports the tables in the projection that belong to the
// context subsystems, as a set.
//
// The set is a difference rather than a list, because a list would be a second
// copy of four subsystems' schemas that nothing keeps current. A fresh
// projection is built in memory, its tables are what a projection is entitled
// to hold, and everything else in the real file came from a subsystem that has
// moved out.
func embeddedContextTables(ctx context.Context, projection *storage.DB) (map[string]bool, error) {
	present, err := tableNames(ctx, projection)
	if err != nil {
		return nil, err
	}
	reference, err := referenceProjectionTables(ctx)
	if err != nil {
		return nil, err
	}
	residue := map[string]bool{}
	for name, virtual := range present {
		if _, own := reference[name]; !own {
			residue[name] = virtual
		}
	}
	return residue, nil
}

// referenceProjectionTables builds an empty projection in memory and reports
// every table its own subsystems create, shadow tables of the block cache's
// full-text index included.
func referenceProjectionTables(ctx context.Context) (map[string]bool, error) {
	ref, err := storage.OpenWith(":memory:", storage.Options{})
	if err != nil {
		return nil, fmt.Errorf("projectdb: build a reference projection: %w", err)
	}
	defer func() { _ = ref.Close() }()

	if err := storage.Migrate(ref, metaMigrationsTable, metaMigrations); err != nil {
		return nil, fmt.Errorf("projectdb: build a reference projection: %w", err)
	}
	if _, err := sqlitestore.NewFromDB(ref, false); err != nil {
		return nil, fmt.Errorf("projectdb: build a reference projection: %w", err)
	}
	return tableNames(ctx, ref)
}

// tableNames reports every table in a database, paired with whether it is a
// virtual one. A virtual table owns shadow tables that go with it, so it has to
// be dropped first and its shadows never dropped directly.
func tableNames(ctx context.Context, db *storage.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name, COALESCE(sql, '') FROM sqlite_master
		 WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, fmt.Errorf("projectdb: read a store's schema: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]bool{}
	for rows.Next() {
		var name, ddl string
		if err := rows.Scan(&name, &ddl); err != nil {
			return nil, fmt.Errorf("projectdb: read a store's schema: %w", err)
		}
		out[name] = strings.Contains(strings.ToUpper(ddl), "CREATE VIRTUAL TABLE")
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projectdb: read a store's schema: %w", err)
	}
	return out, nil
}

// dropTables removes a set of tables from a database, virtual ones first.
//
// Dropping a virtual table removes the shadow tables behind it, so by the time
// the ordinary tables are reached some of the set is already gone; every drop
// is therefore IF EXISTS. Foreign keys are off for the connection that does it,
// because the order the set happens to iterate in is not the order the
// references point.
//
// Best-effort and quiet: a table that will not drop leaves a projection with
// one more table in it than it needs, which costs disk and nothing else.
func dropTables(ctx context.Context, db *storage.DB, tables map[string]bool) {
	conn, err := db.DB.Conn(ctx)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return
	}
	defer func() { _, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`) }()

	drop := func(name string) {
		_, _ = conn.ExecContext(ctx, `DROP TABLE IF EXISTS "`+strings.ReplaceAll(name, `"`, `""`)+`"`)
	}
	for name, virtual := range tables {
		if virtual {
			drop(name)
		}
	}
	// Re-read: the virtual drops took their shadow tables with them.
	remaining, err := tableNames(ctx, db)
	if err != nil {
		return
	}
	for name := range tables {
		if _, still := remaining[name]; still {
			drop(name)
		}
	}
}
