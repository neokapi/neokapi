package host

import (
	"context"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/termbase"
	"github.com/neokapi/neokapi/termbase/klftb"
)

func (a *App) OpenTermbaseSQLite(cmd Command) (termbase.TermBase, string, error) {
	if a.TBBackend != nil {
		return a.TBBackend, "(in-memory)", nil
	}
	dbPath, err := a.ResolveTermbaseCmdPath(cmd)
	if err != nil {
		return nil, "", err
	}
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	if err != nil {
		return nil, dbPath, fmt.Errorf("open termbase: %w", err)
	}
	return tb, dbPath, nil
}

// ResolveTermbaseCmdPath picks the SQLite termbase file a `kapi termbase`
// subcommand operates on. An explicit --name/--file/--local flag always wins.
// Otherwise, when run inside a .kapi project, it defaults to the project's bound
// termbase (defaults.termbase, else <root>/.kapi/termbase.db) so that
// `kapi termbase lookup`/`import` see the same glossary that `kapi check --ship` and
// `kapi term-check` enforce — without it, a lookup inside a project silently
// hit an empty ./termbase.db. Falls back to ./termbase.db outside a project.
func (a *App) ResolveTermbaseCmdPath(cmd Command) (string, error) {
	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	file, _ := cmd.Flags().GetString("file")
	if name != "" || file != "" || local {
		return resolveResourcePath(cmd, "termbases", "termbase.db")
	}
	if p, err := a.resolveProjectTermbasePath(cmd); err == nil && p != "" {
		return p, nil
	}
	return resolveResourcePath(cmd, "termbases", "termbase.db")
}

// ImportKLFTBFile imports a native .klftb document. Concepts keep their
// serialized identity (AddConcept upserts by concept ID), so a
// wipe-and-reseed from a committed .klftb reproduces the termbase exactly.
func ImportKLFTBFile(ctx context.Context, tb termbase.TermBase, r io.Reader) (int, error) {
	file, err := klftb.Decode(r)
	if err != nil {
		return 0, fmt.Errorf("parse klftb: %w", err)
	}
	for _, c := range file.Concepts {
		if err := tb.AddConcept(ctx, c); err != nil {
			return 0, fmt.Errorf("add concept %s: %w", c.ID, err)
		}
	}
	return len(file.Concepts), nil
}

// ExportKLFTB writes the whole termbase as a deterministic, lossless .klftb
// document — the native form for committing a termbase to git.
func ExportKLFTB(ctx context.Context, tb termbase.TermBase, w io.Writer) error {
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return fmt.Errorf("list concepts: %w", err)
	}
	data, err := klftb.Marshal(klftb.FromConcepts(concepts))
	if err != nil {
		return fmt.Errorf("marshal klftb: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write klftb: %w", err)
	}
	return nil
}
