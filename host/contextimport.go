package host

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
)

// Reading a `.kapi/` layout into a project's store.
//
// `kapi up` already does this on every run: it compiles the committed terms
// bundle, the content-memory bundles and the voice profiles, skipping whatever
// is unchanged. `kapi context import` is the same pass with a name, so a
// project can be told to read its context now rather than as a side effect of
// converging, and so a layout that belongs to another checkout can be read at
// all. Both reach compileContextSources, and each committed file therefore has
// exactly one reader.
//
// The two entry points differ in three places, and only there:
//
//   - The compile stamps are keyed by project-relative path, so a pass over a
//     directory elsewhere records none and compiles everything.
//   - The committed-record absorb reads the project's own target documents, so
//     it runs only for a pass over the project's own layout.
//   - A foreign layout's decision record is read explicitly; the project's own
//     is already what the working set is built from.

// ContextImportRequest names the layout to read.
type ContextImportRequest struct {
	// Dir is the `.kapi/` directory to read, or a project root holding one.
	// Empty means the project's own.
	Dir string
	// Force compiles every source, including one whose bytes have not moved
	// since the last compile.
	Force bool
}

// ContextImport reports what an import read.
type ContextImport struct {
	// Dir is the layout that was read, as the caller named it.
	Dir string `json:"dir"`
	// Concepts, Entries and VoiceProfiles count what the sources carried.
	Concepts      int `json:"concepts,omitempty"`
	Entries       int `json:"entries,omitempty"`
	VoiceProfiles int `json:"voiceProfiles,omitempty"`
	// Decisions counts the rows read from a layout's decision record.
	Decisions int `json:"decisions,omitempty"`
	// Unchanged counts the sources already in the store at their current
	// bytes, which is what a second run of the same import reports.
	Unchanged int `json:"unchanged,omitempty"`
}

// Read reports whether the import put anything into the store.
func (r ContextImport) Read() bool {
	return r.Concepts > 0 || r.Entries > 0 || r.VoiceProfiles > 0 || r.Decisions > 0
}

// FormatText renders the import for a reader.
func (r ContextImport) FormatText(w io.Writer) error {
	if !r.Read() {
		if r.Unchanged > 0 {
			_, err := fmt.Fprintf(w, "Nothing to read: %s already in the store at these bytes.\n",
				pluralUnit(r.Unchanged, "source", "sources"))
			return err
		}
		_, err := fmt.Fprintf(w, "Nothing to read: %s holds no context.\n", r.Dir)
		return err
	}
	if _, err := fmt.Fprintf(w, "Read %s into the project store:\n", r.Dir); err != nil {
		return err
	}
	for _, line := range []struct {
		n    int
		one  string
		many string
	}{
		{r.Concepts, "concept", "concepts"},
		{r.Entries, "content-memory entry", "content-memory entries"},
		{r.VoiceProfiles, "voice profile", "voice profiles"},
		{r.Decisions, "recorded decision", "recorded decisions"},
	} {
		if line.n == 0 {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s\n", pluralUnit(line.n, line.one, line.many)); err != nil {
			return err
		}
	}
	if r.Unchanged > 0 {
		if _, err := fmt.Fprintf(w, "  %s already at these bytes\n",
			pluralUnit(r.Unchanged, "source", "sources")); err != nil {
			return err
		}
	}
	return nil
}

// pluralUnit renders a count with the right noun.
func pluralUnit(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// ImportProjectContext reads a `.kapi/` layout into the project's store.
//
// Running it twice changes nothing: every importer upserts by the identity the
// file carries, so the second pass finds the store already holding what the
// file says. Identities are preserved throughout — a concept keeps its id, an
// entry keeps its id and its origins, a voice profile keeps the id it is stored
// under, and a decision keeps the unit it is about.
//
// projectPath is the recipe or the project root; both resolve the way `-p`
// does.
func (a *App) ImportProjectContext(ctx context.Context, projectPath string, req ContextImportRequest) (ContextImport, error) {
	var res ContextImport

	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return res, fmt.Errorf("load project: %w", err)
	}
	from, own, err := resolveContextLayout(layout, req.Dir)
	if err != nil {
		return res, err
	}
	res.Dir = reportedPath(layout.Root, from.StateDir)

	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return res, err
	}

	opts := contextCompileOptions{layout: from, force: req.Force, stamp: own, absorb: own}
	if own {
		opts.proj = proj
		opts.projectPath = projectPath
	}
	seeded, err := a.compileContextSources(ctx, db, opts)
	if err != nil {
		return res, err
	}
	res.Concepts = seeded.Concepts
	res.Entries = seeded.Entries
	res.VoiceProfiles = seeded.VoiceFiles
	res.Unchanged = seeded.Skipped

	if own {
		// The working set is built from this layout's record when the store
		// opens, so there is nothing further to read.
		return res, nil
	}
	n, err := a.importDecisionRecord(ctx, db, from.UnitStateDir())
	if err != nil {
		return res, err
	}
	res.Decisions = n
	return res, nil
}

// resolveContextLayout turns the directory a caller named into the layout to
// read, and reports whether it is the project's own.
//
// The name may be the `.kapi/` directory itself or the project root holding
// one, because both are what a person has in hand: the path they would `ls`,
// and the path git gave them.
func resolveContextLayout(own project.Layout, dir string) (project.Layout, bool, error) {
	if dir == "" {
		return own, true, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return project.Layout{}, false, fmt.Errorf("resolve %s: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return project.Layout{}, false, fmt.Errorf("read %s: %w", dir, err)
	}
	if !info.IsDir() {
		return project.Layout{}, false, fmt.Errorf("%s is not a directory: a context import reads a %s layout", dir, project.StateDirName)
	}
	if nested := filepath.Join(abs, project.StateDirName); filepath.Base(abs) != project.StateDirName {
		if ni, nerr := os.Stat(nested); nerr == nil && ni.IsDir() {
			abs = nested
		}
	}
	layout := project.Layout{Root: filepath.Dir(abs), StateDir: abs}
	layout.RecipePath = filepath.Join(layout.Root, project.RecipeFileName)
	return layout, abs == own.StateDir, nil
}

// reportedPath renders a path the way a reader recognises it: relative to the
// project when it sits inside, absolute otherwise. A home directory never
// reaches a report of the project's own layout this way.
func reportedPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || filepath.IsAbs(rel) ||
		(len(rel) > 2 && rel[:3] == ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.ToSlash(rel)
}

// importDecisionRecord reads a layout's committed decision record into the
// project's working set and makes it durable.
//
// Every row is recorded rather than staged: these decisions were committed
// where they came from, so they are part of the record rather than something
// waiting for a review. The set is agreed with the record on disk before the
// rows go in, so a record that moved under the handle cannot take them out
// again on the way through.
func (a *App) importDecisionRecord(ctx context.Context, db *projectdb.DB, dir string) (int, error) {
	units, err := state.ReadCommitted(dir)
	if err != nil {
		return 0, err
	}
	if len(units) == 0 {
		return 0, nil
	}
	st := db.Work()
	if st == nil {
		return 0, projectdb.ErrNoStore
	}
	if err := st.SyncWithCommitted(ctx); err != nil {
		return 0, err
	}
	for _, u := range units {
		if err := st.Record(ctx, u); err != nil {
			return 0, err
		}
	}
	if err := st.PersistRecords(ctx); err != nil {
		return 0, err
	}
	return len(units), nil
}
