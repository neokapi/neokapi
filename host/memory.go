package host

import (
	"compress/gzip"
	"context"
	"fmt"
	"github.com/neokapi/neokapi/core/format"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
)

// OpenMemorySQLite opens the content memory a `kapi memory` subcommand operates
// on, and returns it with a label naming where it came from and a release
// function the caller must defer.
//
// The release function exists because the answer is no longer always a file the
// caller owns: inside a project the store is the App's shared handle, whose
// pool also carries the terms store, the block cache and the working set, so
// closing it would take those with it. Standalone stores close as before.
func (a *App) OpenMemorySQLite(cmd Command) (memory.Store, string, func(), error) {
	noop := func() {}
	if a.MemoryBackend != nil {
		return a.MemoryBackend, "(in-memory)", noop, nil
	}
	sel, err := a.ResolveMemoryStore(cmd)
	if err != nil {
		return nil, "", noop, err
	}
	if sel.InProject() {
		db, err := a.ProjectDB(CmdContext(cmd), sel.Root)
		if err != nil {
			return nil, "", noop, err
		}
		tm := db.Memory()
		if tm == nil {
			return nil, db.Path(), noop, fmt.Errorf("open content memory: %w", projectdb.ErrNoStore)
		}
		return tm, db.Path(), noop, nil
	}
	tm, err := memory.NewSQLiteStore(sel.Path)
	if err != nil {
		return nil, sel.Path, noop, fmt.Errorf("open content memory: %w", err)
	}
	return tm, sel.Path, func() { _ = tm.Close() }, nil
}

// ResolveMemoryStore picks the content memory a `kapi memory` subcommand
// operates on. An explicit --name/--file/--local flag always wins and selects a
// standalone store. Otherwise, inside a project, the project's own store is
// selected, so `kapi memory lookup`/`import`/`stats` see the same content memory
// `kapi extract` pre-fills from and `kapi merge` writes back to — without it,
// those commands silently hit an empty ./memory.db. Outside a project it falls
// back to ./memory.db. This mirrors ResolveTermsStore.
func (a *App) ResolveMemoryStore(cmd Command) (StoreSelection, error) {
	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	file, _ := cmd.Flags().GetString("file")
	if name == "" && file == "" && !local {
		root, err := a.projectRootFor(cmd)
		if err != nil {
			return StoreSelection{}, err
		}
		if root != "" {
			return StoreSelection{Root: root}, nil
		}
	}
	path, err := resolveResourcePath(cmd, "memory", "memory.db")
	if err != nil {
		return StoreSelection{}, err
	}
	return StoreSelection{Path: path}, nil
}

// projectRootFor returns the root of the project in scope, or "" when there is
// none.
func (a *App) projectRootFor(cmd Command) (string, error) {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return "", err
	}
	return filepath.Dir(projectPath), nil
}

// MemoryFileFormats names the content-memory file formats, in the order they
// are offered to a user who has to pick one.
var MemoryFileFormats = []string{"bundle", "tmx"}

// ResolveMemoryImportFormat maps the --format flag (or, for "auto", the file
// extension) to the format `kapi memory import` should read path as.
//
// "auto" identifies the file; it never guesses. An extension it does not
// recognise is an error, because reading an unidentified file as TMX yields
// "Imported 0 entries" — success-shaped output for a file the importer could
// not read at all.
func ResolveMemoryImportFormat(flag, path string) (string, error) {
	if name := strings.ToLower(flag); name != "" && name != "auto" {
		return name, nil
	}
	switch {
	case kmb.IsBundlePath(path):
		return "bundle", nil
	case isTMXPath(path):
		return "tmx", nil
	}
	return "", fmt.Errorf(
		"cannot identify %s: %q is not a content-memory extension (expected %s, .tmx or .tmx.gz); pass --format (%s) to say what it is",
		filepath.Base(path), format.Ext(path), kmb.Ext, strings.Join(MemoryFileFormats, ", "))
}

// ResolveMemoryExportFormat maps the --format flag (or, for "auto", the -o
// extension) to the format `kapi memory export` should write.
//
// Unlike import, "auto" may fall back to a default: an output path says what
// the caller wants written, not what an unread file already is, and stdout
// carries no extension at all. TMX is that default.
func ResolveMemoryExportFormat(flag, path string) string {
	if name := strings.ToLower(flag); name != "" && name != "auto" {
		return name
	}
	if kmb.IsBundlePath(path) {
		return "bundle"
	}
	return "tmx"
}

// isTMXPath reports whether path names a TMX file, plain or gzipped.
func isTMXPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tmx") || strings.HasSuffix(lower, ".tmx.gz")
}

// ImportKMBFile imports a native .memory.json document. Entries keep their
// serialized identity (BulkAddWithStream upserts by entry ID), and any
// import sessions recorded in the file are recreated when absent, so a
// wipe-and-reseed produces a byte-identical content memory state.
func ImportKMBFile(ctx context.Context, tm memory.Store, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	file, err := kmb.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return importKMB(ctx, tm, path, file)
}

func importKMB(ctx context.Context, tm memory.Store, path string, file *kmb.File) (int, error) {
	entries := file.ModelEntries()
	if len(entries) > 0 {
		if bulk, ok := tm.(memory.BulkAdder); ok {
			if err := bulk.BulkAddWithStream(ctx, entries, ""); err != nil {
				return 0, fmt.Errorf("add entries from %s: %w", path, err)
			}
		} else {
			for _, e := range entries {
				if err := tm.AddWithStream(ctx, e, ""); err != nil {
					return 0, fmt.Errorf("add entry %s from %s: %w", e.ID, path, err)
				}
			}
		}
	}
	for _, s := range file.ModelImportSessions() {
		if _, exists, err := tm.GetImportSession(ctx, s.ID); err == nil && !exists {
			if err := tm.CreateImportSession(ctx, s); err != nil {
				return 0, fmt.Errorf("recreate import session %s: %w", s.ID, err)
			}
		}
	}
	return len(entries), nil
}

// ExportKMB writes the whole content memory as a deterministic, lossless
// .memory.json bundle — the native form for committing a content memory to git
// and for re-seeding one exactly.
func ExportKMB(ctx context.Context, tm memory.Store, w io.Writer) error {
	return exportMemory(ctx, tm, w, kmb.Marshal)
}

func exportMemory(ctx context.Context, tm memory.Store, w io.Writer, marshal func(*kmb.File) ([]byte, error)) error {
	entries, err := tm.Entries(ctx)
	if err != nil {
		return fmt.Errorf("list content-memory entries: %w", err)
	}
	sessions, err := tm.ListImportSessions(ctx)
	if err != nil {
		return fmt.Errorf("list import sessions: %w", err)
	}
	data, err := marshal(kmb.FromModel(entries, sessions))
	if err != nil {
		return fmt.Errorf("marshal content memory bundle: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write content memory bundle: %w", err)
	}
	return nil
}

// ParseLocaleList parses a comma-separated locale list, trimming whitespace.
func ParseLocaleList(raw string) []model.LocaleID {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]model.LocaleID, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			// Canonical BCP-47, so `--locales nb_NO,pt_BR` names the same
			// locales as `nb-NO,pt-BR` and keys the same store rows.
			out = append(out, locale.Normalize(model.LocaleID(p)))
		}
	}
	return out
}

// ImportTMXFile imports a single TMX file (plain or .gz) into the content memory.
// Uses ImportTMXLocalePairs when allPairs is true, otherwise single-pair import.
func ImportTMXFile(ctx context.Context, tm memory.Store, path, srcLocale, tgtLocale string, allPairs bool, locales []model.LocaleID) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var reader io.Reader = f
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return 0, fmt.Errorf("gunzip %s: %w", path, err)
		}
		defer gz.Close()
		reader = gz
	}

	opts := memory.ImportTMXOptions{
		OriginKey:     filepath.Base(path),
		OriginAddedBy: "kapi memory import",
		WarnFunc: func(msg string) {
			fmt.Fprintf(os.Stderr, "warning: %s\n", msg)
		},
		// The CLI rebuilds the search/fuzzy side-tables itself — once, after the
		// whole batch (import-dir) or the single file — via
		// RebuildMemorySearchIndexes. Defer the per-session rebuild so a
		// directory import of thousands of files does not rebuild N times.
		SkipIndexRebuild: true,
	}

	if allPairs {
		return memory.ImportTMXLocalePairs(ctx, tm, reader, locales, opts)
	}
	return memory.ImportTMXWithOptions(ctx, tm, reader,
		model.LocaleID(srcLocale), model.LocaleID(tgtLocale), opts)
}

// RebuildMemorySearchIndexes restores the FTS5 search + fuzzy side-tables after a
// bulk TMX import. ImportTMXWithOptions / ImportTMXLocalePairs use the bulk
// add path, which deliberately skips per-row FTS5 inserts (the dominant cost on
// large corpora), leaving tm_variant_search / tm_variant_trigram empty until
// they are rebuilt set-wise here. Without this, imported entries are invisible
// to `kapi memory search` and fuzzy lookup even though exact lookup still works.
// RebuildSearchIndex / RebuildFuzzyIndex are SQLite-specific; in-memory
// backends keep their indexes live and skip this step.
func (a *App) RebuildMemorySearchIndexes(ctx context.Context, tm memory.Store) {
	sq, ok := tm.(*memory.SQLiteStore)
	if !ok {
		return
	}
	ctx = ctxOrBackground(ctx)
	if !a.Quiet {
		fmt.Fprintln(os.Stderr, "Rebuilding search index...")
	}
	if err := sq.RebuildSearchIndex(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: rebuild search index: %v\n", err)
	}
	if !a.Quiet {
		fmt.Fprintln(os.Stderr, "Rebuilding fuzzy index...")
	}
	if err := sq.RebuildFuzzyIndex(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: rebuild fuzzy index: %v\n", err)
	}
}

// ListTMXFiles returns all .tmx and .tmx.gz files in dir matching pattern.
func ListTMXFiles(dir, pattern string, recursive bool) ([]string, error) {
	var files []string
	walk := func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if !recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".tmx") && !strings.HasSuffix(lower, ".tmx.gz") {
			return nil
		}
		if pattern != "" {
			matched, err := filepath.Match(pattern, name)
			if err != nil {
				return fmt.Errorf("invalid pattern %q: %w", pattern, err)
			}
			if !matched {
				return nil
			}
		}
		files = append(files, path)
		return nil
	}
	if err := filepath.WalkDir(dir, walk); err != nil {
		return nil, err
	}
	return files, nil
}

func TruncateID(id string, max int) string {
	if len(id) <= max {
		return id
	}
	return id[:max]
}

// markupTokenRe matches KBF runtime-projection markup tokens ({=m0},
// {/=m0}) serialized as literal text.
var markupTokenRe = regexp.MustCompile(`\{/?=m\d+\}`)

// WarnSuspectTokenEntries scans the TM for entries whose variants disagree on
// their placeholder sets, in either representation:
//
//   - literal markup tokens in the text — e.g. a plain-text source paired with
//     a target carrying {=m0} tokens. Such entries bake one format's runtime
//     projection into format-neutral text: matched from another surface, the
//     tokens leak verbatim into the output (the class behind the docs
//     "{=m0} Installer" leak).
//   - inline-code *runs* — a variant that carries a Ph / PcOpen / PcClose / Sub
//     its peers do not. That is a pair one side of which has already lost a
//     placeholder, so leveraging it can only produce a translation with a hole
//     in it. `recycle` refuses to fill from such an entry (see
//     model.DiffRunCodes), which is a silent coverage loss unless the entry
//     itself is repaired — hence the warning at ingest, where the seed author
//     can still act on it.
//
// The durable fix in both cases is a symmetric, run-structured entry.
func WarnSuspectTokenEntries(ctx context.Context, tm memory.Store, out io.Writer) {
	entries, err := tm.Entries(ctx)
	if err != nil {
		return
	}
	tokenSet := func(text string) string {
		toks := markupTokenRe.FindAllString(text, -1)
		slices.Sort(toks)
		return strings.Join(toks, " ")
	}
	codeSet := func(runs []model.Run) string {
		counts := model.RunCodeCounts(runs)
		sigs := make([]string, 0, len(counts))
		for sig, n := range counts {
			sigs = append(sigs, fmt.Sprintf("%s*%d", sig, n))
		}
		slices.Sort(sigs)
		return strings.Join(sigs, " ")
	}
	var suspects []string
	for i := range entries {
		e := &entries[i]
		var firstTokens, firstCodes string
		firstSet := false
		mismatch := false
		for locale, runs := range e.Variants {
			tokens, codes := tokenSet(e.VariantText(locale)), codeSet(runs)
			if !firstSet {
				firstTokens, firstCodes, firstSet = tokens, codes, true
				continue
			}
			if tokens != firstTokens || codes != firstCodes {
				mismatch = true
				break
			}
		}
		if mismatch {
			suspects = append(suspects, e.ID)
		}
	}
	if len(suspects) == 0 {
		return
	}
	slices.Sort(suspects)
	show := suspects
	if len(show) > 5 {
		show = show[:5]
	}
	fmt.Fprintf(out, "Warning: %d TM entr%s whose variants disagree on their placeholder set — markup tokens ({=mN}) or inline-code runs present in some variants but not others (e.g. %s). Leveraging such an entry would drop a placeholder, so recycle refuses to fill from it. Store these entries run-structured and symmetric.\n",
		len(suspects), map[bool]string{true: "y", false: "ies"}[len(suspects) == 1], strings.Join(show, ", "))
}
