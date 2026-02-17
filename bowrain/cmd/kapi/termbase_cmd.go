package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	sqltb "github.com/gokapi/gokapi/bowrain/termbase"
	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/termbase"
	"github.com/spf13/cobra"
)

var termbaseCmd = &cobra.Command{
	Use:   "termbase",
	Short: "Manage Bowrain Termbase",
	Long: `Commands for creating, importing, exporting, and querying Bowrain Termbase.

Bowrain Termbase is a concept-oriented terminology database that stores approved
terms with their translations, lifecycle status, domain, and definitions. Use these
commands to manage termbases independently or as part of a translation workflow.

Examples:
  kapi termbase import --format csv --source-locale en --target-locale fr terms.csv
  kapi termbase lookup --db terms.db --source-locale en "Save"
  kapi termbase search --db terms.db "repository"
  kapi termbase stats --db terms.db
  kapi termbase export --format json --db terms.db -o terms.json`,
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

	tb, err := sqltb.NewSQLiteTermBase(dbPath)
	if err != nil {
		return fmt.Errorf("open termbase: %w", err)
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
		count, err = termbase.ImportCSV(tb, f, opts)
	case "json":
		count, err = termbase.ImportJSON(tb, f)
	default:
		return fmt.Errorf("unsupported format: %s (use csv, tsv, or json)", format)
	}

	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	if !quiet {
		fmt.Printf("Imported %d concepts into %s (total: %d)\n", count, dbPath, tb.Count())
	}
	return nil
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
	output, _ := cmd.Flags().GetString("output")
	srcLocale, _ := cmd.Flags().GetString("source-locale")
	tgtLocale, _ := cmd.Flags().GetString("target-locale")
	name, _ := cmd.Flags().GetString("name")

	tb, err := sqltb.NewSQLiteTermBase(dbPath)
	if err != nil {
		return fmt.Errorf("open termbase: %w", err)
	}
	defer tb.Close()

	w := os.Stdout
	if output != "" {
		w, err = os.Create(output)
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

	if !quiet && output != "" {
		fmt.Printf("Exported %d concepts to %s\n", tb.Count(), output)
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

	tb, err := sqltb.NewSQLiteTermBase(dbPath)
	if err != nil {
		return fmt.Errorf("open termbase: %w", err)
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

	matches := tb.Lookup(args[0], opts)
	if len(matches) == 0 {
		fmt.Println("No matches found.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "TERM\tLOCALE\tSTATUS\tMATCH\tSCORE\tCONCEPT\tDOMAIN\n")
	for _, m := range matches {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%.0f%%\t%s\t%s\n",
			m.Term.Text, m.Term.Locale, m.Term.Status,
			m.MatchType, m.Score*100,
			m.Concept.ID, m.Concept.Domain)

		// Show target terms.
		if !model.LocaleID(tgtLocale).IsEmpty() {
			for _, tt := range m.Concept.TargetTerms(model.LocaleID(tgtLocale)) {
				fmt.Fprintf(tw, "  -> %s\t%s\t%s\t\t\t\t\n",
					tt.Text, tt.Locale, tt.Status)
			}
		}
	}
	tw.Flush()
	return nil
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

	tb, err := sqltb.NewSQLiteTermBase(dbPath)
	if err != nil {
		return fmt.Errorf("open termbase: %w", err)
	}
	defer tb.Close()

	results, total := tb.Search(args[0], srcLocale, tgtLocale, 0, limit)

	if total == 0 {
		fmt.Println("No concepts found.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "CONCEPT\tDOMAIN\tTERMS\tDEFINITION\n")
	for _, c := range results {
		var termParts []string
		for _, t := range c.Terms {
			termParts = append(termParts, fmt.Sprintf("%s [%s]", t.Text, t.Locale))
		}
		def := c.Definition
		if len(def) > 50 {
			def = def[:47] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			c.ID, c.Domain, strings.Join(termParts, ", "), def)
	}
	tw.Flush()

	if total > len(results) {
		fmt.Printf("\nShowing %d of %d results. Use --limit to see more.\n", len(results), total)
	}
	return nil
}

// --- stats ---

var tbStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show termbase statistics",
	RunE:  runTermbaseStats,
}

func runTermbaseStats(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")

	tb, err := sqltb.NewSQLiteTermBase(dbPath)
	if err != nil {
		return fmt.Errorf("open termbase: %w", err)
	}
	defer tb.Close()

	concepts := tb.Concepts()
	totalConcepts := len(concepts)
	totalTerms := 0
	locales := make(map[string]int)
	domains := make(map[string]int)
	statuses := make(map[model.TermStatus]int)

	for _, c := range concepts {
		for _, t := range c.Terms {
			totalTerms++
			locales[string(t.Locale)]++
			statuses[t.Status]++
		}
		if c.Domain != "" {
			domains[c.Domain]++
		}
	}

	fmt.Printf("Termbase: %s\n\n", dbPath)
	fmt.Printf("  Concepts:  %d\n", totalConcepts)
	fmt.Printf("  Terms:     %d\n\n", totalTerms)

	if len(locales) > 0 {
		fmt.Println("  Locales:")
		for loc, count := range locales {
			fmt.Printf("    %-10s %d terms\n", loc, count)
		}
		fmt.Println()
	}

	if len(domains) > 0 {
		fmt.Println("  Domains:")
		for dom, count := range domains {
			fmt.Printf("    %-20s %d concepts\n", dom, count)
		}
		fmt.Println()
	}

	if len(statuses) > 0 {
		fmt.Println("  Term statuses:")
		for status, count := range statuses {
			fmt.Printf("    %-12s %d\n", status, count)
		}
	}

	return nil
}

func init() {
	// Shared flags for all termbase subcommands.
	for _, cmd := range []*cobra.Command{tbImportCmd, tbExportCmd, tbLookupCmd, tbSearchCmd, tbStatsCmd} {
		cmd.Flags().String("db", "termbase.db", "termbase database path")
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
	tbLookupCmd.Flags().Bool("fuzzy", false, "enable fuzzy matching")

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
