package sat

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanBlocks(t *testing.T) {
	tests := []struct {
		name      string
		n, bs, st int
		want      []block
	}{
		{"empty", 0, 512, 256, nil},
		{"single short", 10, 512, 256, []block{{0, 10}}},
		{"exact block", 512, 512, 256, []block{{0, 512}}},
		{
			name: "two overlapping",
			n:    600, bs: 512, st: 256,
			// j=0 -> [0,512); j=256 -> end 768>=600 -> [600-512,600)=[88,600)
			want: []block{{0, 512}, {88, 600}},
		},
		{
			name: "three windows",
			n:    1100, bs: 512, st: 256,
			// j=0 [0,512); j=256 [256,768); j=512 end1024<1100 [512,1024);
			// j=768 end1280>=1100 -> [588,1100)
			want: []block{{0, 512}, {256, 768}, {512, 1024}, {588, 1100}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planBlocks(tt.n, tt.bs, tt.st)
			assert.Equal(t, tt.want, got)
			// Every token must be covered.
			if tt.n > 0 {
				covered := make([]bool, tt.n)
				for _, b := range got {
					for i := b.Start; i < b.End; i++ {
						covered[i] = true
					}
				}
				for i, c := range covered {
					require.True(t, c, "token %d uncovered", i)
				}
			}
		})
	}
}

func TestCombineLogitsAverages(t *testing.T) {
	// n=4, two blocks [0,3) and [1,4) overlapping on tokens 1,2.
	blocks := []block{{0, 3}, {1, 4}}
	blockLogits := [][]float64{
		{1.0, 2.0, 3.0},    // tokens 0,1,2
		{10.0, 20.0, 30.0}, // tokens 1,2,3
	}
	got := combineLogits(4, blocks, blockLogits)
	require.Len(t, got, 4)
	assert.InDelta(t, 1.0, got[0], 1e-9)          // only block 0
	assert.InDelta(t, (2.0+10.0)/2, got[1], 1e-9) // avg
	assert.InDelta(t, (3.0+20.0)/2, got[2], 1e-9) // avg
	assert.InDelta(t, 30.0, got[3], 1e-9)         // only block 1
}

func TestCombineLogitsUncoveredIsNegInf(t *testing.T) {
	got := combineLogits(3, []block{{0, 1}}, [][]float64{{5.0}})
	assert.InDelta(t, 5.0, got[0], 1e-9)
	assert.True(t, math.IsInf(got[1], -1))
	assert.True(t, math.IsInf(got[2], -1))
}

func TestSigmoid(t *testing.T) {
	assert.InDelta(t, 0.5, sigmoid(0), 1e-9)
	assert.Greater(t, sigmoid(10), 0.99)
	assert.Less(t, sigmoid(-10), 0.01)
}

func TestBuildByteToRuneASCII(t *testing.T) {
	text := "abc"
	f := buildByteToRune(text)
	assert.Equal(t, 0, f(0))
	assert.Equal(t, 1, f(1))
	assert.Equal(t, 3, f(3)) // end
}

func TestBuildByteToRuneMultibyte(t *testing.T) {
	// "héllo": h(1) é(2 bytes) l l o => 6 bytes, 5 runes.
	text := "héllo"
	require.Equal(t, 6, len(text))
	require.Equal(t, 5, len([]rune(text)))
	f := buildByteToRune(text)
	assert.Equal(t, 0, f(0)) // before h
	assert.Equal(t, 1, f(1)) // before é (byte 1)
	assert.Equal(t, 2, f(3)) // after é (é occupies bytes 1,2) -> rune index 2
	assert.Equal(t, 5, f(6)) // end
}

func TestBuildByteToRuneInvalidUTF8(t *testing.T) {
	// Invalid UTF-8 must not panic. A lone continuation byte (0x80) is invalid:
	// `for range` yields U+FFFD and advances exactly ONE byte, so the byte
	// width must follow the decoder, not the replacement rune's nominal 3-byte
	// width (the old buildByteToRune overran its index here).
	tests := []struct {
		name string
		text string
	}{
		{"lone continuation byte", "a\x80b"},
		{"truncated multibyte", "a\xc3"},
		{"invalid lead byte", "\xffz"},
		{"all invalid", "\x80\x80\x80"},
		{"valid then invalid", "héllo\x80"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f func(int) int
			require.NotPanics(t, func() { f = buildByteToRune(tt.text) })
			// f must be total over [0, len(text)] and agree with the rune count
			// at the boundaries.
			assert.Equal(t, 0, f(0))
			assert.Equal(t, len([]rune(tt.text)), f(len(tt.text)), "end maps to total rune count")
			// Every in-range byte offset must be queryable without panic and
			// produce a monotonically non-decreasing rune index.
			prev := 0
			for b := 0; b <= len(tt.text); b++ {
				ri := f(b)
				assert.GreaterOrEqual(t, ri, prev, "rune index must be non-decreasing")
				prev = ri
			}
		})
	}
}

func TestBoundaryRuneOffsets(t *testing.T) {
	// "Hello world. How are you?"
	//  0123456789012345678901234
	// Token whose span ends at byte 12 (the '.') with high logit -> boundary.
	// Skipping the following space (byte 12 is '.', byte 12.. wait recompute).
	text := "Hello world. How are you?"
	// Find positions: '.' is index 11, space is index 12, 'H' of How is 13.
	require.Equal(t, byte('.'), text[11])
	require.Equal(t, byte(' '), text[12])
	require.Equal(t, byte('H'), text[13])

	// One content token covering the period, ending at byte 12 (just past '.').
	spans := []tokenSpan{{Start: 9, End: 12}} // "ld." region; End=12 (exclusive, past '.')
	logits := []float64{5.0}                  // sigmoid(5) ~ 0.993 >= 0.25
	f := buildByteToRune(text)
	got := boundaryRuneOffsets(text, spans, logits, 0.25, f)
	// End=12 is '.', then skip the space at byte 12 -> byte 13 ('H') -> rune 13.
	assert.Equal(t, []int{13}, got)
}

func TestBoundaryRuneOffsetsBelowThreshold(t *testing.T) {
	text := "Hello world."
	spans := []tokenSpan{{Start: 0, End: 5}}
	logits := []float64{-5.0} // sigmoid ~ 0.0067 < 0.25
	f := buildByteToRune(text)
	got := boundaryRuneOffsets(text, spans, logits, 0.25, f)
	assert.Empty(t, got)
}

func TestBoundaryRuneOffsetsExcludesEnds(t *testing.T) {
	text := "Hello."
	// Token ends at the very end -> boundary at end is excluded.
	spans := []tokenSpan{{Start: 0, End: 6}}
	logits := []float64{5.0}
	f := buildByteToRune(text)
	got := boundaryRuneOffsets(text, spans, logits, 0.25, f)
	assert.Empty(t, got, "boundary at text end must be excluded")
}

func TestBoundaryRuneOffsetsAscendingDedup(t *testing.T) {
	text := "a b c d"
	// Two tokens whose post-whitespace cut maps to the same rune -> dedup.
	spans := []tokenSpan{
		{Start: 0, End: 1}, // 'a', end byte1 (space), skip -> byte2 'b' rune2
		{Start: 1, End: 1}, // zero-width same place -> also rune2
		{Start: 2, End: 3}, // 'b', end byte3 (space) skip -> byte4 'c' rune4
	}
	logits := []float64{5.0, 5.0, 5.0}
	f := buildByteToRune(text)
	got := boundaryRuneOffsets(text, spans, logits, 0.25, f)
	assert.Equal(t, []int{2, 4}, got)
}
