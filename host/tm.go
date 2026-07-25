package host

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/sievepen/kmb"
)

func (a *App) OpenTMSQLite(cmd Command) (sievepen.TMStore, string, error) {
	if a.TMBackend != nil {
		return a.TMBackend, "(in-memory)", nil
	}
	dbPath, err := a.ResolveTMCmdPath(cmd)
	if err != nil {
		return nil, "", err
	}
	tm, err := sievepen.NewSQLiteTM(dbPath)
	if err != nil {
		return nil, dbPath, fmt.Errorf("open TM: %w", err)
	}
	return tm, dbPath, nil
}

// ResolveTMCmdPath picks the SQLite TM file a `kapi tm` subcommand operates on.
// An explicit --name/--file/--local flag always wins. Otherwise, when run inside
// a .kapi project, it defaults to the project's authoritative TM
// (<projectRoot>/.kapi/tm.db) so that `kapi tm lookup`/`import`/`stats` see the
// same TM that `kapi extract` pre-fills from and `kapi merge` writes back to —
// without it, those commands silently hit an empty ./tm.db. Falls back to
// ./tm.db outside a project. This mirrors ResolveTermbaseCmdPath.
func (a *App) ResolveTMCmdPath(cmd Command) (string, error) {
	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	file, _ := cmd.Flags().GetString("file")
	if name != "" || file != "" || local {
		return resolveResourcePath(cmd, "tm", "tm.db")
	}
	if p, err := a.resolveProjectTMPath(cmd); err == nil && p != "" {
		return p, nil
	}
	return resolveResourcePath(cmd, "tm", "tm.db")
}

// resolveProjectTMPath returns the authoritative TM path for the .kapi project
// in scope, or "" (with nil error) when no project can be located. Unlike the
// termbase (which can be re-bound via defaults.termbase), the project TM is
// always the conventional <projectRoot>/.kapi/tm.db — the same file
// kapi extract and kapi merge use (see cli/extract.go and cli/merge.go).
func (a *App) resolveProjectTMPath(cmd Command) (string, error) {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil {
		return "", err
	}
	if projectPath == "" {
		return "", nil
	}
	root := filepath.Dir(projectPath)
	return filepath.Join(root, project.StateDirName, "tm.db"), nil
}

// ResolveTMFileFormat maps the --format flag (or, for "auto", the file
// extension) to a TM file format name. Anything that is neither .kmb nor .kmz
// is treated as TMX, matching the historical default.
func ResolveTMFileFormat(flag, path string) string {
	switch strings.ToLower(flag) {
	case "", "auto":
		switch {
		case strings.HasSuffix(strings.ToLower(path), kmb.Ext):
			return "kmb"
		case strings.HasSuffix(strings.ToLower(path), kmb.ArchiveExt):
			return "kmz"
		}
		return "tmx"
	default:
		return strings.ToLower(flag)
	}
}

// ImportKMBFile imports a native .kmb document. Entries keep their
// serialized identity (BulkAddWithStream upserts by entry ID), and any
// import sessions recorded in the file are recreated when absent, so a
// wipe-and-reseed produces a byte-identical TM state.
func ImportKMBFile(ctx context.Context, tm sievepen.TMStore, path string) (int, error) {
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

// ImportKMZFile imports a .kmz archive — the compressed single-member container
// around a .kmb bundle. Semantics are identical to ImportKMBFile.
func ImportKMZFile(ctx context.Context, tm sievepen.TMStore, path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	file, err := kmb.UnmarshalArchive(data)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return importKMB(ctx, tm, path, file)
}

func importKMB(ctx context.Context, tm sievepen.TMStore, path string, file *kmb.File) (int, error) {
	entries := file.ModelEntries()
	if len(entries) > 0 {
		if bulk, ok := tm.(sievepen.BulkAdder); ok {
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

// ExportKMB writes the whole TM as a deterministic, lossless .kmb document —
// the native form for committing a TM to git and for re-seeding one exactly.
func ExportKMB(ctx context.Context, tm sievepen.TMStore, w io.Writer) error {
	return exportTM(ctx, tm, w, kmb.Marshal)
}

// ExportKMZ writes the whole TM as a .kmz archive: the same .kmb bytes inside
// the compressed single-member container.
func ExportKMZ(ctx context.Context, tm sievepen.TMStore, w io.Writer) error {
	return exportTM(ctx, tm, w, kmb.MarshalArchive)
}

func exportTM(ctx context.Context, tm sievepen.TMStore, w io.Writer, marshal func(*kmb.File) ([]byte, error)) error {
	entries, err := tm.Entries(ctx)
	if err != nil {
		return fmt.Errorf("list TM entries: %w", err)
	}
	sessions, err := tm.ListImportSessions(ctx)
	if err != nil {
		return fmt.Errorf("list import sessions: %w", err)
	}
	data, err := marshal(kmb.FromModel(entries, sessions))
	if err != nil {
		return fmt.Errorf("marshal TM bundle: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write TM bundle: %w", err)
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
			out = append(out, model.LocaleID(p))
		}
	}
	return out
}

// ImportTMXFile imports a single TMX file (plain or .gz) into the TM.
// Uses ImportTMXLocalePairs when allPairs is true, otherwise single-pair import.
func ImportTMXFile(ctx context.Context, tm sievepen.TMStore, path, srcLocale, tgtLocale string, allPairs bool, locales []model.LocaleID) (int, error) {
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

	opts := sievepen.ImportTMXOptions{
		OriginKey:     filepath.Base(path),
		OriginAddedBy: "kapi tm import",
		WarnFunc: func(msg string) {
			fmt.Fprintf(os.Stderr, "warning: %s\n", msg)
		},
	}

	if allPairs {
		return sievepen.ImportTMXLocalePairs(ctx, tm, reader, locales, opts)
	}
	return sievepen.ImportTMXWithOptions(ctx, tm, reader,
		model.LocaleID(srcLocale), model.LocaleID(tgtLocale), opts)
}

// RebuildTMSearchIndexes restores the FTS5 search + fuzzy side-tables after a
// bulk TMX import. ImportTMXWithOptions / ImportTMXLocalePairs use the bulk
// add path, which deliberately skips per-row FTS5 inserts (the dominant cost on
// large corpora), leaving tm_variant_search / tm_variant_trigram empty until
// they are rebuilt set-wise here. Without this, imported entries are invisible
// to `kapi tm search` and fuzzy lookup even though exact lookup still works.
// RebuildSearchIndex / RebuildFuzzyIndex are SQLite-specific; in-memory
// backends keep their indexes live and skip this step.
func (a *App) RebuildTMSearchIndexes(tm sievepen.TMStore) {
	sq, ok := tm.(*sievepen.SQLiteTM)
	if !ok {
		return
	}
	if !a.Quiet {
		fmt.Fprintln(os.Stderr, "Rebuilding search index...")
	}
	if err := sq.RebuildSearchIndex(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: rebuild search index: %v\n", err)
	}
	if !a.Quiet {
		fmt.Fprintln(os.Stderr, "Rebuilding fuzzy index...")
	}
	if err := sq.RebuildFuzzyIndex(); err != nil {
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
func WarnSuspectTokenEntries(ctx context.Context, tm sievepen.TMStore, out io.Writer) {
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
