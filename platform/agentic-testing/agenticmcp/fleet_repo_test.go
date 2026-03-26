package agenticmcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitFleetRepo_ListWorkspaces(t *testing.T) {
	// Create a fake fleet repo directory (no git, just files).
	dir := t.TempDir()

	// Create two workspace directories with plan.yaml and status.yaml.
	ws1 := filepath.Join(dir, "workspaces", "excalidraw-l10n")
	require.NoError(t, os.MkdirAll(ws1, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws1, "plan.yaml"), []byte(`
name: Excalidraw
source_language: en-US
target_languages: [fr-FR, de-DE]
upstream_repo: excalidraw/excalidraw
mode: real-time
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws1, "status.yaml"), []byte(`
phase: active
current_release: v0.18.1
`), 0o644))

	ws2 := filepath.Join(dir, "workspaces", "docusaurus-l10n")
	require.NoError(t, os.MkdirAll(ws2, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws2, "plan.yaml"), []byte(`
name: Docusaurus
source_language: en-US
target_languages: [fr-FR]
upstream_repo: facebook/docusaurus
mode: accelerated
`), 0o644))

	// Create a .git marker so ensureClone thinks it's already cloned.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	repo := &GitFleetRepo{LocalDir: dir}
	ctx := context.Background()

	workspaces, err := repo.ListWorkspaces(ctx)
	require.NoError(t, err)
	assert.Len(t, workspaces, 2)

	// Sort is by directory listing order (alphabetical on most OS).
	var slugs []string
	for _, ws := range workspaces {
		slugs = append(slugs, ws.Slug)
	}
	assert.Contains(t, slugs, "excalidraw-l10n")
	assert.Contains(t, slugs, "docusaurus-l10n")

	// Find excalidraw and verify fields.
	for _, ws := range workspaces {
		if ws.Slug == "excalidraw-l10n" {
			assert.Equal(t, "Excalidraw", ws.ProjectName)
			assert.Equal(t, "excalidraw/excalidraw", ws.UpstreamRepo)
			assert.Equal(t, []string{"fr-FR", "de-DE"}, ws.TargetLanguages)
			assert.Equal(t, "real-time", ws.Mode)
			assert.Equal(t, "active", ws.Phase)
		}
	}
}

func TestGitFleetRepo_GetWorkspacePlan(t *testing.T) {
	dir := t.TempDir()

	ws := filepath.Join(dir, "workspaces", "test-l10n")
	require.NoError(t, os.MkdirAll(ws, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "plan.yaml"), []byte(`
name: TestProject
source_language: en-US
target_languages: [ja-JP]
upstream_repo: test/project
mode: accelerated
release_strategy:
  mode: accelerated-then-realtime
  start_tag: v1.0.0
  end_tag: latest
  pace: 2h
content:
  hint: "Locale files are flat JSON in a locales/ directory."
  format: json
  source_file_pattern: "**/locales/en.json"
`), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	repo := &GitFleetRepo{LocalDir: dir}
	ctx := context.Background()

	plan, err := repo.GetWorkspacePlan(ctx, "test-l10n")
	require.NoError(t, err)

	assert.Equal(t, "TestProject", plan.ProjectName)
	assert.Equal(t, "en-US", plan.SourceLanguage)
	assert.Equal(t, []string{"ja-JP"}, plan.TargetLanguages)
	assert.Equal(t, "test/project", plan.UpstreamRepo)
	assert.Equal(t, "accelerated-then-realtime", plan.ReleaseStrategy.Mode)
	assert.Equal(t, "v1.0.0", plan.ReleaseStrategy.StartTag)
	assert.Equal(t, "json", plan.Content.Format)
	assert.Equal(t, "**/locales/en.json", plan.Content.SourceFilePattern)
	assert.Contains(t, plan.Content.Hint, "Locale files")
}

func TestGitFleetRepo_GetWorkspacePlan_NotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "workspaces"), 0o755))

	repo := &GitFleetRepo{LocalDir: dir}
	ctx := context.Background()

	_, err := repo.GetWorkspacePlan(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestGitFleetRepo_ListWorkspaces_Empty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	// No workspaces directory at all.

	repo := &GitFleetRepo{LocalDir: dir}
	ctx := context.Background()

	workspaces, err := repo.ListWorkspaces(ctx)
	require.NoError(t, err)
	assert.Empty(t, workspaces)
}

func TestGitFleetRepo_ListWorkspaces_SkipInvalidPlan(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	// Valid workspace.
	ws1 := filepath.Join(dir, "workspaces", "good")
	require.NoError(t, os.MkdirAll(ws1, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws1, "plan.yaml"), []byte("name: Good\n"), 0o644))

	// Workspace with no plan.yaml — should be skipped.
	ws2 := filepath.Join(dir, "workspaces", "no-plan")
	require.NoError(t, os.MkdirAll(ws2, 0o755))

	repo := &GitFleetRepo{LocalDir: dir}
	ctx := context.Background()

	workspaces, err := repo.ListWorkspaces(ctx)
	require.NoError(t, err)
	assert.Len(t, workspaces, 1)
	assert.Equal(t, "good", workspaces[0].Slug)
}

func TestGitFleetRepo_RepoURL(t *testing.T) {
	r := &GitFleetRepo{RepoURL: "https://github.com/neokapi/agentic-fleet.git"}
	assert.Equal(t, "https://github.com/neokapi/agentic-fleet.git", r.repoURL())

	r.Token = "ghp_abc123"
	assert.Equal(t, "https://x-access-token:ghp_abc123@github.com/neokapi/agentic-fleet.git", r.repoURL())

	r.RepoURL = "git@github.com:neokapi/agentic-fleet.git"
	assert.Equal(t, "git@github.com:neokapi/agentic-fleet.git", r.repoURL(), "SSH URLs should not be modified")
}

func TestReadStatus_Missing(t *testing.T) {
	r := &GitFleetRepo{}
	status := r.readStatus("/nonexistent/path")
	assert.Equal(t, "unknown", status.Phase)
}
