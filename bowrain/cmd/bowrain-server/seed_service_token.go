package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/storage"
)

const (
	serviceUserEmail = "agentic-orchestrator@bowrain.internal"
	serviceUserName  = "Agentic Orchestrator"
	serviceTokenName = "agentic-orchestrator"
)

// seedServiceToken creates a service account user, adds it to the given
// workspace, and creates an API token. It is idempotent — running it again
// rotates the token. The plaintext token is printed to stdout.
func seedServiceToken(dbURL, dbAuth, azureClientID, workspaceSlug string) {
	if err := runSeedServiceToken(dbURL, dbAuth, azureClientID, workspaceSlug); err != nil {
		slog.Error("seed-service-token failed", "error", err)
		os.Exit(1)
	}
}

func runSeedServiceToken(dbURL, dbAuth, azureClientID, workspaceSlug string) error {
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if workspaceSlug == "" {
		return errors.New("WORKSPACE is required (set BOWRAIN_SERVICE_WORKSPACE)")
	}

	var db *storage.PgDB
	var err error
	if dbAuth == "azure" {
		db, err = storage.OpenPostgresAzure(dbURL, azureClientID)
	} else {
		db, err = storage.OpenPostgres(dbURL)
	}
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := auth.NewAuthStoreFromDB(db)
	if err != nil {
		return fmt.Errorf("init auth store: %w", err)
	}

	// 1. Find or create the service user.
	user, err := store.GetUserByEmail(ctx, serviceUserEmail)
	if err != nil {
		user = &platauth.User{
			Email: serviceUserEmail,
			Name:  serviceUserName,
		}
		if err := store.CreateUser(ctx, user); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		slog.Info("created service user", "user_id", user.ID)
	} else {
		slog.Info("service user already exists", "user_id", user.ID)
	}

	// 2. Look up the workspace.
	ws, err := store.GetWorkspaceBySlug(ctx, workspaceSlug)
	if err != nil {
		return fmt.Errorf("workspace %q not found: %w", workspaceSlug, err)
	}

	// 3. Ensure membership with member role (remove + re-add to fix stale viewer role).
	if m, err := store.GetMembership(ctx, ws.ID, user.ID); err == nil {
		if m.Role != platauth.RoleMember {
			_ = store.RemoveMember(ctx, ws.ID, user.ID)
			if err := store.AddMember(ctx, ws.ID, user.ID, platauth.RoleMember); err != nil {
				return fmt.Errorf("upgrade member: %w", err)
			}
			slog.Info("upgraded service user to member", "workspace", workspaceSlug)
		} else {
			slog.Info("service user already a member", "workspace", workspaceSlug)
		}
	} else {
		if err := store.AddMember(ctx, ws.ID, user.ID, platauth.RoleMember); err != nil {
			return fmt.Errorf("add member: %w", err)
		}
		slog.Info("added service user to workspace", "workspace", workspaceSlug)
	}

	// 4. Delete any existing token for this service account (rotate).
	tokens, err := store.ListAPITokens(ctx, ws.ID)
	if err == nil {
		for _, t := range tokens {
			if t.UserID == user.ID && t.Name == serviceTokenName {
				_ = store.DeleteAPIToken(ctx, t.ID)
				slog.Info("rotated existing service token", "token_id", t.ID)
			}
		}
	}

	// 5. Create new API token.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	plaintext := "bwt_" + hex.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	apiToken := &platauth.APIToken{
		UserID:      user.ID,
		WorkspaceID: ws.ID,
		Name:        serviceTokenName,
		TokenPrefix: plaintext[:8],
		Scopes:      `["*"]`,
	}
	if err := store.CreateAPIToken(ctx, apiToken, tokenHash); err != nil {
		return fmt.Errorf("create token: %w", err)
	}

	fmt.Println(plaintext)
	return nil
}
