package graph

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// AfterConnect is a pgx AfterConnect hook that loads the AGE extension
// and sets the search path. Use with pgxpool.Config.AfterConnect.
func AfterConnect(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, "LOAD 'age'")
	if err != nil {
		return fmt.Errorf("load age extension: %w", err)
	}
	_, err = conn.Exec(ctx, `SET search_path = ag_catalog, "$user", public`)
	if err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	return nil
}
