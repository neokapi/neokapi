package host

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

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
// must change when one is installed or removed (the parse cache). It is
// worked out once per process, from the plugins found then, without the
// notes a read prints; RefreshMediaEngines works it out again after a plugin
// is installed or removed in a long-lived process (Kapi Desktop).
func mediaEngineStamp() string {
	engineStampMu.Lock()
	defer engineStampMu.Unlock()
	if engineStamp == "" {
		_, _, asrErr := findASRPlugin()
		_, visionErr := findVisionPlugin()
		engineStamp = fmt.Sprintf("asr=%t,vision=%t", asrErr == nil, visionErr == nil)
	}
	return engineStamp
}

var (
	engineStampMu sync.Mutex
	engineStamp   string
)

// RefreshMediaEngines forgets the run's media-plugin discovery and the engine
// stamp, so the next read and the next parse-cache key reflect the plugins
// installed now. Kapi Desktop calls it wherever its plugin set can change.
func RefreshMediaEngines() {
	forgetMediaPlugins()
	engineStampMu.Lock()
	engineStamp = ""
	engineStampMu.Unlock()
}

// UnreadMedia is a source file read without an engine its content needs.
type UnreadMedia struct {
	File string `json:"file"`
	// Collection is the recipe collection the file belongs to, when it still
	// resolves to one.
	Collection string   `json:"collection,omitempty"`
	Engines    []string `json:"engines"`
}

// unreadMedia lists the project's files the store last read without an
// engine they need, from the drift stamps, in path order, each with its
// collection from collectionOf (project-relative path to collection name).
func unreadMedia(ctx context.Context, db *projectdb.DB, collectionOf map[string]string) []UnreadMedia {
	if db == nil {
		return nil
	}
	var out []UnreadMedia
	for rel, stamp := range db.LoadSourceStamps(ctx) {
		if len(stamp.MissingEngines) > 0 {
			out = append(out, UnreadMedia{File: rel, Collection: collectionOf[rel], Engines: stamp.MissingEngines})
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
	fmt.Fprintf(w, "%d file(s) read without the engine their content needs (%s): %s. No target is written for them. ",
		len(files), strings.Join(engines, ", "), strings.Join(names, ", "))
	var install []string
	for _, e := range engines {
		if reason, refused := mediaPluginRefusal(e); refused {
			fmt.Fprintf(w, "The %q plugin is installed but not loaded: %s. ", e, reason)
			continue
		}
		install = append(install, e)
	}
	if len(install) > 0 {
		fmt.Fprintf(w, "Install the plugin (kapi plugins install %s). ", strings.Join(install, " "))
	}
	fmt.Fprintln(w, "The next run reads them again.")
}

// collectionsOf maps each of the project's source files (project-relative) to
// the collection that claims it, or returns nil when the content does not
// resolve.
func (a *App) collectionsOf(proj *project.KapiProject, projectPath string) map[string]string {
	files, err := project.NewProjectContext(proj, projectPath).ResolveContent(a.FormatReg)
	if err != nil {
		return nil
	}
	out := make(map[string]string, len(files))
	for _, f := range files {
		out[f.Relative] = f.Collection
	}
	return out
}

// unreadMediaRows are the coverage rows for the collections whose files wait
// for an engine, one per target language the grid shows: such a collection has
// no blocks to measure, and without a row it would be missing from the grid.
func unreadMediaRows(files []UnreadMedia, locales []LocaleCoverage) [][2]string {
	var langs []string
	for _, lc := range locales {
		if !slices.Contains(langs, lc.Locale) {
			langs = append(langs, lc.Locale)
		}
	}
	var collections []string
	engines := map[string][]string{}
	for _, f := range files {
		if f.Collection == "" {
			continue
		}
		if _, seen := engines[f.Collection]; !seen {
			collections = append(collections, f.Collection)
		}
		for _, e := range f.Engines {
			if !slices.Contains(engines[f.Collection], e) {
				engines[f.Collection] = append(engines[f.Collection], e)
			}
		}
	}
	var rows [][2]string
	for _, lang := range langs {
		for _, c := range collections {
			rows = append(rows, [2]string{lang + "/" + c, "waits for " + strings.Join(engines[c], ", ")})
		}
	}
	return rows
}
