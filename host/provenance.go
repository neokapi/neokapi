package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
)

// What a read says about itself.
//
// A retrieval answer is quoted, acted on and sometimes committed, often an
// hour after it was read and by a process that has since asked the same
// question of another project. Three facts make one answer distinguishable
// from another: which project answered, where the workspace's record of that
// project had got to, and whether the blocks kapi holds still match the files
// on disk.
//
// The freshness note beside it (host/freshness.go) reports MOVEMENT, once, to
// the answer that first spans it. This reports POSITION, on every answer, so
// two answers can be compared without either of them having watched the other.

// ContextProvenance says which project a retrieval answer came from and what
// state it was read at.
//
// The type is declared in core/check, because a check result carries the same
// three facts about the context it was evaluated against and the report it
// carries them in is framework-level. One declaration keeps a retrieval answer
// and a check result reporting one shape under one set of field names.
type ContextProvenance = check.ContextProvenance

// provenanceLine renders the provenance as the one line a text answer carries,
// so the CLI reader and the JSON reader are told the same three things. Empty
// when no project stood behind the answer.
func provenanceLine(p *ContextProvenance) string {
	if p == nil {
		return ""
	}
	name := p.Project
	if name == "" {
		name = p.Name
	}
	if name == "" {
		name = "this project"
	}
	state := "current"
	if p.Stale {
		// Which way it is stale is a note of its own, right below this line.
		state = "stale"
	}
	return fmt.Sprintf("Read from project `%s` at workspace revision %d, content %s.", name, p.Revision, state)
}

// contextProvenance assembles the provenance of one read: the project the
// command resolves to, the workspace revision, and the projection's state.
//
// Best-effort throughout, like the rest of the retrieval assembly. A workspace
// that will not open or a store that will not answer costs the caller the
// fields it could not fill, never the answer.
func (a *App) contextProvenance(cmd Command, proj *project.KapiProject) *ContextProvenance {
	recipePath, err := ResolveProjectPath(cmd)
	if err != nil || recipePath == "" {
		return nil
	}
	root := filepath.Dir(recipePath)

	out := &ContextProvenance{}
	if proj != nil {
		out.Project = proj.Identity()
		if proj.Name != out.Project {
			out.Name = proj.Name
		}
	}

	ctx := CmdContext(cmd)
	if ws, werr := a.Workspace(ctx); werr == nil && ws != nil {
		// The operation log's head, the same number the desktop polls to learn
		// that something changed. One query, on a path an agent hits
		// repeatedly inside a single thought.
		if rev, rerr := ws.Head(ctx); rerr == nil {
			out.Revision = rev
		}
	}
	if db, derr := a.ProjectDB(ctx, root); derr == nil && db != nil {
		out.Stale, out.StaleReason = projectionDrift(ctx, db, root)
	}
	return out
}

// projectionDrift reports whether the blocks a project holds still describe
// the files they were read from.
//
// It compares the extract-time stamps the store already carries against the
// files on disk, one stat each, with a hash only where the stat moved. That is
// the same comparison a convergence makes, narrowed to the files kapi has
// already read: it costs no walk of the tree, which matters on a path an agent
// hits repeatedly inside one thought. A file the project declares and nobody
// has read yet therefore shows up the first time it is read rather than here.
func projectionDrift(ctx context.Context, db *projectdb.DB, root string) (bool, string) {
	extracted, err := db.HasBlocks(ctx)
	if err != nil {
		return false, ""
	}
	if !extracted {
		return true, "kapi has read no content from this project yet, so anything counted over its content is empty. `kapi up` reads it"
	}
	if db.BlockStoreStale(ctx) {
		return true, "the content kapi holds was read by a different version of kapi. `kapi up` reads it again"
	}

	var changed, removed int
	for rel, stamp := range db.LoadSourceStamps(ctx) {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		info, serr := os.Stat(abs)
		if serr != nil {
			removed++
			continue
		}
		if info.Size() == stamp.Size && info.ModTime().UnixNano() == stamp.MTimeNS {
			continue
		}
		// The stat moved, which a touch does as readily as an edit, so the
		// bytes decide.
		if hash, herr := project.HashFile(abs); herr != nil || hash != stamp.Hash {
			changed++
		}
	}
	switch {
	case changed > 0 && removed > 0:
		return true, fmt.Sprintf("%d file(s) changed and %d were removed since kapi read them. `kapi up` reads them again", changed, removed)
	case changed > 0:
		return true, fmt.Sprintf("%d file(s) changed since kapi read them. `kapi up` reads them again", changed)
	case removed > 0:
		return true, fmt.Sprintf("%d file(s) were removed since kapi read them. `kapi up` reads them again", removed)
	}
	return false, ""
}
