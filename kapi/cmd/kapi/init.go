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
	initAnonymous   bool
	initEmail       string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new .kapi/ project",
	Long: `Initialize a new .kapi/ project directory in the current directory.

Creates .kapi/config.yaml with project configuration, .kapi/flows/ for flow
definitions, and .kapi/.gitignore to exclude sync state.

In interactive mode (default when stdin is a terminal), presents a guided setup
wizard. Use flags for non-interactive CI/CD usage.

Server URL is resolved from (first match wins):
  1. --server flag
  2. KAPI_SERVER_URL environment variable / server.url in ~/.config/kapi/kapi.yaml
  3. Existing auth state (from kapi auth login)
  4. Built-in default (http://localhost:8080)

Use --anonymous to create an anonymous project (prints claim URL).
Use --email to create an anonymous project and receive the claim link by email.`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}

	// Non-interactive: flags provided or stdin is not a TTY.
	if initAnonymous || initEmail != "" || initServerURL != "" || initProjectID != "" || !isTTY() {
		return runInitNonInteractive(cwd)
	}

	// Interactive mode.
	return runInitInteractive(cwd)
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// resolveServerURL resolves the server URL using the init --server flag as the
// explicit override, then falling back to the shared resolution chain.
func resolveServerURL() string {
	return resolveServerURLFrom(initServerURL)
}

const serverURLHelp = `Server URL not configured. Set it via one of:
  kapi config --global server.url https://bowrain.example.com
  export KAPI_SERVER_URL=https://bowrain.example.com
  kapi init --server https://bowrain.example.com`

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

	if initSource != "" {
		cfg.Project.SourceLocale = model.LocaleID(initSource)
	}
	if initTargets != "" {
		cfg.Project.TargetLocales = parseTargetLocales(initTargets)
	}

	// If --project is specified, use it directly (requires auth).
	if initProjectID != "" {
		serverURL := resolveServerURL()
		if serverURL == "" {
			return fmt.Errorf("--server or KAPI_SERVER_URL is required when --project is specified")
		}
		auth, err := loadAuth()
		if err != nil {
			return fmt.Errorf("not authenticated with server (run: kapi auth login)")
		}
		if auth.ServerURL != serverURL {
			return fmt.Errorf("authenticated with different server (%s), please login to %s first", auth.ServerURL, serverURL)
		}
		cfg.Server = &project.ServerConfig{
			URL:       serverURL,
			ProjectID: initProjectID,
		}
		fmt.Printf("Connecting to Bowrain Server: %s\n", serverURL)
		fmt.Printf("Project ID: %s\n", initProjectID)
		return finishInit(cwd, cfg)
	}

	// Anonymous mode: --anonymous or --email.
	if initAnonymous || initEmail != "" {
		serverURL := resolveServerURL()
		if serverURL == "" {
			return fmt.Errorf("%s", serverURLHelp)
		}
		return runInitAnonymous(cwd, cfg, serverURL, initEmail)
	}

	// Default non-interactive: use auth if available, otherwise local only.
	auth, err := loadAuth()
	if err != nil {
		// No auth available — local only.
		return finishInit(cwd, cfg)
	}

	// Authenticated: create project on server.
	return runInitCreateAuthenticated(cwd, cfg, auth)
}

func runInitInteractive(cwd string) error {
	// Check if already logged in.
	stored, authErr := loadAuth()
	serverURL := resolveServerURL()

	if authErr == nil && stored.ServerURL != "" {
		// Already logged in — create project directly.
		dirName := filepath.Base(cwd)
		var projectName, sourceLocale string

		err := huh.NewForm(
			huh.NewGroup(
				huh.NewNote().Title(fmt.Sprintf("Bowrain: %s (signed in as %s)", stored.ServerURL, stored.User.Email)),
				huh.NewInput().
					Title("Project name").
					Value(&projectName).
					Placeholder(dirName),
				huh.NewInput().
					Title("Source locale").
					Value(&sourceLocale).
					Placeholder("en"),
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
		if initTargets != "" {
			cfg.Project.TargetLocales = parseTargetLocales(initTargets)
		}

		return runInitCreateAuthenticated(cwd, cfg, stored)
	}

	// Not logged in — show menu.
	options := []huh.Option[string]{
		huh.NewOption(fmt.Sprintf("Sign in to Bowrain (%s)", serverURL), "signin"),
		huh.NewOption("Email me a claim link", "email"),
		huh.NewOption("Continue without signing in", "anonymous"),
		huh.NewOption("Local only (no server)", "local"),
	}

	var choice string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("How would you like to set up your project?").
				Options(options...).
				Value(&choice),
		),
	).Run()
	if err != nil {
		return err
	}

	switch choice {
	case "signin":
		return runInitSignIn(cwd, serverURL)
	case "email":
		return runInitEmailClaim(cwd, serverURL)
	case "anonymous":
		return runInitAnonymousInteractive(cwd, serverURL)
	case "local":
		return runInitLocal(cwd)
	default:
		return fmt.Errorf("unexpected choice: %s", choice)
	}
}

// runInitSignIn performs the device auth flow, then creates the project.
func runInitSignIn(cwd, serverURL string) error {
	stored, err := performLogin(serverURL)
	if err != nil {
		return err
	}

	dirName := filepath.Base(cwd)
	var projectName, sourceLocale string

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Value(&projectName).
				Placeholder(dirName),
			huh.NewInput().
				Title("Source locale").
				Value(&sourceLocale).
				Placeholder("en"),
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
	if initTargets != "" {
		cfg.Project.TargetLocales = parseTargetLocales(initTargets)
	}

	return runInitCreateAuthenticated(cwd, cfg, stored)
}

// runInitEmailClaim creates an anonymous project and sends the claim email.
func runInitEmailClaim(cwd, serverURL string) error {
	dirName := filepath.Base(cwd)
	var projectName, sourceLocale, email string

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
				Title("Your email address").
				Value(&email).
				Placeholder("you@example.com"),
		),
	).Run()
	if err != nil {
		return err
	}

	if projectName == "" {
		projectName = dirName
	}
	if email == "" {
		return fmt.Errorf("email address is required")
	}

	cfg := project.DefaultConfig()
	cfg.Project.Name = projectName
	if sourceLocale != "" {
		cfg.Project.SourceLocale = model.LocaleID(sourceLocale)
	}
	if initTargets != "" {
		cfg.Project.TargetLocales = parseTargetLocales(initTargets)
	}

	return runInitAnonymous(cwd, cfg, serverURL, email)
}

// runInitAnonymousInteractive creates an anonymous project (no email).
func runInitAnonymousInteractive(cwd, serverURL string) error {
	dirName := filepath.Base(cwd)
	var projectName, sourceLocale string

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
	if initTargets != "" {
		cfg.Project.TargetLocales = parseTargetLocales(initTargets)
	}

	return runInitAnonymous(cwd, cfg, serverURL, "")
}

func runInitLocal(cwd string) error {
	dirName := filepath.Base(cwd)
	var projectName, sourceLocale string

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
	if initTargets != "" {
		cfg.Project.TargetLocales = parseTargetLocales(initTargets)
	}

	return finishInit(cwd, cfg)
}

// runInitAnonymous creates an anonymous project on the server.
// If email is non-empty, the server sends a claim email.
func runInitAnonymous(cwd string, cfg *project.Config, serverURL, email string) error {
	if cfg.Project.SourceLocale == "" {
		cfg.Project.SourceLocale = "en"
	}

	// Convert LocaleIDs to strings for API call.
	var targets []string
	for _, t := range cfg.Project.TargetLocales {
		targets = append(targets, string(t))
	}

	fmt.Printf("Creating project on %s...\n", serverURL)

	projectID, claimToken, err := client.CreateAnonymousProject(
		serverURL,
		cfg.Project.Name,
		string(cfg.Project.SourceLocale),
		targets,
		email,
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

	// Print next steps.
	claimURL := strings.TrimRight(serverURL, "/") + "/claim/" + claimToken
	fmt.Printf("\nProject created: %s\n", projectID)

	if email != "" {
		fmt.Printf("A claim link has been sent to %s\n", email)
	} else {
		fmt.Printf("Claim URL: %s\n", claimURL)
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  1. Run: kapi push    — upload content to the server")
	if email != "" {
		fmt.Println("  2. Check your email for the claim link to take ownership")
	} else {
		fmt.Println("  2. Open the claim URL to take ownership of the project")
	}
	fmt.Println("  3. Invite translators from the web dashboard")

	return nil
}

// runInitCreateAuthenticated creates a project on the server using existing auth.
func runInitCreateAuthenticated(cwd string, cfg *project.Config, auth *StoredAuth) error {
	if cfg.Project.SourceLocale == "" {
		cfg.Project.SourceLocale = "en"
	}

	var targets []string
	for _, t := range cfg.Project.TargetLocales {
		targets = append(targets, string(t))
	}

	fmt.Printf("Creating project on %s...\n", auth.ServerURL)

	projectID, err := client.CreateAuthenticatedProject(
		auth.ServerURL,
		auth.AccessToken,
		cfg.Project.Name,
		string(cfg.Project.SourceLocale),
		targets,
	)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}

	cfg.Server = &project.ServerConfig{
		URL:       auth.ServerURL,
		ProjectID: projectID,
	}

	if err := finishInit(cwd, cfg); err != nil {
		return err
	}

	fmt.Printf("\nProject created: %s\n", projectID)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Run: kapi push    — upload content to the server")
	fmt.Println("  2. Invite translators from the web dashboard")

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
		fmt.Println("  3. Run: kapi auth login")
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
    input: "locales/en.json"
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
	initCmd.Flags().StringVar(&initServerURL, "server", "", "Bowrain Server URL (overrides KAPI_SERVER_URL)")
	initCmd.Flags().StringVar(&initProjectID, "project", "", "Bowrain Server project ID (for connecting to existing project)")
	initCmd.Flags().StringVar(&initProjectName, "name", "", "Project name (default: current directory name)")
	initCmd.Flags().StringVar(&initSource, "source", "", "Source locale (default: en)")
	initCmd.Flags().StringVar(&initTargets, "targets", "", "Target locales, comma-separated (e.g., nb,fr)")
	initCmd.Flags().BoolVar(&initAnonymous, "anonymous", false, "Create an anonymous project on the server (prints claim URL)")
	initCmd.Flags().StringVar(&initEmail, "email", "", "Create anonymous project and send claim link to this email")

	rootCmd.AddCommand(initCmd)
}
