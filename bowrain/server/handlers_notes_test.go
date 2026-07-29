package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noteCtx builds a request context for the delete route as the router would,
// with the caller's project permissions and identity claims already resolved —
// which is what the handler authorizes on.
func noteCtx(t *testing.T, srv *Server, pid, bid, nid string, perms platauth.Permission, name string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "id", "ref", "bid", "nid")
	c.SetParamValues("test", pid, "main", bid, nid)
	c.Set("project_permissions", perms)
	c.Set("user_id", "someone")
	c.Set("user_claims", map[string]any{"name": name})
	return c, rec
}

// Deleting someone else's note takes more than being able to read it.
//
// The gate was PermViewContent, which is exactly what the built-in observer
// role holds — read-only access to project content. A member given no write
// access at all could delete any note in the project, and the handler's own
// TODO said so. Authorship or PermManageProject is the rule now.
func TestDeleteBlockNoteRequiresAuthorOrManager(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := t.Context()

	const pid, bid = "notes-proj", "block-1"
	seed := func(t *testing.T, author string) string {
		t.Helper()
		note := model.BlockNote{ID: "note-" + author, BlockID: bid, Author: author, Text: "hi"}
		require.NoError(t, srv.ContentStore.AddBlockNote(ctx, pid, "main", bid, note))
		return note.ID
	}

	for _, tc := range []struct {
		name       string
		noteAuthor string
		callerName string
		perms      platauth.Permission
		wantStatus int
		wantGone   bool
	}{
		{
			name:       "a read-only observer cannot delete another person's note",
			noteAuthor: "Alice",
			callerName: "Mallory",
			perms:      platauth.PermViewContent,
			wantStatus: http.StatusForbidden,
			wantGone:   false,
		},
		{
			name:       "the author can delete their own note",
			noteAuthor: "Alice",
			callerName: "Alice",
			perms:      platauth.PermViewContent,
			wantStatus: http.StatusNoContent,
			wantGone:   true,
		},
		{
			name:       "a project manager can delete anyone's note",
			noteAuthor: "Bob",
			callerName: "Mallory",
			perms:      platauth.PermViewContent | platauth.PermManageProject,
			wantStatus: http.StatusNoContent,
			wantGone:   true,
		},
		{
			name:       "a caller with no name claim matches no author",
			noteAuthor: "",
			callerName: "",
			perms:      platauth.PermViewContent,
			wantStatus: http.StatusForbidden,
			wantGone:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nid := seed(t, tc.noteAuthor)
			t.Cleanup(func() {
				_ = srv.ContentStore.DeleteBlockNote(ctx, pid, "main", nid)
			})

			c, rec := noteCtx(t, srv, pid, bid, nid, tc.perms, tc.callerName)
			err := srv.HandleDeleteBlockNote(c)

			if tc.wantStatus == http.StatusForbidden {
				// deny() writes the 403 and returns errAccessDenied so the
				// caller aborts; a nil error here would be the fail-open.
				require.ErrorIs(t, err, errAccessDenied)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantStatus, rec.Code)

			notes, lerr := srv.ContentStore.ListBlockNotes(ctx, pid, "main", bid)
			require.NoError(t, lerr)
			var stillThere bool
			for _, n := range notes {
				if n.ID == nid {
					stillThere = true
				}
			}
			assert.Equal(t, !tc.wantGone, stillThere,
				"the note's survival must match whether deletion was allowed")
		})
	}
}

func TestParseMentions(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"single mention", "Hey @alice check this", []string{"alice"}},
		{"multiple mentions", "@alice and @bob please review", []string{"alice", "bob"}},
		{"duplicate mentions", "@alice @alice @bob", []string{"alice", "bob"}},
		{"no mentions", "No mentions here", nil},
		{"empty string", "", nil},
		{"mention at start", "@admin hello", []string{"admin"}},
		{"mention at end", "hello @admin", []string{"admin"}},
		{"mention with numbers", "@user123 check", []string{"user123"}},
		{"mention with underscore", "@john_doe review", []string{"john_doe"}},
		{"email contains at sign", "email user@example.com", []string{"example"}},
		{"mention in middle of sentence", "Please ask @reviewer to check", []string{"reviewer"}},
		{"consecutive mentions", "@alice@bob", []string{"alice", "bob"}},
		{"mention with punctuation after", "@alice, please review", []string{"alice"}},
		{"only at sign", "@ nothing", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMentions(tt.text)
			assert.Equal(t, tt.want, got)
		})
	}
}
