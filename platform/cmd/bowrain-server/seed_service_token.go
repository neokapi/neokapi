package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/storage"
	platauth "github.com/neokapi/neokapi/platform/auth"
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
	if dbURL == "" {
		log.Fatal("seed-service-token: DATABASE_URL is required")
	}
	if workspaceSlug == "" {
		log.Fatal("seed-service-token: WORKSPACE is required (set BOWRAIN_SERVICE_WORKSPACE)")
	}

	var db *storage.PgDB
	var err error
	if dbAuth == "azure" {
		db, err = storage.OpenPostgresAzure(dbURL, azureClientID)
	} else {
		db, err = storage.OpenPostgres(dbURL)
	}
	if err != nil {
		log.Fatalf("seed-service-token: open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := auth.NewPostgresAuthStoreFromDB(db)
	if err != nil {
		log.Fatalf("seed-service-token: init auth store: %v", err)
	}

	// 1. Find or create the service user.
	user, err := store.GetUserByEmail(ctx, serviceUserEmail)
	if err != nil {
		user = &platauth.User{
			Email: serviceUserEmail,
			Name:  serviceUserName,
		}
		if err := store.CreateUser(ctx, user); err != nil {
			log.Fatalf("seed-service-token: create user: %v", err)
		}
		log.Printf("Created service user %s (%s)", user.Email, user.ID)
	} else {
		log.Printf("Service user already exists: %s (%s)", user.Email, user.ID)
	}

	// 2. Look up the workspace.
	ws, err := store.GetWorkspaceBySlug(ctx, workspaceSlug)
	if err != nil {
		log.Fatalf("seed-service-token: workspace %q not found: %v", workspaceSlug, err)
	}

	// 3. Ensure membership with member role (remove + re-add to fix stale viewer role).
	if m, err := store.GetMembership(ctx, ws.ID, user.ID); err == nil {
		if m.Role != platauth.RoleMember {
			_ = store.RemoveMember(ctx, ws.ID, user.ID)
			if err := store.AddMember(ctx, ws.ID, user.ID, platauth.RoleMember); err != nil {
				log.Fatalf("seed-service-token: upgrade member: %v", err)
			}
			log.Printf("Upgraded service user to member in workspace %s", workspaceSlug)
		} else {
			log.Printf("Service user already a member of workspace %s", workspaceSlug)
		}
	} else {
		if err := store.AddMember(ctx, ws.ID, user.ID, platauth.RoleMember); err != nil {
			log.Fatalf("seed-service-token: add member: %v", err)
		}
		log.Printf("Added service user to workspace %s as member", workspaceSlug)
	}

	// 4. Delete any existing token for this service account (rotate).
	tokens, err := store.ListAPITokens(ctx, ws.ID)
	if err == nil {
		for _, t := range tokens {
			if t.UserID == user.ID && t.Name == serviceTokenName {
				_ = store.DeleteAPIToken(ctx, t.ID)
				log.Printf("Rotated existing service token %s", t.ID)
			}
		}
	}

	// 5. Create new API token.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Fatalf("seed-service-token: generate token: %v", err)
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
		log.Fatalf("seed-service-token: create token: %v", err)
	}

	fmt.Println(plaintext)
}
