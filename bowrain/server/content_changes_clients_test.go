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
