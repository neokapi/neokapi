package model

import (
	"crypto/sha256"
	"encoding/hex"
)

// An edition revision names the content of one edition as a reader saw it. A
// change made against that read names the revision, and lands only while the
// edition still has it. The revision covers the edition's key and its runs,
// inline codes with their data and attributes included, and nothing else:
//
//   - Codes count, so a changed link target or a removed link moves it. The
//     content hash (ComputeContentHash) hashes plain text and is blind to both.
//   - Status and origin are left out. A review decision changes them and no
//     word; it binds to the revision it saw rather than moving it.
//   - Other editions are left out. A derived edition records the revision of
//     the edition it was made from separately (its basis), so a fix to the
//     source wording does not refuse a concurrent save of a translation.
//
// Being a function of content alone, the same token is valid wherever the
// content is held: a file, a server row or a store.

// AbsentRevision is the revision of an edition a block does not hold. A change
// that creates an edition names it as the revision it read.
const AbsentRevision = "absent"

// RunsRevision returns the revision of an edition with key k and content runs:
// "r:" and the first 16 hex digits of sha256(key text, 0x00, canonical run
// JSON). The key text is MarshalText of k canonical; the run JSON is
// CanonicalRunsJSON.
func RunsRevision(k EditionKey, runs []Run) string {
	text, _ := k.Canonical().MarshalText()
	h := sha256.New()
	h.Write(text)
	h.Write([]byte{0})
	h.Write(CanonicalRunsJSON(runs))
	var sum [sha256.Size]byte
	return "r:" + hex.EncodeToString(h.Sum(sum[:0])[:8])
}

// EditionRevision returns the revision of edition k of b, or AbsentRevision
// when b does not hold it. Every key that reaches the edition gives the same
// revision: the zero key and the source language both name the edition the
// block was read in, unless a same-language target holds the source language,
// and then the zero key alone names it (Block.EditionKeyOf).
func EditionRevision(b *Block, k EditionKey) string {
	e, ok := b.Edition(k)
	if !ok {
		return AbsentRevision
	}
	return RunsRevision(b.EditionKeyOf(k), e.Runs)
}

// CanonicalRunsJSON is the JSON array of runs, each written by Run.MarshalJSON
// with no HTML escaping, the form the TypeScript mirror writes. Maps (plural
// forms, select cases, attributes) are written in key order, so equal content
// gives equal bytes. An empty or nil sequence is "[]", and a run that is not a
// valid union (no discriminator, or several) is written as null.
func CanonicalRunsJSON(runs []Run) []byte {
	buf := make([]byte, 0, 64*len(runs)+2)
	buf = append(buf, '[')
	for i, r := range runs {
		if i > 0 {
			buf = append(buf, ',')
		}
		var ok bool
		if buf, ok = appendRunJSON(buf, r); !ok {
			buf = append(buf, "null"...)
		}
	}
	return append(buf, ']')
}
