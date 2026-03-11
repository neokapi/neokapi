package cli

import (
	"fmt"
	"os"

	"github.com/gokapi/gokapi/cli/output"
	sqltm "github.com/gokapi/gokapi/core/sievepen"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/sievepen"
	"github.com/spf13/cobra"
)

// NewTMCmd creates the tm command group.
func (a *App) NewTMCmd() *cobra.Command {
	tmCmd := &cobra.Command{
		Use:   "tm",
		Short: "Manage translation memory",
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
	exportCmd := a.newTMExportCmd()
	lookupCmd := a.newTMLookupCmd()
	searchCmd := a.newTMSearchCmd()
	statsCmd := a.newTMStatsCmd()
	listCmd := a.newTMListCmd()

	for _, cmd := range []*cobra.Command{importCmd, exportCmd, lookupCmd, searchCmd, statsCmd} {
		AddResourceFlags(cmd)
	}

	tmCmd.AddCommand(importCmd, exportCmd, lookupCmd, searchCmd, statsCmd, listCmd)
	return tmCmd
}

func (a *App) openTMSQLite(cmd *cobra.Command) (*sqltm.SQLiteTM, string, error) {
	dbPath, err := ResolveResourcePath(cmd, "tm", "tm.db")
	if err != nil {
		return nil, "", err
	}
	tm, err := sqltm.NewSQLiteTM(dbPath)
	if err != nil {
		return nil, dbPath, fmt.Errorf("open TM: %w", err)
	}
	return tm, dbPath, nil
}

func (a *App) newTMImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Import TMX file into translation memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcLocale, _ := cmd.Flags().GetString("source-locale")
			tgtLocale, _ := cmd.Flags().GetString("target-locale")

			tm, dbPath, err := a.openTMSQLite(cmd)
			if err != nil {
				return err
			}
			defer tm.Close()

			f, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open input: %w", err)
			}
			defer f.Close()

			count, err := sievepen.ImportTMX(tm, f, model.LocaleID(srcLocale), model.LocaleID(tgtLocale))
			if err != nil {
				return fmt.Errorf("import TMX: %w", err)
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

	return cmd
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
					ID:           e.ID,
					Source:       e.SourceText(),
					Target:       e.TargetText(),
					SourceLocale: string(e.SourceLocale),
					TargetLocale: string(e.TargetLocale),
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
