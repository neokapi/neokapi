package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/occurrence"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/terms"
	"github.com/spf13/cobra"
)

// NewTermsCmd creates the terms store command group.
func NewTermsCmd(a *App) *cobra.Command {
	tbCmd := &cobra.Command{
		Use:     "terms",
		Short:   "Manage terminology",
		GroupID: "assets",
		Long: `Manage project terminology.

A terms store holds approved terminology (concepts and their renderings per
locale) as a SQLite database. Use these commands to import, export, look up,
and manage terms.

Inside a project, no flag means the project's own terms, a subsystem of
.kapi/work/store.db, which is why it is not addressed by path. Use -p to name the
project explicitly.

Standalone store instead (mutually exclusive):
  --name <n>      Named terms store in the kapi config dir (terms/<n>.db)
  --local         Terms store in current directory (./terms.db)
  --file <path>   Explicit file path

Outside a project and with no flag: same as --local (uses ./terms.db).`,
		Example: `  kapi terms stats
  kapi terms lookup "dashboard" -s en -t fr
  kapi terms import terms.csv -s en -t fr`,
	}

	importCmd := newTermsImportCmd(a)
	exportCmd := newTermsExportCmd(a)
	lookupCmd := newTermsLookupCmd(a)
	searchCmd := newTermsSearchCmd(a)
	occurrencesCmd := newTermsOccurrencesCmd(a)
	statsCmd := newTermsStatsCmd(a)
	listCmd := newTermsListCmd(a)

	// Shared resource flags for all subcommands (except list).
	for _, cmd := range []*cobra.Command{importCmd, exportCmd, lookupCmd, searchCmd, occurrencesCmd, statsCmd} {
		AddResourceFlags(cmd)
	}
	// The project's terms are a subsystem of `.kapi/work/store.db`, not a file a
	// caller can name — so with no resource flag these resolve the project,
	// and -p is how you say WHICH, exactly as for every other project-aware
	// verb. It also matters under KAPI_NO_PROJECT, where the upward walk is
	// off and an explicit -p is the only way in.
	for _, cmd := range []*cobra.Command{importCmd, exportCmd, lookupCmd, searchCmd, occurrencesCmd, statsCmd} {
		AddProjectFlag(cmd)
	}

	tbCmd.AddCommand(importCmd, exportCmd, lookupCmd, searchCmd, occurrencesCmd, statsCmd, listCmd)
	return tbCmd
}

func newTermsImportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "import [file]",
		Short:   "Import terms from CSV, JSON, TBX, or a native .terms.json bundle",
		Example: "  kapi terms import terms.csv -s en -t fr --header\n  kapi terms import vocab.csv -s en --monolingual --header\n  kapi terms import terms.tbx --format tbx\n  kapi terms import seeds/terms.json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			domain, _ := cmd.Flags().GetString("domain")
			hasHeader, _ := cmd.Flags().GetBool("header")
			delimiter, _ := cmd.Flags().GetString("delimiter")
			monolingual, _ := cmd.Flags().GetBool("monolingual")
			format, err := ResolveTermsImportFormat(format, args[0])
			if err != nil {
				return err
			}

			tb, dbPath, releaseTerms, err := a.OpenTermsSQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseTerms()

			f, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open input: %w", err)
			}
			defer f.Close()

			var count int
			switch strings.ToLower(format) {
			case "csv", "tsv":
				opts := terms.CSVImportOptions{
					SourceLocale: model.LocaleID(srcLocale),
					TargetLocale: model.LocaleID(tgtLocale),
					Domain:       domain,
					HasHeader:    hasHeader,
					Monolingual:  monolingual,
				}
				if delimiter != "" {
					opts.Delimiter = rune(delimiter[0])
				} else if format == "tsv" {
					opts.Delimiter = '\t'
				}
				count, err = terms.ImportCSV(cmd.Context(), tb, f, opts)
			case "json":
				count, err = terms.ImportJSON(cmd.Context(), tb, f)
			case "tbx":
				count, err = terms.ImportTBX(cmd.Context(), tb, f, terms.TBXImportOptions{
					Domain: domain,
				})
			case "bundle":
				count, err = ImportKTBFile(cmd.Context(), tb, f)
			default:
				return fmt.Errorf("unsupported format: %s (use csv, tsv, json, tbx, or bundle)", format)
			}

			if err != nil {
				// Name the reader that ran: with --format left at auto the
				// choice was made from the extension, so a parse error from
				// inside a CSV or TBX reader is otherwise unattributable.
				return fmt.Errorf("read %s as %s: %w. Pass --format (%s) if that is the wrong reader",
					filepath.Base(args[0]), format, err, strings.Join(TermsFileFormats, ", "))
			}

			if a.Quiet {
				return nil
			}
			total, err := tb.Count(cmd.Context())
			if err != nil {
				return fmt.Errorf("count terms: %w", err)
			}
			return output.Print(cmd, output.TermsImportOutput{
				Imported: count,
				DBPath:   dbPath,
				Total:    total,
			})
		},
	}

	cmd.Flags().String("format", "auto", "input format (auto, csv, tsv, json, tbx, bundle); auto selects by file extension")
	cmd.Flags().StringP("source-locale", "s", "en", "source locale for CSV import")
	cmd.Flags().StringP("target-locale", "t", "", "target locale for CSV import")
	cmd.Flags().String("domain", "", "domain to assign to imported concepts")
	cmd.Flags().Bool("header", false, "CSV has header row")
	cmd.Flags().String("delimiter", "", "CSV field delimiter (default: comma)")
	cmd.Flags().Bool("monolingual", false, "import a single-locale concept list (term[, definition]) with no translation pair")

	return cmd
}

func newTermsExportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export terms to CSV, JSON, TBX, or a native .terms.json bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			outputPath, _ := cmd.Flags().GetString("output")
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			tbName, _ := cmd.Flags().GetString("export-name")

			tb, dbPath, releaseTerms, err := a.OpenTermsSQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseTerms()

			w := os.Stdout
			if outputPath != "" {
				w, err = os.Create(outputPath)
				if err != nil {
					return fmt.Errorf("create output: %w", err)
				}
				defer w.Close()
			}

			format = ResolveTermsExportFormat(format, outputPath, cmd.Flags().Changed("format"))
			switch strings.ToLower(format) {
			case "csv":
				err = terms.ExportCSV(cmd.Context(), tb, w, model.LocaleID(srcLocale), model.LocaleID(tgtLocale), true)
			case "json":
				if tbName == "" {
					tbName = dbPath
				}
				err = terms.ExportJSON(cmd.Context(), tb, w, tbName)
			case "tbx":
				err = terms.ExportTBX(cmd.Context(), tb, w, terms.TBXExportOptions{
					SourceLocale: model.LocaleID(srcLocale),
				})
			case "bundle":
				err = ExportKTB(cmd.Context(), tb, w)
			default:
				return fmt.Errorf("unsupported format: %s (use csv, json, tbx, or bundle)", format)
			}

			if err != nil {
				return fmt.Errorf("export: %w", err)
			}

			if !a.Quiet && outputPath != "" {
				total, err := tb.Count(cmd.Context())
				if err != nil {
					return fmt.Errorf("count terms: %w", err)
				}
				return output.Print(cmd, output.TermsExportOutput{
					Count:      total,
					OutputPath: outputPath,
				})
			}
			return nil
		},
	}

	cmd.Flags().StringP("output", "o", "", "output file (default: stdout)")
	cmd.Flags().String("format", "json", "export format (csv, json, tbx, bundle); a .terms.json -o path is detected automatically")
	cmd.Flags().StringP("source-locale", "s", "en", "source locale for CSV export")
	cmd.Flags().StringP("target-locale", "t", "", "target locale for CSV export")
	cmd.Flags().String("export-name", "", "terms name for JSON export")

	return cmd
}

func newTermsLookupCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "lookup [term]",
		Short:   "Look up a term in the terms store",
		Example: "  kapi terms lookup \"dashboard\" -s en -t fr\n  kapi terms lookup \"settings\" -s en -t de --fuzzy",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			domain, _ := cmd.Flags().GetString("domain")
			fuzzy, _ := cmd.Flags().GetBool("fuzzy")

			tb, _, releaseTerms, err := a.OpenTermsSQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseTerms()

			opts := terms.LookupOptions{
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

			entries := make([]output.TermsLookupEntry, len(matches))
			for i, m := range matches {
				entry := output.TermsLookupEntry{
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
						entry.Targets = append(entry.Targets, output.TermsLookupTarget{
							Text:   tt.Text,
							Locale: string(tt.Locale),
							Status: string(tt.Status),
						})
					}
				}
				entries[i] = entry
			}

			return output.Print(cmd, output.TermsLookupOutput{
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

func newTermsSearchCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "search [query]",
		Short:   "Search concepts in the terms store",
		Example: "  kapi terms search \"encrypt\" -s en\n  kapi terms search \"log in\" -s en -t fr",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")
			limit, _ := cmd.Flags().GetInt("limit")

			tb, _, releaseTerms, err := a.OpenTermsSQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseTerms()

			results, total, err := tb.Search(cmd.Context(), args[0], model.LocaleID(srcLocale), model.LocaleID(tgtLocale), 0, limit)
			if err != nil {
				return fmt.Errorf("search: %w", err)
			}

			entries := make([]output.TermsSearchEntry, len(results))
			for i, c := range results {
				terms := make([]output.TermsSearchTerm, len(c.Terms))
				for j, t := range c.Terms {
					terms[j] = output.TermsSearchTerm{
						Text:   t.Text,
						Locale: string(t.Locale),
					}
				}
				entries[i] = output.TermsSearchEntry{
					ID:         c.ID,
					Domain:     c.Domain,
					Definition: c.Definition,
					Terms:      terms,
				}
			}

			return output.Print(cmd, output.TermsSearchOutput{
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

// newTermsOccurrencesCmd answers where a term is actually used, by joining the
// project's terms against the text of its extracted blocks — one query inside
// `.kapi/work/store.db`, with no server and no account.
func newTermsOccurrencesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "occurrences [term-or-concept]",
		Short: "Show where a term is used in the project's content",
		Long: `Show where a term is used in the project's extracted content.

The argument is a term as written, or a concept id. A concept is searched for
under every term it carries, in every language, so an approved English term is
found in the source text and its counterpart in the translated text.

Matching folds case, spans any whitespace between a term's words, and requires a
word boundary, so "AI" is not a use of "again". Scripts written without word
separators are matched without that rule.

The project must have been extracted: occurrences are read from the block cache
inside the project store, which ` + "`kapi up`" + ` and ` + "`kapi extract`" + ` fill.`,
		Example: `  kapi terms occurrences "content memory"
  kapi terms occurrences c-dashboard --locale nb
  kapi terms occurrences "log in" --collection docs --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection, _ := cmd.Flags().GetString("collection")
			limit, _ := cmd.Flags().GetInt("limit")
			locales, err := occurrenceLocales(cmd)
			if err != nil {
				return err
			}

			res, err := a.FindOccurrences(cmd, occurrence.Query{
				Subject:    args[0],
				Locales:    locales,
				Collection: collection,
				Limit:      limit,
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, output.NewTermsOccurrencesOutput(res))
		},
	}

	cmd.Flags().StringSliceP("locale", "l", nil,
		"only text in these locales; 'source' for the source text (default: all)")
	cmd.Flags().String("collection", "", "only blocks in this collection")
	cmd.Flags().Int("limit", 50, "max occurrences to show")

	return cmd
}

// occurrenceLocales turns --locale into the query's locale filter. "source" is
// spelled out because the source text's own key is the empty string, which is
// not something a user can type — and "" on the command line is how you ask for
// no filter at all.
func occurrenceLocales(cmd *cobra.Command) ([]string, error) {
	raw, _ := cmd.Flags().GetStringSlice("locale")
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l == "" {
			return nil, errors.New("--locale: empty locale; use 'source' for the source text")
		}
		if l == "source" {
			l = occurrence.SourceLocale
		}
		out = append(out, l)
	}
	return out, nil
}

func newTermsStatsCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:     "stats",
		Short:   "Show terms statistics",
		Example: "  kapi terms stats\n  kapi terms stats --name product-terms",
		RunE: func(cmd *cobra.Command, args []string) error {
			tb, dbPath, releaseTerms, err := a.OpenTermsSQLite(cmd)
			if err != nil {
				return err
			}
			defer releaseTerms()

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

			return output.Print(cmd, output.TermsStatsOutput{
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

func newTermsListCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List named terms stores in KAPI_HOME",
		RunE: func(cmd *cobra.Command, args []string) error {
			resources, err := ListNamedResources("terms")
			if err != nil {
				return fmt.Errorf("list terms stores: %w", err)
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
				Kind:      "terms",
				Resources: entries,
				Total:     len(entries),
			})
		},
	}
}
