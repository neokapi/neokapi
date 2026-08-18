package model

import (
	"hash/fnv"
	"strconv"
)

// A block's ID is its IDENTITY within the document it was read from.
//
// Everything downstream treats (source document, block id) as the pair that
// addresses a block: it is the block store's key (core/blockstore.StoreKey), it
// is what the translate tools' overlay cache consults before running a
// translator, and it is what a `.kpz` handed on without its raw source has left
// to address an overlay with (core/flow.partsFromSkeleton rebuilds each block as
// NewBlock(id, ""), because the source text lived in the dropped source). So the
// id must be derivable from the id alone, and two blocks of one document that
// share one are two blocks with one target: the first unit's translation is
// served for the second, silently, and a persistent store writes it back as the
// answer for next time.
//
// A counter satisfies that by construction. A reader that instead takes the id
// the document supplies — an XLIFF `unit/@id`, a TMX `tuid`, a Qt `message/@id` —
// is trusting a uniqueness constraint the format states and real documents
// break, so it separates the repeats here, at the point the id is assigned.
//
// Separation is a store-side identity and never something the document is made
// to carry: the block keeps the id its document spells under PropDocumentID, and
// the writer emits that, so a document keeps the ids it arrived with.

// PropDocumentID names the property carrying the id a document spells for a
// block. It is present only where a reader had to separate blocks the document
// called the same thing; a conforming document sets it nowhere, and DocumentID
// reads through to the block's own ID.
const PropDocumentID = "document-id"

// IDOrdinalSeparator precedes the ordinal IDBuilder appends when a document
// hands two blocks one id. It matches NameOrdinalSeparator: a name and an id
// that had to be separated say so the same way.
const IDOrdinalSeparator = NameOrdinalSeparator

// IDBuilder hands out block IDs that are an identity within one document.
//
// The zero value is ready to use. One builder per document read: the ids it
// separates are scoped to that document, and sharing one across files would make
// a block's id depend on which files were read first.
//
// Document-scoped rather than per-sub-document, even where a format's uniqueness
// clause is narrower. XLIFF scopes `unit/@id` to the enclosing `<file>`, so a
// conforming multi-file document repeats ids by construction — while the store
// keys on the document, so it is the document that has to hold the id space.
//
// It remembers a 64-bit digest of each id in a flat open-addressed table, not
// the id in a map, and both halves of that are load-bearing. The readers that
// need it read the biggest documents — a .tmx memory holds tens of thousands of
// units — and several declare themselves StreamingReaders, whose whole promise
// is that peak memory does not grow with the document. Keying on the id itself
// measurably broke that bound (tmx.TestStreamingReaderBoundedMemory, which is
// why tmx.nameOrdinals is digest-keyed too), and a map of digests spent three
// times the bytes the digests themselves occupy — enough, beside the name
// builder already running on the same read, to leave the bound with no headroom.
//
// The trade is a false separation on a digest collision: two unrelated ids
// hashing alike make the second read as `id#2`, which is still an identity, and
// the document's own spelling is what gets written back either way — so the file
// is unaffected and only the store key differs from what it would otherwise have
// been. At 64 bits and the tens of thousands of blocks a real document holds,
// that is far below the rate at which the surrounding assumptions fail.
type IDBuilder struct {
	seen digestSet
}

// Assign gives block an ID no other block of this document holds, recording the
// id the document spelled under PropDocumentID when it has to differ.
//
// Only the repeats move: the first block to claim an id keeps it, so a
// conforming document passes through untouched and nothing about it records a
// separation that did not happen. The ordinal is retried until free, because a
// document may itself contain the spelling the separation would have produced.
//
// A block with no ID is left alone: an empty id is not a claim on the id space,
// and StoreKey already falls back to the source text for one.
func (b *IDBuilder) Assign(block *Block) {
	if block == nil || block.ID == "" {
		return
	}
	original := block.ID
	if b.claim(original) {
		return
	}
	for n := 2; ; n++ {
		candidate := original + IDOrdinalSeparator + strconv.Itoa(n)
		if !b.claim(candidate) {
			continue
		}
		block.ID = candidate
		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}
		block.Properties[PropDocumentID] = original
		return
	}
}

// claim records an id as taken, reporting whether it was free.
func (b *IDBuilder) claim(id string) bool {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return b.seen.add(h.Sum64())
}

// Reset clears the builder for a new document.
func (b *IDBuilder) Reset() { b.seen = digestSet{} }

// digestSet is a set of 64-bit digests held in one flat power-of-two table with
// linear probing: eight bytes a slot and no per-entry header, against the three
// words a runtime map spends to hold the same number. Nothing here needs a map's
// generality — there are no values, the keys are already uniformly distributed,
// and nothing is ever removed — and the difference is what keeps a streaming
// reader's memory bound intact over a document with tens of thousands of blocks.
//
// Zero is a legal digest and cannot also mean "empty slot", so it is held beside
// the table rather than in it.
type digestSet struct {
	slots   []uint64
	count   int
	hasZero bool
}

// maxDigestSetLoad is how full the table is allowed to get before it doubles.
// Linear probing degrades sharply as a table fills; below three quarters the
// probe stays short, and the table is still under half the bytes a map costs.
const maxDigestSetLoad = 4 // grow when count*4 >= len(slots)*3, i.e. at 3/4

// add records a digest, reporting whether it was absent.
func (s *digestSet) add(d uint64) bool {
	if d == 0 {
		if s.hasZero {
			return false
		}
		s.hasZero = true
		return true
	}
	if len(s.slots) == 0 {
		s.slots = make([]uint64, 64)
	} else if (s.count+1)*maxDigestSetLoad >= len(s.slots)*(maxDigestSetLoad-1) {
		s.grow()
	}
	if !insertDigest(s.slots, d) {
		return false
	}
	s.count++
	return true
}

// grow doubles the table and re-seats every digest, which is where the old table
// and the new are both alive — the transient that makes the load factor above a
// memory decision as much as a probe-length one.
func (s *digestSet) grow() {
	bigger := make([]uint64, len(s.slots)*2)
	for _, d := range s.slots {
		if d != 0 {
			insertDigest(bigger, d)
		}
	}
	s.slots = bigger
}

// insertDigest places d in the first free slot at or after its home, reporting
// whether it was absent. slots must have a free slot and a power-of-two length.
func insertDigest(slots []uint64, d uint64) bool {
	mask := uint64(len(slots) - 1)
	for i := d & mask; ; i = (i + 1) & mask {
		switch slots[i] {
		case 0:
			slots[i] = d
			return true
		case d:
			return false
		}
	}
}

// DocumentID is the id a block is written back under: the one its document
// spelled, which differs from the block's own ID exactly where a reader had to
// separate blocks the document called the same thing. The two are the same for
// every conforming document.
func DocumentID(block *Block) string {
	if block == nil {
		return ""
	}
	if original, ok := block.Properties[PropDocumentID]; ok && original != "" {
		return original
	}
	return block.ID
}
