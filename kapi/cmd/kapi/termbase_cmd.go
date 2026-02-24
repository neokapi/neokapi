package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/termbase"
	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	"github.com/spf13/cobra"
)

var termbaseCmd = &cobra.Command{
	Use:   "termbase",
	Short: "Manage terminology",
	Long: `Manage project terminology.

A termbase is a glossary of approved terms stored as a JSON file.
Use these commands to import, export, look up, and manage terms.

Examples:
  kapi termbase import --format csv --source-locale en --target-locale fr terms.csv
  kapi termbase lookup --db terms.json --source-locale en "Save"
  kapi termbase search --db terms.json "repository"
  kapi termbase stats --db terms.json
  kapi termbase export --format json --db terms.json -o terms.json`,
}

// openTermbase loads a JSON termbase from disk into an in-memory TermBase.
func openTermbase(path string) (termbase.TermBase, error) {
	tb := termbase.NewInMemoryTermBase()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tb, nil // new empty termbase
		}
		return nil, fmt.Errorf("open termbase: %w", err)
	}
	defer f.Close()

	if _, err := termbase.ImportJSON(tb, f); err != nil {
		return nil, fmt.Errorf("load termbase: %w", err)
	}
	return tb, nil
}

// saveTermbase persists an in-memory TermBase to a JSON file.
func saveTermbase(tb termbase.TermBase, path, name string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	if name == "" {
		name = path
	}
	return termbase.ExportJSON(tb, f, name)
}

// --- import ---

var tbImportCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import terms from CSV or JSON into a termbase",
	Args:  cobra.ExactArgs(1),
	RunE:  runTermbaseImport,
}

func runTermbaseImport(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")
	format, _ := cmd.Flags().GetString("format")
	srcLocale, _ := cmd.Flags().GetString("source-locale")
	tgtLocale, _ := cmd.Flags().GetString("target-locale")
	domain, _ := cmd.Flags().GetString("domain")
	hasHeader, _ := cmd.Flags().GetBool("header")
	delimiter, _ := cmd.Flags().GetString("delimiter")

	tb, err := openTermbase(dbPath)
	if err != nil {
		return err
	}

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
		count, err = termbase.ImportCSV(tb, f, opts)
	case "json":
		count, err = termbase.ImportJSON(tb, f)
	default:
		return fmt.Errorf("unsupported format: %s (use csv, tsv, or json)", format)
	}

	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	if err := saveTermbase(tb, dbPath, ""); err != nil {
		return fmt.Errorf("save termbase: %w", err)
	}

	if quiet {
		return nil
	}
	return output.Print(cmd, output.TermbaseImportOutput{
		Imported: count,
		DBPath:   dbPath,
		Total:    tb.Count(),
	})
}

// --- export ---

var tbExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export termbase to CSV or JSON",
	RunE:  runTermbaseExport,
}

func runTermbaseExport(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")
	format, _ := cmd.Flags().GetString("format")
	outputPath, _ := cmd.Flags().GetString("output")
	srcLocale, _ := cmd.Flags().GetString("source-locale")
	tgtLocale, _ := cmd.Flags().GetString("target-locale")
	name, _ := cmd.Flags().GetString("name")

	tb, err := openTermbase(dbPath)
	if err != nil {
		return err
	}

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
		err = termbase.ExportCSV(tb, w, model.LocaleID(srcLocale), model.LocaleID(tgtLocale), true)
	case "json":
		if name == "" {
			name = dbPath
		}
		err = termbase.ExportJSON(tb, w, name)
	default:
		return fmt.Errorf("unsupported format: %s (use csv or json)", format)
	}

	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	if !quiet && outputPath != "" {
		return output.Print(cmd, output.TermbaseExportOutput{
			Count:      tb.Count(),
			OutputPath: outputPath,
		})
	}
	return nil
}

// --- lookup ---

var tbLookupCmd = &cobra.Command{
	Use:   "lookup [term]",
	Short: "Look up a term in the termbase",
	Args:  cobra.ExactArgs(1),
	RunE:  runTermbaseLookup,
}

func runTermbaseLookup(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")
	srcLocale, _ := cmd.Flags().GetString("source-locale")
	tgtLocale, _ := cmd.Flags().GetString("target-locale")
	domain, _ := cmd.Flags().GetString("domain")
	fuzzy, _ := cmd.Flags().GetBool("fuzzy")

	tb, err := openTermbase(dbPath)
	if err != nil {
		return err
	}

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

	matches := tb.Lookup(args[0], opts)

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
}

// --- search ---

var tbSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search concepts in the termbase",
	Args:  cobra.ExactArgs(1),
	RunE:  runTermbaseSearch,
}

func runTermbaseSearch(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")
	srcLocale, _ := cmd.Flags().GetString("source-locale")
	tgtLocale, _ := cmd.Flags().GetString("target-locale")
	limit, _ := cmd.Flags().GetInt("limit")

	tb, err := openTermbase(dbPath)
	if err != nil {
		return err
	}

	results, total := tb.Search(args[0], srcLocale, tgtLocale, 0, limit)

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
}

// --- stats ---

var tbStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show termbase statistics",
	RunE:  runTermbaseStats,
}

func runTermbaseStats(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")

	tb, err := openTermbase(dbPath)
	if err != nil {
		return err
	}

	concepts := tb.Concepts()
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
}

func init() {
	// Shared flags for all termbase subcommands.
	for _, cmd := range []*cobra.Command{tbImportCmd, tbExportCmd, tbLookupCmd, tbSearchCmd, tbStatsCmd} {
		cmd.Flags().String("db", "termbase.json", "path to the termbase file")
	}

	// Import flags.
	tbImportCmd.Flags().String("format", "csv", "import format (csv, tsv, json)")
	tbImportCmd.Flags().String("source-locale", "en", "source locale for CSV import")
	tbImportCmd.Flags().String("target-locale", "", "target locale for CSV import")
	tbImportCmd.Flags().String("domain", "", "domain to assign to imported concepts")
	tbImportCmd.Flags().Bool("header", false, "CSV has header row")
	tbImportCmd.Flags().String("delimiter", "", "CSV field delimiter (default: comma)")

	// Export flags.
	tbExportCmd.Flags().StringP("output", "o", "", "output file (default: stdout)")
	tbExportCmd.Flags().String("format", "json", "export format (csv, json)")
	tbExportCmd.Flags().String("source-locale", "en", "source locale for CSV export")
	tbExportCmd.Flags().String("target-locale", "", "target locale for CSV export")
	tbExportCmd.Flags().String("name", "", "termbase name for JSON export")

	// Lookup flags.
	tbLookupCmd.Flags().String("source-locale", "en", "source locale")
	tbLookupCmd.Flags().String("target-locale", "", "target locale to show translations")
	tbLookupCmd.Flags().String("domain", "", "filter by domain")
	tbLookupCmd.Flags().Bool("fuzzy", false, "also show approximate matches")

	// Search flags.
	tbSearchCmd.Flags().String("source-locale", "", "filter by source locale")
	tbSearchCmd.Flags().String("target-locale", "", "filter by target locale")
	tbSearchCmd.Flags().Int("limit", 25, "max results")

	termbaseCmd.AddCommand(tbImportCmd)
	termbaseCmd.AddCommand(tbExportCmd)
	termbaseCmd.AddCommand(tbLookupCmd)
	termbaseCmd.AddCommand(tbSearchCmd)
	termbaseCmd.AddCommand(tbStatsCmd)

	rootCmd.AddCommand(termbaseCmd)
}
