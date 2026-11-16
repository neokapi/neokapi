package contextop

import (
	"regexp"
	"strconv"
	"strings"
)

// The kinds of context file an import reads.
const (
	ImportTerms     = "terms"
	ImportMemory    = "memory"
	ImportVoice     = "voice"
	ImportDecisions = "decisions"
)

// ImportSource is the context file an import operation read.
type ImportSource struct {
	// Source is terms, memory, voice or decisions.
	Source string `json:"source"`
	// Path is the file's project-relative path, or the directory of a
	// decision record.
	Path string `json:"path,omitempty"`
	// Count is how many items the file carried: concepts, content-memory
	// entries, decisions, or one voice profile.
	Count int `json:"count,omitempty"`
}

// importText reads the subject text of an import operation recorded without
// an ImportSource: "terms bundle <path>, 61 concepts", "content-memory bundle
// <path>, 688 entries" and "decision record <path>, 13205 decisions".
var importText = regexp.MustCompile(`^(terms bundle|content-memory bundle|decision record) (.+), ([0-9]+) [a-z-]+$`)

// importTextSource maps the leading words of an import's subject text to the
// kind of source it read.
var importTextSource = map[string]string{
	"terms bundle":          ImportTerms,
	"content-memory bundle": ImportMemory,
	"decision record":       ImportDecisions,
}

// ImportOf is the context file an import operation read, from its
// ImportSource or, for an operation recorded without one, from its subject
// text. ok is false for any other operation.
func (r Record) ImportOf() (ImportSource, bool) {
	if r.Kind != KindImport {
		return ImportSource{}, false
	}
	if r.Import != nil {
		return *r.Import, true
	}
	text := r.Subject.Text
	if m := importText.FindStringSubmatch(text); m != nil {
		n, _ := strconv.Atoi(m[3])
		return ImportSource{Source: importTextSource[m[1]], Path: m[2], Count: n}, true
	}
	if path, ok := strings.CutPrefix(text, "voice profile "); ok {
		return ImportSource{Source: ImportVoice, Path: path, Count: 1}, true
	}
	for _, e := range r.Evidence {
		if e.Path != "" {
			return ImportSource{Path: e.Path}, true
		}
	}
	return ImportSource{}, true
}

// VoiceWrite reports an edit operation that records a whole voice-profile
// write: one carrying a VoiceEdit, or one recorded without it, whose note
// names the profile.
func (r Record) VoiceWrite() bool {
	if r.Voice != nil {
		return true
	}
	return r.Kind == KindEdit && r.Subject.Kind == SubjectNote &&
		strings.HasPrefix(r.Subject.Text, "voice profile ")
}

// Bulk reports an operation that records a whole source at once: an import
// of a context file, or a whole voice-profile write. Such an operation states
// no rule of its own to keep or reject; a person reviews the items it brought
// in, one by one.
func (r Record) Bulk() bool {
	return r.Kind == KindImport || r.VoiceWrite()
}
