package host

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// OpenTermsSQLite opens the terms store a `kapi terms` subcommand operates on,
// and returns it with a label naming where it came from and a release function
// the caller must defer. See OpenMemorySQLite for why the release function
// exists.
func (a *App) OpenTermsSQLite(cmd Command) (terms.Terminology, string, func(), error) {
	noop := func() {}
	if a.TermsBackend != nil {
		return a.TermsBackend, "(in-memory)", noop, nil
	}
	sel, err := a.ResolveTermsCmdStore(cmd)
	if err != nil {
		return nil, "", noop, err
	}
	if sel.InProject() {
		db, err := a.ProjectDB(CmdContext(cmd), sel.Root)
		if err != nil {
			return nil, "", noop, err
		}
		tb := db.Terms()
		if tb == nil {
			return nil, db.Path(), noop, fmt.Errorf("open terms: %w", projectdb.ErrNoStore)
		}
		return tb, db.Path(), noop, nil
	}
	tb, err := terms.NewSQLiteStore(sel.Path)
	if err != nil {
		return nil, sel.Path, noop, fmt.Errorf("open terms: %w", err)
	}
	return tb, sel.Path, func() { _ = tb.Close() }, nil
}

// ResolveTermsCmdStore picks the terms store a `kapi terms` subcommand operates
// on. An explicit --name/--file/--local flag always wins and selects a
// standalone store; otherwise the project resolution applies (ResolveTermsStore)
// so `kapi terms lookup`/`import` see the same vocabulary `kapi check --ship`
// and `kapi term-check` enforce. Outside a project it falls back to ./terms.db.
func (a *App) ResolveTermsCmdStore(cmd Command) (StoreSelection, error) {
	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	file, _ := cmd.Flags().GetString("file")
	if name == "" && file == "" && !local {
		sel, err := a.ResolveTermsStore(cmd, "")
		if err != nil {
			return StoreSelection{}, err
		}
		if sel.Bound() {
			return sel, nil
		}
	}
	path, err := resolveResourcePath(cmd, "terms", "terms.db")
	if err != nil {
		return StoreSelection{}, err
	}
	return StoreSelection{Path: path}, nil
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
