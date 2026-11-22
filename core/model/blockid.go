package model

import (
	"hash/fnv"
	"strconv"
	"strings"
)

// A block's ID is its IDENTITY within the document it was read from.
//
// Everything downstream addresses a block by (source document, block id). The
// pair is the block store's key (core/blockstore.StoreKey). The translate
// tools' overlay cache consults it before running a translator. A `.kpz`
// handed on without its raw source addresses overlays by it alone
// (core/flow.partsFromSkeleton rebuilds each block as NewBlock(id, "")).
// Two blocks of one document that share an id get one target: the first
// unit's translation is served for the second, silently, and a persistent
// store writes it back as the answer for next time.

// A counter gives unique ids by construction. A reader that takes the id the
// document supplies (an XLIFF `unit/@id`, a TMX `tuid`, a Qt `message/@id`)
// trusts a uniqueness constraint real documents break, so it separates the
// repeats here, where the id is assigned. The block keeps the document's id
// under PropDocumentID and the writer emits that, so a document keeps its ids.

// A counter fails once one document is read by several readers. A container
// (EPUB over its spine, JSON handed to an HTML subfilter, ODF over content.xml
// and styles.xml) delegates each member to a fresh sub-reader counting from
// one, so every member's first block is `tu1`. QualifyMemberID prevents that
// collision.

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
// The zero value is ready to use. Use one builder per document: sharing one
// across files would make a block's id depend on which files were read first.
//
// The scope is the document even where a format's is narrower. XLIFF scopes
// `unit/@id` to its `<file>`, so a multi-file document repeats ids, but the
// store keys on the document.
//
// It keeps a 64-bit digest of each id in a flat open-addressed table (a
// DigestSet). Its readers include StreamingReaders over large documents, whose
// peak memory must not grow with the document. Keying on the id breaks that
// bound (tmx.TestStreamingReaderBoundedMemory), and a map of digests costs
// three times the digests' bytes.
//
// A digest collision makes an unrelated id read as `id#2`: still an identity,
// and only the store key changes. At 64 bits the risk is negligible.
type IDBuilder struct {
	seen DigestSet
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
	return b.seen.Add(h.Sum64())
}

// Reset clears the builder for a new document.
func (b *IDBuilder) Reset() { b.seen = DigestSet{} }

// DigestSet is a set of 64-bit digests held in one flat power-of-two table with
// linear probing: eight bytes a slot and no per-entry header, against the three
// words a runtime map spends to hold the same number. Nothing here needs a map's
// generality: there are no values, the keys are already uniformly distributed,
// and nothing is ever removed. The saving keeps a streaming reader's memory
// bound intact over a document with tens of thousands of blocks.
//
// Zero is a legal digest and cannot also mean "empty slot", so it is held beside
// the table rather than in it.
type DigestSet struct {
	slots   []uint64
	count   int
	hasZero bool
}

// The table doubles once it is three quarters full. Linear probing degrades
// sharply as a table fills, and three quarters is where the probe is still short
// while the table costs under half the bytes a map does.
const (
	digestSetInitialSlots = 64
	digestSetLoadNum      = 3
	digestSetLoadDen      = 4
)

// Add records a digest, reporting whether it was absent. The zero value is
// an empty set.
func (s *DigestSet) Add(d uint64) bool {
	if d == 0 {
		if s.hasZero {
			return false
		}
		s.hasZero = true
		return true
	}
	switch {
	case len(s.slots) == 0:
		s.slots = make([]uint64, digestSetInitialSlots)
	case (s.count+1)*digestSetLoadDen >= len(s.slots)*digestSetLoadNum:
		s.grow()
	}
	if !insertDigest(s.slots, d) {
		return false
	}
	s.count++
	return true
}

// grow doubles the table and re-seats every digest, which is where the old table
// and the new are both alive. That transient makes the load factor above a
// memory decision as much as a probe-length one.
func (s *DigestSet) grow() {
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

// MemberIDSeparator joins the member a container delegated to the id that
// member's own reader minted. It is `_` rather than a path separator or the `!`
// of the bang locator because a block id is written into documents that
// constrain it: an XLIFF 2 `unit/@id` is an `xs:NMTOKEN`, whose character set
// admits `_`, `-`, `.` and `:` and admits neither `/` nor `!`. Upstream Okapi
// spells the same qualification the same way: `<parentId>_sf<n>_<subId>`, from
// ISubFilter.buildResourceId.
const MemberIDSeparator = "_"

// QualifyMemberID prefixes a block ID with its container member's layer ID so
// member-local IDs, such as `tu1`, remain unique across the container.
//
// Layer IDs are unique within the container and use the character set accepted
// by MemberIDSeparator. Member paths or names may repeat and may contain
// unsupported characters. Non-delegating container readers maintain uniqueness
// with a shared counter; delegated members use this prefix.
//
// The qualified ID is retained for cross-format export. PropDocumentID is left
// unset because no member-local ID must be restored: container writers locate
// delegated content by position or original text.
//
// The function is stateless, with memory use independent of document size.
func QualifyMemberID(block *Block, member string) {
	if block == nil || block.ID == "" || member == "" {
		return
	}
	block.ID = member + MemberIDSeparator + block.ID
}

// UnqualifyMemberID returns id with the member prefix removed, or id unchanged
// when it does not carry one.
//
// The qualified id is the container's, and stops being the right name the
// moment a block is handed back to the member's own writer: that writer matches
// what its own reader minted, so a member re-serialized against `sf1_tu1` finds
// none of its blocks and emits the source it was given. A container that
// qualifies on the way out therefore un-qualifies on the way back in, and the
// pairing is why the two live together.
func UnqualifyMemberID(id, member string) string {
	if member == "" {
		return id
	}
	return strings.TrimPrefix(id, member+MemberIDSeparator)
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
