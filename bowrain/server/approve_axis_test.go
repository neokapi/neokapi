package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingRecipeChanges is the narrow store the recipe-change handlers take,
// recording what was proposed rather than persisting it. The interface is three
// methods precisely so a test can stand in for it here.
type recordingRecipeChanges struct {
	proposed []*bstore.PendingRecipeChange
}

func (r *recordingRecipeChanges) ProposeRecipeChange(_ context.Context, ch *bstore.PendingRecipeChange) error {
	r.proposed = append(r.proposed, ch)
	return nil
}

func (r *recordingRecipeChanges) PendingRecipeChanges(context.Context, string) ([]*bstore.PendingRecipeChange, error) {
	return r.proposed, nil
}

func (r *recordingRecipeChanges) MarkRecipeChangeApplied(context.Context, string) error { return nil }

// approveAxis posts one approval at the given project and returns the recorder,
// so each case reads as the request it makes.
func approveAxis(t *testing.T, srv *Server, projectID, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/projects/"+projectID+"/axes",
		strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	c := e.NewContext(r, rec)
	c.SetParamNames("id")
	c.SetParamValues(projectID)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", "ws-1")
	c.Set("user_id", "u-1")
	require.NoError(t, srv.HandleApproveAxis(c))
	return rec
}

func axisServer(t *testing.T) (*Server, *recordingRecipeChanges, string) {
	t.Helper()
	srv, projectID := channelRefServer(t)
	changes := &recordingRecipeChanges{}
	srv.RecipeChangeStore = changes
	return srv, changes, projectID
}

// A declared axis is one value on the project's default point, inherited by
// every collection — so it writes `defaults.coordinates.<axis>` and nothing
// else needs saying.
func TestApproveAxis_DeclaredAxisWritesTheDefaultPoint(t *testing.T) {
	srv, changes, projectID := axisServer(t)

	rec := approveAxis(t, srv, projectID, `{"axis":"audience","value":"Developer"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Len(t, changes.proposed, 1)
	ch := changes.proposed[0]
	assert.Equal(t, "defaults.coordinates.audience", ch.Path)
	assert.JSONEq(t, `"developer"`, string(ch.Value), "axis values are normalised to the slug they are compared as")
	assert.Equal(t, projectID, ch.ProjectID)
	assert.Equal(t, "ws-1", ch.WorkspaceID)
	assert.Equal(t, "u-1", ch.CreatedBy)
}

// A structural axis is derived from a collection's `channel:`, so it writes
// that collection's channel — composed with the half that is NOT being
// approved, read from where the collection sits today.
func TestApproveAxis_StructuralAxisWritesTheCollectionChannel(t *testing.T) {
	srv, changes, projectID := axisServer(t)

	rec := approveAxis(t, srv, projectID,
		`{"axis":"product","value":"desktop","collection":"docs"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Len(t, changes.proposed, 1)
	assert.Equal(t, "collections.docs.channel", changes.proposed[0].Path)
	assert.JSONEq(t, `"desktop/guides"`, string(changes.proposed[0].Value),
		"the surface the collection already sits on is kept")
}

// Naming a collection for a declared axis is a narrower claim than the axis
// makes, and is refused rather than silently widened or narrowed.
func TestApproveAxis_DeclaredAxisRefusesACollection(t *testing.T) {
	srv, changes, projectID := axisServer(t)

	rec := approveAxis(t, srv, projectID,
		`{"axis":"audience","value":"developer","collection":"docs"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "narrower claim")
	assert.Empty(t, changes.proposed, "a refused approval proposes nothing")
}

// The refusals are the only instruction the reviewer gets, so each names what
// to do next. The review surface renders them verbatim.
func TestApproveAxis_RefusalsSayWhatToDoNext(t *testing.T) {
	srv, changes, projectID := axisServer(t)

	t.Run("structural axis with no collection", func(t *testing.T) {
		rec := approveAxis(t, srv, projectID, `{"axis":"channel","value":"guides"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Contains(t, rec.Body.String(), "name the collection this applies to")
	})

	t.Run("channel for a collection with no product", func(t *testing.T) {
		rec := approveAxis(t, srv, projectID,
			`{"axis":"channel","value":"guides","collection":"loose"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Contains(t, rec.Body.String(), "product")
	})

	t.Run("unknown collection", func(t *testing.T) {
		rec := approveAxis(t, srv, projectID,
			`{"axis":"product","value":"cloud","collection":"nope"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	assert.Empty(t, changes.proposed, "none of these reached the store")
}

func TestApproveAxis_RequiresBothHalves(t *testing.T) {
	srv, changes, projectID := axisServer(t)

	for _, body := range []string{
		`{"axis":"","value":"developer"}`,
		`{"axis":"audience","value":""}`,
		`{"axis":"  ","value":"  "}`,
	} {
		rec := approveAxis(t, srv, projectID, body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
		assert.Contains(t, rec.Body.String(), "axis and value are required")
	}
	assert.Empty(t, changes.proposed)
}

// Approving is a governed write, so it is gated on the same permission the
// voice surfaces are: a reader cannot reshape a project's context space.
func TestApproveAxis_NeedsManageVoice(t *testing.T) {
	srv, changes, projectID := axisServer(t)

	e := echo.New()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/projects/"+projectID+"/axes",
		strings.NewReader(`{"axis":"audience","value":"developer"}`))
	r.Header.Set("Content-Type", "application/json")
	c := e.NewContext(r, rec)
	c.SetParamNames("id")
	c.SetParamValues(projectID)
	c.Set("project_permissions", platauth.PermViewContent)

	// requirePermission writes the 403 itself and returns the sentinel, so the
	// handler's error is the denial rather than a failure to answer.
	assert.ErrorIs(t, srv.HandleApproveAxis(c), errAccessDenied)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Empty(t, changes.proposed)
}

// The endpoint answers 503 rather than 500 when the server was built without a
// store: nothing is wrong with the request, the deployment simply cannot take
// it yet.
func TestApproveAxis_NoStoreConfigured(t *testing.T) {
	srv, projectID := channelRefServer(t)

	rec := approveAxis(t, srv, projectID, `{"axis":"audience","value":"developer"}`)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body.Error, "recipe changes not configured")
}
