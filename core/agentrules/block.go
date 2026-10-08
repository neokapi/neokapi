// Package agentrules edits the section kapi owns inside an agent rules file.
//
// An agent rules file is the AGENTS.md or CLAUDE.md a coding agent loads from
// the folder it works in, without a tool call. kapi writes the rules that hold
// in that folder into one delimited section of the file and leaves every other
// line to the person who wrote it. This package finds, replaces and removes
// that section; what goes into it is decided by the host, which reads the
// project's context.
package agentrules

import (
	"bytes"
	"errors"
)

const (
	// Start opens kapi's section. Only the prefix is matched, so the hint
	// after it can change without orphaning a section an earlier version
	// wrote.
	Start = "<!-- kapi:rules"
	// StartLine is the opening line as written.
	StartLine = Start + " (written by kapi from this project's context; edit outside this section) -->"
	// End closes the section.
	End = "<!-- /kapi:rules -->"

	// legacyStart and legacyEnd delimit the voice pointer earlier versions of
	// kapi wrote into a project's root file. The rules section takes its
	// place.
	legacyStart = "<!-- kapi:voice"
	legacyEnd   = "<!-- /kapi:voice -->"
)

// Names are the rules files kapi writes into a folder: AGENTS.md, which most
// agents read, and CLAUDE.md, which Claude Code reads.
var Names = []string{"AGENTS.md", "CLAUDE.md"}

// ErrUnterminated reports a file that opens kapi's section and never closes
// it. The section cannot be replaced without knowing where the person's own
// text resumes, so the file is left alone.
var ErrUnterminated = errors.New("the kapi:rules section has no end marker")

// Find locates kapi's section in doc: the byte range from the start of the
// opening marker's line through the end marker's line, trailing newline
// included. ok is false when doc holds no section.
func Find(doc []byte) (start, end int, ok bool, err error) {
	return find(doc, Start, End)
}

func find(doc []byte, open, closing string) (start, end int, ok bool, err error) {
	i := bytes.Index(doc, []byte(open))
	if i < 0 {
		return 0, 0, false, nil
	}
	start = bytes.LastIndexByte(doc[:i], '\n') + 1
	j := bytes.Index(doc[i:], []byte(closing))
	if j < 0 {
		return 0, 0, false, ErrUnterminated
	}
	end = i + j + len(closing)
	if end < len(doc) && doc[end] == '\n' {
		end++
	}
	return start, end, true, nil
}

// Upsert returns doc with section in place of kapi's section. A file holding
// none takes it where an earlier version's voice pointer stood, or after a
// blank line at the end. An empty doc becomes the section alone. section is
// the whole section, markers included, ending in a newline.
func Upsert(doc []byte, section string) ([]byte, error) {
	start, end, ok, err := Find(doc)
	if err != nil {
		return nil, err
	}
	if !ok {
		start, end, ok, err = find(doc, legacyStart, legacyEnd)
		if err != nil {
			return nil, err
		}
	}
	if ok {
		out := make([]byte, 0, len(doc)-(end-start)+len(section))
		out = append(out, doc[:start]...)
		out = append(out, section...)
		out = append(out, doc[end:]...)
		return out, nil
	}
	out := make([]byte, 0, len(doc)+len(section)+2)
	out = append(out, doc...)
	if len(out) > 0 {
		if out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		if !bytes.HasSuffix(out, []byte("\n\n")) {
			out = append(out, '\n')
		}
	}
	out = append(out, section...)
	return out, nil
}

// Remove returns doc without kapi's section, or the voice pointer an earlier
// version wrote, and whether there was one to remove. The blank line that
// separated the section from the text before it goes with it.
func Remove(doc []byte) ([]byte, bool, error) {
	removed := false
	for _, m := range [][2]string{{Start, End}, {legacyStart, legacyEnd}} {
		start, end, ok, err := find(doc, m[0], m[1])
		if err != nil {
			return nil, false, err
		}
		if !ok {
			continue
		}
		head := doc[:start]
		if bytes.HasSuffix(head, []byte("\n\n")) {
			head = head[:len(head)-1]
		}
		out := make([]byte, 0, len(head)+len(doc)-end)
		out = append(out, head...)
		out = append(out, doc[end:]...)
		doc, removed = out, true
	}
	return doc, removed, nil
}

// Holds reports whether doc carries kapi's section or an earlier version's
// voice pointer.
func Holds(doc []byte) bool {
	return bytes.Contains(doc, []byte(Start)) || bytes.Contains(doc, []byte(legacyStart))
}

// Empty reports whether doc holds nothing but white space: what is left of a
// file kapi created once its section is removed.
func Empty(doc []byte) bool { return len(bytes.TrimSpace(doc)) == 0 }

// ImportsAgents reports whether a CLAUDE.md pulls AGENTS.md in with an
// `@AGENTS.md` import line. Such a file reaches the section through the
// import, so kapi writes the section into AGENTS.md alone.
func ImportsAgents(doc []byte) bool {
	for line := range bytes.SplitSeq(doc, []byte("\n")) {
		if string(bytes.TrimSpace(line)) == "@AGENTS.md" {
			return true
		}
	}
	return false
}
