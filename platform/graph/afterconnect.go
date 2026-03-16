package graph

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// AfterConnect is a pgx AfterConnect hook that appends the ag_catalog
// schema to the search path for AGE graph queries.
//
// AGE must be enabled via shared_preload_libraries (loaded at server start)
// and created via CREATE EXTENSION. The ag_catalog schema is appended
// (not prepended) so that default table creation still targets the user's
// schema, while AGE functions like cypher() resolve without qualification.
func AfterConnect(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, `SET search_path = "$user", public, ag_catalog`)
	if err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	return nil
}
