package filehome

import (
	"context"
	"slices"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// An edition that lives in a file of its own, such as de/guide.md beside
// docs/guide.md, is read monolingually: its blocks hold German as their own
// content. The file home joins each of them to the block of the document it
// translates, so an operation addresses {doc: docs/guide.md, block, edition:
// de} whichever file holds the text.
//
// The join pairs blocks the way the bilingual check does: by key first, then
// by translation-invariant address (a heading written as its own identity, so
// a section reads the same in both languages), and only then by position, and
// only when both files hold the same number of blocks, which is what a file
// materialized from the document's own skeleton holds.

// joinedEdition is one edition joined from its own file.
type joinedEdition struct {
	key  model.EditionKey
	file EditionFile
	src  source
	// exists says the edition's file is there.
	exists bool
	// blocks are the edition file's blocks, in order.
	blocks []*model.Block
	// match maps the index of a document block to the index of the edition
	// file's block that holds its edition.
	match map[int]int
}

// blockIndex is what a join needs of each block of the document.
type blockIndex struct {
	keys  []string
	names []string
	ids   []string
	addrs []string
}

// indexDocument reads the document once for the keys and addresses a join
// pairs on.
func (s *session) indexDocument(ctx context.Context) (*blockIndex, error) {
	ix := &blockIndex{}
	err := s.ownPass(func(b *model.Block) error {
		ix.keys = append(ix.keys, change.BlockKey(b))
		ix.names = append(ix.names, b.Name)
		ix.ids = append(ix.ids, b.ID)
		ix.addrs = append(ix.addrs, b.StructuralAddress())
		return nil
	}).run(ctx)
	return ix, err
}

// joinEditions reads the files of the editions in keys and pairs their blocks
// with the document's. It reads nothing when keys is empty.
func (s *session) joinEditions(ctx context.Context, keys []model.EditionKey) ([]*joinedEdition, *blockIndex, error) {
	if len(keys) == 0 {
		return nil, nil, nil
	}
	ix, err := s.indexDocument(ctx)
	if err != nil {
		return nil, nil, err
	}
	var out []*joinedEdition
	for _, k := range keys {
		f, ok := s.editionFile(k)
		if !ok {
			continue
		}
		je := &joinedEdition{key: k.Canonical(), file: f, src: source{path: f.Path}, match: map[int]int{}}
		je.exists = je.src.exists()
		if je.exists {
			err := s.readPass(je.src, f.Format, func(b *model.Block) error {
				je.blocks = append(je.blocks, b)
				return nil
			}).run(ctx)
			if err != nil {
				return nil, nil, err
			}
			je.pair(ix)
		}
		out = append(out, je)
	}
	return out, ix, nil
}

// pair matches the document's blocks to the edition file's.
func (je *joinedEdition) pair(ix *blockIndex) {
	byKey := make(map[string]int, len(je.blocks))
	byAddr := make(map[string]int, len(je.blocks))
	for i, b := range je.blocks {
		if _, dup := byKey[change.BlockKey(b)]; !dup {
			byKey[change.BlockKey(b)] = i
		}
		if a := b.StructuralAddress(); a != "" {
			if _, dup := byAddr[a]; !dup {
				byAddr[a] = i
			}
		}
	}
	taken := make(map[int]bool, len(je.blocks))
	positional := len(ix.keys) == len(je.blocks)
	for si, k := range ix.keys {
		ti, ok := byKey[k]
		if !ok && ix.addrs[si] != "" {
			ti, ok = byAddr[ix.addrs[si]]
		}
		if !ok && positional && !taken[si] {
			ti, ok = si, true
		}
		if !ok || taken[ti] {
			continue
		}
		taken[ti] = true
		je.match[si] = ti
	}
}

// join gives b, the document's block at index si, the content of every
// joined edition that holds it.
func join(editions []*joinedEdition, si int, b *model.Block) {
	for _, je := range editions {
		ti, ok := je.match[si]
		if !ok {
			continue
		}
		ed, _ := je.blocks[ti].Edition(model.EditionKey{})
		b.SetEdition(je.key, model.Edition{Runs: ed.Runs})
	}
}

// unjoin removes the joined editions from b again, so the document's own
// writer sees the block as its reader produced it, with its own edits.
func unjoin(editions []*joinedEdition, b *model.Block) {
	for _, je := range editions {
		b.RemoveEdition(je.key)
	}
}

// documentKey finds the key of the document block that the edition file's
// block keyed key holds the edition of.
func (je *joinedEdition) documentKey(ix *blockIndex, key string) (string, bool) {
	ti := slices.IndexFunc(je.blocks, func(b *model.Block) bool {
		return b.Unit == key || b.Name == key || b.ID == key
	})
	if ti < 0 {
		return "", false
	}
	for si, t := range je.match {
		if t == ti {
			return ix.keys[si], true
		}
	}
	return "", false
}
