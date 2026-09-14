package backend

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerms_AddConceptSetsDoNotTranslate: the desktop edits a local terms store
// directly, as it sets term statuses, so an add-concept request that names the
// do-not-translate flag stores it, and the concept view carries it.
func TestTerms_AddConceptSetsDoNotTranslate(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTerms(t, app)

	var req AddConceptRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"domain": "product", "do_not_translate": true,
		"terms": [{"text": "kapi", "locale": "en", "status": "preferred"}]
	}`), &req))
	require.NoError(t, app.AddConcept(handle, req))

	tb, ok := app.tbHandles.Get(handle)
	require.True(t, ok)
	concepts, err := tb.Concepts(context.Background())
	require.NoError(t, err)
	require.Len(t, concepts, 1)
	assert.True(t, concepts[0].DoNotTranslate, "the store holds the flag")

	view, err := json.Marshal(conceptToDTO(concepts[0]))
	require.NoError(t, err)
	var dto map[string]any
	require.NoError(t, json.Unmarshal(view, &dto))
	assert.Equal(t, true, dto["do_not_translate"], "the concept view carries the flag")
}
