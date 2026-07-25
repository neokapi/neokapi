package host

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

func (a *App) OpenTermsSQLite(cmd Command) (terms.Terminology, string, error) {
	if a.TermsBackend != nil {
		return a.TermsBackend, "(in-memory)", nil
	}
	dbPath, err := a.ResolveTermsCmdPath(cmd)
	if err != nil {
		return nil, "", err
	}
	tb, err := terms.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, dbPath, fmt.Errorf("open terms: %w", err)
	}
	return tb, dbPath, nil
}

// ResolveTermsCmdPath picks the SQLite terms file a `kapi terms`
// subcommand operates on. An explicit --name/--file/--local flag always wins.
// Otherwise, when run inside a .kapi project, it defaults to the project's bound
// terms (defaults.termbase, else <root>/.kapi/termbase.db) so that
// `kapi terms lookup`/`import` see the same glossary that `kapi check --ship` and
// `kapi term-check` enforce — without it, a lookup inside a project silently
// hit an empty ./termbase.db. Falls back to ./termbase.db outside a project.
func (a *App) ResolveTermsCmdPath(cmd Command) (string, error) {
	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	file, _ := cmd.Flags().GetString("file")
	if name != "" || file != "" || local {
		return resolveResourcePath(cmd, "termbases", "termbase.db")
	}
	if p, err := a.resolveProjectTermsPath(cmd); err == nil && p != "" {
		return p, nil
	}
	return resolveResourcePath(cmd, "termbases", "termbase.db")
}

// ResolveTermsFileFormat maps the --format flag to a terms store file format
// name. An unset flag lets a native extension (.ktb bundle, .ktz archive) win
// over the caller's default, so `kapi terms import seeds/termbase.ktb` needs
// no --format.
func ResolveTermsFileFormat(flag, path string, explicit bool) string {
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
// wipe-and-reseed from a committed .ktb reproduces the terms store exactly.
func ImportKTBFile(ctx context.Context, tb terms.Terminology, r io.Reader) (int, error) {
	file, err := ktb.Decode(r)
	if err != nil {
		return 0, fmt.Errorf("parse ktb: %w", err)
	}
	return addConcepts(ctx, tb, file)
}

// ImportKTZFile imports a .ktz archive — the compressed single-member container
// around a .ktb bundle. Semantics are identical to ImportKTBFile. The container
// needs random access, so the whole archive is read before decoding.
func ImportKTZFile(ctx context.Context, tb terms.Terminology, r io.Reader) (int, error) {
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

func addConcepts(ctx context.Context, tb terms.Terminology, file *ktb.File) (int, error) {
	for _, c := range file.Concepts {
		if err := tb.AddConcept(ctx, c); err != nil {
			return 0, fmt.Errorf("add concept %s: %w", c.ID, err)
		}
	}
	return len(file.Concepts), nil
}

// ExportKTB writes the whole terms as a deterministic, lossless .ktb
// document — the native form for committing a terms store to git.
func ExportKTB(ctx context.Context, tb terms.Terminology, w io.Writer) error {
	return exportTerms(ctx, tb, w, ktb.Marshal)
}

// ExportKTZ writes the whole terms as a .ktz archive: the same .ktb bytes
// inside the compressed single-member container.
func ExportKTZ(ctx context.Context, tb terms.Terminology, w io.Writer) error {
	return exportTerms(ctx, tb, w, ktb.MarshalArchive)
}

func exportTerms(ctx context.Context, tb terms.Terminology, w io.Writer, marshal func(*ktb.File) ([]byte, error)) error {
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return fmt.Errorf("list concepts: %w", err)
	}
	data, err := marshal(ktb.FromConcepts(concepts))
	if err != nil {
		return fmt.Errorf("marshal terms bundle: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write terms bundle: %w", err)
	}
	return nil
}
