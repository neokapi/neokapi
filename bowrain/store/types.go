// Package store provides the SQLite implementation of ContentStore.
// Interface types and domain types are defined in platform/store
// and re-exported here via type aliases.
package store

import platstore "github.com/gokapi/gokapi/platform/store"

// Type aliases — canonical definitions live in platform/store.
type (
	ContentStore = platstore.ContentStore
	Project      = platstore.Project
	Item         = platstore.Item
	StoredBlock  = platstore.StoredBlock
	BlockQuery   = platstore.BlockQuery
	Version      = platstore.Version
	VersionDiff  = platstore.VersionDiff
	ChangeType   = platstore.ChangeType
	BlockChange  = platstore.BlockChange
	ChangeEntry  = platstore.ChangeEntry
	ChangeSet    = platstore.ChangeSet
)

// Re-export constants.
const (
	MaxBlocksPerRequest = platstore.MaxBlocksPerRequest
	DefaultBlockLimit   = platstore.DefaultBlockLimit

	ChangeAdded    = platstore.ChangeAdded
	ChangeRemoved  = platstore.ChangeRemoved
	ChangeModified = platstore.ChangeModified
)
