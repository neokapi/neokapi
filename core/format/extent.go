package format

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
)

// LineRange is a 1-based, inclusive range of lines in a file, the unit a
// unified diff hunk is written in.
type LineRange struct {
	First int `json:"first"`
	Last  int `json:"last"`
}

// Overlaps reports whether r and o share at least one line.
func (r LineRange) Overlaps(o LineRange) bool {
	return r.First <= o.Last && o.First <= r.Last
}

// Extent is where one block's content sits in the file it was read from.
type Extent struct {
	// Block is the id the reader gave the block (model.Block.ID).
	Block string `json:"block"`
	// Start and End are the half-open byte span [Start, End) of the block's
	// content in the source bytes. Start == End for content that occupies no
	// bytes, such as an empty value.
	Start int `json:"start"`
	End   int `json:"end"`
	// Lines is the range of lines the span covers. An empty span covers the
	// line Start sits on.
	Lines LineRange `json:"lines"`
}

// LineIndex maps byte offsets in one source to 1-based line numbers. A line
// ends after its '\n', so the '\r' of a CRLF pair sits on the line it ends.
type LineIndex struct {
	starts []int
	size   int
}

// NewLineIndex indexes the line starts of src.
func NewLineIndex(src []byte) *LineIndex {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' && i+1 < len(src) {
			starts = append(starts, i+1)
		}
	}
	return &LineIndex{starts: starts, size: len(src)}
}

// Line returns the line holding the byte at offset. An offset at or past the
// end of the source is on the last line, which is where a diff places a change
// at the end of a file.
func (x *LineIndex) Line(offset int) int {
	return sort.Search(len(x.starts), func(i int) bool { return x.starts[i] > offset })
}

// Range returns the lines the half-open span [start, end) covers.
func (x *LineIndex) Range(start, end int) LineRange {
	first := x.Line(start)
	if end <= start {
		return LineRange{First: first, Last: first}
	}
	return LineRange{First: first, Last: x.Line(end - 1)}
}

// ErrExtentsUnavailable is wrapped by every error that says a document's block
// extents could not be derived. The document itself read correctly; only the
// mapping from blocks back to source positions is missing, and a caller that
// needs positions must report that it could not use them rather than guess.
var ErrExtentsUnavailable = errors.New("block extents unavailable")

func noExtents(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrExtentsUnavailable}, args...)...)
}

// ExtentsFromSkeleton reads every entry of a flushed skeleton store and aligns
// them against src with AlignSkeleton.
func ExtentsFromSkeleton(src []byte, store *SkeletonStore) ([]Extent, error) {
	if store == nil {
		return nil, noExtents("the reader emitted no skeleton")
	}
	var entries []SkeletonEntry
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read skeleton: %w", err)
		}
		entries = append(entries, entry)
	}
	return AlignSkeleton(src, entries)
}

// AlignSkeleton locates each block a skeleton refers to in the source bytes
// the skeleton was read from.
//
// A skeleton interleaves the bytes that are not content with a ref for each
// block. The bytes a ref stands in for are the gap between the text entries on
// either side of it, so aligning the text entries against the source gives
// every block its span. A SkeletonOriginal entry states the next ref's source
// bytes outright; SkeletonLang and SkeletonTrimmed carry source bytes that
// belong to no block.
//
// The alignment is exact or it fails. It fails when the skeleton's bytes do
// not occur in the source in order (a reader that decoded, normalized or
// synthesized them, or a container format whose skeleton describes an inner
// part), when two refs sit next to each other with no text between them to
// divide their span, and when the text between two refs occurs more than once
// where it could sit, so that two different spans would both reconstruct the
// source. The last is detected by aligning once from the start, taking each
// text at its earliest possible position, and once from the end, taking each
// at its latest: the spans are unique exactly when both passes agree.
//
// Every returned error wraps ErrExtentsUnavailable.
func AlignSkeleton(src []byte, entries []SkeletonEntry) ([]Extent, error) {
	pat, err := compileSkeleton(entries)
	if err != nil {
		return nil, err
	}
	earliest, err := pat.alignForward(src)
	if err != nil {
		return nil, err
	}
	latest, err := pat.alignBackward(src)
	if err != nil {
		return nil, err
	}
	for i := range earliest {
		if earliest[i] != latest[i] {
			return nil, noExtents("the text %q around block %s occurs more than once where it could sit, so its span is ambiguous",
				clip(pat.anchors[i].data), pat.anchorNeighbour(i))
		}
	}

	lines := NewLineIndex(src)
	var out []Extent
	emit := func(id string, start, end int) {
		out = append(out, Extent{Block: id, Start: start, End: end, Lines: lines.Range(start, end)})
	}
	cursor := 0
	for i, a := range pat.anchors {
		if len(a.gapBefore) > 0 {
			emit(a.gapBefore[0], cursor, earliest[i])
		}
		for _, k := range a.known {
			emit(k.id, earliest[i]+k.offset, earliest[i]+k.offset+k.length)
		}
		cursor = earliest[i] + len(a.data)
	}
	if len(pat.trailingGap) > 0 {
		emit(pat.trailingGap[0], cursor, len(src))
	}
	return out, nil
}

// skeletonPattern is a skeleton reduced to what alignment needs: runs of bytes
// the source must contain verbatim (anchors), separated by blocks whose length
// is unknown (gaps).
type skeletonPattern struct {
	anchors     []anchor
	trailingGap []string
}

type anchor struct {
	// gapBefore holds the refs of unknown length between the previous anchor
	// and this one. At most one: two cannot be divided.
	gapBefore []string
	data      []byte
	known     []knownRef
}

// knownRef is a ref whose source bytes a SkeletonOriginal entry stated, placed
// at offset inside its anchor.
type knownRef struct {
	id     string
	offset int
	length int
}

func compileSkeleton(entries []SkeletonEntry) (*skeletonPattern, error) {
	p := &skeletonPattern{}
	var gap []string
	var cur *anchor
	var original []byte
	haveOriginal := false

	fixed := func() *anchor {
		if cur == nil {
			p.anchors = append(p.anchors, anchor{gapBefore: gap})
			cur = &p.anchors[len(p.anchors)-1]
			gap = nil
		}
		return cur
	}

	for i, e := range entries {
		if haveOriginal && e.Type != SkeletonRef {
			return nil, noExtents("skeleton entry %d follows original bytes but is not the ref they stand for", i)
		}
		switch e.Type {
		case SkeletonText, SkeletonLang:
			a := fixed()
			a.data = append(a.data, e.Data...)
		case SkeletonTrimmed:
			_, trimmed, ok := DecodeSkeletonPair(e.Data)
			if !ok {
				return nil, noExtents("skeleton entry %d carries a truncated trimmed-bytes payload", i)
			}
			a := fixed()
			a.data = append(a.data, trimmed...)
		case SkeletonOriginal:
			_, orig, ok := DecodeSkeletonPair(e.Data)
			if !ok {
				return nil, noExtents("skeleton entry %d carries a truncated original-bytes payload", i)
			}
			original, haveOriginal = orig, true
		case SkeletonRef:
			id := string(e.Data)
			if haveOriginal {
				a := fixed()
				a.known = append(a.known, knownRef{id: id, offset: len(a.data), length: len(original)})
				a.data = append(a.data, original...)
				original, haveOriginal = nil, false
				continue
			}
			if len(gap) > 0 {
				return nil, noExtents("blocks %s and %s are adjacent with no text between them to divide their span", gap[0], id)
			}
			// p.anchors may reallocate on the next append; cur is only valid
			// until then, so it is dropped here rather than kept across it.
			cur = nil
			gap = []string{id}
		default:
			return nil, noExtents("skeleton entry %d has type %d, which alignment does not know", i, e.Type)
		}
	}
	if haveOriginal {
		return nil, noExtents("the skeleton ends with original bytes and no ref for them")
	}
	p.trailingGap = gap
	return p, nil
}

// alignForward places each anchor at its earliest position that still lets
// the rest of the skeleton match, returning the anchors' start offsets.
func (p *skeletonPattern) alignForward(src []byte) ([]int, error) {
	pos := make([]int, len(p.anchors))
	cursor := 0
	for i, a := range p.anchors {
		last := i == len(p.anchors)-1 && len(p.trailingGap) == 0
		switch {
		case len(a.gapBefore) == 0:
			if !bytes.HasPrefix(src[cursor:], a.data) {
				return nil, p.mismatch(src, i, cursor)
			}
			pos[i] = cursor
		case last:
			at := len(src) - len(a.data)
			if at < cursor || !bytes.HasSuffix(src, a.data) {
				return nil, p.mismatch(src, i, cursor)
			}
			pos[i] = at
		default:
			at := bytes.Index(src[cursor:], a.data)
			if at < 0 {
				return nil, p.mismatch(src, i, cursor)
			}
			pos[i] = cursor + at
		}
		cursor = pos[i] + len(a.data)
		if last && cursor != len(src) {
			return nil, noExtents("the skeleton ends at byte %d of a %d-byte source", cursor, len(src))
		}
	}
	if len(p.anchors) == 0 && len(p.trailingGap) == 0 && len(src) > 0 {
		return nil, noExtents("the skeleton is empty but the source is %d bytes", len(src))
	}
	return pos, nil
}

// alignBackward is alignForward from the end: each anchor at its latest
// feasible position.
func (p *skeletonPattern) alignBackward(src []byte) ([]int, error) {
	pos := make([]int, len(p.anchors))
	cursor := len(src)
	for i := len(p.anchors) - 1; i >= 0; i-- {
		a := p.anchors[i]
		gapAfter := i < len(p.anchors)-1 || len(p.trailingGap) > 0
		first := i == 0 && len(a.gapBefore) == 0
		switch {
		case !gapAfter:
			if !bytes.HasSuffix(src[:cursor], a.data) {
				return nil, p.mismatch(src, i, cursor-len(a.data))
			}
			pos[i] = cursor - len(a.data)
		case first:
			if len(a.data) > cursor || !bytes.HasPrefix(src, a.data) {
				return nil, p.mismatch(src, i, 0)
			}
			pos[i] = 0
		default:
			at := bytes.LastIndex(src[:cursor], a.data)
			if at < 0 {
				return nil, p.mismatch(src, i, cursor)
			}
			pos[i] = at
		}
		cursor = pos[i]
		if first && cursor != 0 {
			return nil, noExtents("the skeleton starts at byte %d of the source", cursor)
		}
	}
	return pos, nil
}

func (p *skeletonPattern) mismatch(src []byte, i, at int) error {
	if at < 0 {
		at = 0
	}
	if at > len(src) {
		at = len(src)
	}
	return noExtents("the text %q near block %s does not occur in the source at or after byte %d",
		clip(p.anchors[i].data), p.anchorNeighbour(i), at)
}

// anchorNeighbour names a block beside anchor i, for an error message.
func (p *skeletonPattern) anchorNeighbour(i int) string {
	if len(p.anchors[i].gapBefore) > 0 {
		return p.anchors[i].gapBefore[0]
	}
	if len(p.anchors[i].known) > 0 {
		return p.anchors[i].known[0].id
	}
	if i+1 < len(p.anchors) && len(p.anchors[i+1].gapBefore) > 0 {
		return p.anchors[i+1].gapBefore[0]
	}
	if i+1 == len(p.anchors) && len(p.trailingGap) > 0 {
		return p.trailingGap[0]
	}
	return "(none)"
}

func clip(b []byte) string {
	const limit = 40
	if len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + "…"
}
