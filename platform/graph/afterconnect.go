package graph

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// AfterConnect is a pgx AfterConnect hook that sets the search path
// for the AGE extension. Use with pgxpool.Config.AfterConnect.
//
// AGE must be enabled via shared_preload_libraries (loaded at server start)
// and created via CREATE EXTENSION. This hook only sets the search path
// so ag_catalog functions are available without schema-qualifying.
func AfterConnect(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, `SET search_path = ag_catalog, "$user", public`)
	if err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	return nil
}
