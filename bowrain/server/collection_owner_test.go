package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ownership gate on the collections API, against a real SQLite content
// store: a collection the recipe declares cannot be updated, deleted or
// uploaded into over the API, and the refusal says the recipe decides. A
// workspace-owned collection is untouched by the gate.

// seedOwnedCollection stores a non-default collection under the given owner.
func seedOwnedCollection(t *testing.T, srv *Server, projectID, name, owner string) *platstore.Collection {
	t.Helper()
	coll := &platstore.Collection{
		ProjectID: projectID,
		Name:      name,
		Kind:      platstore.CollectionUploaded,
		ItemLabel: "item",
		Owner:     owner,
	}
	require.NoError(t, srv.ContentStore.CreateCollection(t.Context(), coll))
	return coll
}

func TestOwnerGate_RecipeOwnedCollectionRefusesUpdate(t *testing.T) {
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "recipe-owned-update")
	coll := seedOwnedCollection(t, srv, proj.ID, "docs", venue.ContextOwnerRecipe)
	e := echo.New()

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/acme/"+proj.ID+"/collections/main/"+coll.ID,
		strings.NewReader(`{"name":"renamed-by-hand"}`))
	r.Header.Set("Content-Type", "application/json")
	c := originCtx(e, r, rec, proj.ID, [2]string{"ref", "main"}, [2]string{"cid", coll.ID})
	require.NoError(t, srv.HandleUpdateCollection(c))

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.Contains(t, body, "kapi.yaml", "the refusal must name the recipe as the authority")
	assert.Contains(t, body, "push", "the refusal must name the path to changing it")
	assert.Contains(t, body, "docs")

	// The row is untouched — refused, not applied.
	stored, err := cs.GetCollection(t.Context(), proj.ID, coll.ID)
	require.NoError(t, err)
	assert.Equal(t, "docs", stored.Name)
}

func TestOwnerGate_RecipeOwnedCollectionRefusesDelete(t *testing.T) {
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "recipe-owned-delete")
	require.NoError(t, EnsureDefaultCollection(t.Context(), cs, proj.ID))
	coll := seedOwnedCollection(t, srv, proj.ID, "marketing", venue.ContextOwnerRecipe)
	e := echo.New()

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/acme/"+proj.ID+"/collections/main/"+coll.ID, nil)
	c := originCtx(e, r, rec, proj.ID, [2]string{"ref", "main"}, [2]string{"cid", coll.ID})
	require.NoError(t, srv.HandleDeleteCollection(c))

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "kapi.yaml")

	// The collection still stands.
	stored, err := cs.GetCollection(t.Context(), proj.ID, coll.ID)
	require.NoError(t, err)
	assert.Equal(t, "marketing", stored.Name)
}

func TestOwnerGate_RecipeOwnedCollectionRefusesUpload(t *testing.T) {
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "recipe-owned-upload")
	coll := seedOwnedCollection(t, srv, proj.ID, "docs", venue.ContextOwnerRecipe)
	e := echo.New()

	body, ct := multipartFiles(t, map[string]string{"app.json": `{"a":"b"}`})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/"+proj.ID+"/collections/main/"+coll.ID+"/items", body)
	r.Header.Set("Content-Type", ct)
	c := originCtx(e, r, rec, proj.ID, [2]string{"ref", "main"}, [2]string{"cid", coll.ID})
	require.NoError(t, srv.HandleUploadToCollection(c))

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "kapi.yaml")

	items, err := cs.ListItems(t.Context(), proj.ID, "main")
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestOwnerGate_WorkspaceOwnedCollectionStillMutable(t *testing.T) {
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "workspace-owned")
	require.NoError(t, EnsureDefaultCollection(t.Context(), cs, proj.ID))
	coll := seedOwnedCollection(t, srv, proj.ID, "uploads", venue.ContextOwnerWorkspace)
	e := echo.New()

	// Update applies.
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/acme/"+proj.ID+"/collections/main/"+coll.ID,
		strings.NewReader(`{"name":"uploads-renamed"}`))
	r.Header.Set("Content-Type", "application/json")
	c := originCtx(e, r, rec, proj.ID, [2]string{"ref", "main"}, [2]string{"cid", coll.ID})
	require.NoError(t, srv.HandleUpdateCollection(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	stored, err := cs.GetCollection(t.Context(), proj.ID, coll.ID)
	require.NoError(t, err)
	assert.Equal(t, "uploads-renamed", stored.Name)

	// Delete applies.
	rec2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodDelete, "/api/v1/acme/"+proj.ID+"/collections/main/"+coll.ID, nil)
	c2 := originCtx(e, r2, rec2, proj.ID, [2]string{"ref", "main"}, [2]string{"cid", coll.ID})
	require.NoError(t, srv.HandleDeleteCollection(c2))
	require.Equal(t, http.StatusNoContent, rec2.Code, rec2.Body.String())

	_, err = cs.GetCollection(t.Context(), proj.ID, coll.ID)
	assert.Error(t, err)
}

// A collection stored with no owner reads as workspace-owned: the conservative
// default, so rows predating the context content type stay mutable.
func TestOwnerGate_EmptyOwnerIsNotRecipeOwned(t *testing.T) {
	refused, err := guardRecipeOwned(nil, &platstore.Collection{Name: "legacy"})
	assert.False(t, refused)
	require.NoError(t, err)

	refused, err = guardRecipeOwned(nil, nil)
	assert.False(t, refused)
	require.NoError(t, err)

	assert.Contains(t, recipeOwnedMessage(&platstore.Collection{Name: "docs"}), `collection "docs"`)
	assert.Contains(t, recipeOwnedMessage(nil), "this collection")
}
