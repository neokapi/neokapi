package host

import (
	"context"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/project"
)

// The loop's basis lives in the project's context. Every document a run writes
// is recorded in the block history with the source each translation was made
// from (host/loopbasis.go), and the history is part of the workspace log, which
// a checkout reads from the project's context backend with kapi context sync.
// A fresh clone holds the translations the repository carries and none of that
// record: until it pulls the context, a translation whose source was edited
// before this checkout's first pass is not graded stale here, because nothing
// on this machine says which source it was made from.
//
// kapi up and kapi status say so in one line when the project shares its
// context through a backend, the translations exist on disk and the block
// history records nothing. A project whose context stays on this machine has
// no record to pull, and its passes record the translations they write.

// HistoryNotPulledNote is the line kapi up and kapi status print for a
// checkout whose translations exist and whose block history records none.
const HistoryNotPulledNote = "This checkout holds no record of the source its translations were made from; " +
	"run `kapi context sync` to read the project's context before a pass grades them"

// historyNotPulled reports whether any translation the units name exists on
// disk while the project's block history records no change at all, in a
// project that shares its context through a backend.
func (a *App) historyNotPulled(ctx context.Context, root string, units []VerifyUnit) bool {
	if root == "" {
		return false
	}
	for _, u := range units {
		if u.TargetPath == "" {
			continue
		}
		if fi, err := os.Stat(u.TargetPath); err == nil && fi.Mode().IsRegular() {
			return !a.newLoopWrites(ctx, root).recorded() && a.sharesContext(ctx, root)
		}
	}
	return false
}

// sharesContext reports whether the project at root shares its context
// through the backend its recipe declares: whether kapi context sync has
// somewhere to read it from.
func (a *App) sharesContext(_ context.Context, root string) bool {
	layout, err := project.LayoutFor(root)
	if err != nil {
		return false
	}
	declared, err := recipeContextBackend(layout.RecipePath)
	if err != nil {
		return false
	}
	return declared.Shared()
}

// noteHistoryNotPulled prints HistoryNotPulledNote on the command's error
// stream, for a run that is about to grade the project's translations.
func (a *App) noteHistoryNotPulled(cmd Command, root string, units []VerifyUnit) {
	if a.Quiet || a.printOps != nil {
		return
	}
	if a.historyNotPulled(CmdContext(cmd), root, units) {
		fmt.Fprintln(cmd.ErrOrStderr(), "note: "+HistoryNotPulledNote)
	}
}
