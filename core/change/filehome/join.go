package filehome

import (
	"context"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// An edition that lives in a file of its own, such as de/guide.md beside
// docs/guide.md, is read monolingually: its blocks hold German as their own
// content. A bilingual file of its own, such as po/de.po beside the catalog
// po/en.po, is read with the edition's language instead, and its blocks hold
// German as their translation (EditionFile.Bilingual). The file home joins
// each of them to the block of the document it translates, so an operation
// addresses {doc: docs/guide.md, block, edition: de} whichever file holds the
// text.
//
// The join pairs blocks the way the bilingual check does: by key first, then
// by translation-invariant address (a heading written as its own identity, so
// a section reads the same in both languages), and only then by position, and
// only when both files hold the same number of blocks, which is what a file
// materialized from the document's own skeleton holds. A file in a language of
// its own whose keys all sit under a root named for its language, where the
// document's sit under one named for the document's (a Rails catalog: en: in
// the document, de: in the German file), pairs by key below the root, so a
// translation one of them lacks leaves the rest paired.

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
	// kept is what the keeper holds of an edition with no file yet
	// (EditionFile.Kept); its blocks pair with the document's by key alone.
	kept *Kept
}

// blockIndex is what a join needs of each block of the document.
type blockIndex struct {
	keys  []string
	names []string
	ids   []string
	addrs []string
}

// add indexes b, the document's next block.
func (ix *blockIndex) add(b *model.Block) {
	ix.keys = append(ix.keys, change.BlockKey(b))
	ix.names = append(ix.names, b.Name)
	ix.ids = append(ix.ids, b.ID)
	ix.addrs = append(ix.addrs, b.StructuralAddress())
}

// indexDocument reads the document once for the keys and addresses a join
// pairs on.
func (s *session) indexDocument(ctx context.Context) (*blockIndex, error) {
	ix := &blockIndex{}
	err := s.ownPass(func(b *model.Block) error {
		ix.add(b)
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
	out, err := s.joinIndexed(ctx, keys, ix)
	if err != nil {
		return nil, nil, err
	}
	return out, ix, nil
}

// joinIndexed reads the files of the editions in keys and pairs their blocks
// with the document's blocks ix indexes.
func (s *session) joinIndexed(ctx context.Context, keys []model.EditionKey, ix *blockIndex) ([]*joinedEdition, error) {
	var out []*joinedEdition
	for _, k := range keys {
		f, ok := s.editionFile(k)
		if !ok {
			continue
		}
		je := &joinedEdition{key: k.Canonical(), file: f, src: s.fileSource(source{path: f.Path}), match: map[int]int{}}
		if f.Kept != nil {
			kept, err := f.Kept.Edition(ctx, s.doc.Ref, je.key)
			if err != nil {
				return nil, err
			}
			je.kept = &kept
			je.blocks = keptBlocks(kept)
			je.exists = len(je.blocks) > 0
			je.pair(ix, s.doc.SourceLocale)
			out = append(out, je)
			continue
		}
		je.exists = je.src.exists()
		if je.exists {
			p := s.readPass(je.src, f.Format, func(b *model.Block) error {
				je.blocks = append(je.blocks, b)
				return nil
			})
			if f.Bilingual {
				p.target = je.key.Locale
			}
			if err := p.run(ctx); err != nil {
				return nil, err
			}
			je.pair(ix, s.doc.SourceLocale)
		}
		out = append(out, je)
	}
	return out, nil
}

// pair matches the document's blocks, read in language source, to the
// edition file's.
func (je *joinedEdition) pair(ix *blockIndex, source model.LocaleID) {
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
	// A kept edition is keyed by the document's own block keys, so a block
	// pairs by its key or not at all.
	positional := je.kept == nil && len(ix.keys) == len(je.blocks)
	docRoot, fileRoot := "", ""
	if je.kept == nil && !je.file.Bilingual {
		fileKeys := make([]string, len(je.blocks))
		for i, b := range je.blocks {
			fileKeys[i] = change.BlockKey(b)
		}
		docRoot, fileRoot = keyRoot(ix.keys), keyRoot(fileKeys)
		if docRoot == fileRoot || !namesLocale(docRoot, source) || !namesLocale(fileRoot, je.key.Locale) {
			docRoot, fileRoot = "", ""
		}
	}
	for si, k := range ix.keys {
		ti, ok := byKey[k]
		if !ok && docRoot != "" && fileRoot != "" {
			ti, ok = byKey[fileRoot+strings.TrimPrefix(k, docRoot)]
		}
		if !ok && je.kept == nil && ix.addrs[si] != "" {
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

// keyRoot is the first segment every key of keys starts with, ahead of a
// dot, or "" when they do not all share one.
func keyRoot(keys []string) string {
	root := ""
	for i, k := range keys {
		r, _, ok := strings.Cut(k, ".")
		if !ok || r == "" || (i > 0 && r != root) {
			return ""
		}
		root = r
	}
	return root
}

// namesLocale reports whether root, the first segment of a key, names locale
// l: its tag or its language, in any case, with - or _ between the parts.
func namesLocale(root string, l model.LocaleID) bool {
	if root == "" || l.IsEmpty() {
		return false
	}
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "-")) }
	r, tag := norm(root), norm(string(l))
	lang, _, _ := strings.Cut(tag, "-")
	return r == tag || r == lang
}

// held is what the edition file's block at index ti holds of the edition:
// its own content in a monolingual file, its translation into the edition's
// language in a bilingual one, which an untranslated unit does not hold.
func (je *joinedEdition) held(ti int) ([]model.Run, bool) {
	b := je.blocks[ti]
	if !je.file.Bilingual {
		ed, _ := b.Edition(model.EditionKey{})
		return ed.Runs, true
	}
	ed, ok := b.Edition(je.key)
	return ed.Runs, ok
}

// drifted reports whether the edition file and the document no longer hold
// the same blocks: a block of either has no partner in the other.
func (je *joinedEdition) drifted(ix *blockIndex) bool {
	return je.exists && (len(je.match) != len(ix.keys) || len(je.match) != len(je.blocks))
}

// join gives b, the document's block at index si, the content of every
// joined edition that holds it.
func join(editions []*joinedEdition, si int, b *model.Block) {
	for _, je := range editions {
		ti, ok := je.match[si]
		if !ok {
			continue
		}
		if je.kept != nil {
			// The keeper holds the edition whole: its status and origin
			// travel with its runs.
			ed, _ := je.blocks[ti].Edition(model.EditionKey{})
			b.SetEdition(je.key, ed)
			continue
		}
		if runs, held := je.held(ti); held {
			b.SetEdition(je.key, model.Edition{Runs: runs})
		}
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
