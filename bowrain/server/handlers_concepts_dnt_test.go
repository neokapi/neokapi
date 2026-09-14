package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// TestConceptAPI_CarriesDoNotTranslate creates a do-not-translate concept, reads
// it back, and updates it without and then with the flag. The create stores the
// flag and both responses carry it; an update that omits the flag keeps the
// stored value, and one that names it sets it.
func TestConceptAPI_CarriesDoNotTranslate(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()

	c, rec := h.req(http.MethodPost, "/concepts", `{
		"domain": "product",
		"definition": "The command-line tool.",
		"do_not_translate": true,
		"terms": [{"text": "kapi", "locale": "en", "status": "approved"}]
	}`, platauth.PermManageTerms)
	require.NoError(t, h.srv.HandleCreateConcept(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, true, created["do_not_translate"], "the create response carries the flag")
	id, _ := created["id"].(string)
	require.NotEmpty(t, id)

	stored, ok, err := h.tb(t).GetConcept(ctx, id)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, stored.DoNotTranslate, "the create stores the flag")

	c, rec = h.req(http.MethodGet, "/concepts/"+id, "", platauth.PermViewContent, "cid", id)
	require.NoError(t, h.srv.HandleGetConcept(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, true, got["do_not_translate"], "the get response carries the flag")

	c, rec = h.req(http.MethodPut, "/concepts/"+id, `{
		"domain": "product",
		"definition": "The kapi command-line tool.",
		"terms": [{"text": "kapi", "locale": "en", "status": "approved"}]
	}`, platauth.PermManageTerms, "cid", id)
	require.NoError(t, h.srv.HandleUpdateConcept(c))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	stored, _, err = h.tb(t).GetConcept(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "The kapi command-line tool.", stored.Definition)
	assert.True(t, stored.DoNotTranslate, "an update that omits the flag keeps it")

	c, rec = h.req(http.MethodPut, "/concepts/"+id, `{
		"domain": "product",
		"definition": "The kapi command-line tool.",
		"do_not_translate": false,
		"terms": [{"text": "kapi", "locale": "en", "status": "approved"}]
	}`, platauth.PermManageTerms, "cid", id)
	require.NoError(t, h.srv.HandleUpdateConcept(c))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	stored, _, err = h.tb(t).GetConcept(ctx, id)
	require.NoError(t, err)
	assert.False(t, stored.DoNotTranslate, "an update that names the flag sets it")
}
