package server

import (
	"context"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// A recipe's `source_only: true` collection holds content with no target
// language. The platform drafts no translation of it, counts none as missing,
// and deals none for review. The flag is stored on the collection row
// (Collection.SourceOnly), whose config is sealed at rest, so it is read here
// in Go and handed to queries as collection ids.

// sourceOnlyCollections returns the ids of a project's source-only collections.
func sourceOnlyCollections(ctx context.Context, cs platstore.ContentStore, projectID, stream string) ([]string, error) {
	colls, err := cs.ListCollections(ctx, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	var ids []string
	for _, c := range colls {
		if c.SourceOnly() {
			ids = append(ids, c.ID)
		}
	}
	return ids, nil
}

// sourceOnlyItems returns the names of the items in a project's source-only
// collections. Empty when the project has none, without listing its items.
func sourceOnlyItems(ctx context.Context, cs platstore.ContentStore, projectID, stream string) (map[string]bool, error) {
	ids, err := sourceOnlyCollections(ctx, cs, projectID, stream)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	only := make(map[string]bool, len(ids))
	for _, id := range ids {
		only[id] = true
	}
	items, err := cs.ListItems(ctx, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	out := map[string]bool{}
	for _, it := range items {
		if only[it.CollectionID] {
			out[it.Name] = true
		}
	}
	return out, nil
}
