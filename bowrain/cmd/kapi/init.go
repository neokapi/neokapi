package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/gokapi/gokapi/bowrain/project"
	"github.com/gokapi/gokapi/model"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const defaultServerURL = "https://api.bowrain.cloud"

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

In interactive mode (default when stdin is a terminal), presents a guided setup
wizard. Use --server and --project flags for non-interactive CI/CD usage.`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}

	// Non-interactive: flags provided or stdin is not a TTY.
	if initServerURL != "" || !isTTY() {
		return runInitNonInteractive(cwd)
	}

	// Interactive mode.
	return runInitInteractive(cwd)
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func runInitNonInteractive(cwd string) error {
	cfg := project.DefaultConfig()

	if initProjectName != "" {
		cfg.Project.Name = initProjectName
	} else {
		cfg.Project.Name = filepath.Base(cwd)
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
					huh.NewOption("Quick start (no account needed)", "quick"),
					huh.NewOption("Sign in to Bowrain", "signin"),
					huh.NewOption("Local only (no server)", "local"),
				).
				Value(&choice),
		),
	).Run()
	if err != nil {
		return err
	}

	switch choice {
	case "quick":
		return runInitQuickStart(cwd)
	case "signin":
		return runInitAuthenticated(cwd)
	case "local":
		return runInitLocal(cwd)
	default:
		return fmt.Errorf("unexpected choice: %s", choice)
	}
}

func runInitQuickStart(cwd string) error {
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
				Placeholder("en-US"),
			huh.NewInput().
				Title("Target locales (comma-separated)").
				Value(&targetLocalesStr).
				Placeholder("fr-FR, de-DE, ja-JP"),
		),
	).Run()
	if err != nil {
		return err
	}

	if projectName == "" {
		projectName = dirName
	}
	if sourceLocale == "" {
		sourceLocale = "en-US"
	}
	if targetLocalesStr == "" {
		return fmt.Errorf("at least one target locale is required")
	}

	targetLocales := parseLocaleList(targetLocalesStr)

	// Create anonymous project on the cloud server.
	fmt.Println("Creating project on Bowrain Cloud...")
	projectID, claimToken, err := createAnonymousProject(defaultServerURL, projectName, sourceLocale, targetLocales)
	if err != nil {
		return fmt.Errorf("create anonymous project: %w", err)
	}

	cfg := project.DefaultConfig()
	cfg.Project.Name = projectName
	cfg.Project.SourceLocale = model.LocaleID(sourceLocale)
	cfg.Project.TargetLocales = toLocaleIDs(targetLocales)
	cfg.Server = &project.ServerConfig{
		URL:        defaultServerURL,
		ProjectID:  projectID,
		ClaimToken: claimToken,
	}

	if err := finishInit(cwd, cfg); err != nil {
		return err
	}

	fmt.Printf("\nProject created on Bowrain Cloud (ID: %s)\n", projectID)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Run: kapi push          (push your translations)")
	fmt.Println("  2. Run: kapi auth login    (sign up for a free account)")
	fmt.Println("  3. Run: kapi auth claim    (claim this project into your workspace)")
	return nil
}

func runInitAuthenticated(cwd string) error {
	stored, err := loadAuth()
	if err != nil {
		// Not logged in yet — trigger login.
		fmt.Println("You need to sign in first.")
		var serverURL string
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Bowrain Server URL").
					Value(&serverURL).
					Placeholder(defaultServerURL),
			),
		).Run()
		if err != nil {
			return err
		}
		if serverURL == "" {
			serverURL = defaultServerURL
		}
		fmt.Printf("\nRun this command to sign in:\n\n  kapi auth login --server %s\n\nThen re-run: kapi init\n", serverURL)
		return nil
	}

	// User is authenticated. Set up with server + project ID.
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

	// TODO: fetch workspaces and projects from server for selection

	fmt.Printf("Server: %s\n", stored.ServerURL)
	fmt.Printf("Logged in as: %s\n", stored.User.Email)
	fmt.Println("\nNote: Run kapi push/pull to sync with the server after init.")

	return finishInit(cwd, cfg)
}

func runInitLocal(cwd string) error {
	dirName := filepath.Base(cwd)
	var projectName string

	err := huh.NewForm(
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

	return finishInit(cwd, cfg)
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

func toLocaleIDs(ss []string) []model.LocaleID {
	ids := make([]model.LocaleID, len(ss))
	for i, s := range ss {
		ids[i] = model.LocaleID(s)
	}
	return ids
}

func parseLocaleList(s string) []string {
	parts := strings.Split(s, ",")
	var locales []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			locales = append(locales, p)
		}
	}
	return locales
}

// createAnonymousProject calls POST /api/v1/projects/anonymous on the server.
func createAnonymousProject(serverURL, name, sourceLocale string, targetLocales []string) (projectID, claimToken string, err error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"name":           name,
		"source_locale":  sourceLocale,
		"target_locales": targetLocales,
	})

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+"/api/v1/projects/anonymous", bytes.NewReader(reqBody))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("server returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		ProjectID  string `json:"project_id"`
		ClaimToken string `json:"claim_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	return result.ProjectID, result.ClaimToken, nil
}

func init() {
	initCmd.Flags().StringVar(&initServerURL, "server", "", "Bowrain Server URL (e.g., https://bowrain.example.com)")
	initCmd.Flags().StringVar(&initProjectID, "project", "", "Bowrain Server project ID")
	initCmd.Flags().StringVar(&initProjectName, "name", "", "Project name (default: current directory name)")

	rootCmd.AddCommand(initCmd)
}
