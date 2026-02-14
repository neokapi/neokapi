package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gokapi/gokapi/core/kapiproject"
	"github.com/spf13/cobra"
)

var (
	initServerURL   string
	initProjectID   string
	initProjectName string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new .kapi/ project",
	Long: `Initialize a new .kapi/ project directory in the current directory.

Creates .kapi/config.yaml with project configuration, .kapi/flows/ for flow
definitions, and .kapi/.gitignore to exclude sync state.

Optionally connects to a Bowrain Server by specifying --server and --project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_ = context.Background() // Reserved for future server API calls

		// Get current directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}

		// Create default config
		cfg := kapiproject.DefaultConfig()

		// Override project name if specified
		if initProjectName != "" {
			cfg.Project.Name = initProjectName
		} else {
			// Use directory name as project name
			cfg.Project.Name = filepath.Base(cwd)
		}

		// Configure server connection if provided
		if initServerURL != "" {
			if initProjectID == "" {
				return fmt.Errorf("--project is required when --server is specified")
			}

			// Verify auth token exists
			auth, err := loadAuth()
			if err != nil {
				return fmt.Errorf("not authenticated with server (run: kapi auth login --server %s)", initServerURL)
			}

			if auth.ServerURL != initServerURL {
				return fmt.Errorf("authenticated with different server (%s), please login to %s first", auth.ServerURL, initServerURL)
			}

			cfg.Server = &kapiproject.ServerConfig{
				URL:       initServerURL,
				ProjectID: initProjectID,
			}

			fmt.Printf("Connecting to Bowrain Server: %s\n", initServerURL)
			fmt.Printf("Project ID: %s\n", initProjectID)
			// TODO: Fetch project metadata from server to populate source/target locales
		}

		// Initialize project
		project, err := kapiproject.InitProject(cwd, cfg)
		if err != nil {
			return fmt.Errorf("initialize project: %w", err)
		}

		fmt.Printf("Initialized .kapi/ project in: %s\n", project.Root)
		fmt.Printf("Configuration: %s\n", filepath.Join(project.KapiDir, kapiproject.ConfigFile))

		// Create example flow
		if err := createExampleFlow(project); err != nil {
			return fmt.Errorf("create example flow: %w", err)
		}

		fmt.Println("\nNext steps:")
		if cfg.Server == nil {
			fmt.Println("  1. Edit .kapi/config.yaml to configure your project")
			fmt.Println("  2. Add file mappings to sync with Bowrain Server")
			fmt.Println("  3. Run: kapi auth login --server <URL>")
			fmt.Println("  4. Run: kapi pull to sync translations")
		} else {
			fmt.Println("  1. Edit .kapi/config.yaml to configure file mappings")
			fmt.Println("  2. Run: kapi pull to sync translations from server")
			fmt.Println("  3. Run: kapi flow run <flow-name> to process files")
		}

		return nil
	},
}

func createExampleFlow(project *kapiproject.Project) error {
	flowPath := filepath.Join(project.FlowsDirPath(), "pseudo.yaml")

	exampleFlow := `name: pseudo
description: Generate pseudo-translations for testing

steps:
  - tool: pseudo-translate
    input: "locales/en-US.json"
    output: "locales/qps.json"
    config:
      method: extended
      expansion_rate: 1.3
`

	if err := os.WriteFile(flowPath, []byte(exampleFlow), 0644); err != nil {
		return err
	}

	fmt.Printf("Created example flow: %s\n", flowPath)
	return nil
}

func init() {
	initCmd.Flags().StringVar(&initServerURL, "server", "", "Bowrain Server URL (e.g., https://bowrain.example.com)")
	initCmd.Flags().StringVar(&initProjectID, "project", "", "Bowrain Server project ID")
	initCmd.Flags().StringVar(&initProjectName, "name", "", "Project name (default: current directory name)")

	rootCmd.AddCommand(initCmd)
}
