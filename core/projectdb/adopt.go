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
// terms bundle, the memory bundles, the voice profiles and the unit-state
// record under `.kapi/`. The next command derives them again into the context
// store, so adoption carries none of it.
//
// A STAGED unit state is the exception, and the whole reason this exists.
// Between a review landing and `kapi commit` writing it to `.kapi/state/`, the
// working set holds the only copy, and it is in the projection. Those rows are
// read out and written into the context store before anything is dropped, and
// nothing is dropped if that fails.

// adoptEmbeddedContext carries a project out of the embedded layout: staged
// decisions move into the context store, and the context tables then leave the
// projection.
//
// Best-effort. A project that cannot be adopted keeps both copies and works
// from the context store, which the committed sources re-seed; refusing to open
// the project would be the worse answer.
func adoptEmbeddedContext(ctx context.Context, db *DB) {
	if db.projection == nil || db.context == nil || db.work == nil {
		return
	}
	residue, err := embeddedContextTables(ctx, db.projection)
	if err != nil || len(residue) == 0 {
		return
	}
	if _, staged := residue[unitStateTable]; staged {
		if !carryStagedFromProjection(ctx, db) {
			return
		}
	}
	dropTables(ctx, db.projection, residue)
}

// unitStateTable is the working set's table, and the one table in the
// projection whose rows adoption cannot regenerate.
const unitStateTable = "unit_state"

// carryStagedFromProjection reads the decisions staged in the projection's
// working set and writes them into the context store's. It reports whether
// every one of them landed.
//
// The projection's set is opened against the same committed record, so its
// staged rows survive the sync the open performs (state.WorkStore.SyncWithCommitted
// rebuilds unstaged rows from the record and carries staged ones through
// untouched). Nothing is deleted here: the rows go out of scope when the table
// does.
func carryStagedFromProjection(ctx context.Context, db *DB) bool {
	old, err := state.OpenWorkFromDB(ctx, db.projection, db.layout.UnitStateDir())
	if err != nil {
		return false
	}
	staged, err := old.Staged(ctx)
	if err != nil {
		return false
	}
	for _, u := range staged {
		if err := db.work.Put(ctx, u); err != nil {
			return false
		}
	}
	return true
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
