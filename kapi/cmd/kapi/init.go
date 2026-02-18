package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/platform/client"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	initServerURL   string
	initProjectID   string
	initProjectName string
	initSource      string
	initTargets     string
	initQuickStart  bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new .kapi/ project",
	Long: `Initialize a new .kapi/ project directory in the current directory.

Creates .kapi/config.yaml with project configuration, .kapi/flows/ for flow
definitions, and .kapi/.gitignore to exclude sync state.

In interactive mode (default when stdin is a terminal), presents a guided setup
wizard. Use --server and --project flags for non-interactive CI/CD usage.

Use --quickstart to create an anonymous project on a Bowrain Server that can
be claimed later via the web UI.`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}

	// Non-interactive: flags provided or stdin is not a TTY.
	if initQuickStart || initServerURL != "" || !isTTY() {
		return runInitNonInteractive(cwd)
	}

	// Interactive mode.
	return runInitInteractive(cwd)
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// parseTargetLocales splits a comma-separated locale string into a slice.
func parseTargetLocales(s string) []model.LocaleID {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	locales := make([]model.LocaleID, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			locales = append(locales, model.LocaleID(p))
		}
	}
	return locales
}

func runInitNonInteractive(cwd string) error {
	cfg := project.DefaultConfig()

	if initProjectName != "" {
		cfg.Project.Name = initProjectName
	} else {
		cfg.Project.Name = filepath.Base(cwd)
	}

	// Apply locale flags.
	if initSource != "" {
		cfg.Project.SourceLocale = model.LocaleID(initSource)
	}
	if initTargets != "" {
		cfg.Project.TargetLocales = parseTargetLocales(initTargets)
	}

	// Quick start mode: create anonymous project on server.
	if initQuickStart {
		return runInitQuickStart(cwd, cfg)
	}

	if initServerURL != "" {
		if initProjectID == "" {
			return fmt.Errorf("--project is required when --server is specified")
		}

		auth, err := loadAuth()
		if err != nil {
			return fmt.Errorf("not authenticated with server (run: kapi auth login --server %s)", initServerURL)
		}

		if auth.ServerURL != initServerURL {
			return fmt.Errorf("authenticated with different server (%s), please login to %s first", auth.ServerURL, initServerURL)
		}

		cfg.Server = &project.ServerConfig{
			URL:       initServerURL,
			ProjectID: initProjectID,
		}

		fmt.Printf("Connecting to Bowrain Server: %s\n", initServerURL)
		fmt.Printf("Project ID: %s\n", initProjectID)
	}

	return finishInit(cwd, cfg)
}

func runInitInteractive(cwd string) error {
	var choice string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("How would you like to set up your project?").
				Options(
					huh.NewOption("Sign in to Bowrain", "signin"),
					huh.NewOption("Quick start (create project on server)", "quickstart"),
					huh.NewOption("Local only (no server)", "local"),
				).
				Value(&choice),
		),
	).Run()
	if err != nil {
		return err
	}

	switch choice {
	case "signin":
		return runInitAuthenticated(cwd)
	case "quickstart":
		return runInitQuickStartInteractive(cwd)
	case "local":
		return runInitLocal(cwd)
	default:
		return fmt.Errorf("unexpected choice: %s", choice)
	}
}

func runInitAuthenticated(cwd string) error {
	stored, err := loadAuth()
	if err != nil {
		fmt.Println("You need to sign in first.")
		var serverURL string
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Bowrain Server URL").
					Value(&serverURL).
					Placeholder("https://api.bowrain.cloud"),
			),
		).Run()
		if err != nil {
			return err
		}
		if serverURL == "" {
			serverURL = "https://api.bowrain.cloud"
		}
		fmt.Printf("\nRun this command to sign in:\n\n  kapi auth login --server %s\n\nThen re-run: kapi init\n", serverURL)
		return nil
	}

	dirName := filepath.Base(cwd)
	var projectName string

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Value(&projectName).
				Placeholder(dirName),
		),
	).Run()
	if err != nil {
		return err
	}

	if projectName == "" {
		projectName = dirName
	}

	cfg := project.DefaultConfig()
	cfg.Project.Name = projectName
	cfg.Server = &project.ServerConfig{
		URL: stored.ServerURL,
	}

	fmt.Printf("Server: %s\n", stored.ServerURL)
	fmt.Printf("Logged in as: %s\n", stored.User.Email)
	fmt.Println("\nNote: Run kapi push/pull to sync with the server after init.")

	return finishInit(cwd, cfg)
}

func runInitLocal(cwd string) error {
	dirName := filepath.Base(cwd)
	var projectName, sourceLocale, targetLocalesStr string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Value(&projectName).
				Placeholder(dirName),
			huh.NewInput().
				Title("Source locale").
				Value(&sourceLocale).
				Placeholder("en"),
			huh.NewInput().
				Title("Target locales (comma-separated)").
				Value(&targetLocalesStr).
				Placeholder("nb, fr"),
		),
	).Run()
	if err != nil {
		return err
	}

	if projectName == "" {
		projectName = dirName
	}

	cfg := project.DefaultConfig()
	cfg.Project.Name = projectName
	if sourceLocale != "" {
		cfg.Project.SourceLocale = model.LocaleID(sourceLocale)
	}
	if targetLocalesStr != "" {
		cfg.Project.TargetLocales = parseTargetLocales(targetLocalesStr)
	}

	return finishInit(cwd, cfg)
}

// runInitQuickStartInteractive runs the interactive Quick start flow.
func runInitQuickStartInteractive(cwd string) error {
	dirName := filepath.Base(cwd)
	var projectName, serverURL, sourceLocale, targetLocalesStr string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Value(&projectName).
				Placeholder(dirName),
			huh.NewInput().
				Title("Bowrain Server URL").
				Value(&serverURL).
				Placeholder("http://localhost:8080"),
			huh.NewInput().
				Title("Source locale").
				Value(&sourceLocale).
				Placeholder("en"),
			huh.NewInput().
				Title("Target locales (comma-separated)").
				Value(&targetLocalesStr).
				Placeholder("nb, fr"),
		),
	).Run()
	if err != nil {
		return err
	}

	if projectName == "" {
		projectName = dirName
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	if sourceLocale == "" {
		sourceLocale = "en"
	}

	cfg := project.DefaultConfig()
	cfg.Project.Name = projectName
	cfg.Project.SourceLocale = model.LocaleID(sourceLocale)
	cfg.Project.TargetLocales = parseTargetLocales(targetLocalesStr)

	return runInitQuickStart(cwd, cfg, withServerURL(serverURL))
}

type quickStartOption func(*quickStartConfig)

type quickStartConfig struct {
	serverURL string
}

func withServerURL(s string) quickStartOption {
	return func(c *quickStartConfig) { c.serverURL = s }
}

// runInitQuickStart creates an anonymous project on the server and initializes
// the local .kapi/ directory with the claim token.
func runInitQuickStart(cwd string, cfg *project.Config, opts ...quickStartOption) error {
	qsc := &quickStartConfig{}
	for _, o := range opts {
		o(qsc)
	}

	// Determine server URL (flag > option > default).
	serverURL := initServerURL
	if serverURL == "" && qsc.serverURL != "" {
		serverURL = qsc.serverURL
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	// Ensure we have source and target locales.
	if cfg.Project.SourceLocale == "" {
		if initSource != "" {
			cfg.Project.SourceLocale = model.LocaleID(initSource)
		} else {
			cfg.Project.SourceLocale = "en"
		}
	}
	if len(cfg.Project.TargetLocales) == 0 && initTargets != "" {
		cfg.Project.TargetLocales = parseTargetLocales(initTargets)
	}
	if len(cfg.Project.TargetLocales) == 0 {
		return fmt.Errorf("at least one target locale is required (use --targets flag)")
	}

	// Convert LocaleIDs to strings for API call.
	targets := make([]string, len(cfg.Project.TargetLocales))
	for i, t := range cfg.Project.TargetLocales {
		targets[i] = string(t)
	}

	fmt.Printf("Creating project on %s...\n", serverURL)

	projectID, claimToken, err := client.CreateAnonymousProject(
		serverURL,
		cfg.Project.Name,
		string(cfg.Project.SourceLocale),
		targets,
	)
	if err != nil {
		return fmt.Errorf("create anonymous project: %w", err)
	}

	cfg.Server = &project.ServerConfig{
		URL:        serverURL,
		ProjectID:  projectID,
		ClaimToken: claimToken,
	}

	if err := finishInit(cwd, cfg); err != nil {
		return err
	}

	// Print claim URL and next steps.
	claimURL := strings.TrimRight(serverURL, "/") + "/claim/" + claimToken
	fmt.Printf("\nProject created: %s\n", projectID)
	fmt.Printf("Claim URL: %s\n", claimURL)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Run: kapi push    — upload content to the server")
	fmt.Printf("  2. Open the claim URL to take ownership of the project\n")
	fmt.Println("  3. Invite translators from the web dashboard")

	return nil
}

func finishInit(cwd string, cfg *project.Config) error {
	proj, err := project.InitProject(cwd, cfg)
	if err != nil {
		return fmt.Errorf("initialize project: %w", err)
	}

	fmt.Printf("Initialized .kapi/ project in: %s\n", proj.Root)
	fmt.Printf("Configuration: %s\n", filepath.Join(proj.KapiDir, project.ConfigFile))

	if err := createExampleFlow(proj); err != nil {
		return fmt.Errorf("create example flow: %w", err)
	}

	if cfg.Server == nil {
		fmt.Println("\nNext steps:")
		fmt.Println("  1. Edit .kapi/config.yaml to configure your project")
		fmt.Println("  2. Add file mappings to sync with Bowrain Server")
		fmt.Println("  3. Run: kapi auth login --server <URL>")
		fmt.Println("  4. Run: kapi pull to sync translations")
	}

	return nil
}

func createExampleFlow(proj *project.Project) error {
	flowPath := filepath.Join(proj.FlowsDirPath(), "pseudo.yaml")

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
	initCmd.Flags().StringVar(&initSource, "source", "", "Source locale (e.g., en)")
	initCmd.Flags().StringVar(&initTargets, "targets", "", "Target locales, comma-separated (e.g., nb,fr)")
	initCmd.Flags().BoolVar(&initQuickStart, "quickstart", false, "Create an anonymous project on the server (no login required)")

	rootCmd.AddCommand(initCmd)
}
