package tmx

import (
	"hash/fnv"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// nameOrdinals disambiguates block names that repeat within one document,
// without retaining the names themselves.
//
// model.NameBuilder is the general answer, but not usable here. Its map is
// keyed by the name, and a translation unit with no `tuid` is named by its
// source segment, so the map holds every segment in the file. This reader is
// a StreamingReader, so peak memory must not grow with the document's text.
//
// A 64-bit FNV-1a digest costs the same for any name length, and
// model.DigestSet holds each in eight bytes of a flat table. On the
// 40,000-unit fixture behind TestStreamingReaderBoundedMemory that takes the
// peak from 1.68 MiB with a Go map to 1.04 MiB. A digest collision gives a
// false ordinal ("name#2"), a negligible risk at 64 bits.
//
// The zero value is ready to use. Use one per document: sharing one across
// files would make a name depend on which file was read first.
type nameOrdinals struct {
	seen   model.DigestSet // every digest handed out
	repeat map[uint64]int  // digests handed out more than once → how many times
}

// name returns path, with an ordinal appended when the same path has already
// been used in this document. It mirrors model.NameBuilder.Name exactly; only
// the bookkeeping differs.
func (o *nameOrdinals) name(path string) string {
	if path == "" {
		path = "block"
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(path))
	key := h.Sum64()

	if o.seen.Add(key) {
		return path
	}
	// Repeats are rare, so their counts live in a second map that stays empty
	// for the documents that never need one.
	if o.repeat == nil {
		o.repeat = map[uint64]int{}
	}
	o.repeat[key]++
	return path + "#" + strconv.Itoa(o.repeat[key]+1)
}
