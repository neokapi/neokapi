package client

import (
	"encoding/json"
	"testing"

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestItemMetaTagsMatchTheProtoSchema is the regression for a field that
// travelled under a name nothing was listening for.
//
// ItemMeta is marshaled into the push commit manifest and read back by the sync
// worker as a pb.SyncItemMeta. The two are separate Go types that meet only
// through JSON, so the field names have to agree — and BlockIndex's did not. It
// was tagged `block_index` against the generated `block_index_json`, which
// encoding/json resolves the only way it can: by dropping it. Every pushed
// item's block index went to the server and arrived empty, and nothing failed,
// because a manifest missing a field is a perfectly valid manifest.
//
// The round trip below is the assertion that a rename on either side has to
// keep passing.
func TestItemMetaTagsMatchTheProtoSchema(t *testing.T) {
	encoded, err := json.Marshal(ItemMeta{
		Name:        "locales/en.json",
		Format:      "json",
		BlockIndex:  `{"blocks":[]}`,
		PreviewHTML: "<p>hi</p>",
	})
	require.NoError(t, err)

	var decoded pb.SyncItemMeta
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, "locales/en.json", decoded.Name)
	assert.Equal(t, "json", decoded.Format)
	assert.Equal(t, `{"blocks":[]}`, decoded.BlockIndexJson,
		"the block index must survive the manifest round trip")
	assert.Equal(t, "<p>hi</p>", decoded.PreviewHtml)
}
