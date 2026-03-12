package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/neokapi/neokapi/bowrain-cli/cmd/bowrain/output"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/platform/client"
	"github.com/neokapi/neokapi/platform/config"
	"github.com/neokapi/neokapi/platform/project"
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
	initPreset      string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up a new project",
	Long: `Set up a new bowrain project in the current directory.

Creates the .bowrain/ folder with your project configuration and an example flow.

In interactive mode (default when stdin is a terminal), presents a guided setup
wizard. Use flags for non-interactive CI/CD usage.

The server URL is determined from (first match wins):
  1. --server flag
  2. BOWRAIN_SERVER_URL environment variable / server.url in ~/.config/bowrain/bowrain.yaml
  3. Existing auth state (from bowrain auth login)
  4. Built-in default (http://localhost:8080)

Use --anonymous to create a project without signing in.
Use --email to create a project and email a link to claim it.`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}

	// Fail fast if .bowrain/ already exists — before any server calls or prompts.
	bowrainDir := filepath.Join(cwd, project.BowrainDir)
	if _, err := os.Stat(bowrainDir); err == nil {
		return fmt.Errorf(".bowrain/ directory already exists at %s", cwd)
	}

	var result *output.InitOutput

	// Non-interactive: flags provided or stdin is not a TTY.
	if initAnonymous || initEmail != "" || initServerURL != "" || initProjectID != "" || !isTTY() {
		result, err = runInitNonInteractive(cwd)
	} else {
		result, err = runInitInteractive(cmd, cwd)
	}
	if err != nil {
		return err
	}

	return output.Print(cmd, *result)
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
  bowrain config --global server.url https://bowrain.example.com
  export BOWRAIN_SERVER_URL=https://bowrain.example.com
  bowrain init --server https://bowrain.example.com`

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

// newConfigFromFlags creates a default config with source/target from CLI flags.
func newConfigFromFlags(sourceLocale string) *project.Config {
	cfg := project.DefaultConfig()
	if sourceLocale != "" {
		cfg.Defaults.SourceLanguage = model.LocaleID(sourceLocale)
	} else if initSource != "" {
		cfg.Defaults.SourceLanguage = model.LocaleID(initSource)
	}
	if initTargets != "" {
		cfg.Defaults.TargetLanguages = parseTargetLocales(initTargets)
	}
	return cfg
}

func runInitNonInteractive(cwd string) (*output.InitOutput, error) {
	cfg := newConfigFromFlags("")

	// If --project is specified, use it directly (requires auth).
	if initProjectID != "" {
		serverURL := resolveServerURL()
		if serverURL == "" {
			return nil, fmt.Errorf("--server or BOWRAIN_SERVER_URL is required when --project is specified")
		}
		auth, err := loadAuth()
		if err != nil {
			return nil, fmt.Errorf("not authenticated with server (run: bowrain auth login)")
		}
		if auth.ServerURL != serverURL {
			return nil, fmt.Errorf("authenticated with different server (%s), please login to %s first", auth.ServerURL, serverURL)
		}
		cfg.URL = project.FormatProjectURL(serverURL, "", initProjectID)
		fmt.Printf("Connecting to Bowrain Server: %s\n", serverURL)
		fmt.Printf("Project ID: %s\n", initProjectID)
		return finishInit(cwd, cfg)
	}

	// Anonymous mode: --anonymous or --email.
	if initAnonymous || initEmail != "" {
		serverURL := resolveServerURL()
		if serverURL == "" {
			return nil, fmt.Errorf("%s", serverURLHelp)
		}
		projectName := initProjectName
		if projectName == "" {
			projectName = filepath.Base(cwd)
		}
		return runInitAnonymous(cwd, cfg, serverURL, projectName, initEmail)
	}

	// Default non-interactive: use auth if available, otherwise set server URL if provided.
	serverURL := resolveServerURL()
	auth, err := loadAuth()
	if err != nil {
		// No auth available — set server URL in config if provided, so the
		// project is pre-configured for later auth + push.
		if serverURL != "" {
			cfg.URL = project.FormatProjectURL(serverURL, "", "")
		}
		return finishInit(cwd, cfg)
	}

	// Authenticated: create project on server (defaults to personal workspace).
	projectName := initProjectName
	if projectName == "" {
		projectName = filepath.Base(cwd)
	}
	return runInitCreateAuthenticated(cwd, cfg, auth, "", projectName)
}

func runInitInteractive(cmd *cobra.Command, cwd string) (*output.InitOutput, error) {
	// Check if already logged in.
	stored, authErr := loadAuth()
	serverURL := resolveServerURL()

	if authErr == nil && stored.ServerURL != "" {
		// Already logged in — select workspace first, then project details.
		wsSlug, err := selectWorkspace(stored.ServerURL, stored.AccessToken)
		if err != nil {
			return nil, err
		}

		dirName := filepath.Base(cwd)
		var projectName, sourceLocale string

		err = huh.NewForm(
			huh.NewGroup(
				huh.NewNote().Title(fmt.Sprintf("Bowrain: %s (signed in as %s)", stored.ServerURL, stored.User.Email)),
				huh.NewInput().
					Title("Project name").
					Value(&projectName).
					Placeholder(dirName),
			),
			huh.NewGroup(
				localeInput("Source locale", &sourceLocale),
			),
		).Run()
		if err != nil {
			return nil, err
		}

		if projectName == "" {
			projectName = dirName
		}

		cfg := newConfigFromFlags(sourceLocale)
		return runInitCreateAuthenticated(cwd, cfg, stored, wsSlug, projectName)
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
		return nil, err
	}

	switch choice {
	case "signin":
		return runInitSignIn(cmd, cwd, serverURL)
	case "email":
		return runInitEmailClaim(cwd, serverURL)
	case "anonymous":
		return runInitAnonymousInteractive(cwd, serverURL)
	case "local":
		return runInitLocal(cwd)
	default:
		return nil, fmt.Errorf("unexpected choice: %s", choice)
	}
}

// runInitSignIn performs the device auth flow, then creates the project.
func runInitSignIn(cmd *cobra.Command, cwd, serverURL string) (*output.InitOutput, error) {
	stored, err := performLogin(cmd, serverURL)
	if err != nil {
		return nil, err
	}

	// Select workspace first, then project details.
	wsSlug, err := selectWorkspace(stored.ServerURL, stored.AccessToken)
	if err != nil {
		return nil, err
	}

	dirName := filepath.Base(cwd)
	var projectName, sourceLocale string

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Value(&projectName).
				Placeholder(dirName),
		),
		huh.NewGroup(
			localeInput("Source locale", &sourceLocale),
		),
	).Run()
	if err != nil {
		return nil, err
	}

	if projectName == "" {
		projectName = dirName
	}

	cfg := newConfigFromFlags(sourceLocale)
	return runInitCreateAuthenticated(cwd, cfg, stored, wsSlug, projectName)
}

// runInitEmailClaim creates an anonymous project and sends the claim email.
func runInitEmailClaim(cwd, serverURL string) (*output.InitOutput, error) {
	dirName := filepath.Base(cwd)
	var projectName, sourceLocale, email string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Value(&projectName).
				Placeholder(dirName),
		),
		huh.NewGroup(
			localeInput("Source locale", &sourceLocale),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Your email address").
				Value(&email).
				Placeholder("you@example.com"),
		),
	).Run()
	if err != nil {
		return nil, err
	}

	if projectName == "" {
		projectName = dirName
	}
	if email == "" {
		return nil, fmt.Errorf("email address is required")
	}

	cfg := newConfigFromFlags(sourceLocale)
	return runInitAnonymous(cwd, cfg, serverURL, projectName, email)
}

// runInitAnonymousInteractive creates an anonymous project (no email).
func runInitAnonymousInteractive(cwd, serverURL string) (*output.InitOutput, error) {
	dirName := filepath.Base(cwd)
	var projectName, sourceLocale string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Value(&projectName).
				Placeholder(dirName),
		),
		huh.NewGroup(
			localeInput("Source locale", &sourceLocale),
		),
	).Run()
	if err != nil {
		return nil, err
	}

	if projectName == "" {
		projectName = dirName
	}

	cfg := newConfigFromFlags(sourceLocale)
	return runInitAnonymous(cwd, cfg, serverURL, projectName, "")
}

func runInitLocal(cwd string) (*output.InitOutput, error) {
	dirName := filepath.Base(cwd)
	var projectName, sourceLocale string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Value(&projectName).
				Placeholder(dirName),
		),
		huh.NewGroup(
			localeInput("Source locale", &sourceLocale),
		),
	).Run()
	if err != nil {
		return nil, err
	}

	if projectName == "" {
		projectName = dirName
	}

	_ = projectName // Project name is used only for server creation; local projects derive from directory.
	cfg := newConfigFromFlags(sourceLocale)
	return finishInit(cwd, cfg)
}

// runInitAnonymous creates an anonymous project on the server.
// If email is non-empty, the server sends a claim email.
func runInitAnonymous(cwd string, cfg *project.Config, serverURL, projectName, email string) (*output.InitOutput, error) {
	if cfg.Defaults.SourceLanguage == "" {
		cfg.Defaults.SourceLanguage = "en"
	}

	var targets []string
	for _, t := range cfg.Defaults.TargetLanguages {
		targets = append(targets, string(t))
	}

	fmt.Printf("Creating project on %s...\n", serverURL)

	projectID, claimToken, err := client.CreateAnonymousProject(
		serverURL,
		projectName,
		string(cfg.Defaults.SourceLanguage),
		targets,
		email,
	)
	if err != nil {
		return nil, fmt.Errorf("create anonymous project: %w", err)
	}

	cfg.URL = project.FormatProjectURL(serverURL, "", projectID)

	out, err := finishInit(cwd, cfg)
	if err != nil {
		return nil, err
	}

	// Store claim token in sync cache (gitignored), not in config.yaml.
	bowrainDir := filepath.Join(cwd, project.BowrainDir)
	cache := project.LoadSyncCache(bowrainDir)
	cache.ClaimToken = claimToken
	if saveErr := cache.Save(bowrainDir); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save claim token to .sync-cache: %v\n", saveErr)
	}

	out.ClaimURL = strings.TrimRight(serverURL, "/") + "/claim/" + claimToken
	if email != "" {
		out.ClaimEmail = email
	}

	return out, nil
}

// runInitCreateAuthenticated creates a project on the server using existing auth.
func runInitCreateAuthenticated(cwd string, cfg *project.Config, auth *config.StoredAuth, workspace, projectName string) (*output.InitOutput, error) {
	if cfg.Defaults.SourceLanguage == "" {
		cfg.Defaults.SourceLanguage = "en"
	}

	var targets []string
	for _, t := range cfg.Defaults.TargetLanguages {
		targets = append(targets, string(t))
	}

	fmt.Printf("Creating project on %s...\n", auth.ServerURL)

	projectID, workspaceSlug, err := client.CreateAuthenticatedProject(
		auth.ServerURL,
		auth.AccessToken,
		projectName,
		string(cfg.Defaults.SourceLanguage),
		targets,
		workspace,
	)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	cfg.URL = project.FormatProjectURL(auth.ServerURL, workspaceSlug, projectID)

	return finishInit(cwd, cfg)
}

// selectWorkspace prompts the user to pick a workspace if multiple are
// available. If only one workspace exists it is returned automatically.
func selectWorkspace(serverURL, token string) (string, error) {
	workspaces, err := client.ListWorkspaces(serverURL, token)
	if err != nil {
		// Non-fatal: server may not support workspaces yet.
		return "", nil
	}
	if len(workspaces) == 0 {
		return "", nil
	}

	options := make([]huh.Option[string], 0, len(workspaces)+1)
	for _, ws := range workspaces {
		label := ws.Name
		if ws.Type == "personal" {
			label += " (personal)"
		}
		options = append(options, huh.NewOption(label, ws.Slug))
	}
	options = append(options, huh.NewOption("Create new workspace", "_create_"))

	var selected string
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which workspace?").
				Options(options...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return "", err
	}

	if selected == "_create_" {
		return createWorkspace(serverURL, token)
	}
	return selected, nil
}

// createWorkspace prompts for a workspace name, derives a slug, creates it
// on the server, and returns the new workspace's slug.
func createWorkspace(serverURL, token string) (string, error) {
	var name string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Workspace name").
				Value(&name).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("name is required")
					}
					return nil
				}),
		),
	).Run()
	if err != nil {
		return "", err
	}

	slug := toSlug(name)

	fmt.Printf("Creating workspace %q (%s)...\n", name, slug)
	ws, err := client.CreateWorkspace(serverURL, token, name, slug)
	if err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}
	fmt.Printf("Workspace created: %s\n", ws.Slug)
	return ws.Slug, nil
}

// toSlug converts a name to a URL-friendly slug.
func toSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	// Replace non-alphanumeric runs with a single hyphen.
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// localeSelect returns a filterable huh.Select field populated with BCP-47
// well-known locales. English is pre-selected as the default.
func localeInput(title string, value *string) huh.Field {
	locales := locale.WellKnownLocales()
	options := make([]huh.Option[string], 0, len(locales))
	for _, l := range locales {
		label := fmt.Sprintf("%s (%s)", l.DisplayName, l.Code)
		options = append(options, huh.NewOption(label, l.Code))
	}

	*value = "en"

	return huh.NewSelect[string]().
		Title(title).
		Description("Type / to filter").
		Options(options...).
		Value(value).
		Height(5)
}

func finishInit(cwd string, cfg *project.Config) (*output.InitOutput, error) {
	// Apply framework preset if specified.
	if initPreset != "" {
		if err := applyFrameworkPreset(cfg, initPreset); err != nil {
			return nil, err
		}
	}

	proj, err := project.InitProject(cwd, cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize project: %w", err)
	}

	if err := createExampleFlow(proj); err != nil {
		return nil, fmt.Errorf("create example flow: %w", err)
	}

	out := &output.InitOutput{
		Root:      proj.Root,
		ConfigDir: filepath.Join(proj.ConfigDir, project.ConfigFile),
	}

	if cfg.HasServer() {
		out.Server = cfg.ServerURL()
		out.ProjectID = cfg.ProjectID()
		out.Workspace = cfg.Workspace()
	}

	return out, nil
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

func applyFrameworkPreset(cfg *project.Config, presetName string) error {
	reg := app.PluginLoader.Presets()
	preset.RegisterBuiltins(reg)

	fp := reg.GetFrameworkPreset(presetName)
	if fp == nil {
		available := reg.ListFrameworkPresets()
		names := make([]string, len(available))
		for i, p := range available {
			names[i] = p.Name
		}
		return fmt.Errorf("unknown framework preset %q (available: %s)", presetName, strings.Join(names, ", "))
	}

	cfg.Preset = presetName

	// Apply mappings as content entries.
	for _, m := range fp.Mappings {
		cfg.Content = append(cfg.Content, project.ContentEntry{
			Path:   m.Local,
			Dest:   m.TargetPath,
			Format: m.Format,
		})
	}

	// Apply exclude patterns.
	cfg.Exclude = append(cfg.Exclude, fp.Exclude...)

	// Apply format preset overrides as local presets.
	if len(fp.FormatPresets) > 0 && cfg.FormatPresets == nil {
		cfg.FormatPresets = make(map[string]project.LocalFormatPreset)
	}
	for format, config := range fp.FormatPresets {
		cfg.FormatPresets[format] = project.LocalFormatPreset{
			Config: config,
		}
	}

	// Apply flow defaults.
	if len(fp.Flows) > 0 {
		if cfg.Flows == nil {
			cfg.Flows = make(map[string]map[string]any)
		}
		for flow, config := range fp.Flows {
			cfg.Flows[flow] = config
		}
	}

	return nil
}

func init() {
	output.AddFlags(initCmd)
	initCmd.Flags().StringVar(&initServerURL, "server", "", "server URL")
	initCmd.Flags().StringVar(&initProjectID, "project", "", "connect to an existing project by ID")
	initCmd.Flags().StringVar(&initProjectName, "name", "", "Project name (default: current directory name)")
	initCmd.Flags().StringVar(&initSource, "source", "", "Source locale (default: en)")
	initCmd.Flags().StringVar(&initTargets, "targets", "", "Target locales, comma-separated (e.g., nb,fr)")
	initCmd.Flags().BoolVar(&initAnonymous, "anonymous", false, "Create a project without signing in")
	initCmd.Flags().StringVar(&initEmail, "email", "", "Create a project and email a link to claim it")
	initCmd.Flags().StringVar(&initPreset, "preset", "", "apply a framework preset (e.g., nextjs, react-intl, angular)")

	rootCmd.AddCommand(initCmd)
}
