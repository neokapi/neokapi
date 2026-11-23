package storage

import (
	"context"
	"errors"
	"fmt"
)

// BoundedLock runs lock, a statement that takes a row lock, on tx with
// lock_timeout set to LockTimeout for that statement alone, and returns
// ErrBusy when the wait runs out.
//
// The bound is set inside a savepoint. On success it is put back to the
// value it had before, so the statements the transaction makes afterwards
// wait as they did. On any failure the savepoint is rolled back, which
// undoes the setting and leaves the transaction usable. A lock timeout then
// costs the caller this one write, not the transaction it runs in.
func BoundedLock(ctx context.Context, tx Runner, lock func() error) error {
	if _, err := tx.ExecContext(ctx, `SAVEPOINT bounded_lock`); err != nil {
		return fmt.Errorf("bounded lock: %w", err)
	}
	var previous string
	err := tx.QueryRowContext(ctx, `SELECT current_setting('lock_timeout')`).Scan(&previous)
	if err == nil {
		_, err = tx.ExecContext(ctx, `SELECT set_config('lock_timeout', $1, true)`, LockTimeout)
	}
	if err == nil {
		err = lock()
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `SELECT set_config('lock_timeout', $1, true)`, previous)
	}
	if err != nil {
		if _, rerr := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT bounded_lock`); rerr != nil {
			return errors.Join(err, fmt.Errorf("bounded lock: roll back: %w", rerr))
		}
		if _, rerr := tx.ExecContext(ctx, `RELEASE SAVEPOINT bounded_lock`); rerr != nil {
			return errors.Join(err, fmt.Errorf("bounded lock: release: %w", rerr))
		}
		if LockTimedOut(err) {
			return ErrBusy
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `RELEASE SAVEPOINT bounded_lock`); err != nil {
		return fmt.Errorf("bounded lock: release: %w", err)
	}
	return nil
}
