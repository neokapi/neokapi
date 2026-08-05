package model

import (
	"strconv"
	"strings"
)

// A block's name is its STRUCTURAL address: where the block sits in the document
// according to the document's own structure.
//
// It is not decoration and it is not a counter. reconcile.Identify folds the
// name into a block's context hash, and matching a re-read against the previous
// read grades content and context as independent signals. A name that counts
// blocks as they go past ("para3", "line5", "tu7") makes that signal worthless:
// delete one paragraph and every name below it changes, so an unchanged block
// looks moved and a decision recorded about it names something else.
//
// A structural name changes only when the document's structure changes:
//
//	getting-started/install/p2      a paragraph, under two headings
//	messages.errors.notFound        a key path
//	greeting                        a natural key the format already provides
//
// Formats that already had one — JSON and YAML name blocks by key path — were
// stable from the start, which is the evidence for doing this everywhere.
//
// Two rules make a name usable as an identity signal:
//
//   - Derive it from structure the AUTHOR controls (headings, keys, element
//     paths), never from a running count. Where a format gives a natural key —
//     a PO msgid, a properties key, an XLIFF unit id — that key IS the name;
//     inventing a counter beside it throws away the best signal in the file.
//   - Where structure genuinely repeats, disambiguate with an ordinal scoped to
//     the smallest enclosing structure, so an edit elsewhere cannot shift it.
//   - NEVER derive a block's own name from its own text. Ancestors' text is
//     fine and is what makes a path readable — a paragraph under "Setup" is
//     genuinely addressed by that heading. But a HEADING must not be named by
//     its own title, because reconcile.Identify folds the name into the context
//     hash: name a heading after itself and editing it changes both hashes at
//     once, so the block grades as new and loses the history it should have
//     kept. Name a heading by its parent trail plus its ordinal among sibling
//     headings instead. Then rewording it changes content alone and grades as
//     an edit, which is what it is.
//
// Block.ID is a separate thing and is not affected. It is the skeleton join —
// the reader writes a ref, the writer matches on it — lives inside one
// read/write pass, and is free to stay a counter.

// PathSeparator joins structural segments. A path reads like a location because
// it is one.
const PathSeparator = "/"

// NameOrdinalSeparator precedes the occurrence ordinal NameBuilder appends when
// structure alone cannot tell two blocks apart.
//
// Exported because its presence is meaningful to readers of a name, not just to
// the writer of one: a name carrying an ordinal is the reader saying "these two
// are indistinguishable by structure", so anything that aligns or matches on
// names must treat that side as positional rather than keyed.
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
