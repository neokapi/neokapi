package storeutil

import (
	"context"
	"database/sql"
	"errors"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
)

// EditProject holds the write lock from the dialect's locking read through commit.
func EditProject(ctx context.Context, db *sql.DB, projectID, lockQuery string,
	scan func(storage.Scanner) (*platstore.Project, error),
	write func(context.Context, storage.Runner, *platstore.Project) error,
	edit platstore.ProjectEdit,
) (*platstore.Project, bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	p, err := scan(tx.QueryRowContext(ctx, lockQuery, projectID))
	if err != nil {
		return nil, false, err
	}
	if p.Properties == nil {
		p.Properties = make(map[string]string)
	}
	changed, err := edit(p)
	if err != nil {
		return nil, false, err
	}
	if changed {
		if p.ID != projectID {
			return nil, false, errors.New("a project edit cannot change its id")
		}
		if err := write(ctx, tx, p); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return p, changed, nil
}
