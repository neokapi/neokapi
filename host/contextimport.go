package host

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/workspace"
)

// Reading a `.kapi/` layout into a project's store.
//
// `kapi context import` is the only reader of a context file in a checkout:
// the terms bundle, the content-memory bundles, the voice profiles and the
// decision record. A person runs it, and what it reads is in force from then
// on because the store is what every other surface answers from.
//
// The layout it reads is the project's own, or one a caller names: a donor
// checkout, a directory a colleague sent, or a directory of context files that
// ships beside a sample. Either way the files are found where the layout keeps
// them; the recipe names none of them.
//
// A recipe binds a voice by name. An import that brings voice profiles into a
// project whose recipe binds none at the default point binds the project's one
// voice in kapi.yaml, so what was just read is also what governs.

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
	// Unrecorded reports a project whose recipe carries neither an id nor a
	// name. Its context has nowhere to be logged, so the import read the files
	// and left no history behind.
	Unrecorded bool `json:"unrecorded,omitempty"`
	// Voice reports what the import did about the recipe's voice binding. nil
	// when the recipe already binds a voice at the default point, or the
	// layout carries none.
	Voice *ImportedVoiceBinding `json:"voice,omitempty"`
}

// ImportedVoiceBinding is the voice binding an import settled, or the choice
// it left to the person.
type ImportedVoiceBinding struct {
	// Recipe is the recipe file the binding is written in, as a reader names it.
	Recipe string `json:"recipe"`
	// Bound is the profile the import bound under `defaults.voice`, empty when
	// it bound none.
	Bound *VoiceProfileRef `json:"bound,omitempty"`
	// Candidates are the profiles the layout carries when there is more than
	// one to choose from and the import bound none of them.
	Candidates []VoiceProfileRef `json:"candidates,omitempty"`
	// FileBinding is the path a recipe's `defaults.voice` names in a form that
	// names a file, which the import leaves for the person to replace.
	FileBinding string `json:"fileBinding,omitempty"`
}

// VoiceProfileRef names a voice profile in the project's store.
type VoiceProfileRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// label renders the profile as a person reads it: its name, else its id.
func (r VoiceProfileRef) label() string {
	if r.Name != "" {
		return r.Name
	}
	return r.ID
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
	if r.Unrecorded {
		if _, err := fmt.Fprintf(w, "Give the recipe a `name:` for this to appear in `kapi context log`.\n"); err != nil {
			return err
		}
	}
	return r.Voice.formatText(w)
}

// formatText renders what the import did about the voice binding.
func (v *ImportedVoiceBinding) formatText(w io.Writer) error {
	switch {
	case v == nil:
		return nil
	case v.Bound != nil:
		_, err := fmt.Fprintf(w, "Bound voice %s in %s (defaults.voice.profile: %s).\n",
			v.Bound.label(), v.Recipe, v.Bound.ID)
		return err
	case v.FileBinding != "":
		_, err := fmt.Fprintf(w, "%s binds defaults.voice to the file %s, and a recipe binds a voice by name. "+
			"Replace that binding with the profile's id from `kapi voice profiles`:\n  voice:\n    profile: <id>\n",
			v.Recipe, v.FileBinding)
		return err
	case len(v.Candidates) > 0:
		names := make([]string, len(v.Candidates))
		for i, c := range v.Candidates {
			names[i] = c.ID
			if c.Name != "" && c.Name != c.ID {
				names[i] += " (" + c.Name + ")"
			}
		}
		_, err := fmt.Fprintf(w, "%s binds no voice, and this import read %d: %s. "+
			"Bind the one that governs by adding it under defaults: in %s:\n  voice:\n    profile: %s\n",
			v.Recipe, len(v.Candidates), strings.Join(names, ", "), v.Recipe, v.Candidates[0].ID)
		return err
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

// ImportProjectContext reads a `.kapi/` layout into the project's store, and
// records in the project's context history what it read.
//
// Running it twice changes nothing: every importer upserts by the identity the
// file carries, so the second pass finds the store already holding what the
// file says. Identities are preserved throughout — a concept keeps its id, an
// entry keeps its id and its origins, a voice profile keeps the id it is stored
// under, and a decision keeps the unit it is about. A source whose bytes have
// not moved since this checkout last read it is skipped outright, and `--force`
// reads it again.
//
// projectPath is the recipe or the project root; both resolve the way `-p`
// does.
func (a *App) ImportProjectContext(ctx context.Context, projectPath string, req ContextImportRequest) (ContextImport, error) {
	var res ContextImport

	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	// The recipe is not loaded: an import reads files the recipe never names,
	// and a recipe that fails to load for a binding it carries is exactly the
	// one whose context a person is bringing in to fix it.
	from, _, err := resolveContextLayout(layout, req.Dir)
	if err != nil {
		return res, err
	}
	res.Dir = reportedPath(layout.Root, from.StateDir)

	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return res, err
	}

	writer, err := a.Projector(ctx, layout.Root)
	if err != nil {
		return res, err
	}

	sources, err := committedContextSources(from)
	if err != nil {
		return res, err
	}

	scribe, err := a.importScribe(ctx, layout.RecipePath)
	if err != nil {
		return res, err
	}
	res.Unrecorded = !scribe.records()
	checkout := normalizedCheckout(layout.Root)
	identity, _ := recipeIdentity(layout.RecipePath)
	projectKey := workspace.ProjectKey(identity)
	stamps, err := a.loadImportStamps(ctx, projectKey, checkout)
	if err != nil {
		return res, err
	}
	stamped := false

	for _, src := range sources {
		if !storeHolds(db, src.kind) {
			// A build with no file-backed store for this subsystem (the browser
			// build) has nothing to read into.
			res.Unchanged++
			continue
		}
		digest, derr := fileDigest(src.path)
		if derr != nil {
			return res, derr
		}
		key := importStampKey(projectKey, checkout, src.rel)
		if !req.Force && stamps[key] == digest {
			res.Unchanged++
			continue
		}
		n, rerr := a.readContextSource(ctx, db, writer, from.Root, src)
		if rerr != nil {
			return res, rerr
		}
		switch src.kind {
		case sourceKindTerms:
			res.Concepts += n
		case sourceKindMemory:
			res.Entries += n
		case sourceKindVoice:
			res.VoiceProfiles++
		}
		stamps[key], stamped = digest, true
		if err := scribe.read(ctx, src, n, digest); err != nil {
			return res, err
		}
	}
	if stamped {
		if err := a.saveImportStamps(ctx, stamps); err != nil {
			return res, err
		}
	}
	if res.Entries > 0 {
		if tm := projector.MemoryView(db); tm != nil {
			a.RebuildMemorySearchIndexes(ctx, tm)
		}
	}

	voice, err := a.bindImportedVoice(ctx, db, layout, from, sources)
	if err != nil {
		return res, err
	}
	res.Voice = voice

	recordDir := from.Export().UnitStateDir()
	n, err := importDecisionRecord(ctx, db.Work(), recordDir)
	if err != nil {
		return res, err
	}
	res.Decisions = n
	if n > 0 {
		if err := scribe.readRecord(ctx, relSlash(from.Root, recordDir), n); err != nil {
			return res, err
		}
	}
	return res, nil
}

// bindImportedVoice binds the voice an import brought when the recipe binds none
// at the default point, and reports what it did.
//
// The profile it binds is the layout's own: the `voice.yaml` at the top of the
// layout, which is the project's by convention. A layout with none there and
// exactly one voice profile anywhere binds that one. With several and none at
// the top, nothing is bound and the candidates are reported, because which of
// them governs the whole project is the person's decision.
//
// A recipe that already binds a voice at the default point is left alone,
// including one whose binding names a file: that binding is reported for the
// person to replace.
func (a *App) bindImportedVoice(ctx context.Context, db *projectdb.DB, layout, from project.Layout, sources []contextSource) (*ImportedVoiceBinding, error) {
	var voices []contextSource
	for _, src := range sources {
		if src.kind == sourceKindVoice {
			voices = append(voices, src)
		}
	}
	store := db.Voice()
	if len(voices) == 0 || store == nil {
		return nil, nil
	}
	recipeName := reportedPath(layout.Root, layout.RecipePath)
	declared, err := project.DeclaredDefaultVoice(layout.RecipePath)
	if err != nil {
		return nil, err
	}
	if declared != nil {
		if file := declared.NamedFile(); file != "" {
			return &ImportedVoiceBinding{Recipe: recipeName, FileBinding: file}, nil
		}
		return nil, nil
	}

	bindings := loadVoiceBindings(ctx, db)
	ref := func(src contextSource) (VoiceProfileRef, bool) {
		id := bindings[src.rel]
		if id == "" {
			return VoiceProfileRef{}, false
		}
		r := VoiceProfileRef{ID: id}
		if p, gerr := store.GetProfile(ctx, id); gerr == nil {
			r.Name = p.Name
		}
		return r, true
	}

	top := filepath.Join(from.StateDir, VoiceConventionalName)
	var candidates []VoiceProfileRef
	seen := map[string]bool{}
	var chosen *VoiceProfileRef
	for _, src := range voices {
		r, ok := ref(src)
		if !ok || seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		candidates = append(candidates, r)
		if src.path == top {
			chosen = &r
		}
	}
	if chosen == nil && len(candidates) == 1 {
		chosen = &candidates[0]
	}
	if chosen == nil {
		if len(candidates) == 0 {
			return nil, nil
		}
		return &ImportedVoiceBinding{Recipe: recipeName, Candidates: candidates}, nil
	}
	if err := project.BindVoice(layout.RecipePath, "", chosen.ID); err != nil {
		return nil, fmt.Errorf("bind voice %s in %s: %w", chosen.ID, recipeName, err)
	}
	return &ImportedVoiceBinding{Recipe: recipeName, Bound: chosen}, nil
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

// importDecisionRecord reads a directory of committed decision shards into a
// project's ledger and writes the project's own record out again.
//
// Each row is recorded with the origin of a record read in, and recording is
// addressed by content, so a row the ledger already holds costs nothing and a
// second import reads the same layout to the same store. The project's own
// shards are read first, so a record that moved under the handle cannot take
// the rows out again on the way through.
//
// A handle that has no checkout (core/state.OpenLedger, which a whole-workspace
// restore opens) reads and writes no committed record, so for it the pass is
// the ledger writes alone.
func importDecisionRecord(ctx context.Context, st *state.WorkStore, dir string) (int, error) {
	units, err := state.ReadCommitted(dir)
	if err != nil {
		return 0, err
	}
	if len(units) == 0 {
		return 0, nil
	}
	if st == nil {
		return 0, projectdb.ErrNoStore
	}
	if err := st.Import(ctx); err != nil {
		return 0, err
	}
	first, last, err := recordOrderKeepingTheView(ctx, st, units)
	if err != nil {
		return 0, err
	}
	for _, u := range append(first, last...) {
		if err := st.Record(ctx, u); err != nil {
			return 0, err
		}
	}
	if err := st.PersistRecords(ctx); err != nil {
		return 0, err
	}
	return len(units), nil
}

// recordOrderKeepingTheView splits a record into the rows to write first and
// the rows to write last.
//
// Recording an entry points this checkout's view at its pairing, and a context
// bundle carries the project's whole ledger: a unit two branches answer
// differently arrives as a line per pairing, and the last line written would
// decide what this checkout holds. The rows whose pairing the checkout already
// holds go last, so the view comes back to where it was and `kapi commit` here
// still writes this branch's answers. A checkout holding none of them, which is
// what a recovery into a fresh clone is, takes the record's own order.
func recordOrderKeepingTheView(ctx context.Context, st *state.WorkStore, units []state.UnitState) (first, last []state.UnitState, err error) {
	held, err := st.All(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(held) == 0 {
		return units, nil, nil
	}
	inView := make(map[state.Pairing]bool, len(held))
	for _, u := range held {
		inView[u.Pairing()] = true
	}
	for _, u := range units {
		if inView[u.Pairing()] {
			last = append(last, u)
			continue
		}
		first = append(first, u)
	}
	return first, last, nil
}
