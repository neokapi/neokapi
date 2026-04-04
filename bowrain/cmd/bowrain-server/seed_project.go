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
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
)

// seedProject creates a project in a workspace and a dedicated CI push token.
// It is idempotent — re-running finds the existing project and rotates the push token.
//
// Output (stdout):
//
//	project_id=<id>
//	push_token=bwt_<hex>
func seedProject(cfg seedProjectConfig) {
	if err := runSeedProject(cfg); err != nil {
		slog.Error("seed-project failed", "error", err)
		os.Exit(1)
	}
}

func runSeedProject(cfg seedProjectConfig) error {
	if cfg.DatabaseURL == "" {
		return errors.New("BOWRAIN_DATABASE_URL is required")
	}
	if cfg.WorkspaceSlug == "" {
		return errors.New("BOWRAIN_SERVICE_WORKSPACE is required")
	}
	if cfg.ProjectName == "" {
		return errors.New("BOWRAIN_PROJECT_NAME is required")
	}
	if cfg.SourceLanguage == "" {
		return errors.New("BOWRAIN_SOURCE_LANGUAGE is required")
	}
	if cfg.TargetLanguages == "" {
		return errors.New("BOWRAIN_TARGET_LANGUAGES is required")
	}

	var db *storage.PgDB
	var err error
	if cfg.DatabaseAuth == "azure" {
		db, err = storage.OpenPostgresAzure(cfg.DatabaseURL, cfg.AzureClientID)
	} else {
		db, err = storage.OpenPostgres(cfg.DatabaseURL)
	}
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	authStore, err := auth.NewPostgresAuthStoreFromDB(db)
	if err != nil {
		return fmt.Errorf("init auth store: %w", err)
	}
	contentStore, err := store.NewPostgresStoreFromDB(db)
	if err != nil {
		return fmt.Errorf("init content store: %w", err)
	}

	// 1. Find the service user.
	user, err := authStore.GetUserByEmail(ctx, serviceUserEmail)
	if err != nil {
		return fmt.Errorf("service user not found — run seed-service-token first: %w", err)
	}

	// 2. Look up the workspace.
	ws, err := authStore.GetWorkspaceBySlug(ctx, cfg.WorkspaceSlug)
	if err != nil {
		return fmt.Errorf("workspace %q not found: %w", cfg.WorkspaceSlug, err)
	}

	// 3. Find or create the project.
	targets := parseTargetLanguages(cfg.TargetLanguages)
	projectID, err := findOrCreateProject(ctx, contentStore, ws.ID, cfg.ProjectName, cfg.SourceLanguage, targets)
	if err != nil {
		return err
	}

	// 4. Create a dedicated CI push token (rotate if exists).
	pushTokenName := "ci-push"
	tokens, err := authStore.ListAPITokens(ctx, ws.ID)
	if err == nil {
		for _, t := range tokens {
			if t.UserID == user.ID && t.Name == pushTokenName {
				_ = authStore.DeleteAPIToken(ctx, t.ID)
				slog.Info("rotated existing push token", "token_id", t.ID)
			}
		}
	}

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
		Name:        pushTokenName,
		TokenPrefix: plaintext[:8],
		Scopes:      `["*"]`,
	}
	if err := authStore.CreateAPIToken(ctx, apiToken, tokenHash); err != nil {
		return fmt.Errorf("create push token: %w", err)
	}

	fmt.Printf("project_id=%s\n", projectID)
	fmt.Printf("push_token=%s\n", plaintext)
	return nil
}

func findOrCreateProject(ctx context.Context, s *store.PostgresStore, workspaceID, name, sourceLang string, targets []model.LocaleID) (string, error) {
	// Check if project already exists.
	projects, err := s.ListProjects(ctx)
	if err == nil {
		for _, p := range projects {
			if p.WorkspaceID == workspaceID && p.Name == name {
				slog.Info("project already exists", "name", p.Name, "id", p.ID)
				return p.ID, nil
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
		return "", fmt.Errorf("create project: %w", err)
	}
	slog.Info("created project", "name", p.Name, "id", p.ID, "workspace", workspaceID)
	return p.ID, nil
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
