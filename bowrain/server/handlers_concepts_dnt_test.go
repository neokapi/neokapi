package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// TestConceptAPI_DoNotTranslateIsGoverned: setting or clearing the flag through
// the ordinary concept API is refused with the change-set hint, as a governed
// term status edit is. An update that omits the flag, or repeats the stored
// value, applies, and a response carries the stored flag.
func TestConceptAPI_DoNotTranslateIsGoverned(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	tb := h.tb(t)
	now := time.Now()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "c-kapi", Domain: "product", Definition: "The CLI.", DoNotTranslate: true,
		Terms:     []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermApproved}},
		CreatedAt: now, UpdatedAt: now,
	}))
	stored := func() terms.Concept {
		t.Helper()
		c, ok, err := tb.GetConcept(ctx, "c-kapi")
		require.NoError(t, err)
		require.True(t, ok)
		return c
	}
	refused := func(code int, body string) {
		t.Helper()
		assert.Equal(t, http.StatusConflict, code, body)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &got))
		assert.Equal(t, "governed change requires a change-set", got["error"])
	}

	c, rec := h.req(http.MethodPost, "/concepts", `{
		"domain": "product", "do_not_translate": true,
		"terms": [{"text": "Bowrain", "locale": "en", "status": "approved"}]
	}`, platauth.PermManageTerms)
	require.NoError(t, h.srv.HandleCreateConcept(c))
	refused(rec.Code, rec.Body.String())
	all, err := tb.Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1, "a refused create writes nothing")

	c, rec = h.req(http.MethodGet, "/concepts/c-kapi", "", platauth.PermViewContent, "cid", "c-kapi")
	require.NoError(t, h.srv.HandleGetConcept(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, true, got["do_not_translate"], "the response carries the stored flag")

	c, rec = h.req(http.MethodPut, "/concepts/c-kapi", `{
		"domain": "product", "definition": "Cleared.", "do_not_translate": false,
		"terms": [{"text": "kapi", "locale": "en", "status": "approved"}]
	}`, platauth.PermManageTerms, "cid", "c-kapi")
	require.NoError(t, h.srv.HandleUpdateConcept(c))
	refused(rec.Code, rec.Body.String())
	assert.True(t, stored().DoNotTranslate, "a refused update leaves the flag")
	assert.Equal(t, "The CLI.", stored().Definition, "a refused update writes nothing")

	c, rec = h.req(http.MethodPut, "/concepts/c-kapi", `{
		"domain": "product", "definition": "The kapi CLI.",
		"terms": [{"text": "kapi", "locale": "en", "status": "approved"}]
	}`, platauth.PermManageTerms, "cid", "c-kapi")
	require.NoError(t, h.srv.HandleUpdateConcept(c))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, "The kapi CLI.", stored().Definition)
	assert.True(t, stored().DoNotTranslate, "an update that omits the flag keeps it")

	c, rec = h.req(http.MethodPut, "/concepts/c-kapi", `{
		"domain": "product", "definition": "The kapi command-line tool.", "do_not_translate": true,
		"terms": [{"text": "kapi", "locale": "en", "status": "approved"}]
	}`, platauth.PermManageTerms, "cid", "c-kapi")
	require.NoError(t, h.srv.HandleUpdateConcept(c))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, "The kapi command-line tool.", stored().Definition, "repeating the stored flag applies")
}
