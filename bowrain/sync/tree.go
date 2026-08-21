package sync

import (
	"context"
	"sort"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// LoadTree is the stream's whole content, reduced to hashes: per item, its
// durable block keys, the content hash of each block, and the transfer hash of
// each block, in document order.
//
// It exists so a producer can stop asking. The block-level negotiation it
// replaces ran once per item, sequentially, before any content moved — a few
// hundred files were a few hundred round trips, and a first push, having no
// cache, paid that for every item it held. One read answers the same question
// for the whole stream, and the answer is cacheable because it is exactly what
// the stream's ref names.
//
// The item's stored id rides along as the key: identity is the venue's to
// assign, and a producer's declaration is compared against these units to
// decide what was renamed.
func (d *DiffEngine) LoadTree(ctx context.Context, projectID, stream string, scope venue.Scope) (venue.Tree, map[string]string, error) {
	items, err := d.contentStore.ListItems(ctx, projectID, stream)
	if err != nil {
		return nil, nil, err
	}

	tree := make(venue.Tree, len(items))
	itemIDs := make(map[string]string, len(items))
	for _, item := range items {
		if item == nil || item.Name == "" {
			continue
		}
		// An empty scope is not a filter. A producer that declares no scope is
		// making no claim about what is missing, and answering it with a subset
		// would have it compute a diff against a tree that is not the venue's.
		if len(scope) > 0 && !scope.Covers(item.Name) {
			continue
		}
		ti, err := d.loadTreeItem(ctx, projectID, stream, item.Name)
		if err != nil {
			return nil, nil, err
		}
		tree[item.Name] = ti
		itemIDs[item.Name] = item.ID
	}
	return tree, itemIDs, nil
}

// loadTreeItem reduces one item's stored blocks to its tree entry.
//
// Document order is the store's own order, which is the order the reader
// produced — the order rename similarity is measured in. It is read from the
// rows rather than the hash cache for that reason: the cache holds a map, and
// a map has no order.
func (d *DiffEngine) loadTreeItem(ctx context.Context, projectID, stream, itemName string) (venue.TreeItem, error) {
	blocks, err := d.contentStore.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
		ItemName:  itemName,
	})
	if err != nil {
		return venue.TreeItem{}, err
	}
	ti := venue.TreeItem{Path: itemName}
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil {
			continue
		}
		// The durable key, matching what a producer computes and what a prune
		// is expressed in. Falling back to the row id keeps a block stored
		// before durable keying addressable.
		key := sb.SourceID
		if key == "" {
			key = sb.Block.ID
		}
		ti.Keys = append(ti.Keys, key)
		ti.Content = append(ti.Content, sb.ContentHash)
		ti.Record = append(ti.Record, model.ComputeRecordHash(sb.ContentHash, sb.ContextHash))
	}
	return ti, nil
}

// TreeRootHash folds a tree to one value, so a producer holding a warm mirror
// can ask whether anything moved without reading the whole thing back.
func TreeRootHash(tree venue.Tree) string {
	itemHashes := make(map[string]string, len(tree))
	for path, ti := range tree {
		byKey := make(map[string]string, len(ti.Keys))
		for i, k := range ti.Keys {
			if i < len(ti.Record) {
				byKey[k] = ti.Record[i]
			}
		}
		itemHashes[path] = venue.ComputeItemHash(byKey)
	}
	return venue.ComputeRootHash(itemHashes)
}

// SortedPaths is the tree's paths in a stable order, so a response body is
// byte-identical for identical content and an ETag means what it says.
func SortedPaths(tree venue.Tree) []string {
	paths := make([]string, 0, len(tree))
	for p := range tree {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
