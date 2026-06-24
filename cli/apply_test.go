package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadChangeSetJSONLAndArray proves the change-set reader accepts both the
// JSONL (one entry per line) and JSON-array forms, skipping blank lines.
func TestReadChangeSetJSONLAndArray(t *testing.T) {
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "cs.jsonl")
	require.NoError(t, os.WriteFile(jsonl, []byte(
		`{"kind":"content","file":"a.md","id":"p1","text":"x"}

{"kind":"term","op":"upsert","term":"t"}
`), 0o644))
	got, err := readChangeSet(context.Background(), jsonl)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, kindContent, got[0].Kind)
	assert.Equal(t, "p1", got[0].ID)
	assert.Equal(t, kindTerm, got[1].Kind)

	arr := filepath.Join(dir, "cs.json")
	require.NoError(t, os.WriteFile(arr, []byte(
		`[{"kind":"content","file":"a.md","id":"p1","text":"x"}]`), 0o644))
	got, err = readChangeSet(context.Background(), arr)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "p1", got[0].ID)
}

// TestApplyContentFaithfulRoundTrip proves a content change-set lands through
// the faithful round-trip: only the targeted value changes, the JSON skeleton is
// byte-identical, and the report records the applied block. Matched by canonical
// content_hash so the test is independent of the reader's block-id scheme. No
// provider is used.
func TestApplyContentFaithfulRoundTrip(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "en.json")
	const src = `{"greeting":"Hello world","note":"Keep me"}`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	hash := model.ComputeContentHash("Hello world")
	report := &coretools.ApplyReport{}
	byID, byHash := buildEditMaps([]changeEntry{
		{Kind: kindContent, File: path, ContentHash: hash, Text: "Hi planet"},
	})
	tl := coretools.NewApplyEditsTool(byID, byHash, report)
	require.NoError(t, app.editDocument(context.Background(), path, tl, "", true, "", nil))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	// note value + keys + braces are byte-identical; only greeting changed.
	assert.Equal(t, `{"greeting":"Hi planet","note":"Keep me"}`, string(got))
	assert.Len(t, report.Applied, 1)
}

// TestApplyContentNoOpByteIdentical proves feeding a block's current text back
// is an idempotent no-op that leaves the file byte-for-byte unchanged.
func TestApplyContentNoOpByteIdentical(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "en.json")
	const src = `{"greeting":"Hello world"}`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	hash := model.ComputeContentHash("Hello world")
	report := &coretools.ApplyReport{}
	byID, byHash := buildEditMaps([]changeEntry{
		{Kind: kindContent, File: path, ContentHash: hash, Text: "Hello world"},
	})
	tl := coretools.NewApplyEditsTool(byID, byHash, report)
	require.NoError(t, app.editDocument(context.Background(), path, tl, "", true, "", nil))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, src, string(got))
	assert.Len(t, report.Skipped, 1)
	assert.Empty(t, report.Applied)
}

func TestBuildEditMaps_IDAndHash(t *testing.T) {
	byID, byHash := buildEditMaps([]changeEntry{
		{ID: "p1", Text: "a", ContentHash: "h1"},
		{ContentHash: "h2", Text: "b"},
	})
	assert.Equal(t, coretools.Edit{Text: "a", ContentHash: "h1"}, byID["p1"])
	assert.Equal(t, coretools.Edit{Text: "b", ContentHash: "h2"}, byHash["h2"])
	_, hasH2InID := byID["h2"]
	assert.False(t, hasH2InID, "an id-less entry must not be keyed by id")
}
