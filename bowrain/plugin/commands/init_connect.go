package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	clivenue "github.com/neokapi/neokapi/cli/venue"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/neokapi/neokapi/host/venue/project"
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

// venueProject is what the recipe tells a venue about itself when it
// connects: its own id when it has one, the name it goes by, and its
// languages. The venue keeps the id, so the project is named the same way
// locally and there, and a second clone connecting with the same id is
// answered with the project the venue already holds. A recipe with no id
// sends none and the venue mints one.
func venueProject(recipe *project.Recipe, name string) client.NewProject {
	var targets []string
	for _, t := range recipe.Defaults.TargetLanguages {
		targets = append(targets, string(t))
	}
	return client.NewProject{
		ID:            recipe.ID,
		Name:          name,
		SourceLocale:  string(recipe.Defaults.SourceLanguage),
		TargetLocales: targets,
	}
}

func runInitConnect(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// `kapi init` runs us in the project directory (KAPI_PROJECT_DIR); fall back
	// to cwd when invoked directly.
	startDir := os.Getenv("KAPI_PROJECT_DIR")
	if startDir == "" {
		startDir, _ = os.Getwd()
	}

	proj, err := project.Load(startDir)
	if err != nil {
		return fmt.Errorf("no kapi project found (run `kapi init` first): %w", err)
	}
	recipe := proj.Recipe

	// Idempotent: already connected → leave the recipe untouched.
	if recipe.HasServer() {
		fmt.Printf("Already connected to %s\n", recipe.Server.ServerURL())
		return nil
	}

	serverURL := clivenue.ResolveServerURLOrDefault(connectServer)

	if recipe.Defaults.SourceLanguage == "" {
		recipe.Defaults.SourceLanguage = "en"
	}
	projectName := recipe.Name
	if projectName == "" {
		projectName = filepath.Base(proj.Root)
	}
	venueProj := venueProject(recipe, projectName)

	switch {
	case connectProjectID != "":
		// Attach to an existing server project by ID (flat route; auth/claim is
		// resolved at push/pull time from stored credentials).
		setServerURL(recipe, project.FormatProjectURL(serverURL, "", connectProjectID))
		fmt.Printf("Linked to existing project %s on %s\n", connectProjectID, serverURL)

	case connectAnonymous:
		fmt.Printf("Creating project on %s...\n", serverURL)
		projectID, claimToken, err := client.CreateAnonymousProject(ctx, serverURL, venueProj, connectEmail)
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
			return errors.New("server URL not configured. Pass --server or set BOWRAIN_SERVER_URL")
		}
		fmt.Printf("Creating project on %s...\n", targetServer)
		// connectWorkspace ("" → resolve the account's workspace; non-empty →
		// create under that workspace, for users who belong to several).
		projectID, workspaceSlug, err := client.CreateAuthenticatedProject(
			ctx, targetServer, auth.AccessToken, venueProj, connectWorkspace)
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
