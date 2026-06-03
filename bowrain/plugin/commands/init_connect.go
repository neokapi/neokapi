package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/config"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

// init-connect is the bowrain contribution to the built-in `kapi init` command
// (manifest capabilities.command_contributions). `kapi init` scaffolds the
// recipe + state dir; when --server is given it then dispatches this handler to
// connect the project to a Bowrain server. It is idempotent: a project that
// already declares a server: block is left untouched, so re-running `kapi init`
// on a connected project is a no-op.
//
// It is hidden from the user-facing command surface — users run `kapi init`,
// not `kapi init-connect`.

var (
	connectServer    string
	connectAnonymous bool
	connectProjectID string
	connectEmail     string
	connectWorkspace string
)

var initConnectCmd = &cobra.Command{
	Use:          "init-connect",
	Short:        "Connect an existing kapi project to a Bowrain server",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         runInitConnect,
}

func runInitConnect(_ *cobra.Command, _ []string) error {
	// `kapi init` runs us in the project directory (KAPI_PROJECT_DIR); fall back
	// to cwd when invoked directly.
	startDir := os.Getenv("KAPI_PROJECT_DIR")
	if startDir == "" {
		startDir, _ = os.Getwd()
	}

	proj, err := project.FindProject(startDir)
	if err != nil {
		return fmt.Errorf("no kapi project found (run `kapi init` first): %w", err)
	}
	recipe := proj.Recipe

	// Idempotent: already connected → leave the recipe untouched.
	if recipe.HasServer() {
		fmt.Printf("Already connected to %s\n", recipe.Server.ServerURL())
		return nil
	}

	serverURL := connectServer
	if serverURL == "" {
		serverURL = resolveServerURL()
	}

	if recipe.Defaults.SourceLanguage == "" {
		recipe.Defaults.SourceLanguage = "en"
	}
	var targets []string
	for _, t := range recipe.Defaults.TargetLanguages {
		targets = append(targets, string(t))
	}
	projectName := recipe.Name
	if projectName == "" {
		projectName = filepath.Base(proj.Root)
	}

	switch {
	case connectProjectID != "":
		// Attach to an existing server project by ID (flat route; auth/claim is
		// resolved at push/pull time from stored credentials).
		setServerURL(recipe, project.FormatProjectURL(serverURL, "", connectProjectID))
		fmt.Printf("Linked to existing project %s on %s\n", connectProjectID, serverURL)

	case connectAnonymous:
		fmt.Printf("Creating project on %s...\n", serverURL)
		projectID, claimToken, err := client.CreateAnonymousProject(
			serverURL, projectName, string(recipe.Defaults.SourceLanguage), targets, connectEmail)
		if err != nil {
			return fmt.Errorf("create anonymous project on %s: %w", serverURL, err)
		}
		setServerURL(recipe, project.FormatProjectURL(serverURL, "", projectID))
		cache := project.LoadSyncCache(proj.Layout)
		cache.ClaimToken = claimToken
		if err := cache.Save(proj.Layout); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save claim token: %v\n", err)
		}
		fmt.Printf("Connected to %s\n  project: %s\n  claim:   %s/claim/%s\n",
			serverURL, projectID, strings.TrimRight(serverURL, "/"), claimToken)

	default:
		// Authenticated: requires an existing login for this server.
		auth, err := config.LoadAuth()
		if err != nil || auth == nil || auth.AccessToken == "" {
			return fmt.Errorf("not signed in; run `kapi auth login --server %s`, or pass --anonymous", serverURL)
		}
		// Target the explicit --server / BOWRAIN_SERVER_URL; fall back to the
		// server recorded at login. Under BOWRAIN_AUTH_TOKEN (CI/non-interactive)
		// auth.ServerURL is empty, so using it here posted to an empty URL.
		targetServer := serverURL
		if targetServer == "" {
			targetServer = auth.ServerURL
		}
		if targetServer == "" {
			return errors.New("server URL not configured — pass --server or set BOWRAIN_SERVER_URL")
		}
		fmt.Printf("Creating project on %s...\n", targetServer)
		// connectWorkspace ("" → resolve the account's workspace; non-empty →
		// create under that workspace, for users who belong to several).
		projectID, workspaceSlug, err := client.CreateAuthenticatedProject(
			targetServer, auth.AccessToken, projectName, string(recipe.Defaults.SourceLanguage), targets, connectWorkspace)
		if err != nil {
			return fmt.Errorf("create project: %w", err)
		}
		setServerURL(recipe, project.FormatProjectURL(targetServer, workspaceSlug, projectID))
		fmt.Printf("Connected to %s (workspace %s, project %s)\n", targetServer, workspaceSlug, projectID)
	}

	// Persist the server: block into the existing recipe.
	if err := recipe.Save(proj.RecipePath()); err != nil {
		return fmt.Errorf("save recipe: %w", err)
	}
	return nil
}

func init() {
	initConnectCmd.Flags().StringVar(&connectServer, "server", "", "Bowrain server URL")
	initConnectCmd.Flags().BoolVar(&connectAnonymous, "anonymous", false, "Create the project without signing in")
	initConnectCmd.Flags().StringVar(&connectProjectID, "project", "", "Attach to an existing server project by ID")
	initConnectCmd.Flags().StringVar(&connectEmail, "email", "", "Email a link to claim an anonymous project")
	initConnectCmd.Flags().StringVar(&connectWorkspace, "workspace", "", "Create the project in this workspace (slug); defaults to your only/first workspace")
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(initConnectCmd) })
}
