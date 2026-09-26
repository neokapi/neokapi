// Package diffscope reads a unified diff into the lines each file's change
// touched, and resolves those lines to the blocks whose content sits on them.
//
// A diff speaks in lines of the post-image, and the unit worth checking is the
// block those lines belong to. Parse gives the lines; Touched widens them to
// whole blocks, given where each block sits (format.Extent). Both are pure: no
// file is read and nothing runs git.
package diffscope

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
)

// Status is what a diff did to one file.
type Status string

const (
	Modified Status = "modified"
	Added    Status = "added"
	Deleted  Status = "deleted"
	Renamed  Status = "renamed"
	Copied   Status = "copied"
)

// File is one file's entry in a diff.
type File struct {
	// OldPath is the path before the change, and empty for an added file.
	OldPath string
	// NewPath is the path after the change, and empty for a deleted file.
	NewPath string
	Status  Status
	// Binary marks a change git reported without line content ("Binary files
	// differ" or a binary patch). Changes is empty for it, and says nothing about
	// what was touched.
	Binary bool
	// ModeChanged marks a change of file mode.
	ModeChanged bool
	// Changes are the post-image lines the diff touched, in line order.
	Changes []Change
	// PostLines are the post-image lines the diff shows, context and added,
	// keyed by line number and without their line feed. They let a caller
	// confirm that the diff describes the file it is about to read before
	// trusting the diff's line numbers.
	PostLines map[int]string
	// Hunks are the diff's hunks as written, from which PreImage rebuilds the
	// file as it was before the change.
	Hunks []Hunk
}

// Hunk is one hunk of a unified diff: its header's ranges and its lines, each
// still carrying its leading ' ', '-', '+' or '\\'.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Lines              []string
}

// Path is the file's path after the change, or before it for a deleted file.
func (f File) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// Change is one contiguous edit, located in the post-image.
type Change struct {
	// Lines are the post-image lines the edit touched. For lines that were added
	// or replaced, these are the new lines.
	//
	// A deletion leaves no line behind, so it is located at the post-image
	// position the removed lines occupied: the lines on either side of it. A
	// deletion inside a block therefore touches that block, and so does one that
	// removed a block's first or last lines. A deletion between two blocks
	// touches whichever of them sits on an adjacent line.
	Lines format.LineRange
	// Deletion marks an edit that removed lines and added none.
	Deletion bool
	// After is, for a deletion, the post-image line the removed lines followed;
	// 0 when they opened the file.
	After int
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// Parse reads a unified diff, in the form git diff writes it or in the plain
// form diff -u writes. Content lines may end in CRLF. A combined diff (the
// "diff --cc" form of a merge) is refused rather than half-read.
func Parse(diff []byte) ([]File, error) {
	p := &parser{}
	// Lines split at the line feed alone. A carriage return before it is part
	// of a CRLF file's content line, and the post-image and pre-image checks
	// compare those lines with the file byte for byte.
	if len(diff) > 0 {
		p.lines = strings.Split(strings.TrimSuffix(string(diff), "\n"), "\n")
	}
	if err := p.parse(); err != nil {
		return nil, err
	}
	return p.files, nil
}

type parser struct {
	lines []string
	i     int
	files []File
	cur   *File
	// git is true while cur came from a "diff --git" header, whose paths carry
	// the a/ and b/ prefixes.
	git bool
	// headerDone is true once cur has had its ---/+++ pair, so another --- line
	// begins a new plain-diff file.
	headerDone bool
}

func (p *parser) start(git bool) {
	p.finish()
	p.files = append(p.files, File{Status: Modified})
	p.cur = &p.files[len(p.files)-1]
	p.git = git
	p.headerDone = false
}

func (p *parser) finish() {
	if p.cur == nil {
		return
	}
	f := p.cur
	switch {
	case f.Status == Renamed || f.Status == Copied:
	case f.OldPath == "" && f.NewPath != "":
		f.Status = Added
	case f.NewPath == "" && f.OldPath != "":
		f.Status = Deleted
	}
	p.cur = nil
}

func (p *parser) parse() error {
	for p.i < len(p.lines) {
		line := strings.TrimSuffix(p.lines[p.i], "\r")
		switch {
		case strings.HasPrefix(line, "diff --cc ") || strings.HasPrefix(line, "diff --combined ") || strings.HasPrefix(line, "@@@ "):
			return errors.New("combined diffs (a merge commit's diff) are not supported; diff against one parent")
		case strings.HasPrefix(line, "diff --git "):
			p.start(true)
			old, nw := splitGitHeader(strings.TrimPrefix(line, "diff --git "))
			p.cur.OldPath, p.cur.NewPath = old, nw
		case p.cur != nil && strings.HasPrefix(line, "new file mode "):
			p.cur.OldPath = ""
			p.cur.Status = Added
		case p.cur != nil && strings.HasPrefix(line, "deleted file mode "):
			p.cur.NewPath = ""
			p.cur.Status = Deleted
		case p.cur != nil && (strings.HasPrefix(line, "old mode ") || strings.HasPrefix(line, "new mode ")):
			p.cur.ModeChanged = true
		case p.cur != nil && strings.HasPrefix(line, "rename from "):
			p.cur.OldPath = unquotePath(strings.TrimPrefix(line, "rename from "))
			p.cur.Status = Renamed
		case p.cur != nil && strings.HasPrefix(line, "rename to "):
			p.cur.NewPath = unquotePath(strings.TrimPrefix(line, "rename to "))
			p.cur.Status = Renamed
		case p.cur != nil && strings.HasPrefix(line, "copy from "):
			p.cur.OldPath = unquotePath(strings.TrimPrefix(line, "copy from "))
			p.cur.Status = Copied
		case p.cur != nil && strings.HasPrefix(line, "copy to "):
			p.cur.NewPath = unquotePath(strings.TrimPrefix(line, "copy to "))
			p.cur.Status = Copied
		case p.cur != nil && (strings.HasPrefix(line, "Binary files ") || line == "GIT binary patch"):
			p.cur.Binary = true
		case strings.HasPrefix(line, "--- ") && p.i+1 < len(p.lines) && strings.HasPrefix(p.lines[p.i+1], "+++ "):
			if p.cur == nil || p.headerDone || !p.git {
				p.start(false)
			}
			old := headerPath(strings.TrimPrefix(line, "--- "))
			nw := headerPath(strings.TrimSuffix(strings.TrimPrefix(p.lines[p.i+1], "+++ "), "\r"))
			if p.git || (strings.HasPrefix(old, "a/") && strings.HasPrefix(nw, "b/")) {
				old, nw = strings.TrimPrefix(old, "a/"), strings.TrimPrefix(nw, "b/")
			}
			if old == devNull {
				old = ""
			}
			if nw == devNull {
				nw = ""
			}
			p.cur.OldPath, p.cur.NewPath = old, nw
			p.headerDone = true
			p.i++
		case strings.HasPrefix(line, "@@ "):
			if p.cur == nil {
				return fmt.Errorf("diff line %d: a hunk with no file header", p.i+1)
			}
			if err := p.hunk(line); err != nil {
				return err
			}
			continue
		}
		p.i++
	}
	p.finish()
	return nil
}

const devNull = "/dev/null"

// hunk consumes one hunk, starting at its header line, and appends its changes
// to the current file.
func (p *parser) hunk(header string) error {
	m := hunkHeader.FindStringSubmatch(header)
	if m == nil {
		return fmt.Errorf("diff line %d: malformed hunk header %q", p.i+1, header)
	}
	oldCount, newStart, newCount := count(m[2]), atoi(m[3]), count(m[4])
	h := Hunk{OldStart: atoi(m[1]), OldCount: oldCount, NewStart: newStart, NewCount: newCount}
	headerLine := p.i + 1
	p.i++

	// The next post-image line to be numbered. A hunk that adds no lines names
	// the line before its position, so its first line would be the one after.
	next := newStart
	if newCount == 0 {
		next = newStart + 1
	}
	oldLeft, newLeft := oldCount, newCount

	runOpen := false
	plusFirst, plusCount, deletedAfter := 0, 0, 0
	closeRun := func() {
		if !runOpen {
			return
		}
		if plusCount > 0 {
			p.cur.Changes = append(p.cur.Changes, Change{Lines: format.LineRange{First: plusFirst, Last: plusFirst + plusCount - 1}})
		} else {
			// A deletion leaves no line to point at, so it takes the lines on
			// either side of where the removed lines were. Line numbers alone
			// cannot tell a block that lost its first or last line from a block
			// beside lines that were removed, and taking both sides never misses
			// the first. It is also right where a blank line is a boundary:
			// removing the one between two paragraphs or two comment groups
			// merges them. The cost is a block the deletion only borders, such as
			// the entry before a removed JSON key, which Settle clears when the
			// pre-image shows the block unchanged.
			first := max(deletedAfter, 1)
			p.cur.Changes = append(p.cur.Changes, Change{Lines: format.LineRange{First: first, Last: deletedAfter + 1}, Deletion: true, After: deletedAfter})
		}
		runOpen, plusCount = false, 0
	}

	for oldLeft > 0 || newLeft > 0 {
		if p.i >= len(p.lines) {
			return fmt.Errorf("diff line %d: the hunk ends early, %d old and %d new lines short", headerLine, oldLeft, newLeft)
		}
		line := p.lines[p.i]
		if line == "" {
			h.Lines = append(h.Lines, " ")
		} else {
			h.Lines = append(h.Lines, line)
		}
		switch {
		case line == "" || line[0] == ' ':
			// An empty line is a context line whose leading space was stripped
			// by an editor or a mail client.
			closeRun()
			if oldLeft == 0 || newLeft == 0 {
				return fmt.Errorf("diff line %d: a context line beyond the hunk's counts", p.i+1)
			}
			p.post(next, line)
			oldLeft--
			newLeft--
			next++
		case line[0] == '-':
			if oldLeft == 0 {
				return fmt.Errorf("diff line %d: a removed line beyond the hunk's counts", p.i+1)
			}
			if !runOpen {
				runOpen, deletedAfter = true, next-1
			}
			oldLeft--
		case line[0] == '+':
			if newLeft == 0 {
				return fmt.Errorf("diff line %d: an added line beyond the hunk's counts", p.i+1)
			}
			if !runOpen {
				runOpen, deletedAfter = true, next-1
			}
			if plusCount == 0 {
				plusFirst = next
			}
			p.post(next, line)
			plusCount++
			newLeft--
			next++
		case line[0] == '\\':
			// "\ No newline at end of file" describes the line before it.
		default:
			return fmt.Errorf("diff line %d: %q is not a line of the hunk", p.i+1, line)
		}
		p.i++
	}
	closeRun()
	for p.i < len(p.lines) && strings.HasPrefix(p.lines[p.i], `\`) {
		h.Lines = append(h.Lines, p.lines[p.i])
		p.i++
	}
	p.cur.Hunks = append(p.cur.Hunks, h)
	// A removed or added line straight after the counts ran out means the
	// counts are wrong, and the lines past them would be dropped unread. A
	// ---/+++ pair opens the next file, and "-- " is a mail signature.
	if p.i < len(p.lines) {
		next := strings.TrimSuffix(p.lines[p.i], "\r")
		header := strings.HasPrefix(next, "--- ") && p.i+1 < len(p.lines) && strings.HasPrefix(p.lines[p.i+1], "+++ ")
		if !header && next != "-- " && (strings.HasPrefix(next, "+") || strings.HasPrefix(next, "-")) {
			return fmt.Errorf("diff line %d: a changed line beyond the hunk's counts", p.i+1)
		}
	}
	return nil
}

func (p *parser) post(line int, text string) {
	if p.cur.PostLines == nil {
		p.cur.PostLines = map[int]string{}
	}
	if text != "" {
		text = text[1:]
	}
	p.cur.PostLines[line] = text
}

func count(s string) int {
	if s == "" {
		return 1
	}
	return atoi(s)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// headerPath reads the path from a ---/+++ line: quoted when git quoted it, and
// cut at a tab, after which plain diff writes a timestamp.
func headerPath(s string) string {
	if strings.HasPrefix(s, `"`) {
		if end := closingQuote(s); end > 0 {
			return unquotePath(s[:end+1])
		}
	}
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	return s
}

// splitGitHeader reads the two paths of a "diff --git a/X b/Y" line, which is
// the only place a file with no ---/+++ pair (a binary, a pure rename, an empty
// new file) names itself. Unquoted paths may contain spaces, so the split is
// taken where both halves name the same file, which is the case git writes for
// every file that was not renamed.
func splitGitHeader(s string) (string, string) {
	if strings.HasPrefix(s, `"`) {
		if end := closingQuote(s); end > 0 {
			old := unquotePath(s[:end+1])
			nw := unquotePath(strings.TrimSpace(s[end+1:]))
			return strings.TrimPrefix(old, "a/"), strings.TrimPrefix(nw, "b/")
		}
	}
	if n := len(s); n%2 == 1 {
		half := n / 2
		if s[half] == ' ' && strings.HasPrefix(s, "a/") && strings.HasPrefix(s[half+1:], "b/") && s[2:half] == s[half+3:] {
			return s[2:half], s[half+3:]
		}
	}
	if before, after, ok := strings.CutLast(s, " b/"); ok {
		return strings.TrimPrefix(before, "a/"), after
	}
	return s, s
}

func closingQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

// unquotePath undoes git's C-style quoting of a path holding special bytes.
func unquotePath(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
	}
	return s
}

// Touched returns the extents whose lines any change touched, whole and in the
// order given. A change on one line of a seven-line block returns the block.
func Touched(extents []format.Extent, changes []Change) []format.Extent {
	ranges := mergeRanges(changes)
	if len(ranges) == 0 {
		return nil
	}
	var out []format.Extent
	for _, x := range extents {
		// The first merged range that ends at or after the extent's first line
		// is the only one that can overlap it: every later range starts after
		// that one ends.
		i := sort.Search(len(ranges), func(i int) bool { return ranges[i].Last >= x.Lines.First })
		if i < len(ranges) && ranges[i].Overlaps(x.Lines) {
			out = append(out, x)
		}
	}
	return out
}

// mergeRanges sorts the changes' line ranges and joins those that overlap or
// abut, so the result is disjoint and ordered.
func mergeRanges(changes []Change) []format.LineRange {
	rs := make([]format.LineRange, 0, len(changes))
	for _, c := range changes {
		rs = append(rs, c.Lines)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].First < rs[j].First })
	var out []format.LineRange
	for _, r := range rs {
		if n := len(out); n > 0 && r.First <= out[n-1].Last+1 {
			if r.Last > out[n-1].Last {
				out[n-1].Last = r.Last
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

// PreImage rebuilds the file as it was before the change, from post, the file
// after it, and the diff's hunks. An added file had no pre-image, and PreImage
// returns nil for it. It returns an error for a binary change, which has no
// lines, and when a hunk's context or added lines are not post's lines.
func (f File) PreImage(post []byte) ([]byte, error) {
	switch {
	case f.Binary:
		return nil, errors.New("a binary change has no lines to rebuild the file from")
	case f.Status == Added:
		return nil, nil
	}
	lines, eol := splitLines(post)
	var pre []string
	next := 1
	for _, h := range f.Hunks {
		start := h.NewStart
		if h.NewCount == 0 {
			start++
		}
		if start < next || start-1 > len(lines) {
			return nil, fmt.Errorf("the hunk at +%d does not fit a %d-line file", h.NewStart, len(lines))
		}
		pre = append(pre, lines[next-1:start-1]...)
		next = start
		var last byte
		oldLacksNewline := false
		for _, l := range h.Lines {
			switch l[0] {
			case ' ', '+':
				if next > len(lines) || lines[next-1] != l[1:] {
					return nil, fmt.Errorf("the hunk at +%d does not match line %d of the file", h.NewStart, next)
				}
				if l[0] == ' ' {
					pre = append(pre, l[1:])
				}
				next++
			case '-':
				pre = append(pre, l[1:])
			case '\\':
				// "\ No newline at end of file" follows the line it describes.
				oldLacksNewline = oldLacksNewline || last == '-' || last == ' '
				continue
			}
			last = l[0]
		}
		if next > len(lines) {
			eol = !oldLacksNewline
		}
	}
	pre = append(pre, lines[next-1:]...)
	out := strings.Join(pre, "\n")
	if eol && len(pre) > 0 {
		out += "\n"
	}
	return []byte(out), nil
}

// splitLines splits content at line feeds, reporting whether the last line
// ends with one.
func splitLines(content []byte) ([]string, bool) {
	if len(content) == 0 {
		return nil, false
	}
	s := string(content)
	eol := strings.HasSuffix(s, "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n"), eol
}

// PreLine maps a post-image line to its number in the pre-image. ok is false
// for a line the change added.
func (f File) PreLine(post int) (int, bool) {
	offset := 0
	for _, h := range f.Hunks {
		newLine := h.NewStart
		if h.NewCount == 0 {
			newLine++
		}
		if post < newLine {
			return post + offset, true
		}
		oldLine := h.OldStart
		if h.OldCount == 0 {
			oldLine++
		}
		for _, l := range h.Lines {
			switch l[0] {
			case ' ':
				if newLine == post {
					return oldLine, true
				}
				newLine++
				oldLine++
			case '+':
				if newLine == post {
					return 0, false
				}
				newLine++
			case '-':
				oldLine++
			}
		}
		offset = oldLine - newLine
	}
	return post + offset, true
}

// Bordered returns the touched extents that a deletion only borders: every
// change that overlaps them is a deletion, and none falls strictly inside.
// These are the blocks Settle can clear.
func Bordered(touched []format.Extent, changes []Change) []format.Extent {
	var out []format.Extent
	for _, x := range touched {
		if onlyBordered(x, changes) {
			out = append(out, x)
		}
	}
	return out
}

func onlyBordered(x format.Extent, changes []Change) bool {
	bordered := false
	for _, c := range changes {
		if !c.Lines.Overlaps(x.Lines) {
			continue
		}
		if !c.Deletion || (x.Lines.First <= c.After && c.After+1 <= x.Lines.Last) {
			return false
		}
		bordered = true
	}
	return bordered
}

// Settle narrows touched with the pre-image. It drops a block a deletion only
// borders when pre, the file before the change, holds a block with the same
// bytes on the lines this block's lines came from: the removed lines were then
// never part of it. Every other block stays, including a bordered one the
// pre-image cannot account for, so Settle can only narrow a scope to blocks the
// change altered and never drops one it did.
func Settle(f File, post []byte, touched []format.Extent, pre []byte, preExtents []format.Extent) []format.Extent {
	var out []format.Extent
	for _, x := range touched {
		if onlyBordered(x, f.Changes) && unchanged(f, post, x, pre, preExtents) {
			continue
		}
		out = append(out, x)
	}
	return out
}

func unchanged(f File, post []byte, x format.Extent, pre []byte, preExtents []format.Extent) bool {
	first, ok := f.PreLine(x.Lines.First)
	if !ok {
		return false
	}
	last, ok := f.PreLine(x.Lines.Last)
	if !ok || last-first != x.Lines.Last-x.Lines.First || x.End > len(post) {
		return false
	}
	body := post[x.Start:x.End]
	for _, p := range preExtents {
		if p.Lines.First == first && p.Lines.Last == last && p.End <= len(pre) && bytes.Equal(pre[p.Start:p.End], body) {
			return true
		}
	}
	return false
}
