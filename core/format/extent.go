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

// Alignment is where a skeleton places the blocks it refers to in the source
// bytes it was read from.
type Alignment struct {
	// Extents are the blocks the skeleton places exactly, in skeleton order.
	Extents []Extent
	// Confined are the blocks the skeleton confines to a region of the source
	// without fixing their span inside it, in skeleton order.
	Confined []Confined
}

// Confined is a block whose span a skeleton leaves open. Every span the
// skeleton allows for the block lies inside Region, so a change outside
// Region's lines cannot touch the block, and a change on them may.
type Confined struct {
	// Region names the block and bounds its span: Start is the earliest byte
	// the span can start at, End the latest it can end at, and Lines covers
	// every line any span in between covers. Region is not the block's extent.
	Region Extent
	// Reason says why the span is open.
	Reason string
}

// LocateFromSkeleton reads every entry of a flushed skeleton store and locates
// them in src with LocateSkeleton.
func LocateFromSkeleton(src []byte, store *SkeletonStore) (Alignment, error) {
	if store == nil {
		return Alignment{}, noExtents("the reader emitted no skeleton")
	}
	var entries []SkeletonEntry
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Alignment{}, fmt.Errorf("read skeleton: %w", err)
		}
		entries = append(entries, entry)
	}
	return LocateSkeleton(src, entries)
}

// AlignSkeleton is LocateSkeleton for a caller that needs every block placed
// exactly: it fails when the skeleton leaves any block's span open.
//
// Every returned error wraps ErrExtentsUnavailable.
func AlignSkeleton(src []byte, entries []SkeletonEntry) ([]Extent, error) {
	al, err := LocateSkeleton(src, entries)
	if err != nil {
		return nil, err
	}
	if len(al.Confined) > 0 {
		return nil, noExtents("%s", al.Confined[0].Reason)
	}
	return al.Extents, nil
}

// LocateSkeleton locates each block a skeleton refers to in the source bytes
// the skeleton was read from.
//
// A skeleton interleaves the bytes that are not content with a ref for each
// block. The bytes a ref stands in for are the gap between the text entries on
// either side of it, so aligning the text entries (anchors) against the source
// gives every block its span. A SkeletonOriginal entry states the next ref's
// source bytes outright; SkeletonLang and SkeletonTrimmed carry source bytes
// that belong to no block; SkeletonInserted bytes are not in the source at all.
//
// It fails when the skeleton's bytes do not occur in the source in order: a
// reader that decoded, normalized or synthesized them, or a container format
// whose skeleton describes an inner part. Otherwise at least one alignment
// reconstructs the source, and it aligns twice: once from the start, taking
// each anchor at its earliest possible position, and once from the end, taking
// each at its latest. Every alignment that reconstructs the source puts each
// anchor between those two positions, and both passes are such alignments, so
// an anchor sits at one position in every alignment exactly when the passes
// agree on it.
//
// A block is therefore placed exactly when the anchors on both sides of it are
// fixed and no other ref shares its gap. Where either anchor could sit in more
// than one place, or two refs sit next to each other with no text between them
// to divide their span, more than one span reconstructs the source, and the
// block is confined to the region between the earliest start and the latest
// end instead. Each block is judged by its own neighbours, so one open span
// leaves every other block's placement exact.
//
// Every returned error wraps ErrExtentsUnavailable.
func LocateSkeleton(src []byte, entries []SkeletonEntry) (Alignment, error) {
	pat, err := compileSkeleton(entries)
	if err != nil {
		return Alignment{}, err
	}
	earliest, err := pat.alignForward(src)
	if err != nil {
		return Alignment{}, err
	}
	latest, err := pat.alignBackward(src)
	if err != nil {
		return Alignment{}, err
	}

	lines := NewLineIndex(src)
	var al Alignment
	exact := func(id string, start, end int) {
		al.Extents = append(al.Extents, Extent{Block: id, Start: start, End: end, Lines: lines.Range(start, end)})
	}
	// confine records a block whose span starts in [minStart, maxStart] and
	// ends at or before maxEnd.
	confine := func(id string, minStart, maxStart, maxEnd int, reason string) {
		covered := lines.Range(minStart, maxEnd)
		if maxStart == maxEnd {
			// An empty span can sit at the region's end, on the line that
			// byte begins.
			covered.Last = max(covered.Last, lines.Line(maxEnd))
		}
		al.Confined = append(al.Confined, Confined{
			Region: Extent{Block: id, Start: minStart, End: maxEnd, Lines: covered},
			Reason: reason,
		})
	}
	ambiguous := func(i int, id string) string {
		return fmt.Sprintf("the text %q around block %s occurs more than once where it could sit, so its span is ambiguous",
			clip(pat.anchors[i].data), id)
	}
	// placeGap places the refs between anchor prev (-1 for the start of the
	// source) and anchor next (len(pat.anchors) for its end).
	placeGap := func(refs []string, prev, next int) {
		minStart, maxStart := 0, 0
		if prev >= 0 {
			n := len(pat.anchors[prev].data)
			minStart, maxStart = earliest[prev]+n, latest[prev]+n
		}
		minEnd, maxEnd := len(src), len(src)
		if next < len(pat.anchors) {
			minEnd, maxEnd = earliest[next], latest[next]
		}
		fixed := minStart == maxStart && minEnd == maxEnd
		switch {
		case fixed && len(refs) == 1:
			exact(refs[0], minStart, maxEnd)
			return
		case fixed && minStart == maxEnd:
			// Refs that share a gap of no bytes can only all be empty.
			for _, id := range refs {
				exact(id, minStart, minStart)
			}
			return
		}
		var reason string
		switch {
		case len(refs) > 1:
			reason = fmt.Sprintf("blocks %s and %s are adjacent with no text between them to divide their span", refs[0], refs[1])
		case minEnd != maxEnd:
			reason = ambiguous(next, refs[0])
		default:
			reason = ambiguous(prev, refs[0])
		}
		for k, id := range refs {
			latestStart := maxStart
			if k > 0 {
				// A ref after the first starts where the refs before it end,
				// which can be as late as the end of the gap.
				latestStart = maxEnd
			}
			confine(id, minStart, latestStart, maxEnd, reason)
		}
	}

	for i, a := range pat.anchors {
		if len(a.gapBefore) > 0 {
			placeGap(a.gapBefore, i-1, i)
		}
		for _, k := range a.known {
			if earliest[i] == latest[i] {
				exact(k.id, earliest[i]+k.offset, earliest[i]+k.offset+k.length)
				continue
			}
			confine(k.id, earliest[i]+k.offset, latest[i]+k.offset, latest[i]+k.offset+k.length, ambiguous(i, k.id))
		}
	}
	if len(pat.trailingGap) > 0 {
		placeGap(pat.trailingGap, len(pat.anchors)-1, len(pat.anchors))
	}
	return al, nil
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
	// and this one. More than one sit next to each other with no text to
	// divide them.
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
		case SkeletonInserted:
			// The reader added these bytes; the source has none to match, and
			// the text on either side of them is contiguous there.
		case SkeletonRef:
			id := string(e.Data)
			if haveOriginal {
				a := fixed()
				a.known = append(a.known, knownRef{id: id, offset: len(a.data), length: len(original)})
				a.data = append(a.data, original...)
				original, haveOriginal = nil, false
				continue
			}
			// p.anchors may reallocate on the next append; cur is only valid
			// until then, so it is dropped here rather than kept across it.
			cur = nil
			gap = append(gap, id)
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
