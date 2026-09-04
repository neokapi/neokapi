package profile

import (
	"bytes"
	"errors"
	"strings"
)

// VoicePointer is what the pointer section says about a project's voice: the
// section tells an assistant standing in the project that its wording is
// governed and which command retrieves the guidance. It carries nothing about
// how to write. A pointer that repeated the guide would go stale the moment
// the profile changed, and the guide itself is one command away.
type VoicePointer struct {
	// Name is the voice profile's name, as `kapi voice guide` reports it.
	// Empty when the project binds a voice the pointer cannot name yet (a
	// profile file the recipe points at but nobody has written).
	Name string
	// PerFile is true when the recipe declares profiles, so parts of the tree
	// carry a voice of their own and the retrieval command takes the file.
	PerFile bool
}

const (
	// VoicePointerStart opens the section. Only the prefix is matched, so the
	// hint after it can change without orphaning a section an earlier version
	// wrote.
	VoicePointerStart = "<!-- kapi:voice"
	// VoicePointerStartLine is the opening line as written.
	VoicePointerStartLine = VoicePointerStart + " (managed by kapi; refreshed by 'kapi voice pointer') -->"
	// VoicePointerEnd closes the section.
	VoicePointerEnd = "<!-- /kapi:voice -->"
)

// ErrVoicePointerUnterminated reports a document that opens the pointer
// section and never closes it. The section cannot be replaced without knowing
// where the hand-written content resumes, so the document is left alone.
var ErrVoicePointerUnterminated = errors.New("voice pointer section has no end marker")

// RenderVoicePointer renders the section as it is written into the project's
// assistant file, markers included, ending in a newline.
func RenderVoicePointer(p VoicePointer) string {
	var b strings.Builder
	b.WriteString(VoicePointerStartLine)
	b.WriteString("\n## Voice\n\n")
	b.WriteString("This project's voice")
	if name := strings.TrimSpace(p.Name); name != "" {
		b.WriteString(", ")
		b.WriteString(name)
		b.WriteString(",")
	}
	b.WriteString(" is held by kapi and applies to any prose written here.")
	if p.PerFile {
		b.WriteString(" Some collections carry a voice of their own, so retrieve what is in force before writing, with `kapi voice guide <path>` for the file you are writing.")
	} else {
		b.WriteString(" Retrieve what is in force before writing, with `kapi voice guide`.")
	}
	b.WriteString("\n")
	b.WriteString(VoicePointerEnd)
	b.WriteString("\n")
	return b.String()
}

// FindVoicePointer locates the pointer section in doc: the byte range from
// the start of the opening marker's line through the end marker's line,
// trailing newline included. ok is false when doc holds no section.
func FindVoicePointer(doc []byte) (start, end int, ok bool, err error) {
	i := bytes.Index(doc, []byte(VoicePointerStart))
	if i < 0 {
		return 0, 0, false, nil
	}
	// Back up to the start of the marker's line so the replacement keeps the
	// column the marker was written at (which is always zero).
	start = bytes.LastIndexByte(doc[:i], '\n') + 1
	j := bytes.Index(doc[i:], []byte(VoicePointerEnd))
	if j < 0 {
		return 0, 0, false, ErrVoicePointerUnterminated
	}
	end = i + j + len(VoicePointerEnd)
	if end < len(doc) && doc[end] == '\n' {
		end++
	}
	return start, end, true, nil
}

// UpsertVoicePointer returns doc with section in place of its pointer
// section, or appended after a blank line when doc has none. An empty doc
// becomes the section alone. section is what RenderVoicePointer returns.
func UpsertVoicePointer(doc []byte, section string) ([]byte, error) {
	start, end, ok, err := FindVoicePointer(doc)
	if err != nil {
		return nil, err
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

// RemoveVoicePointer returns doc without its pointer section, and whether one
// was there to remove. The blank line that separated the section from the
// content before it goes with it, so a removal leaves the file as it was.
func RemoveVoicePointer(doc []byte) ([]byte, bool, error) {
	start, end, ok, err := FindVoicePointer(doc)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return doc, false, nil
	}
	head := doc[:start]
	if bytes.HasSuffix(head, []byte("\n\n")) {
		head = head[:len(head)-1]
	}
	out := make([]byte, 0, len(head)+len(doc)-end)
	out = append(out, head...)
	out = append(out, doc[end:]...)
	return out, true, nil
}
