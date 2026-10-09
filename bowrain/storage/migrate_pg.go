package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// validTableName ensures the namespace only contains safe characters for a table name.
var validTableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// MigrationLockKey is the fixed pg_advisory_lock key that serializes schema
// migrations across concurrently booting instances (bowrain-server and
// bowrain-worker race on first boot; without the lock both pass the
// check-version step and one crashes applying the same migration). The value
// is arbitrary but must stay stable across releases.
const MigrationLockKey int64 = 0x626F775F6D696772 // "bow_migr"

// migrationCleanupTimeout bounds the statements that run after a pass has
// failed or timed out: the unlock and RESET on the pinned connection, and the
// pg_stat_activity query that says what the pass was waiting on. They run
// under a context detached from the pass's own, which by then may be past its
// deadline.
const migrationCleanupTimeout = 10 * time.Second

// MigratePostgres applies schema migrations to a PostgreSQL database using a
// namespaced migration tracking table. Each subsystem (store, auth, jobs, tm)
// should use a distinct namespace to avoid version collisions when sharing a DB.
func MigratePostgres(db *PgDB, migrations []Migration) error {
	return MigratePostgresNS(db, "schema_migrations", migrations)
}

// MigratePostgresNS applies schema migrations using a custom-named tracking
// table, under DefaultMigrationTimeout. See MigratePostgresNSContext.
func MigratePostgresNS(db *PgDB, tableName string, migrations []Migration) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultMigrationTimeout)
	defer cancel()
	return MigratePostgresNSContext(ctx, db, tableName, migrations)
}

// MigratePostgresNSContext applies schema migrations using a custom-named
// tracking table. The whole check-version-then-apply pass runs under a
// session-scoped Postgres advisory lock so concurrent instances migrating the
// same database serialize instead of racing.
//
// The pass is bounded by the context's deadline (DefaultMigrationTimeout when
// it has none), and the same bound is set as lock_timeout and
// statement_timeout on the pinned connection, so a statement that cannot take
// its lock fails on the server naming itself rather than waiting for the
// process to be killed from outside. When the pass times out, the error ends
// with a summary of the sessions holding or waiting on locks in the database,
// including whichever one holds the migration lock.
func MigratePostgresNSContext(ctx context.Context, db *PgDB, tableName string, migrations []Migration) error {
	if !validTableName.MatchString(tableName) {
		return fmt.Errorf("invalid migration table name: %q", tableName)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultMigrationTimeout)
		defer cancel()
		deadline, _ = ctx.Deadline()
	}

	err := migrateUnderLock(ctx, db, tableName, migrations, deadline)
	if err != nil && isMigrationTimeout(err) {
		return fmt.Errorf("%w\n%s", err, lockSummary(ctx, db))
	}
	return err
}

// migrateUnderLock runs one pass on a pinned connection: session timeouts set,
// advisory lock taken, migrations applied, lock released, timeouts reset. The
// pinned connection is released to the pool only after the lock is, because
// pg_advisory_lock is session-scoped and (*sql.Conn).Close returns the session
// to the pool rather than ending it: an unreleased lock would ride the pooled
// connection into the next caller, and advisory locks are counted, so one
// later unlock would not clear it.
func migrateUnderLock(ctx context.Context, db *PgDB, tableName string, migrations []Migration, deadline time.Time) (err error) {
	budget := time.Until(deadline)
	if budget <= 0 {
		return fmt.Errorf("migration pass has no time left: %w", context.DeadlineExceeded)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Close()

	// The cleanup statements must run even when ctx is already past its
	// deadline, which is the case they exist for.
	cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), migrationCleanupTimeout)
	defer cancelCleanup()

	// The server-side timeouts sit inside the context's deadline so that the
	// server is what cuts a blocked statement short: it then answers with the
	// statement and the lock it wanted, and the connection survives for the
	// unlock and the summary. A context deadline reached first makes pgx close
	// the connection instead. lock_timeout sits below statement_timeout
	// because PostgreSQL fires the statement timeout first when the two are
	// equal, and "lock timeout" is the more telling of the two messages. SET
	// takes no bind parameters; the values are integers this code computed.
	// Both settings are session-scoped and the session goes back to the pool,
	// so they are RESET on the way out.
	ms := budget.Milliseconds()
	for _, setting := range []struct {
		name string
		ms   int64
	}{
		{"lock_timeout", max(ms*8/10, 1)},
		{"statement_timeout", max(ms*9/10, 1)},
	} {
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("SET %s = %d", setting.name, setting.ms)); err != nil {
			return fmt.Errorf("set %s on the migration connection: %w", setting.name, err)
		}
	}
	defer func() {
		for _, setting := range []string{"lock_timeout", "statement_timeout"} {
			_, rerr := conn.ExecContext(cleanupCtx, "RESET "+setting)
			if rerr != nil && !sessionGone(rerr) {
				err = errors.Join(err, fmt.Errorf("reset %s on the migration connection: %w", setting, rerr))
			}
		}
	}()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", MigrationLockKey); err != nil {
		return fmt.Errorf("acquire migration advisory lock (key %d) within %s: %w", MigrationLockKey, budget.Round(time.Millisecond), err)
	}
	defer func() {
		var released bool
		uerr := conn.QueryRowContext(cleanupCtx, "SELECT pg_advisory_unlock($1)", MigrationLockKey).Scan(&released)
		switch {
		case sessionGone(uerr):
			// The backend session is gone (pgx closes the connection when a
			// context is canceled mid-statement), and a session-scoped lock
			// does not outlive its session.
		case uerr != nil:
			err = errors.Join(err, fmt.Errorf("release migration advisory lock: %w", uerr))
		case !released:
			err = errors.Join(err, errors.New("release migration advisory lock: the lock was not held by this session"))
		}
	}()

	return runMigrations(ctx, conn, tableName, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version     INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`, tableName),
		fmt.Sprintf("INSERT INTO %s (version, description) VALUES ($1, $2)", tableName),
		migrations)
}

// sessionGone reports whether err says the pinned connection, and with it
// the backend session, is already closed.
func sessionGone(err error) bool {
	return errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone)
}

// isMigrationTimeout reports whether err is the pass running out of time:
// the context's deadline, or the server cutting a statement short under the
// lock_timeout (55P03) or statement_timeout (57014) the pass set.
func isMigrationTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "55P03" || pgErr.Code == "57014"
	}
	return false
}

// lockSummary describes the sessions in the current database that hold the
// migration advisory lock, are blocked on a lock, or are blocking another
// session, from pg_stat_activity and pg_locks. It runs on a pool connection
// of its own, after the pinned one is released, so it answers even when the
// pool holds a single connection.
func lockSummary(ctx context.Context, db *PgDB) string {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), migrationCleanupTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT a.pid,
		       coalesce(a.state, ''),
		       coalesce(a.wait_event_type, ''),
		       coalesce(a.wait_event, ''),
		       coalesce(a.application_name, ''),
		       left(coalesce(a.query, ''), 200),
		       coalesce(array_to_string(pg_blocking_pids(a.pid), ','), ''),
		       EXISTS (SELECT 1 FROM pg_locks l
		               WHERE l.pid = a.pid AND l.locktype = 'advisory' AND l.granted
		                 AND ((l.classid::bigint << 32) | l.objid::bigint) = $1),
		       coalesce(extract(epoch FROM now() - a.query_start), 0)
		FROM pg_stat_activity a
		WHERE a.datname = current_database()
		  AND (cardinality(pg_blocking_pids(a.pid)) > 0
		       OR a.pid IN (SELECT unnest(pg_blocking_pids(b.pid)) FROM pg_stat_activity b
		                    WHERE b.datname = current_database())
		       OR EXISTS (SELECT 1 FROM pg_locks l
		                  WHERE l.pid = a.pid AND l.locktype = 'advisory'
		                    AND ((l.classid::bigint << 32) | l.objid::bigint) = $1))
		ORDER BY a.pid`, MigrationLockKey)
	if err != nil {
		return "lock summary unavailable: " + err.Error()
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString("sessions holding or waiting on locks in this database:")
	n := 0
	for rows.Next() {
		var (
			pid                             int
			state, waitType, waitEvent, app string
			query, blockedBy                string
			holdsMigrationLock              bool
			seconds                         float64
		)
		if err := rows.Scan(&pid, &state, &waitType, &waitEvent, &app, &query, &blockedBy, &holdsMigrationLock, &seconds); err != nil {
			return "lock summary unavailable: " + err.Error()
		}
		n++
		fmt.Fprintf(&b, "\n  pid %d %s", pid, state)
		if holdsMigrationLock {
			b.WriteString(" (holds the migration lock)")
		}
		// A Client wait is a session waiting for its client to speak, which is
		// what idle looks like, so only server-side waits are shown.
		if waitType != "" && waitType != "Client" {
			fmt.Fprintf(&b, " waiting on %s/%s", waitType, waitEvent)
		}
		if blockedBy != "" {
			fmt.Fprintf(&b, " blocked by pid %s", blockedBy)
		}
		if app != "" {
			fmt.Fprintf(&b, " app=%q", app)
		}
		fmt.Fprintf(&b, " for %.1fs: %s", seconds, strings.Join(strings.Fields(query), " "))
	}
	if err := rows.Err(); err != nil {
		return "lock summary unavailable: " + err.Error()
	}
	if n == 0 {
		b.WriteString(" none")
	}
	return b.String()
}
