package host

import (
	"context"
	"fmt"
	"os"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projectdb"
)

// ResolveVoiceStore picks the voice store a `voice: profile: <name>` binding is
// looked up in. An explicit --name/--local/--file always wins and selects a
// standalone file. Otherwise, inside a project, the project's own store is
// selected — the shared pool that already carries the content memory, the terms
// store and the block cache. Outside a project it falls back to ./voice.db.
// This mirrors ResolveMemoryStore and ResolveTermsStore.
//
// Before the project case existed, every caller resolved ./voice.db against the
// working directory, so `kapi check --voice` run from a subdirectory looked for
// profiles in a file that was never there, while the push resolved the same
// binding against the project root — two halves of one round trip reading two
// different stores depending on where the command was typed.
func (a *App) ResolveVoiceStore(cmd Command) (StoreSelection, error) {
	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	file, _ := cmd.Flags().GetString("file")
	explicit := name != "" || file != "" || local
	if !explicit {
		root, err := a.projectRootFor(cmd)
		if err != nil {
			return StoreSelection{}, err
		}
		if root != "" {
			return StoreSelection{Root: root}, nil
		}
	}
	path, err := resolveResourcePath(cmd, "voice", "voice.db")
	if err != nil {
		return StoreSelection{}, err
	}
	return StoreSelection{Path: path, Explicit: explicit}, nil
}

// OpenVoiceStore opens the voice store a `kapi voice` subcommand writes to, and
// returns it with a label naming where it came from and a release function the
// caller must defer.
//
// The release function exists because the answer is no longer always a file the
// caller owns: inside a project the store is the App's shared handle, whose
// pool also carries the content memory, the terms store and the block cache, so
// closing it would take those with it. Standalone stores close as before.
func (a *App) OpenVoiceStore(cmd Command) (coreprofile.Store, string, func(), error) {
	noop := func() {}
	sel, err := a.ResolveVoiceStore(cmd)
	if err != nil {
		return nil, "", noop, err
	}
	if sel.InProject() {
		db, derr := a.ProjectDB(CmdContext(cmd), sel.Root)
		if derr != nil {
			return nil, "", noop, derr
		}
		store := db.Voice()
		if store == nil {
			return nil, db.Path(), noop, fmt.Errorf("open voice store: %w", projectdb.ErrNoStore)
		}
		return store, db.Path(), noop, nil
	}
	store, err := openVoiceStoreAt(sel.Path)
	if err != nil {
		return nil, sel.Path, noop, err
	}
	return store, sel.Path, func() { _ = store.Close() }, nil
}

// VoiceLookupStore returns the store a resolution should look a bound profile
// name up in, or nil when there is nowhere to look.
//
// nil is an ordinary answer, not a failure: a standalone store is consulted
// only if its file already exists, because resolving a binding must not bring a
// voice.db into being in whatever directory the command was typed. The project
// case has no such hazard — the pool exists because the project does.
func (a *App) VoiceLookupStore(cmd Command) (coreprofile.Store, func(), error) {
	noop := func() {}
	sel, err := a.ResolveVoiceStore(cmd)
	if err != nil {
		return nil, noop, err
	}
	if sel.InProject() {
		return a.ProjectVoiceStore(CmdContext(cmd), sel.Root)
	}
	if _, statErr := os.Stat(sel.Path); statErr != nil {
		return nil, noop, nil
	}
	store, err := openVoiceStoreAt(sel.Path)
	if err != nil {
		return nil, noop, err
	}
	return store, func() { _ = store.Close() }, nil
}

// ProjectVoiceStore returns the voice store on a project's shared pool, or nil
// where this build has no file-backed SQLite driver. The handle belongs to the
// pool, so the release function is a no-op; it is returned anyway so callers
// read the same at both ends of ResolveVoiceStore.
func (a *App) ProjectVoiceStore(ctx context.Context, root string) (coreprofile.Store, func(), error) {
	noop := func() {}
	if root == "" {
		return nil, noop, nil
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return nil, noop, err
	}
	store := db.Voice()
	if store == nil {
		return nil, noop, nil
	}
	return store, noop, nil
}
