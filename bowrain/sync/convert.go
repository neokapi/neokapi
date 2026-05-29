// Package sync hosts the server-side sync engine: the Merkle-tree diff engine
// (diff.go) and the Redis-backed hash cache (cache_redis.go), which depend on
// bowrain/core/store and redis and therefore belong in the platform module.
//
// The pure model<->protobuf converters and the content-hash helpers
// (ComputeItemHash, ComputeRootHash) live in the framework-only package
// github.com/neokapi/neokapi/bowrain/core/sync so the bowrain/core push client
// can use them without importing the platform module. They are re-exported here
// so existing server/jobs callers continue to reference them through
// bowrain/sync unchanged.
package sync

import (
	syncv1 "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/core/model"
)

// BlockToProto converts a model.Block to a SyncBlock protobuf message.
// Re-exported from bowrain/core/sync.
func BlockToProto(b *model.Block, itemName string) *syncv1.SyncBlock {
	return coresync.BlockToProto(b, itemName)
}

// ProtoToBlock converts a SyncBlock protobuf message to a model.Block.
// Re-exported from bowrain/core/sync.
func ProtoToBlock(sb *syncv1.SyncBlock) *model.Block {
	return coresync.ProtoToBlock(sb)
}

// ComputeItemHash computes the Merkle hash for an item by hashing all its block
// content hashes in sorted order. Re-exported from bowrain/core/sync.
func ComputeItemHash(blockHashes map[string]string) string {
	return coresync.ComputeItemHash(blockHashes)
}

// ComputeRootHash computes the project root hash from item hashes.
// Re-exported from bowrain/core/sync.
func ComputeRootHash(itemHashes map[string]string) string {
	return coresync.ComputeRootHash(itemHashes)
}
