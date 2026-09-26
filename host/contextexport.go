package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// A project's context in one file.
//
// A snapshot is files a project commits and reviews. A bundle is the same
// context as a single archive: one thing to copy to another machine, hand to a
// colleague, or keep as the backup a store that has become authoritative needs.
// It is the context package profile of the `.kpz` container (kpz.KindContext),
// so it inherits the container's manifest, its per-member checksums and its
// Merkle root hash, and it carries no blocks and no source documents.
//
// Everything travels in its native lossless form: the terms bundle, the
// content-memory bundle, each voice profile's YAML, and the decision record's
// own shards. Identities are preserved by construction — a restore upserts by
// concept id, entry id, profile id and unit key — so exporting, restoring into
// a fresh store and exporting again yields the same archive bytes.
//
// The decisions a bundle carries are the project's LEDGER, the entry in force
// at every pairing, not one checkout's view of it. One ledger serves every
// checkout of a project, and two of them sit on different branches with
// different translations of the same unit at once, so a bundle built from a
// view would be lossless for the checkout that wrote it and lossy for the
// project. A restore records every entry, and each checkout answers for the
// pairings its own files carry. The `.kapi/` snapshot keeps writing the view,
// because the shards are what that checkout commits and evaluates from.
//
// The redaction vault is not in it, and there is no flag that puts it there.

// ContextBundleExt is the suffix a context bundle carries.
const ContextBundleExt = ".kpz"

// contextBundleGenerator identifies what wrote a bundle, so a reader can name
// the build when it has to refuse one.
const contextBundleGenerator = "kapi-context"

// RestoreMode decides what a restore does to a store that already holds
// context.
type RestoreMode string

const (
	// RestoreRefuse stops rather than writing into a store that holds
	// context. The default: a restore is the recovery path, and the machine
	// it runs on may be holding work nobody meant to overwrite.
	RestoreRefuse RestoreMode = ""
	// RestoreMerge upserts the bundle over what is there, which is idempotent
	// the way an import is.
	RestoreMerge RestoreMode = "merge"
	// RestoreReplace empties the context stores first, so what is left is the
	// bundle and nothing else.
	RestoreReplace RestoreMode = "replace"
)

// ContextExport reports what a bundle holds.
type ContextExport struct {
	// Path is the bundle that was written.
	Path string `json:"path"`
	// RootHash is the bundle's content identity, the digest its manifest
	// carries.
	RootHash string `json:"rootHash"`
	// Bytes is the archive's size.
	Bytes int `json:"bytes"`
	// Concepts, Entries, VoiceProfiles and Decisions count what travelled.
	Concepts      int `json:"concepts"`
	Entries       int `json:"entries"`
	VoiceProfiles int `json:"voiceProfiles"`
	Decisions     int `json:"decisions"`
}

// FormatText renders the export for a reader.
func (r ContextExport) FormatText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "Wrote %s: %s, %s, %s, %s.\nContent identity %s.\n",
		r.Path,
		pluralUnit(r.Concepts, "concept", "concepts"),
		pluralUnit(r.Entries, "content-memory entry", "content-memory entries"),
		pluralUnit(r.VoiceProfiles, "voice profile", "voice profiles"),
		pluralUnit(r.Decisions, "recorded decision", "recorded decisions"),
		r.RootHash)
	return err
}

// ContextRestore reports what a restore read.
type ContextRestore struct {
	// Path is the bundle that was read.
	Path string `json:"path"`
	// Mode is how the restore treated what the store already held.
	Mode string `json:"mode,omitempty"`
	// Concepts, Entries, VoiceProfiles and Decisions count what was written.
	Concepts      int `json:"concepts"`
	Entries       int `json:"entries"`
	VoiceProfiles int `json:"voiceProfiles"`
	Decisions     int `json:"decisions"`
	// Cleared reports that the stores were emptied before the bundle went in.
	Cleared bool `json:"cleared,omitempty"`
}

// FormatText renders the restore for a reader.
func (r ContextRestore) FormatText(w io.Writer) error {
	head := "Restored"
	if r.Cleared {
		head = "Replaced the project's context with"
	}
	_, err := fmt.Fprintf(w, "%s %s: %s, %s, %s, %s.\n",
		head, r.Path,
		pluralUnit(r.Concepts, "concept", "concepts"),
		pluralUnit(r.Entries, "content-memory entry", "content-memory entries"),
		pluralUnit(r.VoiceProfiles, "voice profile", "voice profiles"),
		pluralUnit(r.Decisions, "recorded decision", "recorded decisions"))
	return err
}

// ExportProjectContext writes the project's whole context to one bundle.
//
// Everything in the bundle comes out of the store, so the export reads no file
// in the checkout and writes none but the bundle itself.
func (a *App) ExportProjectContext(ctx context.Context, projectPath, out string) (ContextExport, error) {
	var res ContextExport

	if out == "" {
		return res, errors.New("name the bundle to write with -o")
	}
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return res, err
	}
	stores := contextStoresOf(ctx, db)
	if st := db.Work(); st != nil {
		// The ledger, not this checkout's view of it: a project whose branches
		// answer one unit differently has decided both, and a backup that
		// carried only what is checked out here would drop the rest.
		if stores.units, err = st.Ledger(ctx); err != nil {
			return res, err
		}
	}
	pkg, err := contextPackage(ctx, stores)
	if err != nil {
		return res, err
	}
	if !pkg.HasContent() {
		return res, errors.New("this project's store holds no context to export")
	}
	return writeContextBundle(pkg, out)
}

// contextStoresOf names a project store's authored subsystems.
//
// A handle the build has none of is left out rather than assigned: the
// accessors return concrete pointers, and a nil pointer in an interface field
// is not a nil interface, so every reader of one would call through it. The
// browser build has no file-backed store and returns nil from all three.
func contextStoresOf(ctx context.Context, db *projectdb.DB) contextStores {
	return contextStores{
		bindings: loadVoiceBindings(ctx, db),
		terms:    projector.TermsView(db),
		memory:   projector.MemoryView(db),
		voice:    projector.VoiceView(db),
	}
}

// contextStores is what a context bundle is built from: the subsystems of one
// project that hold authored context, and where each voice profile is authored.
//
// The handles are named rather than a *projectdb.DB, because a whole-workspace
// export reads a project's context store straight out of the workspace and has
// no checkout to open a project store against.
type contextStores struct {
	terms    terms.Terminology
	memory   memory.Store
	voice    coreprofile.Store
	bindings map[string]string
	// units is the decision ledger the bundle carries: the entry in force at
	// every pairing the project has recorded, from whichever checkout recorded
	// it.
	units []state.UnitState
}

// contextPackage assembles the context profile of the container from a
// project's stores.
func contextPackage(ctx context.Context, s contextStores) (*kpz.Package, error) {
	pkg := &kpz.Package{
		Kind:      kpz.KindContext,
		Generator: &kpz.GeneratorInfo{ID: contextBundleGenerator, Version: version.Version},
	}
	var err error
	if pkg.Terms, err = bundleTerms(ctx, s.terms); err != nil {
		return nil, err
	}
	if pkg.Memory, err = bundleMemory(ctx, s.memory); err != nil {
		return nil, err
	}
	if pkg.Voice, err = bundleVoice(ctx, s.voice, s.bindings); err != nil {
		return nil, err
	}
	if pkg.Decisions, err = bundleDecisions(s.units); err != nil {
		return nil, err
	}
	return pkg, nil
}

// writeContextBundle writes a package to out and reports what it holds.
func writeContextBundle(pkg *kpz.Package, out string) (ContextExport, error) {
	var res ContextExport
	n, err := writePackage(pkg, out)
	if err != nil {
		return res, err
	}
	hash, err := pkg.RootHash()
	if err != nil {
		return res, err
	}
	res.Path = out
	res.Bytes = int(n)
	res.RootHash = hash
	res.Concepts = len(pkg.Terms.Concepts)
	res.Entries = len(pkg.Memory.Entries)
	res.VoiceProfiles = len(pkg.Voice)
	res.Decisions = countDecisionRows(pkg.Decisions)
	return res, nil
}

// writePackage streams a package into a file, creating the directory it names.
// The archive is written member by member, so the size of what is being packed
// never decides whether the write fits in memory.
func writePackage(pkg *kpz.Package, out string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, fmt.Errorf("create %s: %w", filepath.Dir(out), err)
	}
	f, err := os.Create(out)
	if err != nil {
		return 0, fmt.Errorf("write %s: %w", out, err)
	}
	n, err := pkg.WriteTo(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(out)
		return 0, fmt.Errorf("write %s: %w", out, err)
	}
	return n, nil
}

// bundleTerms reads the terms store through the same exporter that writes the
// committed bundle, so a member and a snapshot's terms.json carry the same
// bytes.
func bundleTerms(ctx context.Context, tb terms.Terminology) (*ktb.File, error) {
	if tb == nil {
		return ktb.FromConcepts(nil), nil
	}
	var buf bytes.Buffer
	if err := ExportKTB(ctx, tb, &buf); err != nil {
		return nil, err
	}
	return ktb.Unmarshal(buf.Bytes())
}

// bundleMemory reads the content memory through the same exporter that writes
// the committed bundle.
func bundleMemory(ctx context.Context, tm memory.Store) (*kmb.File, error) {
	if tm == nil {
		return kmb.FromModel(nil, nil), nil
	}
	var buf bytes.Buffer
	if err := ExportKMB(ctx, tm, &buf); err != nil {
		return nil, err
	}
	return kmb.Unmarshal(buf.Bytes())
}

// bundleVoice reads every voice profile the store holds, each carrying the id
// it is stored under and the path it is authored at.
func bundleVoice(ctx context.Context, store coreprofile.Store, bindings map[string]string) ([]kpz.VoiceDoc, error) {
	profiles, err := boundVoiceProfiles(ctx, store, bindings)
	if err != nil {
		return nil, err
	}
	out := make([]kpz.VoiceDoc, 0, len(profiles))
	for _, bp := range profiles {
		out = append(out, kpz.VoiceDoc{
			Path:    kpz.VoiceDir + bp.profile.ID + ".yaml",
			ID:      bp.profile.ID,
			Binding: bp.binding,
			Profile: AuthoredVoiceProfile(bp.profile),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// bundleDecisions serializes the decision ledger into the shards a project
// commits, through the writer core/state owns, and carries their bytes.
//
// The shards are produced from the store rather than read out of the checkout,
// so a bundle holds what the project decided and not what happens to be on
// disk beside it. A unit several branches answer differently contributes a line
// per pairing, which is more than any one checkout's shards hold and is what a
// reader of the bundle records back. They are written to a directory that
// exists for the length of the call, because the serializer's unit is a
// directory of shards.
func bundleDecisions(units []state.UnitState) ([]kpz.DecisionDoc, error) {
	if len(units) == 0 {
		return nil, nil
	}
	dir, err := os.MkdirTemp("", "kapi-context-record-")
	if err != nil {
		return nil, fmt.Errorf("stage the decision record: %w", err)
	}
	defer os.RemoveAll(dir)
	if err := state.WriteCommitted(dir, units); err != nil {
		return nil, err
	}
	shards, err := recordShardBytes(dir)
	if err != nil {
		return nil, err
	}
	out := make([]kpz.DecisionDoc, 0, len(shards))
	for name, data := range shards {
		out = append(out, kpz.DecisionDoc{Path: kpz.DecisionsDir + name, Data: data})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// countDecisionRows counts the units the shards hold: one JSON Lines row each.
func countDecisionRows(docs []kpz.DecisionDoc) int {
	n := 0
	for _, d := range docs {
		n += bytes.Count(d.Data, []byte("\n"))
	}
	return n
}

// RestoreProjectContext reads a context bundle into the project's store.
//
// A store that already holds context is left alone unless the caller says what
// to do with it: RestoreMerge upserts the bundle over it, and RestoreReplace
// empties it first. Merging is idempotent, so restoring the same bundle twice
// leaves the same store.
func (a *App) RestoreProjectContext(ctx context.Context, projectPath, bundlePath string, mode RestoreMode) (ContextRestore, error) {
	res := ContextRestore{Path: bundlePath, Mode: string(mode)}

	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		return res, fmt.Errorf("read %s: %w", bundlePath, err)
	}
	pkg, err := kpz.Unmarshal(data)
	if err != nil {
		return res, refusedBundle(bundlePath, err)
	}
	if pkg.Kind != kpz.KindContext {
		return res, fmt.Errorf("%s is a %s package: `kapi context restore` reads a context bundle, which `kapi context export` writes", bundlePath, pkg.Kind)
	}

	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return res, err
	}
	stores := contextStoresOf(ctx, db)
	writer, err := a.Projector(ctx, layout.Root)
	if err != nil {
		return res, err
	}
	writer = writer.With(projector.Origin{By: "restore", Source: filepath.Base(bundlePath)})
	target := contextTarget{
		terms:    termsWriter(writer),
		memory:   memoryWriter(writer),
		voice:    voiceWriter(writer),
		work:     db.Work(),
		bindings: stores.bindings,
	}
	held, err := target.holdsContext(ctx)
	if err != nil {
		return res, err
	}
	switch {
	case !held:
		// An empty store takes the bundle whatever the mode says.
	case mode == RestoreRefuse:
		return res, fmt.Errorf("this project's store already holds context: restore with --merge to upsert %s over it, or --replace to put it in place of it", filepath.Base(bundlePath))
	case mode == RestoreReplace:
		if err := target.clear(ctx); err != nil {
			return res, err
		}
		res.Cleared = true
	}

	if err := a.restoreInto(ctx, target, pkg, &res); err != nil {
		return res, err
	}
	// The bindings the bundle carried are recorded where the project reads them
	// back from, which a workspace holds no room for and a checkout does.
	return res, saveVoiceBindings(ctx, db, target.bindings)
}

// contextTarget is where a restore writes: the subsystems of one project that
// hold authored context, named rather than reached through a *projectdb.DB, so
// a whole-workspace restore can write into a project's context store without a
// checkout of that project on this machine.
type contextTarget struct {
	terms  terms.Terminology
	memory memory.Store
	voice  coreprofile.Store
	work   *state.WorkStore
	// bindings records where each voice profile in the store is authored. A
	// restore adds what the bundle carried; the caller that supplied the map
	// persists it where the project reads it back from.
	bindings map[string]string
}

// restoreInto writes a context package into a target, through the importers a
// committed bundle already goes through, so every identity is preserved.
func (a *App) restoreInto(ctx context.Context, t contextTarget, pkg *kpz.Package, res *ContextRestore) error {
	if err := restoreTerms(ctx, t, pkg, res); err != nil {
		return err
	}
	if err := a.restoreMemory(ctx, t, pkg, res); err != nil {
		return err
	}
	if err := restoreVoice(ctx, t, pkg, res); err != nil {
		return err
	}
	return a.restoreDecisions(ctx, t, pkg, res)
}

// refusedBundle turns a container's refusal into a sentence naming the file and
// what to do about it. A bundle written by a build that speaks a later major
// version of the package format is the case worth naming: the archive is
// intact, and it is this build that cannot read it.
func refusedBundle(path string, err error) error {
	if strings.Contains(err.Error(), "schemaVersion") {
		return fmt.Errorf("%s was written by a later kapi than this one (%s) and cannot be read here: %w",
			filepath.Base(path), version.Version, err)
	}
	return fmt.Errorf("read %s: %w", filepath.Base(path), err)
}

// holdsContext reports whether any of the target's stores has anything in it.
func (t contextTarget) holdsContext(ctx context.Context) (bool, error) {
	if t.terms != nil {
		concepts, err := t.terms.Concepts(ctx)
		if err != nil {
			return false, fmt.Errorf("read terms: %w", err)
		}
		if len(concepts) > 0 {
			return true, nil
		}
	}
	if t.memory != nil {
		// One row answers the question, and a project's content memory is the
		// one store that grows without a bound anybody set.
		page, _, err := t.memory.SearchEntries(ctx, memory.SearchParams{Limit: 1})
		if err != nil {
			return false, fmt.Errorf("read content memory: %w", err)
		}
		if len(page) > 0 {
			return true, nil
		}
	}
	if t.voice != nil {
		profiles, err := t.voice.ListProfiles(ctx, LocalScope)
		if err != nil {
			return false, fmt.Errorf("read voice profiles: %w", err)
		}
		if len(profiles) > 0 {
			return true, nil
		}
	}
	if t.work != nil {
		units, err := t.work.Ledger(ctx)
		if err != nil {
			return false, err
		}
		return len(units) > 0, nil
	}
	return false, nil
}

// restoreTerms writes the bundle's concepts and relations through the same
// importer a committed bundle goes through, so identities are preserved.
func restoreTerms(ctx context.Context, t contextTarget, pkg *kpz.Package, res *ContextRestore) error {
	if pkg.Terms == nil || len(pkg.Terms.Concepts) == 0 {
		return nil
	}
	if t.terms == nil {
		return fmt.Errorf("restore terms: %w", projectdb.ErrNoStore)
	}
	data, err := ktb.Marshal(pkg.Terms)
	if err != nil {
		return fmt.Errorf("restore terms: %w", err)
	}
	n, err := ImportKTBFile(ctx, t.terms, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("restore terms: %w", err)
	}
	res.Concepts += n
	return nil
}

// restoreMemory writes the bundle's entries and import sessions through the
// same importer a committed bundle goes through.
func (a *App) restoreMemory(ctx context.Context, t contextTarget, pkg *kpz.Package, res *ContextRestore) error {
	if pkg.Memory == nil || len(pkg.Memory.Entries) == 0 {
		return nil
	}
	if t.memory == nil {
		return fmt.Errorf("restore content memory: %w", projectdb.ErrNoStore)
	}
	n, err := importKMB(ctx, t.memory, "the context bundle", pkg.Memory)
	if err != nil {
		return fmt.Errorf("restore content memory: %w", err)
	}
	a.RebuildMemorySearchIndexes(ctx, t.memory)
	res.Entries += n
	return nil
}

// restoreVoice writes the bundle's voice profiles into the store and records
// where each one is authored, so a later snapshot puts it back at the path
// governance resolves it from.
func restoreVoice(ctx context.Context, t contextTarget, pkg *kpz.Package, res *ContextRestore) error {
	if len(pkg.Voice) == 0 {
		return nil
	}
	if t.voice == nil {
		return fmt.Errorf("restore voice profiles: %w", projectdb.ErrNoStore)
	}
	for _, doc := range pkg.Voice {
		prof := doc.Profile
		if prof == nil {
			continue
		}
		if doc.ID != "" {
			prof.ID = doc.ID
		}
		if prof.ID == "" {
			prof.ID = slugify(prof.Name)
		}
		prof.Scope = LocalScope
		if err := upsertVoiceProfile(ctx, t.voice, prof); err != nil {
			return fmt.Errorf("restore voice profiles: %w", err)
		}
		if doc.Binding != "" && t.bindings != nil {
			t.bindings[doc.Binding] = prof.ID
		}
		res.VoiceProfiles++
	}
	return nil
}

// restoreDecisions writes every entry the bundle's decision record carries into
// the project's ledger and makes it durable, through the reader core/state
// owns. A unit the bundle answers at several pairings gets a ledger entry for
// each, so every branch that had decided it still answers here.
func (a *App) restoreDecisions(ctx context.Context, t contextTarget, pkg *kpz.Package, res *ContextRestore) error {
	if len(pkg.Decisions) == 0 {
		return nil
	}
	dir, err := os.MkdirTemp("", "kapi-context-record-")
	if err != nil {
		return fmt.Errorf("stage the decision record: %w", err)
	}
	defer os.RemoveAll(dir)
	for _, doc := range pkg.Decisions {
		name := filepath.Base(filepath.FromSlash(doc.Path))
		if err := os.WriteFile(filepath.Join(dir, name), doc.Data, 0o644); err != nil {
			return fmt.Errorf("stage %s: %w", name, err)
		}
	}
	n, err := importDecisionRecord(ctx, t.work, dir)
	if err != nil {
		return fmt.Errorf("restore the decision record: %w", err)
	}
	res.Decisions += n
	return nil
}

// clear empties every context store, for a restore that was asked to put a
// bundle in place of what is there rather than over it.
//
// The content memory is emptied a page at a time. It is the one store in a
// project that grows without a bound anybody set, and reading the whole of it
// to delete it would make the size of a project decide whether the command
// finishes.
func (t contextTarget) clear(ctx context.Context) error {
	if t.terms != nil {
		concepts, err := t.terms.Concepts(ctx)
		if err != nil {
			return fmt.Errorf("clear terms: %w", err)
		}
		for _, c := range concepts {
			if err := t.terms.DeleteConcept(ctx, c.ID); err != nil {
				return fmt.Errorf("clear terms: %w", err)
			}
		}
	}
	if t.memory != nil {
		if err := clearContentMemory(ctx, t.memory); err != nil {
			return err
		}
	}
	if t.voice != nil {
		profiles, err := t.voice.ListProfiles(ctx, LocalScope)
		if err != nil {
			return fmt.Errorf("clear voice profiles: %w", err)
		}
		for _, p := range profiles {
			if err := t.voice.DeleteProfile(ctx, p.ID); err != nil {
				return fmt.Errorf("clear voice profiles: %w", err)
			}
		}
		clear(t.bindings)
	}
	if t.work != nil {
		// The checkout stops holding any unit state; the ledger keeps every
		// entry it ever recorded. A decision the bundle carries is recorded
		// again a moment later and answers for its unit once more, and one the
		// bundle does not carry stops answering here without being erased from
		// the record of what was decided.
		if err := t.work.ClearView(ctx); err != nil {
			return fmt.Errorf("clear the decision record: %w", err)
		}
		if err := t.work.PersistRecords(ctx); err != nil {
			return err
		}
	}
	return nil
}

// clearMemoryPageSize bounds how many entries a clearing pass holds at once.
const clearMemoryPageSize = 500

// clearContentMemory deletes every entry, a page at a time. Each page is taken
// from the front, because deleting the page it just read is what makes the next
// one different.
func clearContentMemory(ctx context.Context, tm memory.Store) error {
	for {
		page, _, err := tm.SearchEntries(ctx, memory.SearchParams{Limit: clearMemoryPageSize})
		if err != nil {
			return fmt.Errorf("clear content memory: %w", err)
		}
		if len(page) == 0 {
			return nil
		}
		for _, e := range page {
			if err := tm.Delete(ctx, e.ID); err != nil {
				return fmt.Errorf("clear content memory: %w", err)
			}
		}
	}
}
