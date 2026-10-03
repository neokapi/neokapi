package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addNote is an annotate that leaves text as a note on a block.
func addNote(item, bid, text string) change.Op {
	value, _ := json.Marshal(map[string]string{"text": text})
	return change.Op{Kind: change.KindAnnotate, At: at(item, bid, ""), Body: &change.Annotate{Type: noteAnnotation, Value: value}}
}

// removeNote is an unannotate that removes the note id from a block.
func removeNote(item, bid, id string) change.Op {
	return change.Op{Kind: change.KindUnannotate, At: at(item, bid, ""), Body: &change.Unannotate{Type: noteAnnotation, ID: id}}
}

// listNotes reads a block's notes through the notes route.
func listNotes(t *testing.T, srv *Server, pid, bid string) []BlockNoteResponse {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	c.SetParamNames("ws", "id", "ref", "bid")
	c.SetParamValues("acme", pid, "main", bid)
	require.NoError(t, srv.HandleListBlockNotes(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var notes []BlockNoteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &notes))
	return notes
}

// A note is an annotation a change set adds: the notes route lists it with the
// author and time the server stamped as it landed, and a change set that
// claims another author is stamped with its sender all the same.
func TestNotes_ANoteLandsWithItsAuthor(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	reader := changeCaller{user: "user-9", name: "Rita", perms: platauth.PermViewContent}
	rec, res := sendChanges(t, srv, pid, reader, change.Set{Ops: []change.Op{addNote("greetings.txt", bid, "Is it hello or hi?")}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.OpApplied, res.Ops[0].Status)
	id := res.Ops[0].ID
	require.NotEmpty(t, id, "the result names the note it wrote")

	notes := listNotes(t, srv, pid, bid)
	require.Len(t, notes, 1)
	assert.Equal(t, id, notes[0].ID)
	assert.Equal(t, "Is it hello or hi?", notes[0].Text)
	assert.Equal(t, "Rita", notes[0].Author)
	assert.NotEmpty(t, notes[0].CreatedAt)
	assert.Equal(t, "Bonjour", getStoredBlock(t, cs, pid, bid).TargetText("fr"), "a note changes no content")
}

// Removing someone else's note takes more than being able to read it: its
// author or a project manager removes it.
func TestNotes_RemovingANoteTakesItsAuthorOrAManager(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	alice := changeCaller{user: "alice", name: "Alice", perms: platauth.PermViewContent}
	mallory := changeCaller{user: "mallory", name: "Mallory", perms: platauth.PermViewContent}
	manager := changeCaller{user: "bob", name: "Bob", perms: platauth.PermViewContent | platauth.PermManageProject}

	for _, tc := range []struct {
		name       string
		remover    changeCaller
		wantStatus int
		wantGone   bool
	}{
		{"a read-only member cannot remove another person's note", mallory, http.StatusForbidden, false},
		{"the author removes their own note", alice, http.StatusOK, true},
		{"a project manager removes anyone's note", manager, http.StatusOK, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, res := sendChanges(t, srv, pid, alice, change.Set{Ops: []change.Op{addNote("greetings.txt", bid, "mine")}})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			id := res.Ops[0].ID
			t.Cleanup(func() {
				_, _ = sendChanges(t, srv, pid, manager, change.Set{Ops: []change.Op{removeNote("greetings.txt", bid, id)}})
			})

			rec, res = sendChanges(t, srv, pid, tc.remover, change.Set{Ops: []change.Op{removeNote("greetings.txt", bid, id)}})
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			if tc.wantStatus == http.StatusForbidden {
				assert.Equal(t, change.CodeNotPermitted, res.Ops[0].Error.Code)
			}
			stillThere := false
			for _, n := range listNotes(t, srv, pid, bid) {
				if n.ID == id {
					stillThere = true
				}
			}
			assert.Equal(t, !tc.wantGone, stillThere, "the note's survival matches whether removing it was allowed")
		})
	}
}

// An entity is an annotation on the source: marking one takes edit source, and
// the block's entity list reads it back.
func TestEntities_AnEntityIsAnAnnotationOnTheSource(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Open Bowrain today")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Open Bowrain today"]

	value, err := json.Marshal(model.EntityAnnotation{Text: "Bowrain", Type: model.EntityType("product"), DNT: true, Source: model.ExtractionSourceManual})
	require.NoError(t, err)
	anchor := model.RangeAnchor(getStoredBlock(t, cs, pid, bid).SourceRuns(), 5, 12)
	mark := change.Op{Kind: change.KindAnnotate, At: at("greetings.txt", bid, ""),
		Body: &change.Annotate{Type: string(model.OverlayEntity), Anchor: &anchor, Value: value}}

	rec, res := sendChanges(t, srv, pid, translateCaller, change.Set{Ops: []change.Op{mark}})
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, change.CodeNotPermitted, res.Ops[0].Error.Code)

	rec, res = sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{mark}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	id := res.Ops[0].ID

	got := getStoredBlock(t, cs, pid, bid)
	span := got.OverlaySpan(model.OverlayEntity, id)
	require.NotNil(t, span)
	entity, ok := span.Value.(*model.EntityAnnotation)
	require.True(t, ok, "the entity reads back as an entity annotation")
	assert.Equal(t, "Bowrain", entity.Text)
	assert.True(t, entity.DNT)

	rec, _ = sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{{Kind: change.KindUnannotate,
		At: at("greetings.txt", bid, ""), Body: &change.Unannotate{Type: string(model.OverlayEntity), ID: id}}}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Nil(t, getStoredBlock(t, cs, pid, bid).OverlaySpan(model.OverlayEntity, id))
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
