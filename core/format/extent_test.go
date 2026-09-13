package format

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func text(s string) SkeletonEntry { return SkeletonEntry{Type: SkeletonText, Data: []byte(s)} }
func ref(id string) SkeletonEntry { return SkeletonEntry{Type: SkeletonRef, Data: []byte(id)} }

func original(rendered, orig string) SkeletonEntry {
	return SkeletonEntry{Type: SkeletonOriginal, Data: EncodeSkeletonPair([]byte(rendered), []byte(orig))}
}

func trimmed(rendered, bytes string) SkeletonEntry {
	return SkeletonEntry{Type: SkeletonTrimmed, Data: EncodeSkeletonPair([]byte(rendered), []byte(bytes))}
}

// spans reduces extents to what each test asserts: the bytes each block covers
// and its lines.
type span struct {
	Block string
	Bytes string
	Lines LineRange
}

func spans(src string, xs []Extent) []span {
	out := make([]span, 0, len(xs))
	for _, x := range xs {
		out = append(out, span{Block: x.Block, Bytes: src[x.Start:x.End], Lines: x.Lines})
	}
	return out
}

func TestAlignSkeleton_LocatesEveryRef(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		entries []SkeletonEntry
		want    []span
	}{
		{
			name:    "refs between text",
			src:     "a: Alpha\nb: Bravo\n",
			entries: []SkeletonEntry{text("a: "), ref("1"), text("\nb: "), ref("2"), text("\n")},
			want: []span{
				{Block: "1", Bytes: "Alpha", Lines: LineRange{1, 1}},
				{Block: "2", Bytes: "Bravo", Lines: LineRange{2, 2}},
			},
		},
		{
			name:    "a ref at each edge",
			src:     "Alpha\n\nBravo",
			entries: []SkeletonEntry{ref("1"), text("\n\n"), ref("2")},
			want: []span{
				{Block: "1", Bytes: "Alpha", Lines: LineRange{1, 1}},
				{Block: "2", Bytes: "Bravo", Lines: LineRange{3, 3}},
			},
		},
		{
			name:    "a block over several lines",
			src:     "# T\n\none\ntwo\nthree\n\nend\n",
			entries: []SkeletonEntry{text("# "), ref("h"), text("\n\n"), ref("p"), text("\n\n"), ref("e"), text("\n")},
			want: []span{
				{Block: "h", Bytes: "T", Lines: LineRange{1, 1}},
				{Block: "p", Bytes: "one\ntwo\nthree", Lines: LineRange{3, 5}},
				{Block: "e", Bytes: "end", Lines: LineRange{7, 7}},
			},
		},
		{
			// The line break closing the block is its last byte, so the block
			// ends on the line that break ends, not on the line after it.
			name:    "content ending in a line break",
			src:     "<pre>a\nb\n</pre>\n",
			entries: []SkeletonEntry{text("<pre>"), ref("1"), text("</pre>\n")},
			want:    []span{{Block: "1", Bytes: "a\nb\n", Lines: LineRange{1, 2}}},
		},
		{
			name:    "an empty value covers the line it sits on",
			src:     "{\n  \"a\": \"\",\n  \"b\": \"x\"\n}\n",
			entries: []SkeletonEntry{text("{\n  \"a\": \""), ref("a"), text("\",\n  \"b\": \""), ref("b"), text("\"\n}\n")},
			want: []span{
				{Block: "a", Bytes: "", Lines: LineRange{2, 2}},
				{Block: "b", Bytes: "x", Lines: LineRange{3, 3}},
			},
		},
		{
			name: "original bytes place their ref exactly",
			src:  "<p>Line one\n  line two</p><p>Next</p>",
			entries: []SkeletonEntry{
				text("<p>"), original("Line one line two", "Line one\n  line two"), ref("1"),
				text("</p><p>"), ref("2"), text("</p>"),
			},
			want: []span{
				{Block: "1", Bytes: "Line one\n  line two", Lines: LineRange{1, 2}},
				{Block: "2", Bytes: "Next", Lines: LineRange{2, 2}},
			},
		},
		{
			name: "lang values and trimmed bytes belong to no block",
			src:  "<html lang=\"en\"><p>Hi  </p></html>",
			entries: []SkeletonEntry{
				text("<html lang=\""), {Type: SkeletonLang, Data: []byte("en")}, text("\"><p>"),
				ref("1"), trimmed("Hi", "  "), text("</p></html>"),
			},
			want: []span{{Block: "1", Bytes: "Hi", Lines: LineRange{1, 1}}},
		},
		{
			name:    "CRLF content keeps its carriage returns",
			src:     "k=first\r\nj=second\r\n",
			entries: []SkeletonEntry{text("k="), ref("k"), text("\r\nj="), ref("j"), text("\r\n")},
			want: []span{
				{Block: "k", Bytes: "first", Lines: LineRange{1, 1}},
				{Block: "j", Bytes: "second", Lines: LineRange{2, 2}},
			},
		},
		{
			name:    "a skeleton with no refs has no extents",
			src:     "# only markup\n",
			entries: []SkeletonEntry{text("# only markup\n")},
			want:    []span{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AlignSkeleton([]byte(tt.src), tt.entries)
			require.NoError(t, err)
			assert.Equal(t, tt.want, spans(tt.src, got))
		})
	}
}

// TestAlignSkeleton_RefusesWhatItCannotPlace is the must-fail half: each case is
// a source the skeleton does not describe exactly, and each must return an error
// rather than a span.
func TestAlignSkeleton_RefusesWhatItCannotPlace(t *testing.T) {
	good := []SkeletonEntry{text("a: "), ref("1"), text("\nb: "), ref("2"), text("\n")}
	tests := []struct {
		name    string
		src     string
		entries []SkeletonEntry
		reason  string
	}{
		{name: "one byte of skeleton text changed", src: "a: Alpha\nc: Bravo\n", entries: good, reason: "does not occur"},
		{name: "a leading byte the skeleton does not have", src: " a: Alpha\nb: Bravo\n", entries: good, reason: "does not occur"},
		{name: "a trailing byte after the skeleton's last text", src: "a: Alpha\nb: Bravo\nX", entries: good, reason: "does not occur"},
		{name: "a source longer than a skeleton with no ref", src: "abc\n", entries: []SkeletonEntry{text("abc")}, reason: "ends at byte"},
		{name: "synthesized skeleton text", src: "<p>Hi</p>", entries: []SkeletonEntry{text("<meta charset=utf-8><p>"), ref("1"), text("</p>")}, reason: "does not occur"},
		{
			name:    "two refs with nothing between them",
			src:     "msgid \"a\"\nmsgstr \"b\"\n",
			entries: []SkeletonEntry{ref("1#msgid"), ref("1")},
			reason:  "adjacent",
		},
		{
			// "\n" after the code block could end after its first or its second
			// line break, and both reconstruct the source.
			name:    "text that could sit in two places",
			src:     "Text\n\n    code\n\nMore\n",
			entries: []SkeletonEntry{ref("1"), text("\n\n    "), ref("2"), text("\n"), ref("3"), text("\n")},
			reason:  "ambiguous",
		},
		{name: "original bytes with no ref after them", src: "x", entries: []SkeletonEntry{original("x", "x")}, reason: "no ref"},
		{name: "an entry type alignment does not know", src: "x", entries: []SkeletonEntry{{Type: 99, Data: []byte("x")}}, reason: "does not know"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AlignSkeleton([]byte(tt.src), tt.entries)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrExtentsUnavailable)
			assert.Contains(t, err.Error(), tt.reason)
			assert.Nil(t, got)
		})
	}
}

// TestAlignSkeleton_ReconstructsTheSource asserts, rather than assumes, that the
// spans an alignment returns and the skeleton's own bytes rebuild the source
// byte for byte.
func TestAlignSkeleton_ReconstructsTheSource(t *testing.T) {
	src := "<head><title>T</title></head><p>Line one\n  line two</p>\n<ul><li>A</li><li lang=\"en\">B  </li></ul>\n"
	entries := []SkeletonEntry{
		text("<head>"), {Type: SkeletonInserted, Data: []byte(`<meta charset="utf-8">`)}, text("<title>"), ref("t"), text("</title></head>"),
		text("<p>"), original("Line one line two", "Line one\n  line two"), ref("1"), text("</p>\n<ul><li>"),
		ref("2"), text("</li><li lang=\""), {Type: SkeletonLang, Data: []byte("en")}, text("\">"),
		ref("3"), trimmed("B", "  "), text("</li></ul>\n"),
	}
	xs, err := AlignSkeleton([]byte(src), entries)
	require.NoError(t, err)
	assert.Equal(t, src, rebuild(t, src, entries, xs))
}

// rebuild replays entries with every ref replaced by the bytes its extent names.
func rebuild(t *testing.T, src string, entries []SkeletonEntry, xs []Extent) string {
	t.Helper()
	var out []byte
	next := 0
	pendingOriginal := false
	for _, e := range entries {
		switch e.Type {
		case SkeletonText, SkeletonLang:
			out = append(out, e.Data...)
		case SkeletonTrimmed:
			_, b, _ := DecodeSkeletonPair(e.Data)
			out = append(out, b...)
		case SkeletonInserted:
			// Not in the source, so not part of rebuilding it.
		case SkeletonOriginal:
			pendingOriginal = true
		case SkeletonRef:
			require.Less(t, next, len(xs), "more refs than extents")
			x := xs[next]
			require.Equal(t, string(e.Data), x.Block)
			if pendingOriginal {
				_, b, _ := DecodeSkeletonPair(entries[indexOfOriginalBefore(entries, e)].Data)
				require.Equal(t, string(b), src[x.Start:x.End])
				pendingOriginal = false
			}
			out = append(out, src[x.Start:x.End]...)
			next++
		}
	}
	require.Equal(t, len(xs), next, "extents left over")
	return string(out)
}

func indexOfOriginalBefore(entries []SkeletonEntry, target SkeletonEntry) int {
	last := -1
	for i, e := range entries {
		if e.Type == SkeletonOriginal {
			last = i
		}
		if e.Type == SkeletonRef && string(e.Data) == string(target.Data) {
			return last
		}
	}
	return last
}

// confinedSpan reduces a confined block to what a test asserts: the bytes its
// region covers and the lines any of its spans can cover.
type confinedSpan struct {
	Block  string
	Region string
	Lines  LineRange
}

func confinedSpans(src string, cs []Confined) []confinedSpan {
	out := make([]confinedSpan, 0, len(cs))
	for _, c := range cs {
		out = append(out, confinedSpan{Block: c.Region.Block, Region: src[c.Region.Start:c.Region.End], Lines: c.Region.Lines})
	}
	return out
}

func TestLocateSkeleton_ConfinesOnlyWhatItCannotPlace(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		entries  []SkeletonEntry
		exact    []span
		confined []confinedSpan
		reason   string
	}{
		{
			// The "\n" after the code can end its first or its second line
			// break, which leaves the code and the text after it open. The text
			// before the code sits in one place, so the first block does not.
			name:     "text that could sit in two places",
			src:      "Text\n\n    code\n\nMore\n",
			entries:  []SkeletonEntry{ref("1"), text("\n\n    "), ref("2"), text("\n"), ref("3"), text("\n")},
			exact:    []span{{Block: "1", Bytes: "Text", Lines: LineRange{1, 1}}},
			confined: []confinedSpan{{Block: "2", Region: "code\n", Lines: LineRange{3, 3}}, {Block: "3", Region: "\nMore", Lines: LineRange{4, 5}}},
			reason:   "ambiguous",
		},
		{
			name:     "two refs with nothing between them",
			src:      "k=ab;x=y",
			entries:  []SkeletonEntry{text("k="), ref("a"), ref("b"), text(";x="), ref("c")},
			exact:    []span{{Block: "c", Bytes: "y", Lines: LineRange{1, 1}}},
			confined: []confinedSpan{{Block: "a", Region: "ab", Lines: LineRange{1, 1}}, {Block: "b", Region: "ab", Lines: LineRange{1, 1}}},
			reason:   "adjacent",
		},
		{
			name:     "refs that share a gap of no bytes are all empty",
			src:      "k=;x=y",
			entries:  []SkeletonEntry{text("k="), ref("a"), ref("b"), text(";x="), ref("c")},
			exact:    []span{{Block: "a", Bytes: "", Lines: LineRange{1, 1}}, {Block: "b", Bytes: "", Lines: LineRange{1, 1}}, {Block: "c", Bytes: "y", Lines: LineRange{1, 1}}},
			confined: []confinedSpan{},
		},
		{
			name:    "original bytes inside text that could sit in two places",
			src:     "aXbXc",
			entries: []SkeletonEntry{ref("1"), original("X", "X"), ref("k"), ref("2")},
			exact:   []span{},
			confined: []confinedSpan{
				{Block: "1", Region: "aXb", Lines: LineRange{1, 1}},
				{Block: "k", Region: "XbX", Lines: LineRange{1, 1}},
				{Block: "2", Region: "bXc", Lines: LineRange{1, 1}},
			},
			reason: "ambiguous",
		},
		{
			// Block 2 is "\n" on line 2, or empty just before "b" on line 3.
			name:     "an empty span can close a region on the next line",
			src:      "a\n\nb",
			entries:  []SkeletonEntry{text("a"), ref("1"), text("\n"), ref("2"), text("b")},
			exact:    []span{},
			confined: []confinedSpan{{Block: "1", Region: "\n", Lines: LineRange{1, 1}}, {Block: "2", Region: "\n", Lines: LineRange{2, 3}}},
			reason:   "ambiguous",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			al, err := LocateSkeleton([]byte(tt.src), tt.entries)
			require.NoError(t, err)
			assert.Equal(t, tt.exact, spans(tt.src, al.Extents))
			assert.Equal(t, tt.confined, confinedSpans(tt.src, al.Confined))
			for _, c := range al.Confined {
				assert.Contains(t, c.Reason, tt.reason)
			}
			// The strict form refuses exactly the skeletons that confine a block.
			xs, err := AlignSkeleton([]byte(tt.src), tt.entries)
			if len(tt.confined) > 0 {
				require.ErrorIs(t, err, ErrExtentsUnavailable)
				assert.Contains(t, err.Error(), tt.reason)
				assert.Nil(t, xs)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.exact, spans(tt.src, xs))
			}
		})
	}
}

// TestLocateSkeleton_AgreesWithEveryAlignment holds LocateSkeleton to the
// alignments themselves. For generated sources and skeletons, some corrupted so
// that they no longer fit, it lists every assignment of spans to refs that
// rebuilds the source, by brute force and without the pattern alignment uses,
// and asserts:
//
//   - LocateSkeleton fails exactly when there is no such assignment;
//   - a block it places exactly has that span in every assignment;
//   - a block it confines has its span inside the region, and its lines inside
//     the region's lines, in every assignment;
//   - a block it confines has at least two different spans across the
//     assignments, so it never confines a block the skeleton does place.
func TestLocateSkeleton_AgreesWithEveryAlignment(t *testing.T) {
	r := rand.New(rand.NewPCG(2679, 1))
	const alphabet = "ab\n"
	var exact, confined, refused int
	for range 6000 {
		b := make([]byte, r.IntN(9))
		for i := range b {
			b[i] = alphabet[r.IntN(len(alphabet))]
		}
		src := string(b)
		entries := generateSkeleton(r, src)
		if r.IntN(3) == 0 {
			corruptText(r, entries, alphabet)
		}
		label := fmt.Sprintf("source %q, skeleton %s", src, describeSkeleton(entries))
		all := enumerateAlignments(src, entries)

		al, err := LocateSkeleton([]byte(src), entries)
		if len(all) == 0 {
			require.ErrorIs(t, err, ErrExtentsUnavailable, label)
			refused++
			continue
		}
		require.NoError(t, err, label)

		lines := NewLineIndex([]byte(src))
		placed := map[string]bool{}
		for _, x := range al.Extents {
			placed[x.Block] = true
			for _, a := range all {
				require.Equal(t, [2]int{x.Start, x.End}, a[x.Block], "%s: block %s is placed exactly", label, x.Block)
			}
			require.Equal(t, lines.Range(x.Start, x.End), x.Lines, label)
			exact++
		}
		for _, c := range al.Confined {
			x := c.Region
			placed[x.Block] = true
			spans := map[[2]int]bool{}
			for _, a := range all {
				s := a[x.Block]
				spans[s] = true
				require.True(t, x.Start <= s[0] && s[1] <= x.End, "%s: block %s span %v lies outside its region [%d,%d)", label, x.Block, s, x.Start, x.End)
				l := lines.Range(s[0], s[1])
				require.True(t, x.Lines.First <= l.First && l.Last <= x.Lines.Last, "%s: block %s lines %v lie outside the region's %v", label, x.Block, l, x.Lines)
			}
			require.Greater(t, len(spans), 1, "%s: block %s is confined, but every alignment gives it the same span", label, x.Block)
			confined++
		}
		refs := 0
		for _, e := range entries {
			if e.Type == SkeletonRef {
				refs++
				require.True(t, placed[string(e.Data)], "%s: block %s is neither placed nor confined", label, e.Data)
			}
		}
		require.Equal(t, refs, len(al.Extents)+len(al.Confined), label)
	}
	// Each outcome must occur, or the assertions above prove nothing about it.
	require.Positive(t, exact, "no generated block was placed exactly")
	require.Positive(t, confined, "no generated block was confined")
	require.Positive(t, refused, "no generated skeleton was refused")
}

// generateSkeleton cuts src into segments and turns each into skeleton text, a
// ref, or original bytes and their ref, with an empty ref slipped in now and
// then, so the skeleton rebuilds src at least one way.
func generateSkeleton(r *rand.Rand, src string) []SkeletonEntry {
	var entries []SkeletonEntry
	n := 0
	next := func() SkeletonEntry { n++; return ref(fmt.Sprintf("b%d", n)) }
	for pos := 0; ; {
		if r.IntN(6) == 0 {
			entries = append(entries, next())
		}
		if pos == len(src) {
			return entries
		}
		seg := src[pos : pos+1+r.IntN(min(3, len(src)-pos))]
		switch r.IntN(4) {
		case 0, 1:
			entries = append(entries, text(seg))
		case 2:
			entries = append(entries, next())
		default:
			entries = append(entries, original(seg, seg), next())
		}
		pos += len(seg)
	}
}

// corruptText changes one byte of one text entry, when there is one.
func corruptText(r *rand.Rand, entries []SkeletonEntry, alphabet string) {
	var texts []int
	for i, e := range entries {
		if e.Type == SkeletonText {
			texts = append(texts, i)
		}
	}
	if len(texts) == 0 {
		return
	}
	e := &entries[texts[r.IntN(len(texts))]]
	data := []byte(string(e.Data))
	data[r.IntN(len(data))] = alphabet[r.IntN(len(alphabet))]
	e.Data = data
}

// enumerateAlignments returns every assignment of a span to each ref that
// rebuilds src from entries, each as a map from ref id to [start, end).
func enumerateAlignments(src string, entries []SkeletonEntry) []map[string][2]int {
	var out []map[string][2]int
	cur := map[string][2]int{}
	var walk func(i, pos int)
	walk = func(i, pos int) {
		if i == len(entries) {
			if pos == len(src) {
				out = append(out, maps.Clone(cur))
			}
			return
		}
		e := entries[i]
		switch e.Type {
		case SkeletonText:
			if strings.HasPrefix(src[pos:], string(e.Data)) {
				walk(i+1, pos+len(e.Data))
			}
		case SkeletonOriginal:
			_, orig, _ := DecodeSkeletonPair(e.Data)
			id := string(entries[i+1].Data)
			if strings.HasPrefix(src[pos:], string(orig)) {
				cur[id] = [2]int{pos, pos + len(orig)}
				walk(i+2, pos+len(orig))
				delete(cur, id)
			}
		case SkeletonRef:
			id := string(e.Data)
			for end := pos; end <= len(src); end++ {
				cur[id] = [2]int{pos, end}
				walk(i+1, end)
			}
			delete(cur, id)
		}
	}
	walk(0, 0)
	return out
}

func describeSkeleton(entries []SkeletonEntry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		switch e.Type {
		case SkeletonText:
			parts = append(parts, fmt.Sprintf("text(%q)", e.Data))
		case SkeletonRef:
			parts = append(parts, fmt.Sprintf("ref(%s)", e.Data))
		case SkeletonOriginal:
			_, orig, _ := DecodeSkeletonPair(e.Data)
			parts = append(parts, fmt.Sprintf("original(%q)", orig))
		}
	}
	return strings.Join(parts, " ")
}

func TestLineIndex(t *testing.T) {
	src := []byte("one\r\ntwo\n\nfour")
	x := NewLineIndex(src)
	tests := []struct {
		name   string
		offset int
		want   int
	}{
		{"first byte", 0, 1},
		{"carriage return of a CRLF", 3, 1},
		{"line feed of a CRLF", 4, 1},
		{"start of line two", 5, 2},
		{"an empty line", 9, 3},
		{"last byte", 13, 4},
		{"end of the source", 14, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, x.Line(tt.offset))
		})
	}

	// A trailing newline ends the last line; it does not open another.
	withNewline := NewLineIndex([]byte("a\nb\n"))
	assert.Equal(t, 2, withNewline.Line(4))
	assert.Equal(t, LineRange{First: 2, Last: 2}, withNewline.Range(2, 4))
	assert.Equal(t, LineRange{First: 1, Last: 1}, NewLineIndex(nil).Range(0, 0))
}

func TestLineRangeOverlaps(t *testing.T) {
	block := LineRange{First: 18, Last: 24}
	assert.True(t, block.Overlaps(LineRange{18, 18}), "first line")
	assert.True(t, block.Overlaps(LineRange{24, 24}), "last line")
	assert.True(t, block.Overlaps(LineRange{21, 21}), "a line inside")
	assert.True(t, block.Overlaps(LineRange{10, 30}), "a range around it")
	assert.False(t, block.Overlaps(LineRange{17, 17}), "the line before")
	assert.False(t, block.Overlaps(LineRange{25, 25}), "the line after")
}
