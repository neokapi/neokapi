package comment

import (
	"bytes"
	"strings"
)

// maxDiffCells bounds the comparison table DiffLines builds. Sequences long
// enough to exceed it are compared as changed throughout rather than not at
// all.
const maxDiffCells = 1 << 22

// DiffLines compares a with b by a longest common subsequence. changed[i]
// reports that a[i] has no counterpart in b; inserted[i] reports that b holds
// lines with no counterpart in a immediately before a[i] (inserted[len(a)] is
// after the last line).
func DiffLines(a, b []string) (changed, inserted []bool) {
	n, m := len(a), len(b)
	changed, inserted = make([]bool, n), make([]bool, n+1)
	if (n+1)*(m+1) > maxDiffCells {
		for i := range changed {
			changed[i] = true
		}
		return changed, inserted
	}
	// lcs[i][j] is the length of the longest common subsequence of a[i:] and b[j:].
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			changed[i] = true
			i++
		default:
			inserted[i] = true
			j++
		}
	}
	for ; i < n; i++ {
		changed[i] = true
	}
	if j < m {
		inserted[n] = true
	}
	return changed, inserted
}

// formatterChurn returns the lines of src, numbered from 1, that formatted
// changes, and the lines on either side of a line it inserts. Line endings are
// not compared.
func formatterChurn(src, formatted []byte) map[int]bool {
	if bytes.Equal(src, formatted) {
		return nil
	}
	a, b := fileLines(src), fileLines(formatted)
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	changed, inserted := DiffLines(a[p:len(a)-s], b[p:len(b)-s])
	out := map[int]bool{}
	for i, c := range changed {
		if c {
			out[p+i+1] = true
		}
	}
	for i, ins := range inserted {
		if !ins {
			continue
		}
		if p+i < len(a) {
			out[p+i+1] = true
		}
		if p+i > 0 {
			out[p+i] = true
		}
	}
	return out
}

// fileLines splits src into its lines, each without its line ending.
func fileLines(src []byte) []string {
	lines := strings.Split(string(src), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}
