package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// A whole workspace in one file.
//
// A project's context store left the checkout: the terms, the voice profiles,
// the content memory and the decision ledger live in a workspace under the
// user's data root, one database per project. That directory holds authored
// work for every project on the machine and is in no version control, so an
// archive of the whole of it is what a person has after losing it.
//
// The archive is the workspace profile of the `.kpz` container
// (kpz.KindWorkspace): one member per project, each a complete context package,
// plus the registry entries that say which project each member is. Unzip it and
// every `projects/<n>.kpz` is a file `kapi context restore` reads on its own.
//
// Nothing about this machine travels. The checkout paths the registry records
// name directories on one computer, so they are left behind, and a project
// whose checkout is gone exports exactly as one that is open in an editor: its
// context store is in the workspace either way, which is the case this exists
// for. The redaction vault is not in it, and there is no flag that puts it
// there.

// workspaceBundleGenerator identifies what wrote a workspace bundle.
const workspaceBundleGenerator = "kapi-workspace"

// ContextWorkspaceProject is one project in a workspace export or listing.
type ContextWorkspaceProject struct {
	// Key identifies the project in the workspace: the recipe's stable `id:`,
	// or its `name:` where the recipe carries none.
	Key string `json:"key"`
	// Name is the display name the recipe carries. It may be empty.
	Name string `json:"name,omitempty"`
	// Concepts, Entries, VoiceProfiles and Decisions count what this project's
	// context store holds.
	Concepts      int `json:"concepts"`
	Entries       int `json:"entries"`
	VoiceProfiles int `json:"voiceProfiles"`
	Decisions     int `json:"decisions"`
	// Bundle is the member this project is carried in, e.g.
	// "projects/prj_9f2k.kpz". Empty in a listing, which writes no archive.
	Bundle string `json:"bundle,omitempty"`
	// CheckedOut reports that the workspace has seen this project at a
	// directory that is still there. A project with none exports the same way;
	// it is reported so a person reading a listing recognises what they have.
	CheckedOut bool `json:"checkedOut"`
}

// holds reports whether this project's context store has anything in it.
func (p ContextWorkspaceProject) holds() bool {
	return p.Concepts > 0 || p.Entries > 0 || p.VoiceProfiles > 0 || p.Decisions > 0
}

// ContextWorkspaceExport reports what a workspace bundle holds, or what one
// would hold for a listing that writes nothing.
type ContextWorkspaceExport struct {
	// Workspace is the directory the context was read from.
	Workspace string `json:"workspace"`
	// Path is the bundle that was written. Empty for a listing.
	Path string `json:"path,omitempty"`
	// RootHash is the bundle's content identity. Empty for a listing.
	RootHash string `json:"rootHash,omitempty"`
	// Bytes is the archive's size. Zero for a listing.
	Bytes int64 `json:"bytes,omitempty"`
	// Projects is what travelled, ordered by key.
	Projects []ContextWorkspaceProject `json:"projects"`
	// Skipped counts registered projects whose context store holds nothing.
	// They keep their registry entry, so a restore knows them, and carry no
	// package.
	Skipped int `json:"skipped,omitempty"`
}

// FormatText renders the export for a reader.
func (r ContextWorkspaceExport) FormatText(w io.Writer) error {
	head := fmt.Sprintf("%s holds %s", r.Workspace, pluralUnit(len(r.Projects), "project", "projects"))
	if r.Path == "" {
		if _, err := fmt.Fprintf(w, "%s:\n", head); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, "Wrote %s from %s:\n", r.Path, head); err != nil {
		return err
	}
	for _, p := range r.Projects {
		label := p.Key
		if p.Name != "" && p.Name != p.Key {
			label = fmt.Sprintf("%s (%s)", p.Name, p.Key)
		}
		note := ""
		if !p.CheckedOut {
			note = ", no checkout here"
		}
		if _, err := fmt.Fprintf(w, "  %s: %s, %s, %s, %s%s\n", label,
			pluralUnit(p.Concepts, "concept", "concepts"),
			pluralUnit(p.Entries, "content-memory entry", "content-memory entries"),
			pluralUnit(p.VoiceProfiles, "voice profile", "voice profiles"),
			pluralUnit(p.Decisions, "recorded decision", "recorded decisions"),
			note); err != nil {
			return err
		}
	}
	if r.Skipped > 0 {
		if _, err := fmt.Fprintf(w, "  %s registered and holding no context\n",
			pluralUnit(r.Skipped, "project", "projects")); err != nil {
			return err
		}
	}
	if r.RootHash != "" {
		if _, err := fmt.Fprintf(w, "Content identity %s.\n", r.RootHash); err != nil {
			return err
		}
	}
	return nil
}

// ContextWorkspaceRestore reports what a workspace restore read.
type ContextWorkspaceRestore struct {
	// Workspace is the directory the context was written into.
	Workspace string `json:"workspace"`
	// Path is the bundle that was read.
	Path string `json:"path"`
	// Mode is how the restore treated what the workspace already held.
	Mode string `json:"mode,omitempty"`
	// Projects is what was written, ordered by key.
	Projects []ContextWorkspaceProject `json:"projects"`
	// Replaced names the projects whose context was emptied first, ordered by
	// key. Only a restore asked to replace has any.
	Replaced []ContextWorkspaceProject `json:"replaced,omitempty"`
}

// FormatText renders the restore for a reader.
func (r ContextWorkspaceRestore) FormatText(w io.Writer) error {
	head := "Restored"
	if len(r.Replaced) > 0 {
		head = "Replaced the context of " + pluralUnit(len(r.Replaced), "project", "projects") + " and restored"
	}
	if _, err := fmt.Fprintf(w, "%s %s into %s:\n", head, r.Path, r.Workspace); err != nil {
		return err
	}
	for _, p := range r.Projects {
		label := p.Key
		if p.Name != "" && p.Name != p.Key {
			label = fmt.Sprintf("%s (%s)", p.Name, p.Key)
		}
		if _, err := fmt.Fprintf(w, "  %s: %s, %s, %s, %s\n", label,
			pluralUnit(p.Concepts, "concept", "concepts"),
			pluralUnit(p.Entries, "content-memory entry", "content-memory entries"),
			pluralUnit(p.VoiceProfiles, "voice profile", "voice profiles"),
			pluralUnit(p.Decisions, "recorded decision", "recorded decisions")); err != nil {
			return err
		}
	}
	return nil
}

// ContextWorkspaceExportRequest names what a whole-workspace export writes.
type ContextWorkspaceExportRequest struct {
	// Out is the bundle to write. Empty is an error unless DryRun is set.
	Out string
	// DryRun reads the workspace and reports what an export would carry
	// without writing anything, which is how a person sees what they have
	// before they back it up.
	DryRun bool
}

// ExportWorkspaceContext writes the context of every project in this machine
// account's workspace to one bundle.
//
// A project is read straight out of the workspace, so one whose checkout has
// been deleted, or which was only ever opened on another machine, travels
// exactly as one that is open right now. A project registered and holding no
// context keeps its registry entry and carries no package.
//
// The archive is written member by member and each project's package is built
// on its own, so the whole of a workspace is never in memory at once.
func (a *App) ExportWorkspaceContext(ctx context.Context, req ContextWorkspaceExportRequest) (ContextWorkspaceExport, error) {
	var res ContextWorkspaceExport

	if req.Out == "" && !req.DryRun {
		return res, errors.New("name the bundle to write with -o")
	}
	ws, err := a.Workspace(ctx)
	if err != nil {
		return res, err
	}
	res.Workspace = ws.Describe().Location

	regs, err := ws.Projects(ctx)
	if err != nil {
		return res, err
	}
	if len(regs) == 0 {
		return res, fmt.Errorf("%s holds no projects: open a project once and its context is registered here", res.Workspace)
	}
	sort.Slice(regs, func(i, j int) bool { return regs[i].Key < regs[j].Key })

	// Each project's package is written to a file that lives for the length of
	// the call, and the workspace archive streams from those. One project at a
	// time is therefore the most that is ever held.
	staging, err := os.MkdirTemp("", "kapi-workspace-export-")
	if err != nil {
		return res, fmt.Errorf("stage the workspace export: %w", err)
	}
	defer os.RemoveAll(staging)

	pkg := &kpz.Package{
		Kind:      kpz.KindWorkspace,
		Generator: &kpz.GeneratorInfo{ID: workspaceBundleGenerator, Version: version.Version},
	}
	for _, reg := range regs {
		held, doc, err := a.exportOneProject(ctx, ws, reg, staging, req.DryRun)
		if err != nil {
			return res, err
		}
		if !held.holds() {
			res.Skipped++
			continue
		}
		res.Projects = append(res.Projects, held)
		if doc != nil {
			pkg.Projects = append(pkg.Projects, *doc)
		}
	}
	if req.DryRun {
		return res, nil
	}
	if len(res.Projects) == 0 {
		return res, fmt.Errorf("no project in %s holds context to export", res.Workspace)
	}

	n, err := writePackage(pkg, req.Out)
	if err != nil {
		return res, err
	}
	hash, err := pkg.RootHash()
	if err != nil {
		return res, err
	}
	res.Path, res.Bytes, res.RootHash = req.Out, n, hash
	return res, nil
}

// exportOneProject reads one project's context store out of the workspace. It
// returns what the project holds, and the member carrying it unless the caller
// only asked what is there.
func (a *App) exportOneProject(ctx context.Context, ws *workspace.Workspace, reg workspace.Registration, staging string, dryRun bool) (ContextWorkspaceProject, *kpz.ProjectDoc, error) {
	out := ContextWorkspaceProject{
		Key:        string(reg.Key),
		Name:       reg.Name,
		CheckedOut: anyCheckoutExists(reg.Checkouts),
	}
	db, err := ws.Context(ctx, reg.Key)
	if err != nil {
		return out, nil, err
	}
	stores, err := openWorkspaceContext(ctx, ws, reg.Key, db)
	if err != nil {
		return out, nil, fmt.Errorf("read the context of %s: %w", reg.Key, err)
	}
	units, err := stores.work.Ledger(ctx)
	if err != nil {
		return out, nil, fmt.Errorf("read the decisions of %s: %w", reg.Key, err)
	}
	pkg, err := contextPackage(ctx, contextStores{
		terms:  stores.terms,
		memory: stores.memory,
		voice:  stores.voice,
		// A workspace records no authoring path for a voice profile, so each
		// one takes the conventional place for a profile of its id, which is
		// where governance resolves a profile that binds no file of its own.
		units: units,
	})
	if err != nil {
		return out, nil, fmt.Errorf("read the context of %s: %w", reg.Key, err)
	}
	out.Concepts = len(pkg.Terms.Concepts)
	out.Entries = len(pkg.Memory.Entries)
	out.VoiceProfiles = len(pkg.Voice)
	out.Decisions = countDecisionRows(pkg.Decisions)
	if !out.holds() || dryRun {
		return out, nil, nil
	}

	member := kpz.ProjectsDir + workspace.FileNameFor(reg.Key) + ContextBundleExt
	out.Bundle = member
	staged := filepath.Join(staging, filepath.Base(member))
	if _, err := writePackage(pkg, staged); err != nil {
		return out, nil, err
	}
	return out, &kpz.ProjectDoc{
		Path:    member,
		Key:     string(reg.Key),
		Name:    reg.Name,
		Content: kpz.FileContent(staged),
	}, nil
}

// stageMember copies one archive member to a file, so a reader that needs to
// seek over it does not need it in memory. The caller removes the file.
func stageMember(c kpz.Content, name string) (string, error) {
	f, err := os.CreateTemp("", "kapi-workspace-member-*"+ContextBundleExt)
	if err != nil {
		return "", fmt.Errorf("stage %s: %w", name, err)
	}
	rc, err := c.Open()
	if err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	_, copyErr := io.Copy(f, rc)
	_ = rc.Close()
	if closeErr := f.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("stage %s: %w", name, copyErr)
	}
	return f.Name(), nil
}

// anyCheckoutExists reports whether any directory the workspace has seen this
// project at is still there.
func anyCheckoutExists(paths []string) bool {
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// workspaceContext is a project's context store opened straight out of the
// workspace, with no checkout of the project on this machine.
//
// It binds the same four subsystems a project store binds, against the same
// database, and runs the same migrations, so a store this creates is one a
// checkout opens later without noticing. What it leaves out is everything that
// belongs to a checkout: the block cache, the store metadata, and the view of
// the ledger a working tree reads through.
type workspaceContext struct {
	terms  terms.Terminology
	memory memory.Store
	voice  coreprofile.Store
	work   *state.WorkStore
}

// openWorkspaceContext binds a project's context subsystems to its workspace
// database, written through a projector over the workspace's log like every
// other writer of them.
func openWorkspaceContext(ctx context.Context, ws *workspace.Workspace, key workspace.ProjectKey, db *storage.DB) (workspaceContext, error) {
	var out workspaceContext
	stores, err := projector.ContextStores(db)
	if err != nil {
		return out, err
	}
	var log projector.Log
	if !ws.Describe().ReadOnly {
		log = ws
	}
	writer, err := projector.New(log, key, stores)
	if err != nil {
		return out, err
	}
	work, err := state.OpenLedger(ctx, db)
	if err != nil {
		return out, fmt.Errorf("bind the decision ledger: %w", err)
	}
	if log != nil {
		work.SetJournal(writer.Units())
		if err := writer.CatchUp(ctx); err != nil {
			return out, fmt.Errorf("apply the context log to %s: %w", key, err)
		}
	}
	out.terms, out.memory, out.voice, out.work = termsWriter(writer), memoryWriter(writer), voiceWriter(writer), work
	return out, nil
}

// target is what a restore writes this project's context into.
func (c workspaceContext) target() contextTarget {
	return contextTarget{terms: c.terms, memory: c.memory, voice: c.voice, work: c.work}
}

// ContextWorkspaceRestoreRequest names what a whole-workspace restore reads.
type ContextWorkspaceRestoreRequest struct {
	// Bundle is the archive to read.
	Bundle string
	// Mode decides what happens to a workspace that already holds context.
	Mode RestoreMode
	// Notice is where a restore says what it is about to replace, before it
	// does. nil sends it nowhere, which is what a caller reading the result
	// wants.
	Notice io.Writer
}

// RestoreWorkspaceContext reads a workspace bundle into this machine account's
// workspace.
//
// An empty workspace takes the bundle outright, which is the case this exists
// for: a machine that lost its data root gets every project's terms, voice
// profiles, approved wording and recorded decisions back, under the identities
// they left with. A workspace that already holds projects is left alone until
// the caller says what should happen to it. RestoreMerge upserts the bundle
// over it and is idempotent. RestoreReplace empties the context of every
// project the workspace holds first, naming each one before it does.
//
// A project is registered and written straight into the workspace, so no
// checkout of it has to exist. Cloning one afterwards picks up its context on
// the first open.
func (a *App) RestoreWorkspaceContext(ctx context.Context, req ContextWorkspaceRestoreRequest) (ContextWorkspaceRestore, error) {
	res := ContextWorkspaceRestore{Path: req.Bundle, Mode: string(req.Mode)}

	pkg, closer, err := kpz.OpenFile(req.Bundle)
	if err != nil {
		return res, refusedBundle(req.Bundle, err)
	}
	defer func() { _ = closer.Close() }()
	if pkg.Kind != kpz.KindWorkspace {
		return res, fmt.Errorf("%s is a %s package: `kapi context restore --workspace` reads a workspace bundle, which `kapi context export --workspace` writes; without --workspace it reads this one into the project", req.Bundle, pkg.Kind)
	}

	ws, err := a.Workspace(ctx)
	if err != nil {
		return res, err
	}
	res.Workspace = ws.Describe().Location
	if ws.Describe().ReadOnly {
		return res, fmt.Errorf("%s is open for reading only: %w", res.Workspace, workspace.ErrReadOnly)
	}

	held, err := ws.Projects(ctx)
	if err != nil {
		return res, err
	}
	switch {
	case len(held) == 0:
		// An empty workspace takes the bundle whatever the mode says.
	case req.Mode == RestoreRefuse:
		return res, fmt.Errorf("%s already holds %s: restore with --merge to read %s over what is there, or --replace to put it in place of it",
			res.Workspace, pluralUnit(len(held), "project", "projects"), filepath.Base(req.Bundle))
	case req.Mode == RestoreReplace:
		if res.Replaced, err = a.replaceWorkspaceContext(ctx, ws, held, req.Notice); err != nil {
			return res, err
		}
	}

	for _, doc := range pkg.Projects {
		project, err := a.restoreOneProject(ctx, ws, doc)
		if err != nil {
			return res, err
		}
		res.Projects = append(res.Projects, project)
	}
	return res, nil
}

// replaceWorkspaceContext empties the context of every project the workspace
// holds, after saying which ones those are.
//
// The naming comes first and in full, because a replace is the one mode that
// destroys work: a person who meant a different workspace reads the list and
// stops rather than learning afterwards what they overwrote.
func (a *App) replaceWorkspaceContext(ctx context.Context, ws *workspace.Workspace, held []workspace.Registration, notice io.Writer) ([]ContextWorkspaceProject, error) {
	sort.Slice(held, func(i, j int) bool { return held[i].Key < held[j].Key })

	replaced := make([]ContextWorkspaceProject, 0, len(held))
	for _, reg := range held {
		replaced = append(replaced, ContextWorkspaceProject{
			Key: string(reg.Key), Name: reg.Name, CheckedOut: anyCheckoutExists(reg.Checkouts),
		})
	}
	if notice != nil {
		if _, err := fmt.Fprintf(notice, "Replacing the context of %s in %s:\n",
			pluralUnit(len(replaced), "project", "projects"), ws.Describe().Location); err != nil {
			return nil, err
		}
		for _, p := range replaced {
			label := p.Key
			if p.Name != "" && p.Name != p.Key {
				label = fmt.Sprintf("%s (%s)", p.Name, p.Key)
			}
			if _, err := fmt.Fprintf(notice, "  %s\n", label); err != nil {
				return nil, err
			}
		}
	}
	for _, reg := range held {
		db, err := ws.Context(ctx, reg.Key)
		if err != nil {
			return nil, err
		}
		stores, err := openWorkspaceContext(ctx, ws, reg.Key, db)
		if err != nil {
			return nil, fmt.Errorf("clear the context of %s: %w", reg.Key, err)
		}
		if err := stores.target().clear(ctx); err != nil {
			return nil, fmt.Errorf("clear the context of %s: %w", reg.Key, err)
		}
	}
	return replaced, nil
}

// restoreOneProject registers a project and reads its context package into the
// workspace's store for it.
func (a *App) restoreOneProject(ctx context.Context, ws *workspace.Workspace, doc kpz.ProjectDoc) (ContextWorkspaceProject, error) {
	out := ContextWorkspaceProject{Key: doc.Key, Name: doc.Name, Bundle: doc.Path}
	key := workspace.ProjectKey(doc.Key)

	// The identity is recorded before the context, so a project whose package
	// fails to read is still a project this workspace knows. No checkout path
	// is recorded: the bundle carries none, and the ones this machine has are
	// added when it opens the project.
	if _, err := ws.Register(ctx, key, doc.Name, ""); err != nil {
		return out, err
	}
	db, err := ws.Context(ctx, key)
	if err != nil {
		return out, err
	}
	stores, err := openWorkspaceContext(ctx, ws, key, db)
	if err != nil {
		return out, fmt.Errorf("open the context store of %s: %w", key, err)
	}

	// The member is drained to a file and read from there rather than into
	// memory: one project's content memory is the one store in a workspace
	// that grows without a bound anybody set, and its size must not decide
	// whether a recovery finishes.
	staged, err := stageMember(doc.Content, doc.Path)
	if err != nil {
		return out, err
	}
	defer os.Remove(staged)
	pkg, closer, err := kpz.OpenFile(staged)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", doc.Path, err)
	}
	defer func() { _ = closer.Close() }()
	if pkg.Kind != kpz.KindContext {
		return out, fmt.Errorf("%s is a %s package, and a workspace carries context packages", doc.Path, pkg.Kind)
	}

	var res ContextRestore
	if err := a.restoreInto(ctx, stores.target(), pkg, &res); err != nil {
		return out, fmt.Errorf("restore %s: %w", key, err)
	}
	out.Concepts, out.Entries = res.Concepts, res.Entries
	out.VoiceProfiles, out.Decisions = res.VoiceProfiles, res.Decisions
	return out, nil
}
