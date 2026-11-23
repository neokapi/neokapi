package store

import (
	"context"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
)

// LedgerReads is the reads a write makes to record itself on a project's
// context ledger: the projects, a project's context key, and its ledger
// objects. Bound to a transaction, they run on that transaction's connection,
// so a write that holds a row lock never waits on the pool for a second one:
// with every pooled connection held by a transaction waiting on that lock,
// the second would never come.
type LedgerReads struct{ q Runner }

// LedgerReadsOn is the ledger reads run on q, a transaction or the pool.
func (s *PostgresStore) LedgerReadsOn(q storage.Runner) LedgerReads { return LedgerReads{q: q} }

// ListProjects returns the projects that are not archived.
func (r LedgerReads) ListProjects(ctx context.Context) ([]*platstore.Project, error) {
	return listProjects(ctx, r.q)
}

// ContextKey returns the project key the project's context is bound to.
func (r LedgerReads) ContextKey(ctx context.Context, projectID string) (string, error) {
	return contextKeyTx(ctx, r.q, projectID)
}

// ListContextObjects returns the names of the project's objects under dir.
func (r LedgerReads) ListContextObjects(ctx context.Context, projectID, dir string) ([]string, error) {
	return listContextObjects(ctx, r.q, projectID, dir)
}

// GetContextObject returns one of the project's objects.
func (r LedgerReads) GetContextObject(ctx context.Context, projectID, name string) (*platstore.ContextObject, error) {
	return getContextObject(ctx, r.q, projectID, name)
}

// LedgerReads is the ledger reads run on the push's own transaction.
func (a pushApply) LedgerReads() LedgerReads { return LedgerReads{q: a.tx} }
