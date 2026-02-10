package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gokapi/gokapi/core/auth"
	"github.com/spf13/cobra"
)

// StoredAuth is the auth token persisted at ~/.config/gokapi/auth.json.
type StoredAuth struct {
	ServerURL    string     `json:"server_url"`
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token,omitempty"`
	Expiry       time.Time  `json:"expiry"`
	User         StoredUser `json:"user"`
}

// StoredUser is the user info stored alongside the token.
type StoredUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

var authServerURL string

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with a gokapi server",
	Long:  "Manage authentication tokens for accessing a remote gokapi server.",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in via OAuth device flow",
	Long:  "Start a device authorization flow to authenticate with a gokapi server.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if authServerURL == "" {
			return fmt.Errorf("--server flag is required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		client := &auth.DeviceFlowClient{
			DeviceAuthURL: authServerURL + "/api/v1/auth/device/start",
			TokenURL:      authServerURL + "/api/v1/auth/device/poll",
			ClientID:      "gokapi-cli",
		}

		fmt.Println("Starting device authorization flow...")
		resp, err := client.StartDeviceAuth(ctx)
		if err != nil {
			return fmt.Errorf("device auth start failed: %w", err)
		}

		fmt.Printf("\nOpen the following URL in your browser:\n\n  %s\n\n", resp.VerificationURI)
		fmt.Printf("Enter code: %s\n\n", resp.UserCode)
		fmt.Println("Waiting for authorization...")

		token, err := client.PollForToken(ctx, resp.DeviceCode, resp.Interval)
		if err != nil {
			return fmt.Errorf("authorization failed: %w", err)
		}

		// Validate the token and extract user info.
		// In a full implementation, we'd decode the ID token or call /auth/me.
		stored := StoredAuth{
			ServerURL:    authServerURL,
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			Expiry:       time.Now().Add(time.Duration(token.ExpiresIn) * time.Second),
		}

		if err := saveAuth(stored); err != nil {
			return fmt.Errorf("save token: %w", err)
		}

		fmt.Println("Login successful! Token saved.")
		return nil
	},
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
			fmt.Println("Not logged in.")
			return nil
		}

		fmt.Printf("Server:  %s\n", stored.ServerURL)
		if stored.User.Email != "" {
			fmt.Printf("User:    %s (%s)\n", stored.User.Name, stored.User.Email)
		}
		if stored.Expiry.IsZero() {
			fmt.Println("Expiry:  unknown")
		} else if time.Now().After(stored.Expiry) {
			fmt.Printf("Expiry:  %s (EXPIRED)\n", stored.Expiry.Format(time.RFC3339))
		} else {
			fmt.Printf("Expiry:  %s\n", stored.Expiry.Format(time.RFC3339))
		}
		return nil
	},
}

func init() {
	authLoginCmd.Flags().StringVar(&authServerURL, "server", "", "gokapi server URL (e.g., http://localhost:8080)")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}

func authFilePath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "gokapi", "auth.json")
}

func saveAuth(a StoredAuth) error {
	path := authFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func loadAuth() (*StoredAuth, error) {
	data, err := os.ReadFile(authFilePath())
	if err != nil {
		return nil, err
	}
	var a StoredAuth
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	return &a, nil
}
