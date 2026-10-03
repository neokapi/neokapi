package host

import (
	"context"
	"fmt"
	"os"
)

// The loop's basis lives in the project's context. Every document a run writes
// is recorded in the block history with the source each translation was made
// from (host/loopbasis.go), and the history is part of the workspace log, which
// a checkout reads from the project's context backend with kapi context pull.
// A fresh clone holds the translations the repository carries and none of that
// record: until it pulls the context, a translation whose source was edited
// before this checkout's first pass is not graded stale here, because nothing
// on this machine says which source it was made from.
//
// kapi up and kapi status say so in one line when the translations exist on
// disk and the block history records nothing.

// HistoryNotPulledNote is the line kapi up and kapi status print for a
// checkout whose translations exist and whose block history records none.
const HistoryNotPulledNote = "This checkout holds no record of the source its translations were made from; " +
	"run `kapi context pull` to read the project's context before a pass grades them"

// historyNotPulled reports whether any translation the units name exists on
// disk while the project's block history records no change at all.
func (a *App) historyNotPulled(ctx context.Context, root string, units []VerifyUnit) bool {
	if root == "" {
		return false
	}
	for _, u := range units {
		if u.TargetPath == "" {
			continue
		}
		if fi, err := os.Stat(u.TargetPath); err == nil && fi.Mode().IsRegular() {
			return !a.newLoopWrites(ctx, root).recorded()
		}
	}
	return false
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
