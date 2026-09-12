package format

import (
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
	src := "<p>Line one\n  line two</p>\n<ul><li>A</li><li lang=\"en\">B  </li></ul>\n"
	entries := []SkeletonEntry{
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
