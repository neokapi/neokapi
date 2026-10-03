package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// readTargetRevision reads a block through the editor's blocks route and
// returns the revision the route serves for one locale's target: the if_match
// a client sends with an operation on that translation.
func readTargetRevision(t *testing.T, srv *Server, pid, bid, locale string) string {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+pid+"/blocks/main?item=greetings.txt", nil)
	w := httptest.NewRecorder()
	c := e.NewContext(r, w)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("acme", pid, "main")
	require.NoError(t, srv.HandleGetFileBlocks(c))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload []struct {
		ID              string            `json:"id"`
		TargetRevisions map[string]string `json:"target_revisions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	for _, b := range payload {
		if b.ID == bid {
			return b.TargetRevisions[locale]
		}
	}
	t.Fatalf("block %s is not served", bid)
	return ""
}

// changeTargetText stores another person's wording for a locale.
func changeTargetText(t *testing.T, cs *bstore.PostgresStore, pid, bid, locale, text string) {
	t.Helper()
	sb, err := cs.GetBlock(t.Context(), pid, "main", bid)
	require.NoError(t, err)
	sb.Block.SetTargetText(model.LocaleID(locale), text)
	require.NoError(t, cs.StoreBlocks(t.Context(), pid, "main", []*model.Block{sb.Block}))
}

// A translator saves over a translation someone else changed after the
// translator read it. The save is refused stale with the translation as it now
// stands, and the other person's wording stays. Saving again on the current
// revision lands, in text and in runs.
func TestApplyChanges_AStaleSaveGetsTheCurrentTranslationAndWritesNothing(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	read := readTargetRevision(t, srv, pid, bid, "fr")
	changeTargetText(t, cs, pid, bid, "fr", "Salut")

	rec, res := sendChanges(t, srv, pid, translateCaller, change.Set{Ops: []change.Op{
		setText(at("greetings.txt", bid, "fr"), read, "Coucou")}})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
	require.NotNil(t, res.Ops[0].Current, "the refusal carries the wording that stands")
	assert.Equal(t, "Salut", res.Ops[0].Current.Text)
	assert.Equal(t, "Salut", getStoredBlock(t, cs, pid, bid).TargetText("fr"), "the other save stays")

	rec, res = sendChanges(t, srv, pid, translateCaller, change.Set{Ops: []change.Op{
		setText(at("greetings.txt", bid, "fr"), res.Ops[0].Current.Rev, "Coucou")}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, change.SetApplied, res.Status)
	require.NotNil(t, res.Record, "the change is recorded")
	assert.Equal(t, "Coucou", getStoredBlock(t, cs, pid, bid).TargetText("fr"))

	runs := change.Op{Kind: change.KindSetContent, At: at("greetings.txt", bid, "fr"), IfMatch: res.Ops[0].After,
		Body: &change.SetContent{Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour à tous"}}}}}
	rec, _ = sendChanges(t, srv, pid, translateCaller, change.Set{Ops: []change.Op{runs}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "Bonjour à tous", getStoredBlock(t, cs, pid, bid).TargetText("fr"))
}

// A reviewer approves wording that changed after the reviewer read it. The
// approval is refused stale, and the changed wording stays unapproved.
func TestApplyChanges_AnApprovalOfWordingThatChangedIsRefused(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	read := readTargetRevision(t, srv, pid, bid, "fr")
	changeTargetText(t, cs, pid, bid, "fr", "Salut")

	rec, res := sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{
		decide(at("greetings.txt", bid, "fr"), read, change.OutcomeEstablish)}})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
	assert.Equal(t, "Salut", res.Ops[0].Current.Text, "the reviewer is shown the wording that stands")
	assert.NotEqual(t, model.TargetStatusEstablished, getStoredBlock(t, cs, pid, bid).Target("fr").Status,
		"wording the reviewer did not read is not approved")

	rec, _ = sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{
		decide(at("greetings.txt", bid, "fr"), res.Ops[0].Current.Rev, change.OutcomeEstablish)}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusEstablished, getStoredBlock(t, cs, pid, bid).Target("fr").Status)
}

// Each refusal is answered with the status its code maps to, and a change set
// that does not decode is refused whole.
func TestApplyChanges_ARefusalIsAnsweredByItsCode(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Open Bowrain")
	b.SetTargetText("fr", "Ouvrir Bowrain")
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceRuns([]model.Run{
		{Text: &model.TextRun{Text: "Read the "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="/guide">`}},
		{Text: &model.TextRun{Text: "guide"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}},
	})
	pid, ids := seedReviewProjectWith(t, cs, map[string]string{"dnt_terms": "Bowrain"}, []*model.Block{b, b2})
	bid, linked := ids["Open Bowrain"], ids["Read the guide"]
	fr := targetRev(t, cs, pid, bid, "fr")

	reader := changeCaller{user: "user-3", perms: platauth.PermViewContent}
	germanOnly := translateCaller
	germanOnly.langs = []string{"de"}

	cases := []struct {
		name   string
		who    changeCaller
		ops    []change.Op
		status int
		code   change.Code
	}{
		{"an item the stream does not hold", fullCaller,
			[]change.Op{setText(at("missing.txt", bid, "fr"), fr, "x")}, http.StatusNotFound, change.CodeNotFound},
		{"a block the item does not hold", fullCaller,
			[]change.Op{setText(at("greetings.txt", "nope", "fr"), fr, "x")}, http.StatusNotFound, change.CodeNotFound},
		{"a translation that moved", fullCaller,
			[]change.Op{setText(at("greetings.txt", bid, "fr"), "r:0000000000000000", "x")}, http.StatusConflict, change.CodeStale},
		{"a sender who may only read", reader,
			[]change.Op{setText(at("greetings.txt", bid, "fr"), fr, "Ouvrir Bowrain !")}, http.StatusForbidden, change.CodeNotPermitted},
		{"a language the sender has no access to", germanOnly,
			[]change.Op{setText(at("greetings.txt", bid, "fr"), fr, "Ouvrir Bowrain !")}, http.StatusForbidden, change.CodeNotPermitted},
		{"a source edit by a translator", translateCaller,
			[]change.Op{setText(at("greetings.txt", bid, ""), sourceRev(t, cs, pid, bid), "Start Bowrain")}, http.StatusForbidden, change.CodeNotPermitted},
		{"an edit that names an inline code the block does not hold", fullCaller,
			[]change.Op{setText(at("greetings.txt", linked, ""), sourceRev(t, cs, pid, linked), `Read the <x id="9"/>manual`)}, http.StatusUnprocessableEntity, change.CodeGuard},
		{"an edit that brings in a failing finding", fullCaller,
			[]change.Op{setText(at("greetings.txt", bid, "fr"), fr, "Ouvrir l'application")}, http.StatusUnprocessableEntity, change.CodeGateFailed},
		{"a language the project does not translate into", fullCaller,
			[]change.Op{setText(at("greetings.txt", bid, "ja"), model.AbsentRevision, "x")}, http.StatusUnprocessableEntity, change.CodeUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := getStoredBlock(t, cs, pid, bid).TargetText("fr")
			rec, res := sendChanges(t, srv, pid, tc.who, change.Set{Ops: tc.ops})
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, change.SetRefused, res.Status)
			require.NotNil(t, res.Ops[0].Error)
			assert.Equal(t, tc.code, res.Ops[0].Error.Code)
			assert.Equal(t, before, getStoredBlock(t, cs, pid, bid).TargetText("fr"), "a refusal writes nothing")
		})
	}

	t.Run("a change set that does not decode", func(t *testing.T) {
		rec, res := sendChangesBody(t, srv, pid, fullCaller, []byte(`{"ops":[{"op":"set_content","at":{"doc":"greetings.txt","block":"b1"},"wat":1}]}`))
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		require.NotNil(t, res.Error)
		assert.Equal(t, change.CodeInvalid, res.Error.Code)
		assert.NotEmpty(t, res.Error.Pointer)
	})

	t.Run("a person who overrides lands the edit with its findings", func(t *testing.T) {
		rec, res := sendChanges(t, srv, pid, fullCaller, change.Set{Gate: change.GateReport, Ops: []change.Op{
			setText(at("greetings.txt", bid, "fr"), fr, "Ouvrir l'application")}})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, change.SetApplied, res.Status)
		assert.NotEmpty(t, res.Ops[0].Findings, "the findings the person overrode are reported")
		assert.Equal(t, "Ouvrir l'application", getStoredBlock(t, cs, pid, bid).TargetText("fr"))
	})
}

// A preview computes and checks the change set and writes nothing.
func TestApplyChanges_APreviewWritesNothing(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec, res := sendChanges(t, srv, pid, fullCaller, change.Set{Mode: change.ModePreview, Ops: []change.Op{
		setText(at("greetings.txt", bid, "fr"), targetRev(t, cs, pid, bid, "fr"), "Salut")}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, change.SetPreviewed, res.Status)
	require.NotEmpty(t, res.Docs)
	assert.Contains(t, res.Docs[0].Diff, "+Salut")
	assert.Equal(t, "Bonjour", getStoredBlock(t, cs, pid, bid).TargetText("fr"))
}

// A source edit names the translations it leaves on an older basis.
func TestApplyChanges_ASourceEditNamesTheTranslationsItMakesStale(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	rec, res := sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{
		setText(at("greetings.txt", bid, ""), sourceRev(t, cs, pid, bid), "Hello there")}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, change.SetApplied, res.Status)
	assert.Contains(t, res.Ops[0].Invalidates, change.Invalidation{Edition: "fr", Reason: change.ReasonBasisMoved})
	assert.Equal(t, "Hello there", getStoredBlock(t, cs, pid, bid).SourceText())
}
