// Package diffscope reads a unified diff into the lines each file's change
// touched, and resolves those lines to the blocks whose content sits on them.
//
// A diff speaks in lines of the post-image, and the unit worth checking is the
// block those lines belong to. Parse gives the lines; Touched widens them to
// whole blocks, given where each block sits (format.Extent). Both are pure: no
// file is read and nothing runs git.
package diffscope

import (
	"bufio"
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
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// Parse reads a unified diff, in the form git diff writes it or in the plain
// form diff -u writes. Content lines may end in CRLF. A combined diff (the
// "diff --cc" form of a merge) is refused rather than half-read.
func Parse(diff []byte) ([]File, error) {
	p := &parser{}
	sc := bufio.NewScanner(bytes.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		p.lines = append(p.lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read diff: %w", err)
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
			first := max(deletedAfter, 1)
			p.cur.Changes = append(p.cur.Changes, Change{Lines: format.LineRange{First: first, Last: deletedAfter + 1}, Deletion: true})
		}
		runOpen, plusCount = false, 0
	}

	for oldLeft > 0 || newLeft > 0 {
		if p.i >= len(p.lines) {
			return fmt.Errorf("diff line %d: the hunk ends early, %d old and %d new lines short", headerLine, oldLeft, newLeft)
		}
		line := p.lines[p.i]
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
		p.i++
	}
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
	if i := strings.LastIndex(s, " b/"); i >= 0 {
		return strings.TrimPrefix(s[:i], "a/"), s[i+3:]
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
