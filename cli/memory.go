package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/memory"
	"github.com/spf13/cobra"
)

// NewMemoryCmd creates the memory command group.
func NewMemoryCmd(a *App) *cobra.Command {
	memoryCmd := &cobra.Command{
		Use:     "memory",
		Short:   "Manage content memory",
		GroupID: "assets",
		Long: `Manage content memory.

A content memory stores previously produced content as a SQLite database: source
segments with the targets settled for them. Use these commands to import/export
TMX, look up matches, and manage entries.

Inside a project, no flag means the project's own content memory, a subsystem of
.kapi/work/store.db, which is why it is addressed by project rather than by path.
Use -p to name the project explicitly.

Standalone store instead (mutually exclusive):
  --name <n>      Named content memory in the kapi config dir (memory/<n>.db)
  --local         Content memory in current directory (./memory.db)
  --file <path>   Explicit file path

Outside a project and with no flag: same as --local (uses ./memory.db).`,
		Example: `  kapi memory stats
  kapi memory lookup "welcome back" -s en -t fr
  kapi memory import corpus.tmx -s en -t fr`,
	}

	importCmd := newMemoryImportCmd(a)
	importDirCmd := newMemoryImportDirCmd(a)
	exportCmd := newMemoryExportCmd(a)
	lookupCmd := newMemoryLookupCmd(a)
	searchCmd := newMemorySearchCmd(a)
	statsCmd := newMemoryStatsCmd(a)
	auditCmd := newMemoryAuditCmd(a)
	listCmd := newMemoryListCmd(a)
	sessionsCmd := newMemorySessionsCmd(a, memoryCmd)

	for _, cmd := range []*cobra.Command{importCmd, importDirCmd, exportCmd, lookupCmd, searchCmd, statsCmd} {
		AddResourceFlags(cmd)
	}
	// The project's content memory is a subsystem of `.kapi/work/store.db`, not a
	// file a caller can name — so with no resource flag these resolve the
	// project, and -p is how you say WHICH, exactly as for every other
	// project-aware verb. It also matters under KAPI_NO_PROJECT, where the
	// upward walk is off and an explicit -p is the only way in.
	// import-dir is left out: its -p is already --pattern, and pflag panics at
	// init on a redefined shorthand rather than failing a test.
	for _, cmd := range []*cobra.Command{importCmd, exportCmd, lookupCmd, searchCmd, statsCmd, auditCmd} {
		AddProjectFlag(cmd)
	}

	memoryCmd.AddCommand(importCmd, importDirCmd, exportCmd, lookupCmd, searchCmd, statsCmd, auditCmd, listCmd, sessionsCmd)
	return memoryCmd
}

func newMemoryImportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import a TMX file or a native .memory.json bundle into content memory",
		Long: `Import a single TMX file (plain or .gz) or a native .memory.json
bundle into the content memory.

The bundle is the native content-memory form: a deterministic, lossless JSON
serialization that round-trips every Entry field (entity mappings, provenance
origins, properties, notes) that TMX drops. Importing one preserves entry
identity verbatim, so re-seeding a content memory from committed bundles is
fully reproducible. The format is selected by file extension; --format
overrides.

By default, imports entries matching the given --source-locale and --target-locale.
Use --all-pairs to emit entries for every (src, tgt) language pair present in
each TU. Multilingual TMX files need this (e.g. EUR-Lex Euramis exports, where
a single TU may carry every official EU language). Combine with --locales to
restrict the pair set (e.g. --all-pairs --locales en-GB,fr,de).

The importer auto-detects UTF-8/UTF-16 from the BOM, so Euramis exports work
without pre-conversion. For web-crawl TMX sets (bitextor output) the per-TUV
<prop type="source-document"> URL is recorded as Origin.Reference.`,
		Example: `  kapi memory import corpus.tmx -s en -t fr
  kapi memory import corpus.tmx --all-pairs --locales en,fr,de --name my-tm
  kapi memory import seeds/builtins-nb.memory.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			allPairs, _ := cmd.Flags().GetBool("all-pairs")
			localesRaw, _ := cmd.Flags().GetString("locales")
			format, _ := cmd.Flags().GetString("format")

			inputFormat, err := ResolveMemoryImportFormat(format, args[0])
			if err != nil {
				return err
			}

			tm, dbPath, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()

			var count int
			switch inputFormat {
			case "bundle":
				count, err = ImportKMBFile(cmd.Context(), tm, args[0])
			case "tmx":
				count, err = ImportTMXFile(cmd.Context(), tm, args[0], srcLocale, tgtLocale, allPairs, ParseLocaleList(localesRaw))
			default:
				return fmt.Errorf("unsupported format: %s (use tmx or bundle)", inputFormat)
			}
			if err != nil {
				return err
			}

			// ImportTMXFile uses the bulk path, which skips per-row FTS5
			// inserts. Rebuild the search/fuzzy side-tables so imported
			// entries are visible to `kapi memory search` and fuzzy lookup —
			// matching what import-dir does after its bulk load.
			a.RebuildMemorySearchIndexes(cmd.Context(), tm)

			if a.Quiet {
				return nil
			}
			WarnSuspectTokenEntries(cmd.Context(), tm, cmd.ErrOrStderr())
			memoryTotal, err := tm.Count(cmd.Context())
			if err != nil {
				return fmt.Errorf("count content-memory entries: %w", err)
			}
			return output.Print(cmd, output.MemoryImportOutput{
				Imported: count,
				DBPath:   dbPath,
				Total:    memoryTotal,
			})
		},
	}

	cmd.Flags().StringP("source-locale", "s", "en", "source locale")
	cmd.Flags().StringP("target-locale", "t", "", "target locale")
	cmd.Flags().Bool("all-pairs", false, "emit entries for every (src,tgt) pair present in each TU (multilingual TMX)")
	cmd.Flags().String("locales", "", "comma-separated locale subset for --all-pairs (empty = all languages in file)")
	cmd.Flags().String("format", "auto", "input format (auto, tmx, bundle); auto selects by file extension")

	return cmd
}

func newMemoryImportDirCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import-dir [directory]",
		Short: "Import all TMX files from a directory into content memory",
		Long: `Walk a directory and import every matching TMX file into the content memory.

Auto-detects plain .tmx and gzipped .tmx.gz files. The filename (without path)
becomes the Origin.Key on each imported entry so you can trace which file a
segment came from.

By default, imports entries matching --source-locale and --target-locale from
every file. Use --pattern to filter (glob against filename) and --all-pairs to
emit the full language cross-product from multilingual files.

Examples:
  kapi memory import-dir ./tmx --name corpus --source-locale en --target-locale nb --pattern "*en-nb*"
  kapi memory import-dir ./eurlex --name corpus --all-pairs --locales en-GB,fr,de`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			allPairs, _ := cmd.Flags().GetBool("all-pairs")
			localesRaw, _ := cmd.Flags().GetString("locales")
			pattern, _ := cmd.Flags().GetString("pattern")
			recursive, _ := cmd.Flags().GetBool("recursive")

			tm, dbPath, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()

			dir := args[0]
			info, err := os.Stat(dir)
			if err != nil {
				return fmt.Errorf("stat directory: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", dir)
			}

			locales := ParseLocaleList(localesRaw)

			files, err := ListTMXFiles(dir, pattern, recursive)
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
				n, err := ImportTMXFile(cmd.Context(), tm, path, srcLocale, tgtLocale, allPairs, locales)
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

			// Rebuild the FTS5 side-tables once the bulk load is done.
			// The bulk path deliberately skips per-row FTS5 inserts —
			// they're the dominant cost on large corpora — and we restore
			// text-search + fuzzy-match capability here.
			a.RebuildMemorySearchIndexes(cmd.Context(), tm)

			if a.Quiet {
				return nil
			}
			WarnSuspectTokenEntries(cmd.Context(), tm, cmd.ErrOrStderr())
			memoryTotal, err := tm.Count(cmd.Context())
			if err != nil {
				return fmt.Errorf("count content-memory entries: %w", err)
			}
			fmt.Fprintf(os.Stderr, "\nDone. %d files processed (%d failed), %d entries imported, content memory now has %d entries\n",
				len(files), failed, totalImported, memoryTotal)
			return output.Print(cmd, output.MemoryImportOutput{
				Imported: totalImported,
				DBPath:   dbPath,
				Total:    memoryTotal,
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

func newMemoryExportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export content memory to TMX or a native .memory.json bundle",
		Long: `Export the content memory to TMX 1.4 (default) or the native
.memory.json bundle.

TMX: each entry is written as a single <tu> with one <tuv> per language
variant present (or the subset requested via --locales). Inline markup is
preserved as TMX <ph>/<bpt>/<ept>/<it>/<hi>. TMX is the lossy interchange
tier: entity mappings, provenance origins, properties, and notes are
dropped.

bundle (--format bundle, or a -o path ending in .memory.json): the
deterministic, lossless native serialization. It is the form to commit a
content memory to git in, and the form that seeds a fresh one exactly. It stays
plain JSON, so it reviews line by line in a diff. --locales does not apply.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputPath, _ := cmd.Flags().GetString("output")
			localesRaw, _ := cmd.Flags().GetString("locales")
			format, _ := cmd.Flags().GetString("format")

			tm, _, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()

			w := os.Stdout
			if outputPath != "" {
				w, err = os.Create(outputPath)
				if err != nil {
					return fmt.Errorf("create output: %w", err)
				}
				defer w.Close()
			}

			switch ResolveMemoryExportFormat(format, outputPath) {
			case "bundle":
				if err := ExportKMB(cmd.Context(), tm, w); err != nil {
					return err
				}
			case "tmx":
				locales := ParseLocaleList(localesRaw)
				if err := memory.ExportTMX(cmd.Context(), tm, w, locales); err != nil {
					return fmt.Errorf("export TMX: %w", err)
				}
			default:
				return fmt.Errorf("unsupported format: %s (use tmx or bundle)", format)
			}

			if !a.Quiet && outputPath != "" {
				total, err := tm.Count(cmd.Context())
				if err != nil {
					return fmt.Errorf("count content-memory entries: %w", err)
				}
				return output.Print(cmd, output.MemoryExportOutput{
					Count:      total,
					OutputPath: outputPath,
				})
			}
			return nil
		},
	}

	cmd.Flags().StringP("output", "o", "", "output file (default: stdout)")
	cmd.Flags().String("locales", "", "comma-separated locale subset (default: all variants present)")
	cmd.Flags().String("format", "auto", "output format (auto, tmx, bundle); auto selects by the -o extension")

	return cmd
}

func newMemoryLookupCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lookup [text]",
		Short: "Look up text in content memory",
		Example: `  kapi memory lookup "welcome back" -s en -t fr
  kapi memory lookup "save" -s en -t de --min-score 0.8`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			minScore, _ := cmd.Flags().GetFloat64("min-score")
			maxResults, _ := cmd.Flags().GetInt("max-results")

			tm, _, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()

			opts := memory.LookupOptions{
				MinScore:   minScore,
				MaxResults: maxResults,
			}

			matches, err := tm.LookupText(cmd.Context(), args[0], model.LocaleID(srcLocale), model.LocaleID(tgtLocale), opts)
			if err != nil {
				return fmt.Errorf("lookup: %w", err)
			}

			entries := make([]output.MemoryLookupEntry, len(matches))
			srcLoc := model.LocaleID(srcLocale)
			tgtLoc := model.LocaleID(tgtLocale)
			for i, m := range matches {
				entries[i] = output.MemoryLookupEntry{
					Source:    m.Entry.VariantText(srcLoc),
					Target:    m.Entry.VariantText(tgtLoc),
					Score:     m.Score,
					MatchType: string(m.MatchType),
					EntryID:   m.Entry.ID,
				}
			}

			return output.Print(cmd, output.MemoryLookupOutput{
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

func newMemorySearchCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search content memory entries",
		Example: `  kapi memory search "dashboard" -s en -t fr
  kapi memory search "settings" --limit 5`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			limit, _ := cmd.Flags().GetInt("limit")

			tm, _, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()

			entries, total, err := tm.SearchEntries(cmd.Context(), memory.SearchParams{
				Query:         args[0],
				AnyLocale:     srcLocale,
				RequireLocale: tgtLocale,
				Limit:         limit,
			})
			if err != nil {
				return fmt.Errorf("search: %w", err)
			}

			results := make([]output.MemorySearchEntry, len(entries))
			srcLoc := model.LocaleID(srcLocale)
			tgtLoc := model.LocaleID(tgtLocale)
			for i, e := range entries {
				actualSrc := srcLoc
				actualTgt := tgtLoc
				if actualSrc == "" {
					actualSrc = e.HintSrcLang
				}
				if actualTgt == "" {
					for loc := range e.Variants {
						if loc != actualSrc {
							actualTgt = loc
							break
						}
					}
				}
				results[i] = output.MemorySearchEntry{
					ID:             e.ID,
					Source:         e.VariantText(actualSrc),
					Target:         e.VariantText(actualTgt),
					SourceLanguage: string(actualSrc),
					TargetLanguage: string(actualTgt),
				}
			}

			return output.Print(cmd, output.MemorySearchOutput{
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

func newMemoryStatsCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:     "stats",
		Short:   "Show content memory statistics",
		Example: "  kapi memory stats\n  kapi memory stats --name my-tm",
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, dbPath, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()

			// With multilingual entries, report per-locale counts instead of
			// locale pairs. Keep the legacy LocalePairs map populated with
			// a single-locale summary for backward compatibility.
			localePairs := make(map[string]int)
			localeStats, err := tm.LocaleStats(cmd.Context())
			if err != nil {
				return fmt.Errorf("locale stats: %w", err)
			}
			for _, lf := range localeStats {
				localePairs[lf.Locale] = lf.Count
			}

			total, err := tm.Count(cmd.Context())
			if err != nil {
				return fmt.Errorf("count content-memory entries: %w", err)
			}
			return output.Print(cmd, output.MemoryStatsOutput{
				DBPath:      dbPath,
				Entries:     total,
				LocalePairs: localePairs,
			})
		},
	}
}

func newMemoryListCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List named Memories in KAPI_HOME",
		RunE: func(cmd *cobra.Command, args []string) error {
			resources, err := ListNamedResources("memory")
			if err != nil {
				return fmt.Errorf("list Memories: %w", err)
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

// newMemorySessionsCmd groups the import-session subcommands.
func newMemorySessionsCmd(a *App, _ *cobra.Command) *cobra.Command {
	sessionsCmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage TMX import sessions",
		Long: `An import session is created every time a TMX file is imported.
Each session records the file's SHA-256 hash, TMX header metadata, and the
number of entries imported. Origins on content-memory entries point back to the session
that added them so you can filter the content memory by import source.`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all import sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, _, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()
			sessions, err := tm.ListImportSessions(cmd.Context())
			if err != nil {
				return fmt.Errorf("list import sessions: %w", err)
			}
			return output.Print(cmd, MemorySessionListOutput{Sessions: MemorySessionRows(sessions)})
		},
	}

	showCmd := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show details for a single import session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, _, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()
			s, ok, err := tm.GetImportSession(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get import session: %w", err)
			}
			if !ok {
				return fmt.Errorf("session not found: %s", args[0])
			}
			return output.Print(cmd, MemorySessionDetailFrom(s))
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <session-id>",
		Short: "Remove a session record (entries are retained, session_id cleared)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, _, releaseMemory, err := a.OpenMemorySQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseMemory()
			if err := tm.DeleteImportSession(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("delete session: %w", err)
			}
			return output.Print(cmd, MemorySessionDeleteOutput{ID: args[0], Deleted: true})
		},
	}

	for _, c := range []*cobra.Command{listCmd, showCmd, deleteCmd} {
		AddResourceFlags(c)
	}
	sessionsCmd.AddCommand(listCmd, showCmd, deleteCmd)
	return sessionsCmd
}
