package format

// A writer can write an edited value as a patch of the bytes it was read from.
// Each character the edit kept is copied as the document spelled it: an
// escape, an entity, a run of whitespace the reader normalized. Only the
// characters the edit added are encoded, so a one-word edit changes one word
// of the file.
//
// MatchEdit is the alignment those writers share. It pairs elements of the
// sequence as read with elements of the edited sequence, in order, so a writer
// can tell a kept element from a removed or an added one.

// maxEditAlignCells bounds the table MatchEdit fills for the part of the two
// sequences between their common prefix and suffix. Past it the middle is
// treated as replaced whole: the write stays correct and the spelling of the
// kept characters inside that middle is not preserved.
const maxEditAlignCells = 1 << 22

// MatchEdit aligns a sequence of n elements as read with an edited sequence of
// m elements. For each element as read it returns the index of the edited
// element paired with it, or -1 when the edit removed it. same reports whether
// element i as read and edited element j are the same element. The pairing is
// a longest common subsequence, in increasing order on both sides. An edited
// element no pair names is one the edit added.
func MatchEdit(n, m int, same func(i, j int) bool) []int {
	match := make([]int, n)
	for i := range match {
		match[i] = -1
	}
	pre := 0
	for pre < n && pre < m && same(pre, pre) {
		match[pre] = pre
		pre++
	}
	suf := 0
	for suf < n-pre && suf < m-pre && same(n-1-suf, m-1-suf) {
		match[n-1-suf] = m - 1 - suf
		suf++
	}
	a, b := n-pre-suf, m-pre-suf
	if a == 0 || b == 0 || a*b > maxEditAlignCells {
		return match
	}
	// lcs[i*(b+1)+j] is the length of the longest common subsequence of the
	// middle elements from i and j on.
	w := b + 1
	lcs := make([]int32, (a+1)*w)
	for i := a - 1; i >= 0; i-- {
		for j := b - 1; j >= 0; j-- {
			switch {
			case same(pre+i, pre+j):
				lcs[i*w+j] = lcs[(i+1)*w+j+1] + 1
			case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
				lcs[i*w+j] = lcs[(i+1)*w+j]
			default:
				lcs[i*w+j] = lcs[i*w+j+1]
			}
		}
	}
	for i, j := 0, 0; i < a && j < b; {
		switch {
		case same(pre+i, pre+j) && lcs[i*w+j] == lcs[(i+1)*w+j+1]+1:
			match[pre+i] = pre + j
			i++
			j++
		case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
			i++
		default:
			j++
		}
	}
	return match
}
