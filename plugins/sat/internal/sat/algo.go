// Package sat implements SaT (Segment any Text / wtpsplit) sentence
// segmentation: tokenize an input with the XLM-RoBERTa SentencePiece
// tokenizer, run the SubwordXLM token-classification ONNX model over
// overlapping blocks, recombine the per-token boundary logits, and turn the
// thresholded probabilities into interior sentence boundaries expressed as
// rune offsets into the input.
//
// The file algo.go holds the deterministic, dependency-free pieces of that
// pipeline (block planning, overlap recombination, sigmoid/threshold, and the
// token→char→rune mapping). It builds and is unit-tested without the ONNX
// runtime or tokenizer native libraries. The cgo-backed model and tokenizer
// live behind the `onnx` build tag in engine_onnx.go.
package sat

import "math"

// Default inference parameters, mirroring wtpsplit-lite for the *-sm models.
const (
	// BlockSize is the maximum number of content tokens per inference window
	// (excluding the CLS/SEP special tokens). XLM-R supports 512.
	BlockSize = 512
	// Stride is the step between successive blocks. Overlap = BlockSize -
	// Stride; wtpsplit's predict_proba uses stride 256.
	Stride = 256
	// DefaultThreshold is the boundary probability cutoff for the *-sm
	// models.
	DefaultThreshold = 0.25
	// NewlineIndex is the logits column carrying the sentence-boundary
	// signal. The SubwordXLM head emits a single label, so the column is 0.
	NewlineIndex = 0
)

// block describes one inference window over the content-token sequence: the
// half-open token range [Start, End) it covers.
type block struct {
	Start int
	End   int
}

// planBlocks splits a content-token sequence of length n into overlapping
// windows of at most BlockSize, advancing by stride. The final window is
// pulled back so it ends exactly at n (matching wtpsplit), which keeps every
// token covered without a ragged tail. For n <= blockSize a single full-range
// window is returned. n == 0 yields no blocks.
func planBlocks(n, blockSize, stride int) []block {
	if n <= 0 {
		return nil
	}
	if n <= blockSize {
		return []block{{Start: 0, End: n}}
	}
	var blocks []block
	for j := 0; j < n; j += stride {
		start, end := j, j+blockSize
		if end >= n {
			end = n
			start = max(end-blockSize, 0)
			blocks = append(blocks, block{Start: start, End: end})
			break
		}
		blocks = append(blocks, block{Start: start, End: end})
	}
	return blocks
}

// combineLogits averages overlapping per-token boundary logits into a single
// value per content token. blockLogits[i] holds the boundary logit for each
// token in blocks[i] (length End-Start). Tokens covered by multiple blocks are
// averaged, matching wtpsplit's weighted-sum-then-divide-by-count (with unit
// weights). The result has length n.
func combineLogits(n int, blocks []block, blockLogits [][]float64) []float64 {
	sum := make([]float64, n)
	count := make([]float64, n)
	for bi, b := range blocks {
		logits := blockLogits[bi]
		for k := 0; k < b.End-b.Start && k < len(logits); k++ {
			idx := b.Start + k
			sum[idx] += logits[k]
			count[idx]++
		}
	}
	out := make([]float64, n)
	for i := range out {
		if count[i] > 0 {
			out[i] = sum[i] / count[i]
		} else {
			out[i] = math.Inf(-1) // uncovered: never a boundary
		}
	}
	return out
}

// sigmoid maps a logit to a probability in (0,1).
func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// tokenSpan is the character span of a content token in the source string,
// expressed as byte offsets [Start, End) (the convention used by the HF
// tokenizer offset mapping).
type tokenSpan struct {
	Start int
	End   int
}

// boundaryRuneOffsets converts per-content-token boundary logits into interior
// sentence boundaries as RUNE offsets into text.
//
// SaT predicts a boundary "at" a token, meaning a sentence ends at that token
// and the next sentence begins at the following character. wtpsplit assigns
// the boundary probability to the LAST character of the predicting token, then
// (when reconstructing sentences) skips whitespace before starting the next
// sentence. We reproduce that: for each token whose probability >= threshold,
// the cut point is the byte offset just past the token's last
// non-whitespace-trimmed character, advanced over any immediately following
// whitespace, then converted to a rune index.
//
// spans and logits are parallel (one per content token, in order). text is the
// exact request text. byteToRune maps a byte offset to its rune index. The
// returned offsets are strictly ascending and exclude 0 and len(runes).
func boundaryRuneOffsets(text string, spans []tokenSpan, logits []float64, threshold float64, byteToRune func(int) int) []int {
	runeLen := len([]rune(text))
	var out []int
	last := -1
	for i, sp := range spans {
		if i >= len(logits) {
			break
		}
		if sigmoid(logits[i]) < threshold {
			continue
		}
		// Boundary falls at the end of this token. Skip following
		// whitespace so the next sentence starts on a non-space character,
		// matching indices_to_sentences' "skip whitespace" behavior.
		bytePos := sp.End
		for bytePos < len(text) && isSpaceByte(text[bytePos]) {
			bytePos++
		}
		rp := byteToRune(bytePos)
		// Interior boundaries only.
		if rp <= 0 || rp >= runeLen {
			continue
		}
		if rp == last {
			continue
		}
		out = append(out, rp)
		last = rp
	}
	return out
}

// isSpaceByte reports whether b is an ASCII whitespace byte. We only skip ASCII
// whitespace here; multibyte Unicode spaces are handled conservatively (left in
// place) so rune offsets stay exact.
func isSpaceByte(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

// buildByteToRune returns a function mapping any byte offset in text to its
// rune index (number of runes before that byte). Offsets that fall in the
// middle of a multibyte rune map to the index of that rune. An offset equal to
// len(text) maps to the total rune count.
func buildByteToRune(text string) func(int) int {
	// runeIndexAt[b] = number of complete runes before byte b.
	runeIndexAt := make([]int, len(text)+1)
	ri := 0
	bi := 0
	for _, r := range text {
		size := runeByteLen(r)
		for k := range size {
			runeIndexAt[bi+k] = ri
		}
		bi += size
		ri++
	}
	runeIndexAt[len(text)] = ri
	return func(b int) int {
		if b < 0 {
			return 0
		}
		if b > len(text) {
			return ri
		}
		return runeIndexAt[b]
	}
}

// runeByteLen returns the UTF-8 byte length of r.
func runeByteLen(r rune) int {
	switch {
	case r < 0x80:
		return 1
	case r < 0x800:
		return 2
	case r < 0x10000:
		return 3
	default:
		return 4
	}
}
