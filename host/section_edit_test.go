package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/sectionedit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sectionTestSource(t *testing.T) (string, []byte) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "guide.md")
	source := []byte("## Share\n\nFirst old paragraph.\n\nSecond old paragraph.\n\n## Keep\n\nUntouched.\n")
	require.NoError(t, os.WriteFile(file, source, 0o640))
	return file, source
}

func TestSectionApplyPreviewAndReinspection(t *testing.T) {
	app := newToolboxApp(t)
	file, original := sectionTestSource(t)
	doc, err := app.InspectSections(t.Context(), file)
	require.NoError(t, err)
	edit := sectionedit.Edit{
		ID: doc.Sections[0].ID, Snapshot: doc.Snapshot,
		Text: "A clearer introduction.\n\n### Next step\n\nDo this.\n\nThen do this.\n",
	}
	preview, err := app.ApplySectionEdit(t.Context(), file, edit, true, "")
	require.NoError(t, err)
	assert.Equal(t, "preview", preview.Status)
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, original, data)
	result, err := app.ApplySectionEdit(t.Context(), file, edit, false, ".bak")
	require.NoError(t, err)
	assert.Equal(t, "applied", result.Status)
	assert.Equal(t, preview.Plan, result.Plan)
	data, err = os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, preview.Data, data)
	backup, err := os.ReadFile(file + ".bak")
	require.NoError(t, err)
	assert.Equal(t, original, backup)
	info, err := os.Stat(file)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	_, err = app.ApplySectionEdit(t.Context(), file, edit, false, "")
	require.ErrorIs(t, err, sectionedit.ErrStale)
	unchanged, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, data, unchanged)
}

func TestSectionCommitRejectsConcurrentChanges(t *testing.T) {
	file, original := sectionTestSource(t)
	info, err := os.Stat(file)
	require.NoError(t, err)
	doc, err := sectionedit.Inspect(t.Context(), "markdown", original)
	require.NoError(t, err)
	prepared, err := sectionedit.Prepare(t.Context(), "markdown", original, sectionedit.Edit{
		ID: doc.Sections[0].ID, Snapshot: doc.Snapshot, Text: "A new body.",
	})
	require.NoError(t, err)
	concurrent := append(bytes.Clone(original), []byte("Someone else's new content.\n")...)
	require.NoError(t, os.WriteFile(file, concurrent, 0o640))
	err = commitSectionFile(t.Context(), sectionCommit{file: file, info: info, original: original, prepared: prepared})
	require.ErrorIs(t, err, sectionedit.ErrStale)
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, concurrent, data)
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(file), ".kapi-section-*"))
	require.NoError(t, err)
	assert.Empty(t, temps)
}

func TestSectionChangeSetRejectsMixedEntriesBeforeWriting(t *testing.T) {
	app := newToolboxApp(t)
	file, original := sectionTestSource(t)
	entries := []changeEntry{
		{Kind: kindContent, File: file, ID: "tu2", Text: "Should not land"},
		{Kind: kindSection, File: file, ID: "tu1", Snapshot: sectionedit.Snapshot(original), Text: "New body"},
	}
	data, err := json.Marshal(entries)
	require.NoError(t, err)
	changes := filepath.Join(t.TempDir(), "changes.json")
	require.NoError(t, os.WriteFile(changes, data, 0o600))
	err = app.RunApply(NewEnvCommand(t.Context(), "apply"), changes, false, "", true)
	require.ErrorContains(t, err, "one section entry")
	data, err = os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, original, data)
}

func TestSectionMCPReadsPreviewsAndAppliesSamePlan(t *testing.T) {
	app := newToolboxApp(t)
	file, original := sectionTestSource(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerEditMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "section-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	read, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "inspect_sections", Arguments: map[string]any{"file": file},
	})
	require.NoError(t, err)
	require.False(t, read.IsError)
	body, err := json.Marshal(read.StructuredContent)
	require.NoError(t, err)
	var doc sectionedit.Document
	require.NoError(t, json.Unmarshal(body, &doc))
	entry := changeEntry{
		Kind: kindSection, File: file, ID: doc.Sections[0].ID, Snapshot: doc.Snapshot,
		Text: "A useful introduction.\n\nA useful next step.\n",
	}
	for _, preview := range []bool{true, false} {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name: "apply_edits", Arguments: applyEditsInput{Changeset: []changeEntry{entry}, Preview: preview},
		})
		require.NoError(t, err)
		require.False(t, result.IsError, "%+v", result)
		body, err := json.Marshal(result.StructuredContent)
		require.NoError(t, err)
		var output applyEditsMCPOutput
		require.NoError(t, json.Unmarshal(body, &output))
		require.NotNil(t, output.Section)
		require.Len(t, output.Section.Plan.Patches, 1)
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		if preview {
			assert.Equal(t, "preview", output.Section.Status)
			assert.Equal(t, original, data)
		} else {
			assert.Equal(t, "applied", output.Section.Status)
			assert.Contains(t, string(data), "A useful next step.")
		}
	}
}
