package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/spf13/cobra"
)

// NewTermbaseCmd creates the termbase command group.
func (a *App) NewTermbaseCmd() *cobra.Command {
	tbCmd := &cobra.Command{
		Use:     "termbase",
		Short:   "Manage terminology",
		GroupID: "management",
		Long: `Manage project terminology.

A termbase is a glossary of approved terms stored as a SQLite database.
Use these commands to import, export, look up, and manage terms.

Resource location (mutually exclusive):
  --name <n>      Named termbase in KAPI_HOME (~/.config/kapi/termbases/<n>.db)
  --local         Termbase in current directory (./termbase.db)
  --file <path>   Explicit file path

Default (no flag): same as --local (uses ./termbase.db).`,
		Example: `  kapi termbase stats
  kapi termbase lookup "dashboard" -s en -t fr
  kapi termbase import glossary.csv -s en -t fr`,
	}

	importCmd := a.newTermbaseImportCmd()
	exportCmd := a.newTermbaseExportCmd()
	lookupCmd := a.newTermbaseLookupCmd()
	searchCmd := a.newTermbaseSearchCmd()
	statsCmd := a.newTermbaseStatsCmd()
	listCmd := a.newTermbaseListCmd()

	// Shared resource flags for all subcommands (except list).
	for _, cmd := range []*cobra.Command{importCmd, exportCmd, lookupCmd, searchCmd, statsCmd} {
		AddResourceFlags(cmd)
	}

	tbCmd.AddCommand(importCmd, exportCmd, lookupCmd, searchCmd, statsCmd, listCmd)
	return tbCmd
}

func (a *App) openTermbaseSQLite(cmd *cobra.Command) (termbase.TermBase, string, error) {
	if a.TBBackend != nil {
		return a.TBBackend, "(in-memory)", nil
	}
	dbPath, err := a.resolveTermbaseCmdPath(cmd)
	if err != nil {
		return nil, "", err
	}
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	if err != nil {
		return nil, dbPath, fmt.Errorf("open termbase: %w", err)
	}
	return tb, dbPath, nil
}

// resolveTermbaseCmdPath picks the SQLite termbase file a `kapi termbase`
// subcommand operates on. An explicit --name/--file/--local flag always wins.
// Otherwise, when run inside a .kapi project, it defaults to the project's bound
// termbase (defaults.termbase, else <root>/.kapi/termbase.db) so that
// `kapi termbase lookup`/`import` see the same glossary that `kapi verify` and
// `kapi term-check` enforce — without it, a lookup inside a project silently
// hit an empty ./termbase.db. Falls back to ./termbase.db outside a project.
func (a *App) resolveTermbaseCmdPath(cmd *cobra.Command) (string, error) {
	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	file, _ := cmd.Flags().GetString("file")
	if name != "" || file != "" || local {
		return ResolveResourcePath(cmd, "termbases", "termbase.db")
	}
	if p, err := a.resolveProjectTermbasePath(cmd); err == nil && p != "" {
		return p, nil
	}
	return ResolveResourcePath(cmd, "termbases", "termbase.db")
}

func (a *App) newTermbaseImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "import [file]",
		Short:   "Import terms from CSV, JSON, or TBX into a termbase",
		Example: "  kapi termbase import glossary.csv -s en -t fr --header\n  kapi termbase import terms.tbx --format tbx",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			domain, _ := cmd.Flags().GetString("domain")
			hasHeader, _ := cmd.Flags().GetBool("header")
			delimiter, _ := cmd.Flags().GetString("delimiter")

			tb, dbPath, err := a.openTermbaseSQLite(cmd)
			if err != nil {
				return err
			}
			defer tb.Close()

			f, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open input: %w", err)
			}
			defer f.Close()

			var count int
			switch strings.ToLower(format) {
			case "csv", "tsv":
				opts := termbase.CSVImportOptions{
					SourceLocale: model.LocaleID(srcLocale),
					TargetLocale: model.LocaleID(tgtLocale),
					Domain:       domain,
					HasHeader:    hasHeader,
				}
				if delimiter != "" {
					opts.Delimiter = rune(delimiter[0])
				} else if format == "tsv" {
					opts.Delimiter = '\t'
				}
				count, err = termbase.ImportCSV(cmd.Context(), tb, f, opts)
			case "json":
				count, err = termbase.ImportJSON(cmd.Context(), tb, f)
			case "tbx":
				count, err = termbase.ImportTBX(cmd.Context(), tb, f, termbase.TBXImportOptions{
					Domain: domain,
				})
			default:
				return fmt.Errorf("unsupported format: %s (use csv, tsv, json, or tbx)", format)
			}

			if err != nil {
				return fmt.Errorf("import: %w", err)
			}

			if a.Quiet {
				return nil
			}
			total, err := tb.Count(cmd.Context())
			if err != nil {
				return fmt.Errorf("count terms: %w", err)
			}
			return output.Print(cmd, output.TermbaseImportOutput{
				Imported: count,
				DBPath:   dbPath,
				Total:    total,
			})
		},
	}

	cmd.Flags().String("format", "csv", "import format (csv, tsv, json, tbx)")
	cmd.Flags().StringP("source-locale", "s", "en", "source locale for CSV import")
	cmd.Flags().StringP("target-locale", "t", "", "target locale for CSV import")
	cmd.Flags().String("domain", "", "domain to assign to imported concepts")
	cmd.Flags().Bool("header", false, "CSV has header row")
	cmd.Flags().String("delimiter", "", "CSV field delimiter (default: comma)")

	return cmd
}

func (a *App) newTermbaseExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export termbase to CSV, JSON, or TBX",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			outputPath, _ := cmd.Flags().GetString("output")
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			tbName, _ := cmd.Flags().GetString("export-name")

			tb, dbPath, err := a.openTermbaseSQLite(cmd)
			if err != nil {
				return err
			}
			defer tb.Close()

			w := os.Stdout
			if outputPath != "" {
				w, err = os.Create(outputPath)
				if err != nil {
					return fmt.Errorf("create output: %w", err)
				}
				defer w.Close()
			}

			switch strings.ToLower(format) {
			case "csv":
				err = termbase.ExportCSV(cmd.Context(), tb, w, model.LocaleID(srcLocale), model.LocaleID(tgtLocale), true)
			case "json":
				if tbName == "" {
					tbName = dbPath
				}
				err = termbase.ExportJSON(cmd.Context(), tb, w, tbName)
			case "tbx":
				err = termbase.ExportTBX(cmd.Context(), tb, w, termbase.TBXExportOptions{
					SourceLocale: model.LocaleID(srcLocale),
				})
			default:
				return fmt.Errorf("unsupported format: %s (use csv, json, or tbx)", format)
			}

			if err != nil {
				return fmt.Errorf("export: %w", err)
			}

			if !a.Quiet && outputPath != "" {
				total, err := tb.Count(cmd.Context())
				if err != nil {
					return fmt.Errorf("count terms: %w", err)
				}
				return output.Print(cmd, output.TermbaseExportOutput{
					Count:      total,
					OutputPath: outputPath,
				})
			}
			return nil
		},
	}

	cmd.Flags().StringP("output", "o", "", "output file (default: stdout)")
	cmd.Flags().String("format", "json", "export format (csv, json, tbx)")
	cmd.Flags().StringP("source-locale", "s", "en", "source locale for CSV export")
	cmd.Flags().StringP("target-locale", "t", "", "target locale for CSV export")
	cmd.Flags().String("export-name", "", "termbase name for JSON export")

	return cmd
}

func (a *App) newTermbaseLookupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "lookup [term]",
		Short:   "Look up a term in the termbase",
		Example: "  kapi termbase lookup \"dashboard\" -s en -t fr\n  kapi termbase lookup \"settings\" -s en -t de --fuzzy",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			domain, _ := cmd.Flags().GetString("domain")
			fuzzy, _ := cmd.Flags().GetBool("fuzzy")

			tb, _, err := a.openTermbaseSQLite(cmd)
			if err != nil {
				return err
			}
			defer tb.Close()

			opts := termbase.LookupOptions{
				SourceLocale: model.LocaleID(srcLocale),
				TargetLocale: model.LocaleID(tgtLocale),
			}
			if domain != "" {
				opts.Domains = []string{domain}
			}
			if fuzzy {
				opts.MatchModes = []model.MatchStrategy{
					model.MatchStrategyExact,
					model.MatchStrategyNormalized,
					model.MatchStrategyFuzzy,
				}
				opts.MinScore = 0.6
			} else {
				opts.MatchModes = []model.MatchStrategy{
					model.MatchStrategyExact,
					model.MatchStrategyNormalized,
				}
			}

			matches, err := tb.Lookup(cmd.Context(), args[0], opts)
			if err != nil {
				return fmt.Errorf("lookup: %w", err)
			}

			entries := make([]output.TermbaseLookupEntry, len(matches))
			for i, m := range matches {
				entry := output.TermbaseLookupEntry{
					Term:      m.Term.Text,
					Locale:    string(m.Term.Locale),
					Status:    string(m.Term.Status),
					MatchType: string(m.MatchType),
					Score:     m.Score,
					ConceptID: m.Concept.ID,
					Domain:    m.Concept.Domain,
				}
				if !model.LocaleID(tgtLocale).IsEmpty() {
					for _, tt := range m.Concept.TargetTerms(model.LocaleID(tgtLocale)) {
						entry.Targets = append(entry.Targets, output.TermbaseLookupTarget{
							Text:   tt.Text,
							Locale: string(tt.Locale),
							Status: string(tt.Status),
						})
					}
				}
				entries[i] = entry
			}

			return output.Print(cmd, output.TermbaseLookupOutput{
				Matches: entries,
				Total:   len(entries),
			})
		},
	}

	cmd.Flags().StringP("source-locale", "s", "en", "source locale")
	cmd.Flags().StringP("target-locale", "t", "", "target locale to show translations")
	cmd.Flags().String("domain", "", "filter by domain")
	cmd.Flags().Bool("fuzzy", false, "also show approximate matches")

	return cmd
}

func (a *App) newTermbaseSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "search [query]",
		Short:   "Search concepts in the termbase",
		Example: "  kapi termbase search \"encrypt\" -s en\n  kapi termbase search \"log in\" -s en -t fr",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			limit, _ := cmd.Flags().GetInt("limit")

			tb, _, err := a.openTermbaseSQLite(cmd)
			if err != nil {
				return err
			}
			defer tb.Close()

			results, total, err := tb.Search(cmd.Context(), args[0], model.LocaleID(srcLocale), model.LocaleID(tgtLocale), 0, limit)
			if err != nil {
				return fmt.Errorf("search: %w", err)
			}

			entries := make([]output.TermbaseSearchEntry, len(results))
			for i, c := range results {
				terms := make([]output.TermbaseSearchTerm, len(c.Terms))
				for j, t := range c.Terms {
					terms[j] = output.TermbaseSearchTerm{
						Text:   t.Text,
						Locale: string(t.Locale),
					}
				}
				entries[i] = output.TermbaseSearchEntry{
					ID:         c.ID,
					Domain:     c.Domain,
					Definition: c.Definition,
					Terms:      terms,
				}
			}

			return output.Print(cmd, output.TermbaseSearchOutput{
				Concepts: entries,
				Total:    total,
				Shown:    len(entries),
			})
		},
	}

	cmd.Flags().StringP("source-locale", "s", "", "filter by source locale")
	cmd.Flags().StringP("target-locale", "t", "", "filter by target locale")
	cmd.Flags().Int("limit", 25, "max results")

	return cmd
}

func (a *App) newTermbaseStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "stats",
		Short:   "Show termbase statistics",
		Example: "  kapi termbase stats\n  kapi termbase stats --name product-terms",
		RunE: func(cmd *cobra.Command, args []string) error {
			tb, dbPath, err := a.openTermbaseSQLite(cmd)
			if err != nil {
				return err
			}
			defer tb.Close()

			concepts, err := tb.Concepts(cmd.Context())
			if err != nil {
				return fmt.Errorf("list concepts: %w", err)
			}
			totalTerms := 0
			locales := make(map[string]int)
			domains := make(map[string]int)
			statusCounts := make(map[model.TermStatus]int)

			for _, c := range concepts {
				for _, t := range c.Terms {
					totalTerms++
					locales[string(t.Locale)]++
					statusCounts[t.Status]++
				}
				if c.Domain != "" {
					domains[c.Domain]++
				}
			}

			statuses := make(map[string]int, len(statusCounts))
			for status, count := range statusCounts {
				statuses[string(status)] = count
			}

			return output.Print(cmd, output.TermbaseStatsOutput{
				DBPath:   dbPath,
				Concepts: len(concepts),
				Terms:    totalTerms,
				Locales:  locales,
				Domains:  domains,
				Statuses: statuses,
			})
		},
	}
}

func (a *App) newTermbaseListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List named termbases in KAPI_HOME",
		RunE: func(cmd *cobra.Command, args []string) error {
			resources, err := ListNamedResources("termbases")
			if err != nil {
				return fmt.Errorf("list termbases: %w", err)
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
				Kind:      "termbase",
				Resources: entries,
				Total:     len(entries),
			})
		},
	}
}
