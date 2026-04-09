package cli

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/spf13/cobra"
)

// NewTMCmd creates the tm command group.
func (a *App) NewTMCmd() *cobra.Command {
	tmCmd := &cobra.Command{
		Use:     "tm",
		Short:   "Manage translation memory",
		GroupID: "management",
		Long: `Manage translation memory.

A translation memory (TM) stores previously translated segments as a SQLite
database. Use these commands to import/export TMX, look up matches, and manage
TM entries.

Resource location (mutually exclusive):
  --name <n>      Named TM in KAPI_HOME (~/.config/kapi/tm/<n>.db)
  --local         TM in current directory (./tm.db)
  --file <path>   Explicit file path

Default (no flag): same as --local (uses ./tm.db).`,
	}

	importCmd := a.newTMImportCmd()
	importDirCmd := a.newTMImportDirCmd()
	exportCmd := a.newTMExportCmd()
	lookupCmd := a.newTMLookupCmd()
	searchCmd := a.newTMSearchCmd()
	statsCmd := a.newTMStatsCmd()
	listCmd := a.newTMListCmd()

	for _, cmd := range []*cobra.Command{importCmd, importDirCmd, exportCmd, lookupCmd, searchCmd, statsCmd} {
		AddResourceFlags(cmd)
	}

	tmCmd.AddCommand(importCmd, importDirCmd, exportCmd, lookupCmd, searchCmd, statsCmd, listCmd)
	return tmCmd
}

func (a *App) openTMSQLite(cmd *cobra.Command) (*sievepen.SQLiteTM, string, error) {
	dbPath, err := ResolveResourcePath(cmd, "tm", "tm.db")
	if err != nil {
		return nil, "", err
	}
	tm, err := sievepen.NewSQLiteTM(dbPath)
	if err != nil {
		return nil, dbPath, fmt.Errorf("open TM: %w", err)
	}
	return tm, dbPath, nil
}

func (a *App) newTMImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import a TMX file into translation memory",
		Long: `Import a single TMX file (plain or .gz) into the TM.

By default, imports entries matching the given --source-locale and --target-locale.
Use --all-pairs to emit entries for every (src, tgt) language pair present in
each TU — useful for multilingual TMX files (e.g. EUR-Lex Euramis exports where
a single TU may contain 24+ languages). Combine with --locales to restrict the
pair set (e.g. --all-pairs --locales en-GB,fr-FR,de-DE).

The importer auto-detects UTF-8/UTF-16 from the BOM, so Euramis exports work
without pre-conversion. For web-crawl TMX sets (bitextor output) the per-TUV
<prop type="source-document"> URL is recorded as Origin.Reference.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			allPairs, _ := cmd.Flags().GetBool("all-pairs")
			localesRaw, _ := cmd.Flags().GetString("locales")

			tm, dbPath, err := a.openTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			count, err := importTMXFile(tm, args[0], srcLocale, tgtLocale, allPairs, parseLocaleList(localesRaw))
			if err != nil {
				return err
			}

			if a.Quiet {
				return nil
			}
			return output.Print(cmd, output.TMImportOutput{
				Imported: count,
				DBPath:   dbPath,
				Total:    tm.Count(),
			})
		},
	}

	cmd.Flags().StringP("source-locale", "s", "en", "source locale")
	cmd.Flags().StringP("target-locale", "t", "", "target locale")
	cmd.Flags().Bool("all-pairs", false, "emit entries for every (src,tgt) pair present in each TU (multilingual TMX)")
	cmd.Flags().String("locales", "", "comma-separated locale subset for --all-pairs (empty = all languages in file)")

	return cmd
}

func (a *App) newTMImportDirCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import-dir [directory]",
		Short: "Import all TMX files from a directory into translation memory",
		Long: `Walk a directory and import every matching TMX file into the TM.

Auto-detects plain .tmx and gzipped .tmx.gz files. The filename (without path)
becomes the Origin.Key on each imported entry so you can trace which file a
segment came from.

By default, imports entries matching --source-locale and --target-locale from
every file. Use --pattern to filter (glob against filename) and --all-pairs to
emit the full language cross-product from multilingual files.

Examples:
  kapi tm import-dir ./tmx --name corpus --source-locale en --target-locale nb --pattern "*en-nb*"
  kapi tm import-dir ./eurlex --name corpus --all-pairs --locales en-GB,fr-FR,de-DE`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			allPairs, _ := cmd.Flags().GetBool("all-pairs")
			localesRaw, _ := cmd.Flags().GetString("locales")
			pattern, _ := cmd.Flags().GetString("pattern")
			recursive, _ := cmd.Flags().GetBool("recursive")

			tm, dbPath, err := a.openTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			dir := args[0]
			info, err := os.Stat(dir)
			if err != nil {
				return fmt.Errorf("stat directory: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", dir)
			}

			locales := parseLocaleList(localesRaw)

			files, err := listTMXFiles(dir, pattern, recursive)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return fmt.Errorf("no TMX files found in %s", dir)
			}

			var totalImported int
			var failed int
			for i, path := range files {
				rel, _ := filepath.Rel(dir, path)
				if !a.Quiet {
					fmt.Fprintf(os.Stderr, "[%d/%d] %s ", i+1, len(files), rel)
				}
				n, err := importTMXFile(tm, path, srcLocale, tgtLocale, allPairs, locales)
				if err != nil {
					failed++
					if !a.Quiet {
						fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
					}
					continue
				}
				totalImported += n
				if !a.Quiet {
					fmt.Fprintf(os.Stderr, "%d entries\n", n)
				}
			}

			if a.Quiet {
				return nil
			}
			fmt.Fprintf(os.Stderr, "\nDone. %d files processed (%d failed), %d entries imported, TM now has %d entries\n",
				len(files), failed, totalImported, tm.Count())
			return output.Print(cmd, output.TMImportOutput{
				Imported: totalImported,
				DBPath:   dbPath,
				Total:    tm.Count(),
			})
		},
	}

	cmd.Flags().StringP("source-locale", "s", "en", "source locale")
	cmd.Flags().StringP("target-locale", "t", "", "target locale")
	cmd.Flags().Bool("all-pairs", false, "emit entries for every (src,tgt) pair present in each TU")
	cmd.Flags().String("locales", "", "comma-separated locale subset for --all-pairs")
	cmd.Flags().StringP("pattern", "p", "", "filename glob to filter (default: all .tmx and .tmx.gz)")
	cmd.Flags().BoolP("recursive", "r", false, "recurse into subdirectories")

	return cmd
}

// parseLocaleList parses a comma-separated locale list, trimming whitespace.
func parseLocaleList(raw string) []model.LocaleID {
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

// importTMXFile imports a single TMX file (plain or .gz) into the TM.
// Uses ImportTMXLocalePairs when allPairs is true, otherwise single-pair import.
func importTMXFile(tm *sievepen.SQLiteTM, path, srcLocale, tgtLocale string, allPairs bool, locales []model.LocaleID) (int, error) {
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
	}

	if allPairs {
		return sievepen.ImportTMXLocalePairs(tm, reader, locales, opts)
	}
	return sievepen.ImportTMXWithOptions(tm, reader,
		model.LocaleID(srcLocale), model.LocaleID(tgtLocale), opts)
}

// listTMXFiles returns all .tmx and .tmx.gz files in dir matching pattern.
func listTMXFiles(dir, pattern string, recursive bool) ([]string, error) {
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

func (a *App) newTMExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export translation memory to TMX",
		RunE: func(cmd *cobra.Command, args []string) error {
			outputPath, _ := cmd.Flags().GetString("output")
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")

			tm, _, err := a.openTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			w := os.Stdout
			if outputPath != "" {
				w, err = os.Create(outputPath)
				if err != nil {
					return fmt.Errorf("create output: %w", err)
				}
				defer w.Close()
			}

			if err := sievepen.ExportTMX(tm, w, model.LocaleID(srcLocale), model.LocaleID(tgtLocale)); err != nil {
				return fmt.Errorf("export TMX: %w", err)
			}

			if !a.Quiet && outputPath != "" {
				return output.Print(cmd, output.TMExportOutput{
					Count:      tm.Count(),
					OutputPath: outputPath,
				})
			}
			return nil
		},
	}

	cmd.Flags().StringP("output", "o", "", "output file (default: stdout)")
	cmd.Flags().StringP("source-locale", "s", "en", "source locale")
	cmd.Flags().StringP("target-locale", "t", "", "target locale")

	return cmd
}

func (a *App) newTMLookupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lookup [text]",
		Short: "Look up text in translation memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			minScore, _ := cmd.Flags().GetFloat64("min-score")
			maxResults, _ := cmd.Flags().GetInt("max-results")

			tm, _, err := a.openTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			opts := sievepen.LookupOptions{
				MinScore:   minScore,
				MaxResults: maxResults,
			}

			matches, err := tm.LookupText(args[0], model.LocaleID(srcLocale), model.LocaleID(tgtLocale), opts)
			if err != nil {
				return fmt.Errorf("lookup: %w", err)
			}

			entries := make([]output.TMLookupEntry, len(matches))
			for i, m := range matches {
				entries[i] = output.TMLookupEntry{
					Source:    m.Entry.SourceText(),
					Target:    m.Entry.TargetText(),
					Score:     m.Score,
					MatchType: string(m.MatchType),
					EntryID:   m.Entry.ID,
				}
			}

			return output.Print(cmd, output.TMLookupOutput{
				Matches: entries,
				Total:   len(entries),
			})
		},
	}

	cmd.Flags().StringP("source-locale", "s", "en", "source locale")
	cmd.Flags().StringP("target-locale", "t", "", "target locale")
	cmd.Flags().Float64("min-score", 0.7, "minimum match score (0.0-1.0)")
	cmd.Flags().Int("max-results", 10, "maximum results to return")

	return cmd
}

func (a *App) newTMSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search translation memory entries",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			limit, _ := cmd.Flags().GetInt("limit")

			tm, _, err := a.openTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			entries, total := tm.SearchEntries(args[0], srcLocale, tgtLocale, 0, limit)

			results := make([]output.TMSearchEntry, len(entries))
			for i, e := range entries {
				results[i] = output.TMSearchEntry{
					ID:             e.ID,
					Source:         e.SourceText(),
					Target:         e.TargetText(),
					SourceLanguage: string(e.SourceLocale),
					TargetLanguage: string(e.TargetLocale),
				}
			}

			return output.Print(cmd, output.TMSearchOutput{
				Entries: results,
				Total:   total,
				Shown:   len(results),
			})
		},
	}

	cmd.Flags().StringP("source-locale", "s", "", "filter by source locale")
	cmd.Flags().StringP("target-locale", "t", "", "filter by target locale")
	cmd.Flags().Int("limit", 25, "max results")

	return cmd
}

func (a *App) newTMStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show translation memory statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, dbPath, err := a.openTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			// Get locale pair counts by scanning entries.
			localePairs := make(map[string]int)
			entries := tm.Entries()
			for _, e := range entries {
				pair := string(e.SourceLocale) + " → " + string(e.TargetLocale)
				localePairs[pair]++
			}

			return output.Print(cmd, output.TMStatsOutput{
				DBPath:      dbPath,
				Entries:     tm.Count(),
				LocalePairs: localePairs,
			})
		},
	}
}

func (a *App) newTMListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List named TMs in KAPI_HOME",
		RunE: func(cmd *cobra.Command, args []string) error {
			resources, err := ListNamedResources("tm")
			if err != nil {
				return fmt.Errorf("list TMs: %w", err)
			}

			entries := make([]output.ResourceListEntry, len(resources))
			for i, r := range resources {
				entries[i] = output.ResourceListEntry{
					Name:     r.Name,
					Path:     r.Path,
					Size:     r.Size,
					Modified: r.Modified,
				}
			}

			return output.Print(cmd, output.ResourceListOutput{
				Kind:      "tm",
				Resources: entries,
				Total:     len(entries),
			})
		},
	}
}
