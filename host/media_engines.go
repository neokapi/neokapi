package host

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/vision"
)

// The media readers enrich a file through plugin-backed engines: speech
// recognition for audio and video (asr), and OCR for images and video frames
// (vision). A file read while its engine is missing reads as media with no
// text, which is not the same as a file with nothing to translate. The drift
// stamp records which engines were missing (project.MissingReaderEngines), so
// installing one re-reads the file, and the run summary and kapi status name
// the files that wait for one.
func init() {
	project.MissingReaderEngines = missingMediaEngines
}

// missingMediaEngines names the engines rf's reader needs and cannot use now.
func missingMediaEngines(rf project.ResolvedFile) []string {
	var missing []string
	needs := func(engine string, available func(string) bool) {
		if !available("") {
			missing = append(missing, engine)
		}
	}
	switch rf.Format {
	case "audio":
		needs("asr", asr.Available)
	case "video":
		needs("asr", asr.Available)
		needs("vision", vision.Available)
	case "image":
		if imageOCRRequested(rf) {
			needs("vision", vision.Available)
		}
	}
	return missing
}

// imageOCRRequested reports whether rf's reader runs OCR: the default, unless
// its item's format config turns it off.
func imageOCRRequested(rf project.ResolvedFile) bool {
	if rf.Item == nil || rf.Item.Format == nil {
		return true
	}
	ocr, ok := rf.Item.Format.Config["ocr"].(bool)
	return !ok || ocr
}

// mediaEngineStamp is the availability of every media engine, for keys that
// must change when one is installed or removed (the parse cache).
func mediaEngineStamp() string {
	return fmt.Sprintf("asr=%t,vision=%t", asr.Available(""), vision.Available(""))
}

// UnreadMedia is a source file read without an engine its content needs.
type UnreadMedia struct {
	File    string   `json:"file"`
	Engines []string `json:"engines"`
}

// unreadMedia lists the project's files the store last read without an
// engine they need, from the drift stamps, in path order.
func unreadMedia(ctx context.Context, db *projectdb.DB) []UnreadMedia {
	if db == nil {
		return nil
	}
	var out []UnreadMedia
	for rel, stamp := range db.LoadSourceStamps(ctx) {
		if len(stamp.MissingEngines) > 0 {
			out = append(out, UnreadMedia{File: rel, Engines: stamp.MissingEngines})
		}
	}
	slices.SortFunc(out, func(a, b UnreadMedia) int { return strings.Compare(a.File, b.File) })
	return out
}

// writeUnreadMedia says which files wait for an engine and how to install it.
func writeUnreadMedia(w io.Writer, files []UnreadMedia) {
	if len(files) == 0 {
		return
	}
	var engines []string
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.File)
		for _, e := range f.Engines {
			if !slices.Contains(engines, e) {
				engines = append(engines, e)
			}
		}
	}
	slices.Sort(engines)
	fmt.Fprintf(w, "%d file(s) read without the engine their content needs (%s): %s. Their speech or image text is not translated. "+
		"Install the plugin (kapi plugins install %s) and the next run reads them again.\n",
		len(files), strings.Join(engines, ", "), strings.Join(names, ", "), strings.Join(engines, " "))
}
