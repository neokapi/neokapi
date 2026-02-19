package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	"github.com/gokapi/gokapi/platform/auth"
	"github.com/gokapi/gokapi/platform/config"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

// Type aliases for backwards compatibility within this package.
type StoredAuth = config.StoredAuth
type StoredUser = config.StoredUser

var authServerURL string

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Bowrain Server",
	Long:  "Manage authentication tokens for accessing a remote Bowrain Server.",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in via OAuth device flow",
	Long: `Start a device authorization flow to authenticate with Bowrain Server.

Server URL is resolved from (first match wins):
  1. --server flag
  2. KAPI_SERVER_URL environment variable / server.url in ~/.config/kapi/kapi.yaml
  3. Built-in default (http://localhost:8080)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := resolveServerURLFrom(authServerURL)
		if serverURL == "" {
			return fmt.Errorf("server URL not configured — set KAPI_SERVER_URL or use --server")
		}
		_, err := performLogin(serverURL)
		return err
	},
}

// performLogin runs the device authorization flow for the given server URL.
// On success, stores the credentials and returns the StoredAuth.
func performLogin(serverURL string) (*StoredAuth, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := &auth.DeviceFlowClient{
		DeviceAuthURL: serverURL + "/api/v1/auth/device/start",
		TokenURL:      serverURL + "/api/v1/auth/device/poll",
		ClientID:      "kapi-cli",
	}

	fmt.Println("Starting device authorization flow...")
	resp, err := client.StartDeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("device auth start failed: %w", err)
	}

	fmt.Printf("\nOpen the following URL in your browser:\n\n  %s\n\n", resp.VerificationURI)
	fmt.Printf("Enter code: %s\n\n", resp.UserCode)
	fmt.Println("Waiting for authorization...")

	token, err := client.PollForToken(ctx, resp.DeviceCode, resp.Interval)
	if err != nil {
		return nil, fmt.Errorf("authorization failed: %w", err)
	}

	stored := StoredAuth{
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

	if stored.User.Email != "" {
		fmt.Printf("Logged in as %s\n", stored.User.Email)
	} else {
		fmt.Println("Login successful! Token saved.")
	}
	return &stored, nil
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored authentication token",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := authFilePath()
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No stored token found.")
				return nil
			}
			return fmt.Errorf("remove token: %w", err)
		}
		fmt.Println("Logged out. Token removed.")
		return nil
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, err := loadAuth()
		if err != nil {
			out := output.AuthStatusOutput{LoggedIn: false}
			return output.Print(cmd, out)
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
	Short: "Claim an anonymous project into your workspace",
	Long: `Claim an anonymous project by providing a claim token.

If no token argument is given, reads the claim_token from .kapi/config.yaml.
Requires authentication (run: kapi auth login first).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		stored, err := loadAuth()
		if err != nil {
			return fmt.Errorf("not authenticated — run: kapi auth login")
		}

		var claimToken string
		if len(args) > 0 {
			claimToken = args[0]
		} else {
			// Try to read from .kapi/config.yaml.
			proj, err := project.FindProject("")
			if err != nil {
				return fmt.Errorf("no claim token provided and no .kapi/ project found")
			}
			if proj.Config.Server == nil || proj.Config.Server.ClaimToken == "" {
				return fmt.Errorf("no claim_token in .kapi/config.yaml — provide token as argument")
			}
			claimToken = proj.Config.Server.ClaimToken
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

		// Update .kapi/config.yaml: remove claim_token, update project_id.
		proj, err := project.FindProject("")
		if err == nil && proj.Config.Server != nil {
			proj.Config.Server.ClaimToken = ""
			proj.Config.Server.ProjectID = result.ProjectID
			proj.Config.Server.Workspace = result.WorkspaceSlug
			if saveErr := project.SaveConfig(proj.KapiDir, proj.Config); saveErr != nil {
				fmt.Printf("Warning: could not update .kapi/config.yaml: %v\n", saveErr)
			}
		}

		fmt.Printf("Project claimed into workspace %q\n", result.WorkspaceSlug)
		fmt.Printf("Project ID: %s\n", result.ProjectID)
		return nil
	},
}

func init() {
	authLoginCmd.Flags().StringVar(&authServerURL, "server", "", "Bowrain Server URL (e.g., http://localhost:8080)")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authClaimCmd)
	rootCmd.AddCommand(authCmd)
}

// resolveServerURLFrom resolves the server URL from an explicit override,
// then global config (env + file), then auth state, then baked-in default.
func resolveServerURLFrom(explicit string) string {
	if explicit != "" {
		return explicit
	}
	// Check global config (includes KAPI_SERVER_URL env via Viper BindEnv).
	cfg := config.NewAppConfig()
	_ = cfg.Load()
	if u := cfg.ServerURL(); u != "" {
		return u
	}
	// Fall back to auth state.
	if stored, err := loadAuth(); err == nil && stored.ServerURL != "" {
		return stored.ServerURL
	}
	return ""
}

func authFilePath() string { return config.AuthFilePath() }

func saveAuth(a StoredAuth) error { return config.SaveAuth(a) }

func loadAuth() (*StoredAuth, error) { return config.LoadAuth() }

// fetchUserInfo calls /api/v1/auth/me to get user details from the server.
func fetchUserInfo(serverURL, token string) (*StoredUser, error) {
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/v1/auth/me", nil)
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

	var user StoredUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}
