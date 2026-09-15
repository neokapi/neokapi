package mcp

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// An assistant updates a target that changed after it read the block. The
// update is refused, naming the wording that stands, and nothing is written.
func TestHandleUpdateBlock_AStaleUpdateIsRefused(t *testing.T) {
	ms := newTestMCPServerWithContent(t)
	ctx := t.Context()

	p := &store.Project{Name: "Test", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, ms.contentStore.CreateProject(ctx, p))
	b := model.NewBlock("b1", "Hello")
	b.SetTargetText("fr", "Bonjour")
	require.NoError(t, ms.contentStore.StoreBlocks(ctx, p.ID, "main", []*model.Block{b}))

	var getIn getBlockInput
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"project_id":%q,"block_id":"b1"}`, p.ID)), &getIn))
	_, got, err := ms.handleGetBlock(ctx, nil, getIn)
	require.NoError(t, err)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	var read struct {
		Revisions map[string]string `json:"revisions"`
	}
	require.NoError(t, json.Unmarshal(raw, &read))

	sb, err := ms.contentStore.GetBlock(ctx, p.ID, "main", "b1")
	require.NoError(t, err)
	sb.Block.SetTargetText("fr", "Salut")
	require.NoError(t, ms.contentStore.StoreBlocks(ctx, p.ID, "main", []*model.Block{sb.Block}))

	var updateIn updateBlockInput
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(
		`{"project_id":%q,"block_id":"b1","target_locale":"fr","target_text":"Coucou","base_revision":%q}`,
		p.ID, read.Revisions["fr"])), &updateIn))
	_, _, err = ms.handleUpdateBlock(ctx, nil, updateIn)
	require.Error(t, err, "an update of wording that changed since the read is refused")
	assert.Contains(t, err.Error(), "Salut", "the refusal names the wording that stands")

	sb, err = ms.contentStore.GetBlock(ctx, p.ID, "main", "b1")
	require.NoError(t, err)
	assert.Equal(t, "Salut", sb.Block.TargetText("fr"), "nothing is written")
}
