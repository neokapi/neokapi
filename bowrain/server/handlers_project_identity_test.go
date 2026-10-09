package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postJSON sends body to path with the bearer token (none when empty) and
// decodes the JSON answer.
func postJSON(t *testing.T, e http.Handler, token, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var resp map[string]any
	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), "body: %s", rec.Body.String())
	}
	return rec.Code, resp
}

// A recipe that carries an id connects under it: the venue keeps the id
// instead of minting one, and a second clone sending the same id is answered
// with the project already in the workspace rather than a second project.
func TestCreateWorkspaceProjectKeepsTheRecipeID(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	id := coreproj.NewID()

	code, resp := postJSON(t, e, token, "/api/v1/test/projects",
		fmt.Sprintf(`{"id":%q,"name":"Clone one","default_source_language":"en","target_languages":["fr"]}`, id))
	require.Equal(t, http.StatusCreated, code, "%v", resp)
	assert.Equal(t, id, resp["id"], "the project keeps the id it was sent")

	// The second clone of the same repository: same id, possibly another name
	// on disk. The test workspace's abuse cap allows one project, so this
	// answer also shows the lookup runs before the cap.
	code, resp = postJSON(t, e, token, "/api/v1/test/projects",
		fmt.Sprintf(`{"id":%q,"name":"Clone two","default_source_language":"en"}`, id))
	require.Equal(t, http.StatusOK, code, "%v", resp)
	assert.Equal(t, id, resp["id"])
	assert.Equal(t, "Clone one", resp["name"], "the existing project is answered as it is")

	projects, err := srv.Services.Project.ListProjects(t.Context())
	require.NoError(t, err)
	n := 0
	for _, p := range projects {
		if p.ID == id {
			n++
		}
	}
	assert.Equal(t, 1, n, "one project under the id, not one per clone")
}

func TestCreateWorkspaceProjectRefusesAMalformedID(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()

	code, resp := postJSON(t, e, token, "/api/v1/test/projects",
		`{"id":"not-a-project-id","name":"Clone","default_source_language":"en"}`)
	assert.Equal(t, http.StatusBadRequest, code, "%v", resp)
}

// An id already held by a project outside the caller's workspace is not
// theirs to connect to.
func TestCreateWorkspaceProjectRefusesAnIDHeldElsewhere(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	id := coreproj.NewID()

	require.NoError(t, srv.Services.Project.CreateProject(t.Context(), &store.Project{
		ID:                    id,
		Name:                  "Someone else's",
		DefaultSourceLanguage: "en",
		WorkspaceID:           "ws_elsewhere",
	}))

	code, resp := postJSON(t, e, token, "/api/v1/test/projects",
		fmt.Sprintf(`{"id":%q,"name":"Clone","default_source_language":"en"}`, id))
	assert.Equal(t, http.StatusConflict, code, "%v", resp)
}

// An anonymous connect keeps the recipe's id too, and an id the server
// already holds is refused: an anonymous caller is nobody the existing
// project could be handed to.
func TestCreateAnonymousProjectKeepsTheRecipeID(t *testing.T) {
	srv, _ := newTestServer(t)
	e := srv.GetEcho()
	id := coreproj.NewID()

	code, resp := postJSON(t, e, "", "/api/v1/projects/anonymous",
		fmt.Sprintf(`{"id":%q,"name":"Anon clone","default_source_language":"en"}`, id))
	require.Equal(t, http.StatusCreated, code, "%v", resp)
	assert.Equal(t, id, resp["project_id"])
	assert.NotEmpty(t, resp["claim_token"])

	p, err := srv.Services.Project.GetProject(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "Anon clone", p.Name)

	code, resp = postJSON(t, e, "", "/api/v1/projects/anonymous",
		fmt.Sprintf(`{"id":%q,"name":"Anon clone again","default_source_language":"en"}`, id))
	assert.Equal(t, http.StatusConflict, code, "%v", resp)

	code, resp = postJSON(t, e, "", "/api/v1/projects/anonymous",
		`{"id":"not-a-project-id","name":"Anon","default_source_language":"en"}`)
	assert.Equal(t, http.StatusBadRequest, code, "%v", resp)
}
