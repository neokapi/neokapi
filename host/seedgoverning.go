package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
)

// Compiling a content-memory bundle can restore a unit's governing fingerprint
// from kmb.Origin.ContextFP to state.UnitState.GoverningFingerprint. Conversely,
// the absorber copies that fingerprint from the decision record into the bundle
// when learning a committed target.
//
// Restoration updates only an existing record whose translation hash matches the
// bundle's wording and whose fingerprint is empty. Existing fingerprints are
// preserved. Bundles carry content and context, so they cannot create decision
// records on their own.

// compileMemoryBundle imports one committed content-memory bundle into the
// project store and carries each entry's governing context back onto the
// decision record for the unit it answers. It returns the number of entries the
// bundle carried.
func (a *App) compileMemoryBundle(ctx context.Context, db *projectdb.DB, root, path string) (int, error) {
	tm := db.Memory()
	if tm == nil {
		return 0, fmt.Errorf("compile content memory: %w", projectdb.ErrNoStore)
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	file, err := kmb.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	n, err := importKMB(ctx, tm, path, file)
	if err != nil {
		return 0, err
	}
	if err := a.restoreGoverningFingerprints(ctx, db.Work(), root, file.ModelEntries()); err != nil {
		return n, fmt.Errorf("carry the governing context of %s onto the decision record: %w", filepath.Base(path), err)
	}
	return n, nil
}

// restoreGoverningFingerprints writes each entry's governing fingerprint onto
// the decision record rows for the unit and locales it answers, where the row
// holds none and describes the entry's translation, and makes the rows durable
// when any changed. It reports how many rows it wrote.
//
// The row is keyed by the document the unit belongs to, which the entry names
// through its origins: the source file each absorbed target was paired with,
// resolved to the document's durable key the way every other writer of the
// record resolves it.
func (a *App) restoreGoverningFingerprints(ctx context.Context, st *state.WorkStore, root string, entries []memory.Entry) error {
	if st == nil || root == "" || len(entries) == 0 {
		return nil
	}
	docs := a.documentIndexOrEmpty(ctx, root)
	restored := 0
	for i := range entries {
		e := &entries[i]
		fingerprint := memory.LatestFingerprint(e)
		if e.Unit == "" || fingerprint == "" {
			continue
		}
		for _, scope := range entryScopes(docs, root, e) {
			for locale := range e.Variants {
				if locale == e.HintSrcLang {
					continue
				}
				row, ok := st.Get(ctx, state.Key{Scope: scope, Unit: e.Unit, Variant: model.Variant(locale)})
				if !ok || row.GoverningFingerprint != "" {
					continue
				}
				if row.TargetHash == "" || row.TargetHash != state.TargetHash(e.VariantText(locale)) {
					continue // the row is about a different answer
				}
				row.GoverningFingerprint = fingerprint
				// Recorded rather than staged: nothing here is a decision
				// somebody owes a review, and a row already staged keeps that
				// flag.
				if err := st.Record(ctx, row); err != nil {
					return err
				}
				restored++
			}
		}
	}
	if restored == 0 {
		return nil
	}
	return st.PersistRecords(ctx)
}

// entryScopes resolves the documents an entry's answers were recorded in: the
// durable key of every source file its origins reference. An entry with no
// origin naming a source file (an import, a hand-added pair) answers no
// document and is skipped.
func entryScopes(docs DocumentIndex, root string, e *memory.Entry) []string {
	var out []string
	seen := map[string]bool{}
	for _, o := range e.Origins {
		if o.Reference == "" {
			continue
		}
		scope := docs.Scope(root, filepath.Join(root, filepath.FromSlash(o.Reference)))
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out
}
