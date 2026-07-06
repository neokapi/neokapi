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
	"github.com/neokapi/neokapi/sievepen/klftm"
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
// extension) to a TM file format name. Anything that is not .klftm is
// treated as TMX, matching the historical default.
func ResolveTMFileFormat(flag, path string) string {
	switch strings.ToLower(flag) {
	case "", "auto":
		if strings.HasSuffix(strings.ToLower(path), ".klftm") {
			return "klftm"
		}
		return "tmx"
	default:
		return strings.ToLower(flag)
	}
}

// ImportKLFTMFile imports a native .klftm document. Entries keep their
// serialized identity (BulkAddWithStream upserts by entry ID), and any
// import sessions recorded in the file are recreated when absent, so a
// wipe-and-reseed produces a byte-identical TM state.
func ImportKLFTMFile(ctx context.Context, tm sievepen.TMStore, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	file, err := klftm.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}

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

// markupTokenRe matches KLF runtime-projection markup tokens ({=m0},
// {/=m0}) serialized as literal text.
var markupTokenRe = regexp.MustCompile(`\{/?=m\d+\}`)

// WarnSuspectTokenEntries scans the TM for entries whose variants disagree
// on their markup-token sets — e.g. a plain-text source paired with a
// target carrying {=m0} tokens. Such entries bake one format's runtime
// projection into format-neutral text: matched from another surface, the
// tokens leak verbatim into the output (the class behind the docs
// "{=m0} Installer" leak). The durable fix is run-structured entries;
// until then, importing one earns a warning.
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
	var suspects []string
	for i := range entries {
		e := &entries[i]
		var first string
		firstSet := false
		mismatch := false
		for locale := range e.Variants {
			set := tokenSet(e.VariantText(locale))
			if !firstSet {
				first, firstSet = set, true
				continue
			}
			if set != first {
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
	fmt.Fprintf(out, "Warning: %d TM entr%s with markup tokens ({=mN}) in some variants but not others — format-specific projections in format-neutral text leak into other surfaces (e.g. %s). Store these entries run-structured instead.\n",
		len(suspects), map[bool]string{true: "y", false: "ies"}[len(suspects) == 1], strings.Join(show, ", "))
}
