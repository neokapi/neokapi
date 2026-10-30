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

// maxEditAlignWork bounds the steps MatchEdit spends on one alignment. Its
// cost grows with the length of the sequences times the number of elements
// the edit changed, so a short edit of a long value is cheap and only an edit
// that rewrites most of a very long value reaches the bound. Past it, the
// part of the sequences still unaligned is treated as replaced whole: the
// write stays correct, and the kept characters inside that part are spelled
// as the writer spells new ones.
const maxEditAlignWork = 1 << 26

// MatchEdit aligns a sequence of n elements as read with an edited sequence of
// m elements. For each element as read it returns the index of the edited
// element paired with it, or -1 when the edit removed it. same reports whether
// element i as read and edited element j are the same element. The pairing is
// a shortest edit script (Myers' O(ND) difference algorithm in linear space),
// in increasing order on both sides. An edited element no pair names is one
// the edit added.
func MatchEdit(n, m int, same func(i, j int) bool) []int {
	match := make([]int, n)
	for i := range match {
		match[i] = -1
	}
	d := differ{same: same, match: match, budget: maxEditAlignWork}
	d.compare(0, n, 0, m)
	return match
}

// differ holds one alignment's state.
type differ struct {
	same   func(i, j int) bool
	match  []int
	budget int
}

// compare aligns elements [a0, a1) as read with edited elements [b0, b1).
func (d *differ) compare(a0, a1, b0, b1 int) {
	for a0 < a1 && b0 < b1 && d.same(a0, b0) {
		d.match[a0] = b0
		a0++
		b0++
	}
	for a0 < a1 && b0 < b1 && d.same(a1-1, b1-1) {
		d.match[a1-1] = b1 - 1
		a1--
		b1--
	}
	if a0 == a1 || b0 == b1 {
		return
	}
	x, y, ok := d.bisect(a0, a1, b0, b1)
	if !ok || (x == a0 && y == b0) || (x == a1 && y == b1) {
		return
	}
	d.compare(a0, x, b0, y)
	d.compare(x, a1, y, b1)
}

// bisect finds a point on a shortest edit path between [a0, a1) and [b0, b1)
// where the path searched forward from the start meets the path searched
// backward from the end, so the two halves can be aligned apart. It reports
// false when the alignment's work bound runs out first.
func (d *differ) bisect(a0, a1, b0, b1 int) (int, int, bool) {
	n, m := a1-a0, b1-b0
	maxD := (n + m + 1) / 2
	off := maxD
	size := 2*maxD + 2
	v1 := make([]int, size)
	v2 := make([]int, size)
	for i := range v1 {
		v1[i], v2[i] = -1, -1
	}
	v1[off+1], v2[off+1] = 0, 0
	delta := n - m
	// With an odd delta the forward path meets the backward one; with an
	// even delta the backward path meets the forward one.
	front := delta%2 != 0
	k1start, k1end, k2start, k2end := 0, 0, 0, 0
	for step := range maxD {
		d.budget -= 2*step + 1
		if d.budget < 0 {
			return 0, 0, false
		}
		for k1 := -step + k1start; k1 <= step-k1end; k1 += 2 {
			k1off := off + k1
			var x1 int
			if k1 == -step || (k1 != step && v1[k1off-1] < v1[k1off+1]) {
				x1 = v1[k1off+1]
			} else {
				x1 = v1[k1off-1] + 1
			}
			y1 := x1 - k1
			for x1 < n && y1 < m && d.same(a0+x1, b0+y1) {
				x1++
				y1++
			}
			v1[k1off] = x1
			switch {
			case x1 > n:
				k1end += 2
			case y1 > m:
				k1start += 2
			case front:
				if k2off := off + delta - k1; k2off >= 0 && k2off < size && v2[k2off] != -1 {
					if x1 >= n-v2[k2off] {
						return a0 + x1, b0 + y1, true
					}
				}
			}
		}
		for k2 := -step + k2start; k2 <= step-k2end; k2 += 2 {
			k2off := off + k2
			var x2 int
			if k2 == -step || (k2 != step && v2[k2off-1] < v2[k2off+1]) {
				x2 = v2[k2off+1]
			} else {
				x2 = v2[k2off-1] + 1
			}
			y2 := x2 - k2
			for x2 < n && y2 < m && d.same(a1-x2-1, b1-y2-1) {
				x2++
				y2++
			}
			v2[k2off] = x2
			switch {
			case x2 > n:
				k2end += 2
			case y2 > m:
				k2start += 2
			case !front:
				if k1off := off + delta - k2; k1off >= 0 && k1off < size && v1[k1off] != -1 {
					x1 := v1[k1off]
					y1 := off + x1 - k1off
					if x1 >= n-x2 {
						return a0 + x1, b0 + y1, true
					}
				}
			}
		}
	}
	return 0, 0, false
}
