package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/workspace"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// commitWithParts posts a push commit carrying body to the handler, with the
// path and caller set as the router sets them.
func commitWithParts(t *testing.T, s *Server, projectID string, perms platauth.Permission, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := s.GetEcho().NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("test", projectID, "main")
	c.Set("workspace_id", "test-ws")
	c.Set("user_id", "test-user")
	c.Set("project_permissions", perms)
	// A denial writes its 403 and returns the sentinel the router swallows.
	if err := s.HandleSyncPushCommit(c); err != nil {
		require.ErrorIs(t, err, errAccessDenied)
	}
	return rec
}

// queuedManifest reads the manifest the commit handed the worker.
func queuedManifest(t *testing.T, s *Server, rec *httptest.ResponseRecorder) pushCommitManifest {
	t.Helper()
	var resp struct {
		PushID string `json:"push_id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	list, err := s.JobStore.ListJobsByPushID(t.Context(), resp.PushID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	r, err := s.BlobStore.Download(t.Context(), list[0].Model)
	require.NoError(t, err)
	defer r.Close()
	raw, err := io.ReadAll(r)
	require.NoError(t, err)
	var m pushCommitManifest
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// The commit checks the terminology and context ledger parts before it accepts
// a push: a part it can already refuse refuses the whole push, with nothing
// queued. What it accepts goes to the worker with the attribution and the
// binding it decided, never the client's.
func TestSyncPushCommit_PartsPrecheck(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)

	manager := platauth.PermManageFiles | platauth.PermManageTerms | platauth.PermViewContent
	ordinary := &apiclient.TermsEdits{Creates: []apiclient.CreateConceptParams{{
		Domain: "ui", Terms: []apiclient.TermInfo{{Text: "Sign up", Locale: "en", Status: "approved"}},
	}}}
	governed := &apiclient.TermsEdits{Creates: []apiclient.CreateConceptParams{{
		Domain: "ui", Terms: []apiclient.TermInfo{{Text: "Click here", Locale: "en", Status: "forbidden"}},
	}}}

	for _, tc := range []struct {
		name  string
		perms platauth.Permission
		body  map[string]any
		code  int
	}{
		{
			name:  "terminology edits need manage_terms",
			perms: platauth.PermManageFiles | platauth.PermViewContent,
			body:  map[string]any{"terms": ordinary},
			code:  http.StatusForbidden,
		},
		{
			name:  "a governed edit on the direct path is refused",
			perms: manager,
			body:  map[string]any{"terms": governed},
			code:  http.StatusConflict,
		},
		{
			name:  "a ledger object outside the context layout is refused",
			perms: manager | platauth.PermManageVoice,
			body:  map[string]any{"context_objects": []apiclient.ContextObjectWire{{Name: "notes.txt", Data: []byte("x")}}},
			code:  http.StatusBadRequest,
		},
		{
			name:  "a segment that holds no operations of a project is refused",
			perms: manager | platauth.PermManageVoice,
			body: map[string]any{"context_objects": []apiclient.ContextObjectWire{{
				Name: workspace.SegmentName("w1", "000000000000000000000001"), Data: []byte("not a segment\n"),
			}}},
			code: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := commitWithParts(t, srv, pid, tc.perms, tc.body)
			assert.Equal(t, tc.code, rec.Code, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "push_id", "a refused part queues nothing")
		})
	}

	t.Run("accepted terms reach the worker attributed to the caller", func(t *testing.T) {
		rec := commitWithParts(t, srv, pid, manager, map[string]any{
			"terms":        ordinary,
			"terms_author": "someone-else",
			"context_key":  "prj_forged",
		})
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		m := queuedManifest(t, srv, rec)
		assert.Equal(t, "test-user", m.TermsAuthor, "the author is the signed-in caller")
		assert.Empty(t, m.ContextKey, "a binding is the handler's to decide")
		var edits apiclient.TermsEdits
		require.NoError(t, json.Unmarshal(m.Terms, &edits))
		require.Len(t, edits.Creates, 1)
	})
}
