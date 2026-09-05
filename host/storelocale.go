package host

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
)

// Every store keys its rows by the canonical locale beside the text, and every
// lookup asks in that form. Rows a store wrote before it normalized locales are
// keyed by whatever spelling the recipe or a caller used at the time, and no
// lookup finds them again: the leverage they hold reads as absent, which is
// indistinguishable from content nobody has translated. The stores are
// projections of committed sources, so the remedy is a rebuild rather than a
// migration; the commands that measure the project say so, once, and name it.

// WarnStoreLocaleDrift prints one stderr line when the project's store holds
// rows keyed by a locale spelling that is not canonical, naming the rows and
// how to rebuild the store. A project with no store yet has nothing to audit
// and the check opens none: a status must not create one.
func (a *App) WarnStoreLocaleDrift(cmd Command, projectPath string) {
	if a.Quiet || !projectStoreExists(projectPath) {
		return
	}
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return
	}
	db, err := a.ProjectDB(CmdContext(cmd), layout.Root)
	if err != nil || db == nil {
		return
	}
	drift, err := db.NonCanonicalLocales(CmdContext(cmd))
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not audit the project store's locales: %v\n", err)
		return
	}
	if len(drift) == 0 {
		return
	}
	fmt.Fprintln(cmd.ErrOrStderr(), StoreLocaleDriftWarning(drift, layout))
}

// StoreLocaleDriftWarning is the line a command prints for the drift it found:
// which rows carry which spelling, and the rebuild that re-derives the store
// from its committed sources while keeping the staged decisions only the store
// holds.
func StoreLocaleDriftWarning(drift []projectdb.LocaleDrift, layout project.Layout) string {
	parts := make([]string, 0, len(drift))
	for _, d := range drift {
		parts = append(parts, d.String())
	}
	store := layout.StorePath()
	if rel, err := filepath.Rel(layout.Root, store); err == nil {
		store = filepath.ToSlash(rel)
	}
	return fmt.Sprintf("warning: the project store holds rows keyed by a locale spelling no lookup asks for (%s). "+
		"They are never matched. Rebuild the store from the committed sources: run `kapi commit` to write "+
		"staged decisions, delete %s, then run `kapi up`.", strings.Join(parts, "; "), store)
}
