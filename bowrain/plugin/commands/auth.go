package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/config"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
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
  3. Built-in default (http://localhost:8080)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := resolveServerURLFrom(authServerURL)
		if serverURL == "" {
			return errors.New("server URL not configured — set BOWRAIN_SERVER_URL or use --server")
		}
		_, err := performLogin(cmd, serverURL)
		return err
	},
}

// performLogin runs the device authorization flow for the given server URL.
// On success, stores the credentials and returns the stored auth info.
func performLogin(cmd *cobra.Command, serverURL string) (*config.StoredAuth, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := &auth.DeviceFlowClient{
		DeviceAuthURL: serverURL + "/api/v1/auth/device/start",
		TokenURL:      serverURL + "/api/v1/auth/device/poll",
		ClientID:      "kapi-cli",
	}

	fmt.Fprintln(os.Stderr, "Starting device authorization flow...")
	resp, err := client.StartDeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("device auth start failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nOpen the following URL in your browser:\n\n  %s\n\n", resp.VerificationURI)
	fmt.Fprintf(os.Stderr, "Enter code: %s\n\n", resp.UserCode)
	fmt.Fprintln(os.Stderr, "Waiting for authorization...")

	token, err := client.PollForToken(ctx, resp.DeviceCode, resp.Interval)
	if err != nil {
		return nil, fmt.Errorf("authorization failed: %w", err)
	}

	stored := config.StoredAuth{
		ServerURL:    serverURL,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(token.ExpiresIn) * time.Second),
	}

	// Fetch user info from /auth/me using the new token.
	if user, err := fetchUserInfo(serverURL, token.AccessToken); err == nil {
		stored.User = *user
	}

	if err := saveAuth(stored); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}

	if err := output.Print(cmd, output.AuthLoginOutput{
		Server: serverURL,
		User:   stored.User.Email,
	}); err != nil {
		return nil, err
	}
	return &stored, nil
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out",
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, loadErr := loadAuth()
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
		stored, err := loadAuth()
		if err != nil {
			out := output.AuthStatusOutput{LoggedIn: false}
			return output.Print(cmd, out)
		}

		// In token mode (BOWRAIN_AUTH_TOKEN, e.g. CI) LoadAuth carries only the
		// server + token, not a local profile. Fetch the user from the server so
		// status shows who you are instead of a blank user; degrade gracefully
		// (keep it blank) if the lookup fails.
		if stored.User.Email == "" && stored.AccessToken != "" && stored.ServerURL != "" {
			if u, ferr := fetchUserInfo(stored.ServerURL, stored.AccessToken); ferr == nil && u != nil {
				stored.User = *u
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
(<project>/.kapi/cache/sync-cache.json).
Requires authentication (run 'kapi auth login' first).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, err := loadAuth()
		if err != nil {
			return errors.New("not authenticated — run: kapi auth login")
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
				return errors.New("no claim token in the project sync cache — provide token as argument")
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
			return fmt.Errorf("claim failed (HTTP %d): %s", resp.StatusCode, respBody)
		}

		var result struct {
			ProjectID     string `json:"project_id"`
			WorkspaceSlug string `json:"workspace_slug"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("decode claim response: %w", err)
		}

		// Update the recipe's `server.url` to point at the workspace project.
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

// resolveServerURLFrom resolves the server URL from an explicit override,
// then global config (env + file), then auth state, then baked-in default.
func resolveServerURLFrom(explicit string) string {
	if explicit != "" {
		return explicit
	}
	// Check bowrain config (includes BOWRAIN_SERVER_URL env via Viper BindEnv).
	cfg := newBowrainAppConfig()
	_ = cfg.Load()
	if u := cfg.GetString("server.url"); u != "" {
		return u
	}
	// Fall back to auth state.
	if stored, err := loadAuth(); err == nil && stored.ServerURL != "" {
		return stored.ServerURL
	}
	return ""
}

func saveAuth(a config.StoredAuth) error { return config.SaveAuth(a) }

func loadAuth() (*config.StoredAuth, error) { return config.LoadAuth() }

// fetchUserInfo calls /api/v1/auth/me to get user details from the server.
func fetchUserInfo(serverURL, token string) (*config.StoredUser, error) {
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/v1/auth/me", nil) //nolint:noctx // CLI auth helper
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("auth/me returned %d: %s", resp.StatusCode, body)
	}

	var user config.StoredUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}
