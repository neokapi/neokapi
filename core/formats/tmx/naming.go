package tmx

import (
	"hash/fnv"
	"strconv"
)

// nameOrdinals disambiguates block names that repeat within one document,
// without retaining the names themselves.
//
// model.NameBuilder is the general answer and the one to reach for first. It
// cannot be used here. Its map is keyed by the name, and a translation unit with
// no `tuid` is named by its source segment — so the map would grow to hold every
// segment in the file, and this reader declares itself a StreamingReader, whose
// whole promise is that peak memory does not grow with the document. Measured on
// the 40,000-unit fixture behind TestStreamingPair_BoundedWriterTMX it retained
// 35% of the input, at the edge of the bound; a memory file with longer segments
// than that fixture's would be well over it.
//
// Keying on a 64-bit FNV-1a digest costs the same regardless of how long the
// name is, which restores the bound. The trade is a false ordinal on a digest
// collision — two unrelated names hashing alike would make the second read as
// "name#2". At 64 bits and the tens of thousands of units a real .tmx file
// holds, that is far below the rate at which the surrounding assumptions fail.
//
// The zero value is ready to use. One per document read: the ordinals are scoped
// to that document, and sharing one across files would make a name depend on
// which file was read first.
type nameOrdinals struct {
	seen   map[uint64]struct{} // every digest handed out
	repeat map[uint64]int      // digests handed out more than once → how many times
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

	if o.seen == nil {
		o.seen = map[uint64]struct{}{}
	}
	if _, ok := o.seen[key]; !ok {
		o.seen[key] = struct{}{}
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
