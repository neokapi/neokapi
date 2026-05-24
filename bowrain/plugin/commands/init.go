package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/config"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	coreproj "github.com/neokapi/neokapi/core/project"
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

Creates a <name>.kapi recipe and a .kapi/ state directory next to it,
plus an example flow under .kapi/flows/.

In interactive mode (default when stdin is a terminal), presents a guided setup
wizard. Use flags for non-interactive CI/CD usage.

The server URL is determined from (first match wins):
  1. --server flag
  2. BOWRAIN_SERVER_URL environment variable / server.url in ~/.config/bowrain/bowrain.yaml
  3. Existing auth state (from kapi auth login)
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

	// Fail fast if a recipe or .kapi/ state dir already exists in cwd —
	// before any server calls or prompts.
	if existing, err := existingRecipePath(cwd); err == nil && existing != "" {
		return fmt.Errorf("kapi recipe already exists at %s", existing)
	}
	if info, err := os.Stat(filepath.Join(cwd, coreproj.StateDirName)); err == nil && info.IsDir() {
		return fmt.Errorf("kapi state directory already exists at %s", filepath.Join(cwd, coreproj.StateDirName))
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

// existingRecipePath returns the path of any *.kapi recipe already present
// in dir, or "" if none. An error is returned only on read failures.
func existingRecipePath(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == coreproj.RecipeExt {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", nil
}

// resolveServerURL resolves the server URL using the init --server flag as the
// explicit override, then falling back to the shared resolution chain.
func resolveServerURL() string {
	return resolveServerURLFrom(initServerURL)
}

const serverURLHelp = `Server URL not configured. Set it via one of:
  kapi config --global server.url https://bowrain.example.com
  export BOWRAIN_SERVER_URL=https://bowrain.example.com
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

// newRecipeFromFlags builds a default recipe with source/target from CLI flags.
func newRecipeFromFlags(sourceLocale string) *project.Recipe {
	r := &project.Recipe{
		KapiProject: coreproj.KapiProject{
			Version: coreproj.CurrentVersion,
		},
	}
	switch {
	case sourceLocale != "":
		r.Defaults.SourceLanguage = model.LocaleID(sourceLocale)
	case initSource != "":
		r.Defaults.SourceLanguage = model.LocaleID(initSource)
	}
	if initTargets != "" {
		r.Defaults.TargetLanguages = parseTargetLocales(initTargets)
	}
	return r
}

// setServerURL sets the server URL on a recipe, allocating ServerSpec when
// needed. An empty url leaves Server unset.
func setServerURL(r *project.Recipe, url string) {
	if url == "" {
		return
	}
	if r.Server == nil {
		r.Server = &project.ServerSpec{}
	}
	r.Server.URL = url
}

func runInitNonInteractive(cwd string) (*output.InitOutput, error) {
	recipe := newRecipeFromFlags("")

	// If --project is specified, use it directly (requires auth).
	if initProjectID != "" {
		serverURL := resolveServerURL()
		if serverURL == "" {
			return nil, errors.New("--server or BOWRAIN_SERVER_URL is required when --project is specified")
		}
		auth, err := loadAuth()
		if err != nil {
			return nil, errors.New("not authenticated with server (run: kapi auth login)")
		}
		if auth.ServerURL != serverURL {
			return nil, fmt.Errorf("authenticated with different server (%s), please login to %s first", auth.ServerURL, serverURL)
		}
		setServerURL(recipe, project.FormatProjectURL(serverURL, "", initProjectID))
		fmt.Printf("Connecting to Bowrain Server: %s\n", serverURL)
		fmt.Printf("Project ID: %s\n", initProjectID)
		return finishInit(cwd, recipe)
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
		return runInitAnonymous(cwd, recipe, serverURL, projectName, initEmail)
	}

	// Default non-interactive: use auth if available, otherwise set server URL if provided.
	serverURL := resolveServerURL()
	auth, err := loadAuth()
	if err != nil {
		// No auth available — set server URL in the recipe if provided, so
		// the project is pre-configured for later auth + push.
		if serverURL != "" {
			setServerURL(recipe, project.FormatProjectURL(serverURL, "", ""))
		}
		return finishInit(cwd, recipe)
	}

	// Authenticated: create project on server (defaults to personal workspace).
	projectName := initProjectName
	if projectName == "" {
		projectName = filepath.Base(cwd)
	}
	return runInitCreateAuthenticated(cwd, recipe, auth, "", projectName)
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

		recipe := newRecipeFromFlags(sourceLocale)
		return runInitCreateAuthenticated(cwd, recipe, stored, wsSlug, projectName)
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

	recipe := newRecipeFromFlags(sourceLocale)
	return runInitCreateAuthenticated(cwd, recipe, stored, wsSlug, projectName)
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
		return nil, errors.New("email address is required")
	}

	recipe := newRecipeFromFlags(sourceLocale)
	return runInitAnonymous(cwd, recipe, serverURL, projectName, email)
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

	recipe := newRecipeFromFlags(sourceLocale)
	return runInitAnonymous(cwd, recipe, serverURL, projectName, "")
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
	recipe := newRecipeFromFlags(sourceLocale)
	return finishInit(cwd, recipe)
}

// runInitAnonymous creates an anonymous project on the server.
// If email is non-empty, the server sends a claim email.
func runInitAnonymous(cwd string, recipe *project.Recipe, serverURL, projectName, email string) (*output.InitOutput, error) {
	if recipe.Defaults.SourceLanguage == "" {
		recipe.Defaults.SourceLanguage = "en"
	}

	var targets []string
	for _, t := range recipe.Defaults.TargetLanguages {
		targets = append(targets, string(t))
	}

	fmt.Printf("Creating project on %s...\n", serverURL)

	projectID, claimToken, err := client.CreateAnonymousProject(
		serverURL,
		projectName,
		string(recipe.Defaults.SourceLanguage),
		targets,
		email,
	)
	if err != nil {
		return nil, fmt.Errorf("create anonymous project: %w", err)
	}

	setServerURL(recipe, project.FormatProjectURL(serverURL, "", projectID))

	out, err := finishInit(cwd, recipe)
	if err != nil {
		return nil, err
	}

	// Store claim token in the sync cache (gitignored), not in the recipe.
	proj, err := project.FindProject(cwd)
	if err == nil {
		cache := project.LoadSyncCache(proj.Layout)
		cache.ClaimToken = claimToken
		if saveErr := cache.Save(proj.Layout); saveErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save claim token to sync cache: %v\n", saveErr)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Warning: could not load project to save claim token: %v\n", err)
	}

	out.ClaimURL = strings.TrimRight(serverURL, "/") + "/claim/" + claimToken
	if email != "" {
		out.ClaimEmail = email
	}

	return out, nil
}

// runInitCreateAuthenticated creates a project on the server using existing auth.
func runInitCreateAuthenticated(cwd string, recipe *project.Recipe, auth *config.StoredAuth, workspace, projectName string) (*output.InitOutput, error) {
	if recipe.Defaults.SourceLanguage == "" {
		recipe.Defaults.SourceLanguage = "en"
	}

	var targets []string
	for _, t := range recipe.Defaults.TargetLanguages {
		targets = append(targets, string(t))
	}

	fmt.Printf("Creating project on %s...\n", auth.ServerURL)

	projectID, workspaceSlug, err := client.CreateAuthenticatedProject(
		auth.ServerURL,
		auth.AccessToken,
		projectName,
		string(recipe.Defaults.SourceLanguage),
		targets,
		workspace,
	)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	setServerURL(recipe, project.FormatProjectURL(auth.ServerURL, workspaceSlug, projectID))

	return finishInit(cwd, recipe)
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
						return errors.New("name is required")
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

func finishInit(cwd string, recipe *project.Recipe) (*output.InitOutput, error) {
	// Apply framework preset if specified.
	if initPreset != "" {
		if err := applyFrameworkPreset(recipe, initPreset); err != nil {
			return nil, err
		}
	}

	if initProjectName != "" && recipe.Name == "" {
		recipe.Name = initProjectName
	}

	proj, err := project.InitProject(cwd, recipe)
	if err != nil {
		return nil, fmt.Errorf("initialize project: %w", err)
	}

	if err := writeStateGitignore(proj); err != nil {
		return nil, fmt.Errorf("write state .gitignore: %w", err)
	}

	if err := createExampleFlow(proj); err != nil {
		return nil, fmt.Errorf("create example flow: %w", err)
	}

	out := &output.InitOutput{
		Root:      proj.Root,
		ConfigDir: proj.RecipePath(),
	}

	if recipe.HasServer() {
		out.Server = recipe.Server.ServerURL()
		out.ProjectID = recipe.Server.ProjectID()
		out.Workspace = recipe.Server.Workspace()
	}

	return out, nil
}

// writeStateGitignore drops a .gitignore inside the .kapi/ state dir so the
// regenerable cache subdir is excluded from version control.
func writeStateGitignore(proj *project.Project) error {
	gitignorePath := filepath.Join(proj.StateDir(), ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		return nil
	}
	return os.WriteFile(gitignorePath, []byte("cache/\n"), 0o644)
}

func createExampleFlow(proj *project.Project) error {
	flowsDir := proj.FlowsDirPath()
	if err := os.MkdirAll(flowsDir, 0o755); err != nil {
		return fmt.Errorf("create flows dir: %w", err)
	}
	flowPath := filepath.Join(flowsDir, "pseudo.yaml")

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

func applyFrameworkPreset(recipe *project.Recipe, presetName string) error {
	reg := preset.NewPresetRegistry()
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

	recipe.Preset = presetName

	// Apply mappings as bare content entries on the recipe.
	for _, m := range fp.Mappings {
		entry := coreproj.ContentCollection{
			Path:   m.Local,
			Target: m.TargetPath,
		}
		if m.Format != "" {
			entry.Format = &coreproj.FormatSpec{Name: m.Format}
		}
		recipe.Content = append(recipe.Content, entry)
	}

	// Apply exclude patterns.
	recipe.Defaults.Exclude = append(recipe.Defaults.Exclude, fp.Exclude...)

	// Apply format preset overrides as Defaults.Formats entries.
	if len(fp.FormatPresets) > 0 && recipe.Defaults.Formats == nil {
		recipe.Defaults.Formats = make(map[string]coreproj.FormatDefaults)
	}
	for format, cfg := range fp.FormatPresets {
		recipe.Defaults.Formats[format] = coreproj.FormatDefaults{
			Config: cfg,
		}
	}

	// Per-flow defaults from framework presets are not yet expressible on
	// KapiProject (its Flows map holds StepsSpec definitions, not configs).
	// TODO: re-introduce per-flow config when the framework gains a
	// FlowDefaults map.

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

	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(initCmd) })
}
