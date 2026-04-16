package backend

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests verify that online/offline routing works correctly.
// When disconnected (the default newTestApp state), all methods should
// fall back to local operations.

func TestListWorkspacesOffline(t *testing.T) {
	app := newTestApp(t)

	workspaces := app.ListWorkspaces()
	require.Len(t, workspaces, 1)
	assert.Equal(t, "Personal", workspaces[0].Name)
	assert.Equal(t, "personal", workspaces[0].Slug)
	assert.Equal(t, "owner", workspaces[0].Role)
}

func TestGetCurrentWorkspaceOffline(t *testing.T) {
	app := newTestApp(t)

	ws := app.GetCurrentWorkspace()
	assert.Equal(t, "Personal", ws.Name)
	assert.Equal(t, "personal", ws.Slug)
}

func TestListProjectsOffline(t *testing.T) {
	app := newTestApp(t)

	// No projects initially.
	projects := app.ListProjects()
	assert.Empty(t, projects)

	// Create a project.
	_, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// Should find it.
	projects = app.ListProjects()
	assert.Len(t, projects, 1)
	assert.Equal(t, "Test", projects[0].Name)
}

func TestGetProjectOffline(t *testing.T) {
	app := newTestApp(t)

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	got, err := app.GetProject(proj.ID)
	require.NoError(t, err)
	assert.Equal(t, proj.ID, got.ID)
	assert.Equal(t, "Test", got.Name)
}

func TestGetItemBlocksOffline(t *testing.T) {
	app := newTestApp(t)

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// Add an item with blocks.
	tmpDir := t.TempDir()
	writeTestFile(t, tmpDir, "hello.txt", "Hello World")
	_, err = app.AddItems(proj.ID, []string{tmpDir + "/hello.txt"})
	require.NoError(t, err)

	// Get blocks — should work offline.
	blocks, err := app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	assert.NotEmpty(t, blocks)
}

func TestUpdateBlockTargetOffline(t *testing.T) {
	app := newTestApp(t)

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	writeTestFile(t, tmpDir, "hello.txt", "Hello World")
	_, err = app.AddItems(proj.ID, []string{tmpDir + "/hello.txt"})
	require.NoError(t, err)

	blocks, err := app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	err = app.UpdateBlockTarget(UpdateBlockRequest{
		ProjectID:    proj.ID,
		ItemName:     "hello.txt",
		BlockID:      blocks[0].ID,
		TargetLocale: "fr",
		Text:         "Bonjour le monde",
	})
	require.NoError(t, err)

	// Verify the update persisted.
	blocks, err = app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", flattenTargetRuns(blocks[0], "fr"))
}

func TestReviewBlockOffline(t *testing.T) {
	app := newTestApp(t)

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	writeTestFile(t, tmpDir, "hello.txt", "Hello World")
	_, err = app.AddItems(proj.ID, []string{tmpDir + "/hello.txt"})
	require.NoError(t, err)

	blocks, err := app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	// First set a target.
	err = app.UpdateBlockTarget(UpdateBlockRequest{
		ProjectID:    proj.ID,
		ItemName:     "hello.txt",
		BlockID:      blocks[0].ID,
		TargetLocale: "fr",
		Text:         "Bonjour",
	})
	require.NoError(t, err)

	// Mark as reviewed.
	err = app.ReviewBlock(proj.ID, "hello.txt", blocks[0].ID, "fr", true)
	require.NoError(t, err)

	// Verify the review status.
	blocks, err = app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "reviewed", blocks[0].Properties["translation-status"])

	// Unmark reviewed.
	err = app.ReviewBlock(proj.ID, "hello.txt", blocks[0].ID, "fr", false)
	require.NoError(t, err)

	blocks, err = app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "translated", blocks[0].Properties["translation-status"])
}

func TestGetServerWorkspacesOffline(t *testing.T) {
	app := newTestApp(t)

	// Should return error when not connected.
	_, err := app.GetServerWorkspaces()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestTMOperationsOffline(t *testing.T) {
	app := newTestApp(t)

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// Add TM entry.
	entry, err := app.AddTMEntry(proj.ID, "Hello", "Bonjour", "en", "fr")
	require.NoError(t, err)
	assert.NotEmpty(t, entry.ID)

	// Get count.
	count, err := app.GetTMCount(proj.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Search.
	result, err := app.GetTMEntries(proj.ID, "", "en", "fr", 0, 10)
	require.NoError(t, err)
	assert.Len(t, result.Entries, 1)

	// Delete.
	err = app.DeleteTMEntry(proj.ID, entry.ID)
	require.NoError(t, err)

	count, err = app.GetTMCount(proj.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestTermOperationsOffline(t *testing.T) {
	app := newTestApp(t)

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// Add concept.
	concept, err := app.AddConcept(AddConceptRequest{
		ProjectID:  proj.ID,
		Domain:     "IT",
		Definition: "A program",
		Terms: []TermInfo{
			{Text: "software", Locale: "en", Status: "approved"},
			{Text: "logiciel", Locale: "fr", Status: "approved"},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, concept.ID)

	// Count.
	count, err := app.GetTermCount(proj.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Search.
	result, err := app.GetTerms(proj.ID, "", "en", "fr", 0, 10)
	require.NoError(t, err)
	assert.Len(t, result.Concepts, 1)

	// Delete.
	err = app.DeleteConcept(proj.ID, concept.ID)
	require.NoError(t, err)

	count, err = app.GetTermCount(proj.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// --- helpers ---

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := dir + "/" + name
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}
