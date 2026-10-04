package backend

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/editorclient"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// servedShape is the block as the shared editor reads it (BlockInfo in
// @neokapi/ui, before normalizeServerBlock): the fields a run-based path reads.
type servedShape struct {
	Source          string                     `json:"source"`
	SourceRuns      json.RawMessage            `json:"source_runs"`
	Targets         map[string]BlockTargetInfo `json:"targets"`
	TargetsRuns     map[string]json.RawMessage `json:"targets_runs"`
	HasInlineCodes  bool                       `json:"has_inline_codes"`
	TargetRevisions map[string]string          `json:"target_revisions"`
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

// The working copy serves a block in the shape the shared editor reads, the
// one the server's blocks route serves: the source's text and its runs, each
// target and its runs, in the canonical run form. A code in the source or a
// translation reaches the editor as a code, so its run-based paths (a term
// insert, the plural seed) keep it.
func TestBlockInfo_ServesTheShapeTheSharedEditorReads(t *testing.T) {
	source := []model.Run{model.TextR("Hello "), model.PhR(model.PlaceholderRun{ID: "1", Equiv: "{name}"})}
	target := []model.Run{model.TextR("Bonjour "), model.PhR(model.PlaceholderRun{ID: "1", Equiv: "{name}"})}
	b := model.NewRunsBlock("b1", source)
	b.Translatable = true
	b.SetTargetRuns("fr", target)
	b.SetEditionStatus(model.Variant("fr"), model.Status(model.TargetStatusTranslated))

	var got servedShape
	require.NoError(t, json.Unmarshal([]byte(mustJSON(t, storedBlockToBlockInfo(&venue.StoredBlock{Block: b}, []string{"fr"}))), &got))

	assert.Equal(t, b.SourceText(), got.Source)
	assert.JSONEq(t, mustJSON(t, source), string(got.SourceRuns))
	assert.JSONEq(t, mustJSON(t, target), string(got.TargetsRuns["fr"]))
	assert.Equal(t, BlockTargetInfo{Text: b.TargetText("fr"), Status: "translated"}, got.Targets["fr"])
	assert.True(t, got.HasInlineCodes)
	assert.NotEmpty(t, got.TargetRevisions["fr"])
}

// The server serves a plain source as its text alone. The working copy keeps
// it when it caches the block, so the offline editor shows the source.
func TestBlockInfo_APlainServedBlockKeepsItsTextInTheCache(t *testing.T) {
	var served editorclient.EditorBlock
	require.NoError(t, json.Unmarshal([]byte(`{"id":"b1","source_id":"greeting","source":"Hello",
		"targets":{"fr":{"text":"Bonjour","status":"draft"}},"translatable":true,"has_inline_codes":false,
		"properties":{},"target_revisions":{"fr":"r:1"}}`), &served))

	info := editorBlockToInfo(served)
	assert.Equal(t, "Hello", info.Source)
	assert.Equal(t, "greeting", info.SourceID)

	var got servedShape
	require.NoError(t, json.Unmarshal([]byte(mustJSON(t, info)), &got))
	assert.Equal(t, "Hello", got.Source, "the editor reads the source's text")

	cached := blockInfoToBlock(info)
	assert.Equal(t, "Hello", cached.SourceText())
	assert.Equal(t, "Bonjour", cached.TargetText("fr"))
	fr, ok := cached.Edition(model.Variant("fr"))
	require.True(t, ok)
	assert.Equal(t, model.Status(model.TargetStatusDraft), fr.Status)
}
