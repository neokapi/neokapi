package host

import (
	"context"
	"sort"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
)

// The first meeting between a checkout carrying context files and a store that
// has never held this project's context.
//
// A project's context lives in the user's workspace, and a checkout that
// arrived with `.kapi/terms.json`, `.kapi/voice.yaml`, `.kapi/memory/` or
// `.kapi/state/` carries a copy of someone's. Reading those files would make
// every checkout of the project answer with whatever its branch happens to
// hold, so kapi names them and the command that reads them, and leaves them
// alone. This is the whole of what the files do on their own.

// ContextImportCommand is what a person runs to read a layout into the store.
const ContextImportCommand = "kapi context import"

// ContextFilesNotice names the context files a checkout holds and the command
// that reads them.
type ContextFilesNotice struct {
	// Files are project-relative and sorted. A file outside the project is
	// named by its own path.
	Files []string `json:"files"`
	// Command is what reads them.
	Command string `json:"command"`
}

// Message renders the notice as the one line a surface prints.
func (n ContextFilesNotice) Message() string {
	return "this project carries context files (" + n.list() +
		") and its store holds no context yet. They are read by `" + n.Command +
		"`, and until then nothing in them is in force"
}

// list renders the files, naming at most three and counting the rest.
func (n ContextFilesNotice) list() string {
	const shown = 3
	if len(n.Files) <= shown {
		return joinFiles(n.Files)
	}
	return joinFiles(n.Files[:shown]) + " and " + pluralUnit(len(n.Files)-shown, "other", "others")
}

func joinFiles(files []string) string {
	out := ""
	for i, f := range files {
		if i > 0 {
			out += ", "
		}
		out += f
	}
	return out
}

// ContextFilesUnread reports a checkout holding context files whose project
// store has never held context: what a clone of a project looks like before
// anyone has read its layout in.
//
// It stats the layout and asks the store what it holds. No context file is
// opened, so a checkout carrying one cannot reach an answer through the notice
// either. projectPath is the recipe or the project root; both resolve the way
// `-p` does.
//
// A store that has held anything at all — a concept, a content-memory entry, a
// voice profile, a decision — is a store someone has already read this project
// into, and gets no notice.
func (a *App) ContextFilesUnread(ctx context.Context, projectPath string) (ContextFilesNotice, bool) {
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return ContextFilesNotice{}, false
	}
	// Best-effort: a recipe that will not load still has its conventional
	// files stated, which is the case a person most needs named.
	proj, _ := project.LoadWithOptions(layout.RecipePath, project.LoadOptions{SkipRequiresCheck: true})

	files := contextFilesIn(proj, layout)
	if len(files) == 0 {
		return ContextFilesNotice{}, false
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return ContextFilesNotice{}, false
	}
	held, err := storeHoldsContext(ctx, db)
	if err != nil || held {
		return ContextFilesNotice{}, false
	}
	return ContextFilesNotice{Files: files, Command: ContextImportCommand}, true
}

// contextFilesIn lists the context files a checkout holds, project-relative and
// sorted: the sources a read would compile, and the decision record's shards
// named by their directory, since a person reads them as one record.
func contextFilesIn(proj *project.KapiProject, layout project.Layout) []string {
	sources, err := committedContextSources(proj, layout)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(sources)+1)
	for _, src := range sources {
		files = append(files, src.rel)
	}
	if shards := recordShardsIn(layout.Export().UnitStateDir()); len(shards) > 0 {
		files = append(files, relSlash(layout.Root, layout.Export().UnitStateDir())+"/")
	}
	sort.Strings(files)
	return files
}

// storeHoldsContext reports whether a project's store has ever held context.
//
// Every subsystem's tables exist from the store's first open, so the question
// is a row question: a concept, a content-memory entry, a voice profile or a
// recorded decision. A build with no store for a subsystem answers for the
// ones it has.
func storeHoldsContext(ctx context.Context, db *projectdb.DB) (bool, error) {
	for _, has := range []func(context.Context) (bool, error){
		db.HasTerms, db.HasMemory, db.HasVoice, db.HasDecisions,
	} {
		held, err := has(ctx)
		if err != nil {
			return false, err
		}
		if held {
			return true, nil
		}
	}
	return false, nil
}
