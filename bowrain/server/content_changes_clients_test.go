package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// servedEntities reads a block through the editor's blocks route and returns
// the entities the route serves on it.
func servedEntities(t *testing.T, srv *Server, pid, bid string) []EntityInfoResponse {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+pid+"/blocks/main?item=greetings.txt", nil)
	w := httptest.NewRecorder()
	c := e.NewContext(r, w)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("acme", pid, "main")
	require.NoError(t, srv.HandleGetFileBlocks(c))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload []BlockInfoResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	for _, b := range payload {
		if b.ID == bid {
			return b.Entities
		}
	}
	t.Fatalf("block %s is not served", bid)
	return nil
}

// An entity the web editor marks is the change set the editor sends
// (bowrain/packages/ui contentChanges.markEntity), byte for byte: the anchor at
// the run positions of the words, the value in the entity annotation's own
// fields. It is served back as an entity over those words, and the id the
// result names removes it.
func TestApplyChanges_AnEntityTheEditorMarksIsServedAsAnEntity(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceRuns([]model.Run{
		{Text: &model.TextRun{Text: "Read the "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="/guide">`}},
		{Text: &model.TextRun{Text: "guide"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}},
	})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Read the guide"]

	mark := fmt.Sprintf(`{"schema":"kapi.change/v1","ops":[{"op":"annotate","at":{"doc":"greetings.txt","block":%q},`+
		`"type":"entity","anchor":{"kind":"range","start":{"run":1,"offset":0},"end":{"run":3,"offset":0}},`+
		`"value":{"Text":"guide","Type":"product","Locale":"","DNT":true,"Source":"manual"}}]}`, bid)
	rec, res := sendChangesBody(t, srv, pid, fullCaller, []byte(mark))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.OpApplied, res.Ops[0].Status)
	id := res.Ops[0].ID
	require.NotEmpty(t, id, "the result names the entity it wrote")

	entities := servedEntities(t, srv, pid, bid)
	require.Len(t, entities, 1)
	assert.Equal(t, EntityInfoResponse{Key: id, Text: "guide", Type: "product", Start: 9, End: 14, DNT: true, Source: "manual"}, entities[0])

	unmark := fmt.Sprintf(`{"ops":[{"op":"unannotate","at":{"doc":"greetings.txt","block":%q},"type":"entity","id":%q}]}`, bid, id)
	rec, _ = sendChangesBody(t, srv, pid, fullCaller, []byte(unmark))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, servedEntities(t, srv, pid, bid))
}

// servedBlock reads one block through the editor's blocks route.
func servedBlock(t *testing.T, srv *Server, pid, bid string) BlockInfoResponse {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+pid+"/blocks/main?item=greetings.txt", nil)
	w := httptest.NewRecorder()
	c := e.NewContext(r, w)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("acme", pid, "main")
	require.NoError(t, srv.HandleGetFileBlocks(c))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload []BlockInfoResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	for _, b := range payload {
		if b.ID == bid {
			return b
		}
	}
	t.Fatalf("block %s is not served", bid)
	return BlockInfoResponse{}
}

// A message that is one plural (an ARB or ICU message) is translated in the
// editor's plural mode, which saves the whole plural as one run with its forms
// (bowrain/packages/ui UnifiedTargetEditor serialiseForms, sent through
// toChangeRuns). That is the change set below, byte for byte: it lands as a
// plural whose forms keep the variable. The same translation as ICU text is
// refused, because text cannot replace a structure.
func TestApplyChanges_APluralTheEditorSavesLandsAsAPlural(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	n := model.Run{Ph: &model.PlaceholderRun{ID: "n", Type: "code:variable", Equiv: "#", Data: "#"}}
	b := &model.Block{ID: "p1", Translatable: true}
	b.SetSourceRuns([]model.Run{{Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {n, {Text: &model.TextRun{Text: " item"}}},
		model.PluralOther: {n, {Text: &model.TextRun{Text: " items"}}},
	}}}})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids[" items"]
	require.NotEmpty(t, bid)

	icu := fmt.Sprintf(`{"schema":"kapi.change/v1","ops":[{"op":"set_content","at":{"doc":"greetings.txt","block":%q,"edition":"fr"},`+
		`"if_match":"absent","text":"{count, plural, one {# article} other {# articles}}"}]}`, bid)
	rec, res := sendChangesBody(t, srv, pid, fullCaller, []byte(icu))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.SubcodeStructureLost, res.Ops[0].Error.Subcode)

	plural := fmt.Sprintf(`{"schema":"kapi.change/v1","ops":[{"op":"set_content","at":{"doc":"greetings.txt","block":%q,"edition":"fr"},`+
		`"if_match":"absent","runs":[{"plural":{"pivot":"count","forms":{`+
		`"one":[{"ph":{"id":"n","type":"code:variable","equiv":"#"}},{"text":" article"}],`+
		`"other":[{"ph":{"id":"n","type":"code:variable","equiv":"#"}},{"text":" articles"}]}}}]}]}`, bid)
	rec, res = sendChangesBody(t, srv, pid, fullCaller, []byte(plural))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.OpApplied, res.Ops[0].Status)

	fr := getStoredBlock(t, cs, pid, bid).TargetRuns("fr")
	require.Len(t, fr, 1)
	require.NotNil(t, fr[0].Plural, "the translation is a plural")
	assert.Equal(t, "count", fr[0].Plural.Pivot)
	assert.Equal(t, "# article", model.RenderRunsWithData(fr[0].Plural.Forms[model.PluralOne]))
	assert.Equal(t, "# articles", model.RenderRunsWithData(fr[0].Plural.Forms[model.PluralOther]))

	// Read back, the translation is served as its runs, which reopen the
	// editor on its forms.
	served := servedBlock(t, srv, pid, bid)
	require.Len(t, served.TargetsRuns["fr"], 1)
	assert.NotNil(t, served.TargetsRuns["fr"][0].Plural)
}

// A term the editor inserts into a translation no editor has open is appended
// to the translation's runs (contentChanges.appendText over translationRuns),
// so a translation with inline codes keeps them. The change set below is that
// shape for a bold word: it lands with the bold kept.
func TestApplyChanges_ATermInsertKeepsTheTranslationsCodes(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceRuns([]model.Run{
		{Text: &model.TextRun{Text: "Save "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
		{Text: &model.TextRun{Text: "now"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
	})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Save now"]
	editAsPerson(t, srv, pid, bid, "fr", textContent(`Enregistrer <x id="1"/>maintenant<x id="/1"/>`))

	rev := targetRev(t, cs, pid, bid, "fr")
	insert := fmt.Sprintf(`{"schema":"kapi.change/v1","ops":[{"op":"set_content","at":{"doc":"greetings.txt","block":%q,"edition":"fr"},`+
		`"if_match":%q,"runs":[{"text":"Enregistrer "},{"pcOpen":{"id":"1","type":"fmt:bold"}},{"text":"maintenant"},`+
		`{"pcClose":{"id":"1","type":"fmt:bold"}},{"text":" sauvegarde"}]}]}`, bid, rev)
	rec, res := sendChangesBody(t, srv, pid, fullCaller, []byte(insert))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.OpApplied, res.Ops[0].Status)
	assert.Equal(t, "Enregistrer <b>maintenant</b> sauvegarde",
		model.RenderRunsWithData(getStoredBlock(t, cs, pid, bid).TargetRuns("fr")))
}

// A content-memory match whose target holds an inline code is served with its
// runs beside its plain text, and the editor applies the runs: the variable
// the text leaves out lands in the translation.
func TestLookupMemoryForBlock_ServesTheRunsAMatchSaves(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	name := model.Run{Ph: &model.PlaceholderRun{ID: "name", Type: "code:variable", Equiv: "{name}", Data: "{name}"}}
	b := &model.Block{ID: "v1", Translatable: true}
	b.SetSourceRuns([]model.Run{{Text: &model.TextRun{Text: "Hello "}}, name})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello "]

	mem, err := srv.wsStores.getMemory("acme")
	require.NoError(t, err)
	require.NoError(t, mem.Add(t.Context(), memory.Entry{
		ID: "m1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello "}}, name},
			"fr": {{Text: &model.TextRun{Text: "Bonjour "}}, name},
		},
	}))

	e := echo.New()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+pid+"/blocks/main/"+bid+"/tm-matches?target_locale=fr", nil)
	w := httptest.NewRecorder()
	c := e.NewContext(r, w)
	c.SetParamNames("ws", "id", "ref", "bid")
	c.SetParamValues("acme", pid, "main", bid)
	require.NoError(t, srv.HandleLookupMemoryForBlock(c))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var matches []MemoryMatchInfoResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &matches))
	require.NotEmpty(t, matches)
	require.Len(t, matches[0].TargetRuns, 2, "the match carries its runs: %s", w.Body.String())
	require.NotNil(t, matches[0].TargetRuns[1].Ph)
	assert.Equal(t, "name", matches[0].TargetRuns[1].Ph.ID)

	apply := fmt.Sprintf(`{"schema":"kapi.change/v1","ops":[{"op":"set_content","at":{"doc":"greetings.txt","block":%q,"edition":"fr"},`+
		`"if_match":"absent","runs":[{"text":"Bonjour "},{"ph":{"id":"name","type":"code:variable","equiv":"{name}"}}]}]}`, bid)
	rec, res := sendChangesBody(t, srv, pid, fullCaller, []byte(apply))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.OpApplied, res.Ops[0].Status)
	assert.Equal(t, "Bonjour {name}", model.RenderRunsWithData(getStoredBlock(t, cs, pid, bid).TargetRuns("fr")))
}

// A source split into several text runs (a do-not-translate word inside a
// sentence) is served with its runs, so the editor anchors a mark at the run
// positions of the words (contentChanges.textRangeAnchor); an anchor computed
// against the flat text alone would point past the first run.
func TestEditorBlocks_ServeASourceSplitIntoTextRunsAsRuns(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "s1", Translatable: true}
	b.SetSourceRuns([]model.Run{
		{Text: &model.TextRun{Text: "Run "}},
		{Text: &model.TextRun{Text: "kapi", NoTranslate: true}},
		{Text: &model.TextRun{Text: " daily"}},
	})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Run kapi daily"]

	served := servedBlock(t, srv, pid, bid)
	require.Len(t, served.SourceRuns, 3, "the source is served as its runs")
	assert.False(t, served.HasInlineCodes, "text runs are not inline codes")

	mark := fmt.Sprintf(`{"schema":"kapi.change/v1","ops":[{"op":"annotate","at":{"doc":"greetings.txt","block":%q},`+
		`"type":"entity","anchor":{"kind":"range","start":{"run":1,"offset":0},"end":{"run":2,"offset":0}},`+
		`"value":{"Text":"kapi","Type":"product","Locale":"","DNT":true,"Source":"manual"}}]}`, bid)
	rec, res := sendChangesBody(t, srv, pid, fullCaller, []byte(mark))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.OpApplied, res.Ops[0].Status)
	entities := servedEntities(t, srv, pid, bid)
	require.Len(t, entities, 1)
	assert.Equal(t, "kapi", entities[0].Text)
	assert.Equal(t, 4, entities[0].Start)
	assert.Equal(t, 8, entities[0].End)
}
