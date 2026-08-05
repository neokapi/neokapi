package host

import (
	"fmt"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/occurrence"
)

// OccurrenceSources binds the two stores a term-occurrence query joins: the
// terms store the command resolves to, and the project's block cache.
//
// The release function is the terms store's — a standalone store opened for
// this command is closed by it, while the project's shared handle is not. The
// block store is never released here: it is either the App's shared project
// handle or an injected backend, and neither belongs to the caller.
//
// A project with no blocks yet is not a failure. It is the ordinary state of a
// fresh checkout, and the honest answer to "where is this term used?" there is
// "nowhere yet" rather than an error.
func (a *App) OccurrenceSources(cmd Command) (occurrence.Sources, func(), error) {
	tb, _, release, err := a.OpenTermsSQLite(cmd)
	if err != nil {
		return occurrence.Sources{}, func() {}, err
	}
	return occurrence.Sources{Terms: tb, Blocks: a.OccurrenceBlocks(cmd)}, release, nil
}

// OccurrenceBlocks resolves the block cache to search. It returns nil rather
// than an error whenever there is no project or no cache: the query degrades to
// "this term exists, and nothing here uses it".
func (a *App) OccurrenceBlocks(cmd Command) blockstore.Store {
	if a.BlocksBackend != nil {
		return a.BlocksBackend
	}
	db, _, err := a.ProjectStoreFor(cmd)
	if err != nil || db == nil {
		return nil
	}
	// Autocommit: this is a read, and a read-only session on the transactional
	// handle would still take the write permit for its whole life, parking any
	// converge run that is writing beside it.
	return db.BlocksAutocommit()
}

// FindOccurrences answers where a term is used in the project the command
// resolves to. It is the one path the CLI, MCP and context search all take, so
// the three cannot drift in what they consider a use.
func (a *App) FindOccurrences(cmd Command, q occurrence.Query) (*occurrence.Result, error) {
	src, release, err := a.OccurrenceSources(cmd)
	if err != nil {
		return nil, err
	}
	defer release()

	res, err := occurrence.Find(CmdContext(cmd), src, q)
	if err != nil {
		return nil, fmt.Errorf("occurrences: %w", err)
	}
	return res, nil
}
