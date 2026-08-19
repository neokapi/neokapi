package json_test

import (
	"bytes"
	"encoding/json"
	"testing"

	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A block the collection does not declare translatable writes its source, even
// when it is carrying a target.
//
// `translatable: false` is the reader's answer to "may this be rewritten",
// derived from the collection's extraction rules. A target on such a block is
// leftover state — written when the rules were broader, or by a pass that did
// not consult them — and it is not a translation anyone decided on.
//
// It shipped: `tools.case-transform.category: "text-processing"` was written to
// a committed catalog as `"tekstbehandling"`, an identifier that a group-by
// splits on and an overlay matches its master by, rewritten into something that
// matches nothing. Thirty such blocks in one item were correctly marked
// non-translatable and each still carried a target; nothing asked them on the
// way out.
func TestWriterKeepsSourceForNonTranslatableBlocks(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	input := `{"displayName": "Case Transform", "category": "text-processing"}`

	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Both carry a target. Only the first may be rewritten.
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		block := p.Resource.(*model.Block)
		switch block.SourceText() {
		case "Case Transform":
			block.Translatable = true
			block.SetTargetText(model.LocaleID("nb"), "Endre bokstavtype")
		case "text-processing":
			block.Translatable = false
			block.SetTargetText(model.LocaleID("nb"), "tekstbehandling")
		}
	}

	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleID("nb"))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))

	assert.Equal(t, "Endre bokstavtype", result["displayName"],
		"a translatable block writes the translation")
	assert.Equal(t, "text-processing", result["category"],
		"a non-translatable block writes its source, target or no target")
}
