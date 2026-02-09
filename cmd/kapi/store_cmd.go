package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/gokapi/gokapi/core/store"
	"github.com/spf13/cobra"
)

var storePath string

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Manage the content store",
	Long:  "Commands for working with the versioned content store.",
}

var storeVersionCmd = &cobra.Command{
	Use:   "version <project-id> <label> [description]",
	Short: "Create a version snapshot",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		projectID := args[0]
		label := args[1]
		description := ""
		if len(args) > 2 {
			description = args[2]
		}

		v, err := s.CreateVersion(context.Background(), projectID, label, description)
		if err != nil {
			return fmt.Errorf("create version: %w", err)
		}

		fmt.Printf("Created version %s (%s) with %d blocks\n", v.Label, v.ID, v.BlockCount)
		return nil
	},
}

var storeVersionsCmd = &cobra.Command{
	Use:   "versions <project-id>",
	Short: "List versions for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		versions, err := s.ListVersions(context.Background(), args[0])
		if err != nil {
			return fmt.Errorf("list versions: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tLABEL\tBLOCKS\tCREATED")
		for _, v := range versions {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
				v.ID[:8], v.Label, v.BlockCount, v.CreatedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
		return nil
	},
}

var storeProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List projects in the store",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		projects, err := s.ListProjects(context.Background())
		if err != nil {
			return fmt.Errorf("list projects: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSOURCE\tTARGETS")
		for _, p := range projects {
			fmt.Fprintf(w, "%s\t%s\t%s\t%v\n",
				p.ID[:8], p.Name, p.SourceLocale, p.TargetLocales)
		}
		w.Flush()
		return nil
	},
}

var storeExportCmd = &cobra.Command{
	Use:   "export <project-id> <output.kaz>",
	Short: "Export a project as a KAZ file",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		f, err := os.Create(args[1])
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()

		if err := s.ExportKAZ(context.Background(), args[0], f); err != nil {
			return fmt.Errorf("export: %w", err)
		}
		fmt.Printf("Exported project to %s\n", args[1])
		return nil
	},
}

var storeImportCmd = &cobra.Command{
	Use:   "import <input.kaz>",
	Short: "Import a project from a KAZ file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return err
		}
		defer s.Close()

		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("open input: %w", err)
		}
		defer f.Close()

		projectID, err := s.ImportKAZ(context.Background(), f)
		if err != nil {
			return fmt.Errorf("import: %w", err)
		}
		fmt.Printf("Imported project: %s\n", projectID)
		return nil
	},
}

func openStore() (*store.SQLiteStore, error) {
	path := storePath
	if path == "" {
		path = "gokapi.db"
	}
	return store.NewSQLiteStore(path)
}

func init() {
	storeCmd.PersistentFlags().StringVar(&storePath, "store", "", "Path to store database (default: gokapi.db)")

	storeCmd.AddCommand(storeVersionCmd)
	storeCmd.AddCommand(storeVersionsCmd)
	storeCmd.AddCommand(storeProjectsCmd)
	storeCmd.AddCommand(storeExportCmd)
	storeCmd.AddCommand(storeImportCmd)
	rootCmd.AddCommand(storeCmd)
}
