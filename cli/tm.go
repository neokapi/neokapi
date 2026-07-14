package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/sievepen/klftm"
	"github.com/spf13/cobra"
)

// NewTMCmd creates the tm command group.
func NewTMCmd(a *App) *cobra.Command {
	tmCmd := &cobra.Command{
		Use:     "tm",
		Short:   "Manage translation memory",
		GroupID: "assets",
		Long: `Manage translation memory.

A translation memory (TM) stores previously translated segments as a SQLite
database. Use these commands to import/export TMX, look up matches, and manage
TM entries.

Resource location (mutually exclusive):
  --name <n>      Named TM in KAPI_HOME (~/.config/kapi/tm/<n>.db)
  --local         TM in current directory (./tm.db)
  --file <path>   Explicit file path

Default (no flag): same as --local (uses ./tm.db).`,
		Example: `  kapi tm stats
  kapi tm lookup "welcome back" -s en -t fr
  kapi tm import corpus.tmx -s en -t fr`,
	}

	importCmd := newTMImportCmd(a)
	importDirCmd := newTMImportDirCmd(a)
	exportCmd := newTMExportCmd(a)
	lookupCmd := newTMLookupCmd(a)
	searchCmd := newTMSearchCmd(a)
	statsCmd := newTMStatsCmd(a)
	auditCmd := newTMAuditCmd(a)
	listCmd := newTMListCmd(a)
	sessionsCmd := newTMSessionsCmd(a, tmCmd)

	for _, cmd := range []*cobra.Command{importCmd, importDirCmd, exportCmd, lookupCmd, searchCmd, statsCmd} {
		AddResourceFlags(cmd)
	}

	tmCmd.AddCommand(importCmd, importDirCmd, exportCmd, lookupCmd, searchCmd, statsCmd, auditCmd, listCmd, sessionsCmd)
	return tmCmd
}

func newTMImportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import a TMX or .klftm file into translation memory",
		Long: `Import a single TMX file (plain or .gz) or a native .klftm file into the TM.

klftm is the KLF-family native TM form: a deterministic, lossless JSON
serialization that round-trips every TMEntry field (entity mappings,
provenance origins, properties, notes) that TMX drops. Importing a .klftm
preserves entry identity verbatim, so re-seeding a TM from committed .klftm
files is fully reproducible. The format is selected by file extension;
--format overrides.

By default, imports entries matching the given --source-locale and --target-locale.
Use --all-pairs to emit entries for every (src, tgt) language pair present in
each TU — useful for multilingual TMX files (e.g. EUR-Lex Euramis exports where
a single TU may contain 24+ languages). Combine with --locales to restrict the
pair set (e.g. --all-pairs --locales en-GB,fr-FR,de-DE).

The importer auto-detects UTF-8/UTF-16 from the BOM, so Euramis exports work
without pre-conversion. For web-crawl TMX sets (bitextor output) the per-TUV
<prop type="source-document"> URL is recorded as Origin.Reference.`,
		Example: `  kapi tm import corpus.tmx -s en -t fr
  kapi tm import corpus.tmx --all-pairs --locales en,fr,de --name my-tm
  kapi tm import seeds/builtins-nb.klftm`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			allPairs, _ := cmd.Flags().GetBool("all-pairs")
			localesRaw, _ := cmd.Flags().GetString("locales")
			format, _ := cmd.Flags().GetString("format")

			tm, dbPath, err := a.OpenTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			var count int
			switch ResolveTMFileFormat(format, args[0]) {
			case "klftm":
				count, err = ImportKLFTMFile(cmd.Context(), tm, args[0])
			case "tmx":
				count, err = ImportTMXFile(cmd.Context(), tm, args[0], srcLocale, tgtLocale, allPairs, ParseLocaleList(localesRaw))
			default:
				return fmt.Errorf("unsupported format: %s (use tmx or klftm)", format)
			}
			if err != nil {
				return err
			}

			// ImportTMXFile uses the bulk path, which skips per-row FTS5
			// inserts. Rebuild the search/fuzzy side-tables so imported
			// entries are visible to `kapi tm search` and fuzzy lookup —
			// matching what import-dir does after its bulk load.
			a.RebuildTMSearchIndexes(tm)

			if a.Quiet {
				return nil
			}
			WarnSuspectTokenEntries(cmd.Context(), tm, cmd.ErrOrStderr())
			tmTotal, err := tm.Count(cmd.Context())
			if err != nil {
				return fmt.Errorf("count TM entries: %w", err)
			}
			return output.Print(cmd, output.TMImportOutput{
				Imported: count,
				DBPath:   dbPath,
				Total:    tmTotal,
			})
		},
	}

	cmd.Flags().StringP("source-locale", "s", "en", "source locale")
	cmd.Flags().StringP("target-locale", "t", "", "target locale")
	cmd.Flags().Bool("all-pairs", false, "emit entries for every (src,tgt) pair present in each TU (multilingual TMX)")
	cmd.Flags().String("locales", "", "comma-separated locale subset for --all-pairs (empty = all languages in file)")
	cmd.Flags().String("format", "auto", "input format (auto, tmx, klftm); auto selects by file extension")

	return cmd
}

func newTMImportDirCmd(a *App) *cobra.Command {
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

			tm, dbPath, err := a.OpenTMSQLite(cmd)
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
			a.RebuildTMSearchIndexes(tm)

			if a.Quiet {
				return nil
			}
			WarnSuspectTokenEntries(cmd.Context(), tm, cmd.ErrOrStderr())
			tmTotal, err := tm.Count(cmd.Context())
			if err != nil {
				return fmt.Errorf("count TM entries: %w", err)
			}
			fmt.Fprintf(os.Stderr, "\nDone. %d files processed (%d failed), %d entries imported, TM now has %d entries\n",
				len(files), failed, totalImported, tmTotal)
			return output.Print(cmd, output.TMImportOutput{
				Imported: totalImported,
				DBPath:   dbPath,
				Total:    tmTotal,
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

func newTMExportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export translation memory to TMX or .klftm",
		Long: `Export the TM to TMX 1.4 (default) or the native .klftm form.

TMX: each entry is written as a single <tu> with one <tuv> per language
variant present (or the subset requested via --locales). Inline markup is
preserved as TMX <ph>/<bpt>/<ept>/<it>/<hi>. TMX is the lossy interchange
tier — entity mappings, provenance origins, properties, and notes are
dropped.

klftm (--format klftm, or a -o path ending in .klftm): the deterministic,
lossless native serialization — the right form for committing a TM to git
and for seeding a fresh TM exactly. --locales does not apply.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputPath, _ := cmd.Flags().GetString("output")
			localesRaw, _ := cmd.Flags().GetString("locales")
			format, _ := cmd.Flags().GetString("format")

			tm, _, err := a.OpenTMSQLite(cmd)
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

			switch ResolveTMFileFormat(format, outputPath) {
			case "klftm":
				entries, err := tm.Entries(cmd.Context())
				if err != nil {
					return fmt.Errorf("list TM entries: %w", err)
				}
				sessions, err := tm.ListImportSessions(cmd.Context())
				if err != nil {
					return fmt.Errorf("list import sessions: %w", err)
				}
				data, err := klftm.Marshal(klftm.FromModel(entries, sessions))
				if err != nil {
					return fmt.Errorf("marshal klftm: %w", err)
				}
				if _, err := w.Write(data); err != nil {
					return fmt.Errorf("write klftm: %w", err)
				}
			case "tmx":
				locales := ParseLocaleList(localesRaw)
				if err := sievepen.ExportTMX(cmd.Context(), tm, w, locales); err != nil {
					return fmt.Errorf("export TMX: %w", err)
				}
			default:
				return fmt.Errorf("unsupported format: %s (use tmx or klftm)", format)
			}

			if !a.Quiet && outputPath != "" {
				total, err := tm.Count(cmd.Context())
				if err != nil {
					return fmt.Errorf("count TM entries: %w", err)
				}
				return output.Print(cmd, output.TMExportOutput{
					Count:      total,
					OutputPath: outputPath,
				})
			}
			return nil
		},
	}

	cmd.Flags().StringP("output", "o", "", "output file (default: stdout)")
	cmd.Flags().String("locales", "", "comma-separated locale subset (default: all variants present)")
	cmd.Flags().String("format", "auto", "output format (auto, tmx, klftm); auto selects by the -o extension")

	return cmd
}

func newTMLookupCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lookup [text]",
		Short: "Look up text in translation memory",
		Example: `  kapi tm lookup "welcome back" -s en -t fr
  kapi tm lookup "save" -s en -t de --min-score 0.8`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			minScore, _ := cmd.Flags().GetFloat64("min-score")
			maxResults, _ := cmd.Flags().GetInt("max-results")

			tm, _, err := a.OpenTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			opts := sievepen.LookupOptions{
				MinScore:   minScore,
				MaxResults: maxResults,
			}

			matches, err := tm.LookupText(cmd.Context(), args[0], model.LocaleID(srcLocale), model.LocaleID(tgtLocale), opts)
			if err != nil {
				return fmt.Errorf("lookup: %w", err)
			}

			entries := make([]output.TMLookupEntry, len(matches))
			srcLoc := model.LocaleID(srcLocale)
			tgtLoc := model.LocaleID(tgtLocale)
			for i, m := range matches {
				entries[i] = output.TMLookupEntry{
					Source:    m.Entry.VariantText(srcLoc),
					Target:    m.Entry.VariantText(tgtLoc),
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

func newTMSearchCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search translation memory entries",
		Example: `  kapi tm search "dashboard" -s en -t fr
  kapi tm search "settings" --limit 5`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			limit, _ := cmd.Flags().GetInt("limit")

			tm, _, err := a.OpenTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			entries, total, err := tm.SearchEntries(cmd.Context(), sievepen.SearchParams{
				Query:         args[0],
				AnyLocale:     srcLocale,
				RequireLocale: tgtLocale,
				Limit:         limit,
			})
			if err != nil {
				return fmt.Errorf("search: %w", err)
			}

			results := make([]output.TMSearchEntry, len(entries))
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
				results[i] = output.TMSearchEntry{
					ID:             e.ID,
					Source:         e.VariantText(actualSrc),
					Target:         e.VariantText(actualTgt),
					SourceLanguage: string(actualSrc),
					TargetLanguage: string(actualTgt),
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

func newTMStatsCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:     "stats",
		Short:   "Show translation memory statistics",
		Example: "  kapi tm stats\n  kapi tm stats --name my-tm",
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, dbPath, err := a.OpenTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

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
				return fmt.Errorf("count TM entries: %w", err)
			}
			return output.Print(cmd, output.TMStatsOutput{
				DBPath:      dbPath,
				Entries:     total,
				LocalePairs: localePairs,
			})
		},
	}
}

func newTMListCmd(a *App) *cobra.Command {
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

// newTMSessionsCmd groups the import-session subcommands.
func newTMSessionsCmd(a *App, _ *cobra.Command) *cobra.Command {
	sessionsCmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage TMX import sessions",
		Long: `An import session is created every time a TMX file is imported.
Each session records the file's SHA-256 hash, TMX header metadata, and the
number of entries imported. Origins on TM entries point back to the session
that added them so you can filter the TM by import source.`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all import sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, _, err := a.OpenTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()
			sessions, err := tm.ListImportSessions(cmd.Context())
			if err != nil {
				return fmt.Errorf("list import sessions: %w", err)
			}
			if len(sessions) == 0 {
				if !a.Quiet {
					fmt.Fprintln(os.Stdout, "No import sessions.")
				}
				return nil
			}
			t := output.NewTable(os.Stdout).Accent(0).
				Headers("SESSION", "FILE", "TOOL", "ENTRIES", "IMPORTED")
			for _, s := range sessions {
				t.Rowf(TruncateID(s.ID, 12), s.FileKey, s.ToolName, s.EntryCount,
					s.ImportedAt.Format("2006-01-02 15:04"))
			}
			t.Render()
			return nil
		},
	}

	showCmd := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show details for a single import session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, _, err := a.OpenTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()
			s, ok, err := tm.GetImportSession(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get import session: %w", err)
			}
			if !ok {
				return fmt.Errorf("session not found: %s", args[0])
			}
			fmt.Fprintf(os.Stdout, "ID:               %s\n", s.ID)
			fmt.Fprintf(os.Stdout, "File key:         %s\n", s.FileKey)
			fmt.Fprintf(os.Stdout, "File hash:        %s\n", s.FileHash)
			fmt.Fprintf(os.Stdout, "File size:        %d bytes\n", s.FileSizeBytes)
			fmt.Fprintf(os.Stdout, "Imported at:      %s\n", s.ImportedAt.Format("2006-01-02 15:04:05 MST"))
			fmt.Fprintf(os.Stdout, "Imported by:      %s\n", s.ImportedBy)
			fmt.Fprintf(os.Stdout, "Tool:             %s %s\n", s.ToolName, s.ToolVersion)
			fmt.Fprintf(os.Stdout, "Segment type:     %s\n", s.SegType)
			fmt.Fprintf(os.Stdout, "Admin language:   %s\n", s.AdminLang)
			fmt.Fprintf(os.Stdout, "Source language:  %s\n", s.SrcLang)
			fmt.Fprintf(os.Stdout, "Data type:        %s\n", s.DataType)
			fmt.Fprintf(os.Stdout, "Original format:  %s\n", s.OriginalFormat)
			fmt.Fprintf(os.Stdout, "Original encoding:%s\n", s.OriginalEncoding)
			fmt.Fprintf(os.Stdout, "Entry count:      %d\n", s.EntryCount)
			if len(s.Properties) > 0 {
				fmt.Fprintln(os.Stdout, "Properties:")
				for k, v := range s.Properties {
					fmt.Fprintf(os.Stdout, "  %s = %s\n", k, v)
				}
			}
			return nil
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete <session-id>",
		Short: "Remove a session record (entries are retained, session_id cleared)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tm, _, err := a.OpenTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()
			if err := tm.DeleteImportSession(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("delete session: %w", err)
			}
			if !a.Quiet {
				fmt.Fprintf(os.Stdout, "Deleted session %s.\n", args[0])
			}
			return nil
		},
	}

	for _, c := range []*cobra.Command{listCmd, showCmd, deleteCmd} {
		AddResourceFlags(c)
	}
	sessionsCmd.AddCommand(listCmd, showCmd, deleteCmd)
	return sessionsCmd
}
