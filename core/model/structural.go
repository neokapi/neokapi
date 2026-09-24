package model

import (
	"strconv"
	"strings"
)

// A block's name describes its position in the document's structure.
// Reconciliation hashes the name as context and compares context independently
// of content when matching blocks across reads.
//
// Use author-supplied structure such as heading paths, keys and element paths.
// Natural keys, including PO msgids and XLIFF unit IDs, should be retained.
// Disambiguate repeated paths with an ordinal scoped to the smallest enclosing
// structure, so edits in another section do not renumber these blocks.
//
// Do not derive a block's name from its own text. Doing so changes both hashes
// on a text edit and prevents reconciliation from recognizing the edit. Ancestor
// text may be part of a path; a heading itself uses its parent trail and sibling
// ordinal rather than its title.
//
// Block.ID is separate: it must be unique within a read/write pass for skeleton
// joins and storage, but need not be stable across reads. See blockid.go for
// handling document-supplied IDs.

// PathSeparator joins structural path segments.
const PathSeparator = "/"

// NameOrdinalSeparator precedes the ordinal used to distinguish repeated
// structural paths. Matchers can use its presence to recognize a positional
// component rather than an author-supplied key.
const NameOrdinalSeparator = "#"

// StructuralPath joins segments into a block name, skipping empty ones so a
// caller can pass an optional heading trail without branching.
func StructuralPath(segments ...string) string {
	parts := make([]string, 0, len(segments))
	for _, s := range segments {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, sanitizeSegment(s))
		}
	}
	return strings.Join(parts, PathSeparator)
}

// sanitizeSegment keeps a segment readable and unambiguous: the separator cannot
// appear inside a segment, and whitespace is collapsed so re-flowing a heading
// does not rename everything beneath it.
func sanitizeSegment(s string) string {
	s = strings.ReplaceAll(s, PathSeparator, "-")
	return strings.Join(strings.Fields(s), " ")
}

// NameBuilder assigns structural names within one document, disambiguating
// positions that genuinely repeat.
//
// The zero value is ready to use. One builder per document read: the ordinals it
// hands out are scoped to that document, and sharing one across files would make
// a name depend on which files were read first.
type NameBuilder struct {
	seen map[string]int
}

// Name returns path, with an ordinal appended when the same path has already
// been used in this document.
//
// The ordinal is the last resort, not the first: two blocks sharing a structural
// path are genuinely indistinguishable by structure — two cells in a row, two
// paragraphs under one heading — and something has to tell them apart. Scope the
// path as tightly as the format allows and this stays rare.
func (b *NameBuilder) Name(path string) string {
	if path == "" {
		path = "block"
	}
	if b.seen == nil {
		b.seen = map[string]int{}
	}
	b.seen[path]++
	if n := b.seen[path]; n > 1 {
		return path + "#" + strconv.Itoa(n)
	}
	return path
}

// Reset clears the builder for a new document.
func (b *NameBuilder) Reset() { b.seen = nil }
