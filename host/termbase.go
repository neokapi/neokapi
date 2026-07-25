package host

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/termbase"
	"github.com/neokapi/neokapi/termbase/ktb"
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

// ResolveTermbaseFileFormat maps the --format flag to a termbase file format
// name. An unset flag lets a native extension (.ktb bundle, .ktz archive) win
// over the caller's default, so `kapi termbase import seeds/termbase.ktb` needs
// no --format.
func ResolveTermbaseFileFormat(flag, path string, explicit bool) string {
	if explicit {
		return strings.ToLower(flag)
	}
	switch lower := strings.ToLower(path); {
	case strings.HasSuffix(lower, ktb.Ext):
		return "ktb"
	case strings.HasSuffix(lower, ktb.ArchiveExt):
		return "ktz"
	}
	return strings.ToLower(flag)
}

// ImportKTBFile imports a native .ktb document. Concepts keep their
// serialized identity (AddConcept upserts by concept ID), so a
// wipe-and-reseed from a committed .ktb reproduces the termbase exactly.
func ImportKTBFile(ctx context.Context, tb termbase.TermBase, r io.Reader) (int, error) {
	file, err := ktb.Decode(r)
	if err != nil {
		return 0, fmt.Errorf("parse ktb: %w", err)
	}
	return addConcepts(ctx, tb, file)
}

// ImportKTZFile imports a .ktz archive — the compressed single-member container
// around a .ktb bundle. Semantics are identical to ImportKTBFile. The container
// needs random access, so the whole archive is read before decoding.
func ImportKTZFile(ctx context.Context, tb termbase.TermBase, r io.Reader) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read ktz: %w", err)
	}
	file, err := ktb.UnmarshalArchive(data)
	if err != nil {
		return 0, fmt.Errorf("parse ktz: %w", err)
	}
	return addConcepts(ctx, tb, file)
}

func addConcepts(ctx context.Context, tb termbase.TermBase, file *ktb.File) (int, error) {
	for _, c := range file.Concepts {
		if err := tb.AddConcept(ctx, c); err != nil {
			return 0, fmt.Errorf("add concept %s: %w", c.ID, err)
		}
	}
	return len(file.Concepts), nil
}

// ExportKTB writes the whole termbase as a deterministic, lossless .ktb
// document — the native form for committing a termbase to git.
func ExportKTB(ctx context.Context, tb termbase.TermBase, w io.Writer) error {
	return exportTermbase(ctx, tb, w, ktb.Marshal)
}

// ExportKTZ writes the whole termbase as a .ktz archive: the same .ktb bytes
// inside the compressed single-member container.
func ExportKTZ(ctx context.Context, tb termbase.TermBase, w io.Writer) error {
	return exportTermbase(ctx, tb, w, ktb.MarshalArchive)
}

func exportTermbase(ctx context.Context, tb termbase.TermBase, w io.Writer, marshal func(*ktb.File) ([]byte, error)) error {
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return fmt.Errorf("list concepts: %w", err)
	}
	data, err := marshal(ktb.FromConcepts(concepts))
	if err != nil {
		return fmt.Errorf("marshal termbase bundle: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write termbase bundle: %w", err)
	}
	return nil
}
