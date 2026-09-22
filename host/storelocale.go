package host

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
)

// Every store keys its rows by the canonical locale beside the text, and every
// lookup asks in that form. Rows a store wrote before it normalized locales are
// keyed by whatever spelling the recipe or a caller used at the time, and no
// lookup finds them again: the leverage they hold reads as absent, which is
// indistinguishable from content nobody has translated.
//
// What clears it depends on which of the project's two databases holds the
// rows. The projection is a reading of the working tree, so deleting it and
// running the loop again rebuilds every row in it. The context store holds
// terms, approved wording and voice profiles, which exist there and nowhere
// else, so its rows are keyed back where they stand and nothing is thrown away.
// The commands that measure the project say so once and name the verb.

// WarnStoreLocaleDrift prints one stderr line when the project's stores hold
// rows keyed by a locale spelling that is not canonical, naming the rows and
// what clears them. A project with no store yet has nothing to audit and the
// check opens none: a status must not create one.
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
// which rows carry which spelling, and what clears them in the pool they are
// in.
func StoreLocaleDriftWarning(drift []projectdb.LocaleDrift, layout project.Layout) string {
	var authored, derived []string
	for _, d := range drift {
		if d.Pool == projectdb.PoolProjection {
			derived = append(derived, d.String())
			continue
		}
		authored = append(authored, d.String())
	}

	var b strings.Builder
	b.WriteString("warning: this project holds rows keyed by a locale spelling no lookup asks for. They are never matched.")
	if len(authored) > 0 {
		fmt.Fprintf(&b, " In the context store (%s): run `kapi context locales --fix` to key them canonically where they stand.",
			strings.Join(authored, "; "))
	}
	if len(derived) > 0 {
		fmt.Fprintf(&b, " In the projection (%s): delete %s, then run `kapi up`, which reads your files again.",
			strings.Join(derived, "; "), projectionName(layout))
	}
	return b.String()
}

// projectionName is the projection's path as a reader of the project
// recognises it.
func projectionName(layout project.Layout) string {
	store := layout.StorePath()
	if rel, err := filepath.Rel(layout.Root, store); err == nil {
		return filepath.ToSlash(rel)
	}
	return store
}

// StoreLocales reports the locale spellings a project's stored rows are keyed
// by, and what a re-key did to the ones the project authored.
type StoreLocales struct {
	// Drift is every non-canonical spelling found after the pass: nothing for a
	// store every lookup can read, and for a pass that was asked to fix them,
	// only what it could not move.
	Drift []projectdb.LocaleDrift `json:"drift,omitempty"`
	// Rekeyed is what the context store's rows were keyed to. Empty unless the
	// caller asked for the fix.
	Rekeyed []projectdb.LocaleRekey `json:"rekeyed,omitempty"`
	// Projection is the file to delete for the rows a rebuild clears, relative
	// to the project. Empty when no row in it drifted.
	Projection string `json:"projection,omitempty"`
}

// Clean reports that every row is keyed by the spelling lookups ask in.
func (r StoreLocales) Clean() bool { return len(r.Drift) == 0 }

// FormatText renders the report for a reader.
func (r StoreLocales) FormatText(w io.Writer) error {
	for _, done := range r.Rekeyed {
		if _, err := fmt.Fprintf(w, "%s\n", done.String()); err != nil {
			return err
		}
	}
	if r.Clean() {
		_, err := fmt.Fprintln(w, "Every row is keyed by the locale its lookups ask for.")
		return err
	}
	if _, err := fmt.Fprintln(w, "Rows keyed by a locale spelling no lookup asks for:"); err != nil {
		return err
	}
	for _, d := range r.Drift {
		if _, err := fmt.Fprintf(w, "  %s\n", d.String()); err != nil {
			return err
		}
	}
	if r.Projection != "" {
		if _, err := fmt.Fprintf(w,
			"Delete %s and run `kapi up` to rebuild what kapi read out of your files.\n", r.Projection); err != nil {
			return err
		}
	}
	return nil
}

// ProjectStoreLocales reports the locale spellings the project's stored rows
// are keyed by, and with fix set keys the context store's rows to the
// canonical one.
//
// The fix moves authored rows and deletes none: a row whose canonical spelling
// is free is keyed by it, a row saying exactly what the canonical row says is
// folded into it, and a row the canonical spelling already answers differently
// is left where it is and reported. Rows in the projection are a reading of the
// working tree and are rebuilt rather than moved, so the report names the file
// to delete instead.
func (a *App) ProjectStoreLocales(ctx context.Context, projectPath string, fix bool) (StoreLocales, error) {
	var res StoreLocales

	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return res, err
	}
	if fix {
		if res.Rekeyed, err = db.RekeyContextLocales(ctx); err != nil {
			return res, err
		}
		// The content memory's search indexes carry the locale beside each
		// variant they were built from, so a move leaves them answering for a
		// spelling no row has any more.
		for _, done := range res.Rekeyed {
			if done.Subsystem == "content memory" && done.Rekeyed() {
				a.RebuildMemorySearchIndexes(ctx, db.Memory())
				break
			}
		}
	}
	if res.Drift, err = db.NonCanonicalLocales(ctx); err != nil {
		return res, err
	}
	for _, d := range res.Drift {
		if d.Pool == projectdb.PoolProjection {
			res.Projection = projectionName(layout)
			break
		}
	}
	return res, nil
}
