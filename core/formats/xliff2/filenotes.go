package xliff2

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// FileNote represents one XLIFF 2.x file-level <note> element to emit on
// the first <file> block. Empty Category and ID are valid — they map to the
// absent attribute on the wire.
type FileNote struct {
	ID       string
	Category string
	Content  string
}

// Kapi extract / merge conventions for file-level notes (AD-017).
const (
	// FileNoteCategoryKapi is the category kapi uses for its own file-level
	// metadata notes. This namespaces our bookkeeping away from
	// translator/reviewer notes.
	FileNoteCategoryKapi = "kapi"

	// FileNoteIDBatchID is the note id used to stamp the extraction batch
	// id on <file>. Together with Category=kapi this identifies a file
	// that kapi extract produced.
	FileNoteIDBatchID = "batch-id"

	// FileNoteIDSourceFile is the note id used to record the source
	// file path (relative to the project root) of an extraction, so merge
	// can fall back to filename matching if the batch id is missing.
	FileNoteIDSourceFile = "source-file"

	// FileNoteIDSourceHash is the note id used to record the SHA-256 hex
	// of the source file at extract time, for stale-segment detection
	// on merge.
	FileNoteIDSourceHash = "source-hash"
)

// BatchIDNote returns a FileNote carrying the kapi extraction batch id.
func BatchIDNote(batchID string) FileNote {
	return FileNote{Category: FileNoteCategoryKapi, ID: FileNoteIDBatchID, Content: batchID}
}

// SourceFileNote returns a FileNote carrying the source file path (relative
// to the project root) for fallback matching on merge.
func SourceFileNote(relPath string) FileNote {
	return FileNote{Category: FileNoteCategoryKapi, ID: FileNoteIDSourceFile, Content: relPath}
}

// SourceHashNote returns a FileNote carrying the SHA-256 hex of the source
// content at extract time for stale-segment detection.
func SourceHashNote(hash string) FileNote {
	return FileNote{Category: FileNoteCategoryKapi, ID: FileNoteIDSourceHash, Content: hash}
}

// LayerPropertyKey returns the layer.Properties key for this note.
func (n FileNote) LayerPropertyKey() string {
	return FileNotePropertyPrefix + n.Category + ":" + n.ID
}

// BatchIDFromLayer returns the kapi extraction batch id stamped on the
// layer by the reader, or "" when none is present. Kapi merge uses this
// to resolve a returning XLIFF back to its extraction manifest.
func BatchIDFromLayer(layer *model.Layer) string {
	return FilePropertyFromLayer(layer, FileNoteCategoryKapi, FileNoteIDBatchID)
}

// FilePropertyFromLayer looks up a single file-level note value by
// category + id. Returns "" when absent.
func FilePropertyFromLayer(layer *model.Layer, category, id string) string {
	if layer == nil || layer.Properties == nil {
		return ""
	}
	return layer.Properties[FileNotePropertyPrefix+category+":"+id]
}

// setFileNoteProperties copies parsed <note> elements into layer.Properties
// under the file-note:<category>:<id> convention so downstream code
// (notably kapi merge) can access extract-time metadata without re-parsing
// the XML. Notes with no category and no id are ignored — they would all
// collide on the same key with no content distinguishing them.
func setFileNoteProperties(layer *model.Layer, notes []xliff2Note) {
	if layer == nil {
		return
	}
	if len(notes) == 0 {
		return
	}
	if layer.Properties == nil {
		layer.Properties = make(map[string]string, len(notes))
	}
	for _, n := range notes {
		content := strings.TrimSpace(n.Content)
		if content == "" {
			continue
		}
		if n.Category == "" && n.ID == "" {
			continue
		}
		key := FileNotePropertyPrefix + n.Category + ":" + n.ID
		layer.Properties[key] = content
	}
}

// mergeFileNotes combines the notes carried through from the input layer
// (via fileNotesFromLayer) with notes stamped explicitly by the caller.
// When both sources provide a note with the same (category, id), the
// explicit note wins — this is the re-extraction case where a stale
// batch id on the layer must be overwritten.
func mergeFileNotes(layerNotes, explicit []FileNote) []FileNote {
	seen := make(map[[2]string]int, len(layerNotes)+len(explicit))
	out := make([]FileNote, 0, len(layerNotes)+len(explicit))
	for _, n := range layerNotes {
		key := [2]string{n.Category, n.ID}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = len(out)
		out = append(out, n)
	}
	for _, n := range explicit {
		key := [2]string{n.Category, n.ID}
		if idx, ok := seen[key]; ok {
			out[idx] = n
			continue
		}
		seen[key] = len(out)
		out = append(out, n)
	}
	return out
}

// fileNotesFromLayer reverses setFileNoteProperties: any layer.Properties
// entries that use the file-note:<category>:<id> convention are collected
// for re-emission by the writer (byte-exact round-trip preservation).
func fileNotesFromLayer(layer *model.Layer) []FileNote {
	if layer == nil || len(layer.Properties) == 0 {
		return nil
	}
	var notes []FileNote
	for key, content := range layer.Properties {
		if !strings.HasPrefix(key, FileNotePropertyPrefix) {
			continue
		}
		rest := strings.TrimPrefix(key, FileNotePropertyPrefix)
		category, id, _ := strings.Cut(rest, ":")
		notes = append(notes, FileNote{Category: category, ID: id, Content: content})
	}
	return notes
}
