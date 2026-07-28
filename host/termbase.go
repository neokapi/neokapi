package host

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format"
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
// terms (defaults.terms, else <root>/.kapi/terms.db) so that
// `kapi terms lookup`/`import` see the same glossary that `kapi check --ship` and
// `kapi term-check` enforce — without it, a lookup inside a project silently
// hit an empty ./terms.db. Falls back to ./terms.db outside a project.
func (a *App) ResolveTermsCmdPath(cmd Command) (string, error) {
	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	file, _ := cmd.Flags().GetString("file")
	if name != "" || file != "" || local {
		return resolveResourcePath(cmd, "termbases", "terms.db")
	}
	if p, err := a.resolveProjectTermsPath(cmd); err == nil && p != "" {
		return p, nil
	}
	return resolveResourcePath(cmd, "termbases", "terms.db")
}

// TermsFileFormats names the terms file formats, in the order they are offered
// to a user who has to pick one.
var TermsFileFormats = []string{"bundle", "csv", "tsv", "json", "tbx"}

// ResolveTermsImportFormat maps the --format flag (or, for "auto", the file
// extension) to the format `kapi terms import` should read path as.
//
// "auto" identifies the file; it never guesses. Falling back to CSV would not
// even fail loudly: the CSV reader skips every row it cannot turn into a
// concept, so a zip archive imports as "0 concepts" and exits 0 — success-shaped
// output for a file that was never read.
func ResolveTermsImportFormat(flag, path string) (string, error) {
	if name := strings.ToLower(flag); name != "" && name != "auto" {
		return name, nil
	}
	if ktb.IsBundlePath(path) {
		return "bundle", nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return "csv", nil
	case ".tsv":
		return "tsv", nil
	case ".json":
		return "json", nil
	case ".tbx":
		return "tbx", nil
	}
	return "", fmt.Errorf(
		"cannot identify %s: %q is not a terms extension (expected %s, .csv, .tsv, .json or .tbx); pass --format (%s) to say what it is",
		filepath.Base(path), format.Ext(path), ktb.Ext, strings.Join(TermsFileFormats, ", "))
}

// ResolveTermsExportFormat maps the --format flag (or, for "auto", the -o
// extension) to the format `kapi terms export` should write.
//
// Unlike import, "auto" may fall back to a default: an output path says what
// the caller wants written, not what an unread file already is, and stdout
// carries no extension at all. A native bundle path wins over the default even
// when the flag is set, so `kapi terms export -o seeds/terms.json` writes the
// lossless bundle rather than the lossy JSON interchange doc.
func ResolveTermsExportFormat(flag, path string, explicit bool) string {
	if explicit {
		return strings.ToLower(flag)
	}
	if ktb.IsBundlePath(path) {
		return "bundle"
	}
	return strings.ToLower(flag)
}

// ImportKTBFile imports a native .terms.json bundle. Concepts keep their
// serialized identity (AddConcept upserts by concept ID), so a
// wipe-and-reseed from a committed .terms.json reproduces the terms store exactly.
func ImportKTBFile(ctx context.Context, tb terms.Terminology, r io.Reader) (int, error) {
	file, err := ktb.Decode(r)
	if err != nil {
		return 0, fmt.Errorf("parse ktb: %w", err)
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

// ExportKTB writes the whole terms as a deterministic, lossless .terms.json
// bundle — the native form for committing a terms store to git.
func ExportKTB(ctx context.Context, tb terms.Terminology, w io.Writer) error {
	return exportTerms(ctx, tb, w, ktb.Marshal)
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
