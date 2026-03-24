package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	platauth "github.com/neokapi/neokapi/platform/auth"
	platstore "github.com/neokapi/neokapi/platform/store"
)

// seedProject creates a project in a workspace and a dedicated CI push token.
// It is idempotent — re-running finds the existing project and rotates the push token.
//
// Output (stdout):
//
//	project_id=<id>
//	push_token=bwt_<hex>
func seedProject(cfg seedProjectConfig) {
	if cfg.DatabaseURL == "" {
		log.Fatal("seed-project: BOWRAIN_DATABASE_URL is required")
	}
	if cfg.WorkspaceSlug == "" {
		log.Fatal("seed-project: BOWRAIN_SERVICE_WORKSPACE is required")
	}
	if cfg.ProjectName == "" {
		log.Fatal("seed-project: BOWRAIN_PROJECT_NAME is required")
	}
	if cfg.SourceLanguage == "" {
		log.Fatal("seed-project: BOWRAIN_SOURCE_LANGUAGE is required")
	}
	if cfg.TargetLanguages == "" {
		log.Fatal("seed-project: BOWRAIN_TARGET_LANGUAGES is required")
	}

	var db *storage.PgDB
	var err error
	if cfg.DatabaseAuth == "azure" {
		db, err = storage.OpenPostgresAzure(cfg.DatabaseURL, cfg.AzureClientID)
	} else {
		db, err = storage.OpenPostgres(cfg.DatabaseURL)
	}
	if err != nil {
		log.Fatalf("seed-project: open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	authStore, err := auth.NewPostgresAuthStoreFromDB(db)
	if err != nil {
		log.Fatalf("seed-project: init auth store: %v", err)
	}
	contentStore, err := store.NewPostgresStoreFromDB(db)
	if err != nil {
		log.Fatalf("seed-project: init content store: %v", err)
	}

	// 1. Find the service user.
	user, err := authStore.GetUserByEmail(ctx, serviceUserEmail)
	if err != nil {
		log.Fatalf("seed-project: service user not found — run seed-service-token first: %v", err)
	}

	// 2. Look up the workspace.
	ws, err := authStore.GetWorkspaceBySlug(ctx, cfg.WorkspaceSlug)
	if err != nil {
		log.Fatalf("seed-project: workspace %q not found: %v", cfg.WorkspaceSlug, err)
	}

	// 3. Find or create the project.
	targets := parseTargetLanguages(cfg.TargetLanguages)
	projectID := findOrCreateProject(ctx, contentStore, ws.ID, cfg.ProjectName, cfg.SourceLanguage, targets)

	// 4. Create a dedicated CI push token (rotate if exists).
	pushTokenName := "ci-push"
	tokens, err := authStore.ListAPITokens(ctx, ws.ID)
	if err == nil {
		for _, t := range tokens {
			if t.UserID == user.ID && t.Name == pushTokenName {
				_ = authStore.DeleteAPIToken(ctx, t.ID)
				log.Printf("Rotated existing push token %s", t.ID)
			}
		}
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Fatalf("seed-project: generate token: %v", err)
	}
	plaintext := "bwt_" + hex.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	apiToken := &platauth.APIToken{
		UserID:      user.ID,
		WorkspaceID: ws.ID,
		Name:        pushTokenName,
		TokenPrefix: plaintext[:8],
		Scopes:      `["*"]`,
	}
	if err := authStore.CreateAPIToken(ctx, apiToken, tokenHash); err != nil {
		log.Fatalf("seed-project: create push token: %v", err)
	}

	fmt.Printf("project_id=%s\n", projectID)
	fmt.Printf("push_token=%s\n", plaintext)
}

func findOrCreateProject(ctx context.Context, s *store.PostgresStore, workspaceID, name, sourceLang string, targets []model.LocaleID) string {
	// Check if project already exists.
	projects, err := s.ListProjects(ctx)
	if err == nil {
		for _, p := range projects {
			if p.WorkspaceID == workspaceID && p.Name == name {
				log.Printf("Project already exists: %s (%s)", p.Name, p.ID)
				return p.ID
			}
		}
	}

	p := &platstore.Project{
		Name:                  name,
		DefaultSourceLanguage: model.LocaleID(sourceLang),
		TargetLanguages:       targets,
		WorkspaceID:           workspaceID,
	}
	if err := s.CreateProject(ctx, p); err != nil {
		log.Fatalf("seed-project: create project: %v", err)
	}
	log.Printf("Created project %s (%s) in workspace %s", p.Name, p.ID, workspaceID)
	return p.ID
}

func parseTargetLanguages(csv string) []model.LocaleID {
	parts := strings.Split(csv, ",")
	locales := make([]model.LocaleID, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			locales = append(locales, model.LocaleID(p))
		}
	}
	return locales
}

type seedProjectConfig struct {
	DatabaseURL     string
	DatabaseAuth    string
	AzureClientID   string
	WorkspaceSlug   string
	ProjectName     string
	SourceLanguage  string
	TargetLanguages string
}
