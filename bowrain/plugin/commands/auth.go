package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	clivenue "github.com/neokapi/neokapi/cli/venue"

	venueauth "github.com/neokapi/neokapi/host/venue/auth"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/spf13/cobra"
)

var authServerURL string

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Log in to the server",
	Long:  "Log in, log out, or check your authentication status.",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to the server",
	Long: `Log in to a Bowrain server. Opens a browser for authentication.

Server URL is resolved from (first match wins):
  1. --server flag
  2. BOWRAIN_SERVER_URL environment variable / server.url in ~/.config/bowrain/bowrain.yaml
  3. The server of the stored login on this machine
  4. The hosted service (https://app.bowrain.cloud); self-hosted deployments
     set BOWRAIN_SERVER_URL or pass --server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := performLogin(cmd, clivenue.ResolveServerURLOrDefault(authServerURL))
		return err
	},
}

// performLogin runs the device grant and reports the result in this command
// tree's output format. The grant itself is host/venue/auth's — what a
// successful login should print is the caller's call, and `init` folds it into
// the project it is scaffolding rather than announcing it separately.
func performLogin(cmd *cobra.Command, serverURL string) (*config.StoredAuth, error) {
	stored, err := venueauth.PerformLogin(cmd.Context(), serverURL, cmd.ErrOrStderr())
	if err != nil {
		return nil, err
	}
	if err := output.Print(cmd, output.AuthLoginOutput{
		Server: serverURL,
		User:   stored.User.Email,
	}); err != nil {
		return nil, err
	}
	return stored, nil
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out",
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, loadErr := config.LoadAuth()
		serverURL := ""
		if loadErr == nil && stored != nil {
			serverURL = stored.ServerURL
		}
		if err := config.DeleteAuth(serverURL); err != nil {
			return fmt.Errorf("clear credentials: %w", err)
		}
		return output.Print(cmd, output.AuthLogoutOutput{WasLoggedIn: loadErr == nil})
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show login status",
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, err := config.LoadAuth()
		if err != nil {
			out := output.AuthStatusOutput{LoggedIn: false}
			return output.Print(cmd, out)
		}

		// In token mode (BOWRAIN_AUTH_TOKEN, e.g. CI) LoadAuth carries only the
		// server + token, not a local profile. Fetch the user from the server so
		// status shows who you are instead of a blank user; degrade gracefully
		// (keep it blank) if the lookup fails.
		if stored.User.Email == "" && stored.AccessToken != "" && stored.ServerURL != "" {
			if u, ferr := apiclient.FetchUser(cmd.Context(), stored.ServerURL, stored.AccessToken); ferr == nil && u != nil {
				stored.User = config.StoredUser{ID: u.ID, Email: u.Email, Name: u.Name}
			}
		}

		var expiry *time.Time
		if !stored.Expiry.IsZero() {
			expiry = &stored.Expiry
		}

		out := output.AuthStatusOutput{
			LoggedIn:  true,
			Server:    stored.ServerURL,
			User:      stored.User.Email,
			UserID:    stored.User.ID,
			ExpiresAt: expiry,
		}
		if stored.User.Name != "" && stored.User.Name != stored.User.Email {
			out.User = stored.User.Name + " (" + stored.User.Email + ")"
		}

		return output.Print(cmd, out)
	},
}

var authClaimCmd = &cobra.Command{
	Use:   "claim [claim-token]",
	Short: "Claim a project into your workspace",
	Long: `Take ownership of a project by providing a claim token.

If no token is given, it is read from the project's sync cache
(<project>/.kapi/work/cache/sync-cache.json).
Requires authentication (run 'kapi auth login' first).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, err := config.LoadAuth()
		if err != nil {
			return errors.New("not authenticated. Run: kapi auth login")
		}

		var claimToken string
		if len(args) > 0 {
			claimToken = args[0]
		} else {
			// Try to read the claim token from the project's sync cache.
			proj, err := project.FindProject("")
			if err != nil {
				return errors.New("no claim token provided and no kapi project found")
			}
			cache := project.LoadSyncCache(proj.Layout)
			if cache.ClaimToken == "" {
				return errors.New("no claim token in the project sync cache. Provide token as argument")
			}
			claimToken = cache.ClaimToken
		}

		// Call server to claim.
		body, _ := json.Marshal(map[string]string{"claim_token": claimToken})
		req, err := http.NewRequest(http.MethodPost, stored.ServerURL+"/api/v1/projects/claim", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+stored.AccessToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("claim request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return apiclient.NewStatusError("claim", resp.StatusCode, respBody)
		}

		var result struct {
			ProjectID     string `json:"project_id"`
			WorkspaceSlug string `json:"workspace_slug"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("decode claim response: %w", err)
		}

		// Update the recipe's `bowrain.url` to point at the workspace project.
		proj, err := project.FindProject("")
		if err == nil && proj.Recipe.HasServer() {
			proj.Recipe.Server.URL = project.FormatProjectURL(
				proj.Recipe.Server.ServerURL(),
				result.WorkspaceSlug,
				result.ProjectID,
			)
			if saveErr := proj.Save(); saveErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not update recipe %s: %v\n", proj.RecipePath(), saveErr)
			}
			// Clear claim token from sync cache — no longer needed after claim.
			cache := project.LoadSyncCache(proj.Layout)
			cache.ClaimToken = ""
			_ = cache.Save(proj.Layout)
		}

		return output.Print(cmd, output.AuthClaimOutput{
			ProjectID:     result.ProjectID,
			WorkspaceSlug: result.WorkspaceSlug,
		})
	},
}

func init() {
	authLoginCmd.Flags().StringVar(&authServerURL, "server", "", "server URL")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authClaimCmd)
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(authCmd) })
}
