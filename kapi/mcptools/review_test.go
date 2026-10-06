package mcptools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMCPReviewProject scaffolds a two-unit nb project with a pending review
// queue for the MCP review-tool handlers.
func writeMCPReviewProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := t.TempDir()
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: app
    content:
      - path: en.json
        target: "{lang}.json"
ship_gate: { translated: 100, established: 100 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

func TestHandleReviewQueue(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, out, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	require.Len(t, out.Pending, 2)
	assert.Equal(t, "nb", out.Pending[0].Locale)
	assert.Equal(t, "app", out.Pending[0].Collection)
	assert.Equal(t, BlockRef{Doc: "en.json", Block: "a", Edition: "nb"}, out.Pending[0].Ref,
		"a row carries the reference review_block reads it by: the source document and the translation's edition")

	// Locale filter — no de units exist.
	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Locale: "de"})
	require.NoError(t, err)
	assert.Equal(t, 0, out.Total)

	// Collection filter.
	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Collection: "app"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Collection: "other"})
	require.NoError(t, err)
	assert.Equal(t, 0, out.Total)
}

func TestHandleReviewQueue_NoProject(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := testApp()
	_, _, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no kapi project")
}

// review_block reads a queue row by the reference the row carries, and
// reports the revision of the edition under review: the one read_blocks
// reports for the same translation.
func TestHandleReviewBlock(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, queue, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	item := queue.Pending[0]

	_, out, err := handleReviewBlock(t.Context(), a, ReviewBlockInput{Project: proj, At: item.Ref})
	require.NoError(t, err)
	require.NotNil(t, out.Block)
	assert.Equal(t, item.Ref, out.Ref)
	assert.Equal(t, "translated", out.Block.Status)
	assert.Equal(t, "Apple", out.Block.Source)
	assert.Equal(t, "Eple", out.Block.Target)
	assert.Equal(t, readRev(t, a, proj, "en.json", "a", "nb"), out.Rev)

	// The file of the translation names the same edition.
	_, byFile, err := handleReviewBlock(t.Context(), a, ReviewBlockInput{Project: proj, At: BlockRef{Doc: "nb.json", Block: "a"}})
	require.NoError(t, err)
	assert.Equal(t, out.Ref, byFile.Ref)
	assert.Equal(t, out.Rev, byFile.Rev)

	_, _, err = handleReviewBlock(t.Context(), a, ReviewBlockInput{Project: proj, At: BlockRef{Doc: "en.json", Block: "missing", Edition: "nb"}})
	require.Error(t, err)
	_, _, err = handleReviewBlock(t.Context(), a, ReviewBlockInput{Project: proj, At: BlockRef{Doc: "en.json", Block: "a", Edition: "de"}})
	require.Error(t, err, "the project declares no de translation")
}

// review_block reads a project in that project's source language, whatever
// language another project's call left the App in: after a call that
// resolved a project written in Norwegian, this project's Norwegian
// translation is still reviewed as a translation.
func TestHandleReviewBlock_InTheProjectsOwnSourceLanguage(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	a.SourceLang = "nb"
	proj := filepath.Join(root, "kapi.yaml")

	at := BlockRef{Doc: "en.json", Block: "a", Edition: "nb"}
	_, out, err := handleReviewBlock(t.Context(), a, ReviewBlockInput{Project: proj, At: at})
	require.NoError(t, err)
	assert.Equal(t, at, out.Ref)
	require.NotNil(t, out.Block)
	assert.Equal(t, "Eple", out.Block.Target)
	assert.Equal(t, readRev(t, a, proj, "en.json", "a", "nb"), out.Rev)
}

// readRev is the revision the change service reads for one edition of a
// block.
func readRev(t *testing.T, a *cli.App, proj, doc, block, edition string) string {
	t.Helper()
	svc, err := a.ChangeService(t.Context(), host.ChangeServiceOptions{Project: proj})
	require.NoError(t, err)
	k, err := model.ParseEditionKey(edition)
	require.NoError(t, err)
	page, err := svc.Read(t.Context(), change.ReadRequest{Doc: doc, Blocks: []string{block}, Editions: []model.EditionKey{k}})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	ed, ok := page.Blocks[0].Editions[edition]
	require.True(t, ok, "%+v", page.Blocks[0].Editions)
	return ed.Rev
}

// TestHandleReviewBlock_CarriesTheContext holds the bar for the agent surface:
// an agent asked to judge a translation is handed at least what the model that
// produced it was handed.
func TestHandleReviewBlock_CarriesTheContext(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, out, err := handleReviewBlock(t.Context(), a, ReviewBlockInput{
		Project: proj, At: BlockRef{Doc: "en.json", Block: "a", Edition: "nb"},
	})
	require.NoError(t, err)
	require.NotNil(t, out.Block)
	require.NotNil(t, out.Block.Context, "review_block answers with the review model")

	rc := out.Block.Context
	assert.Equal(t, "app", rc.Point.Collection)
	assert.Equal(t, "en.json", rc.Point.Path, "the point is the SOURCE file's coordinate")
	assert.Equal(t, "a", rc.Neighbourhood.Key)
	assert.Equal(t, host.DefaultReviewWindow, rc.Neighbourhood.Window)
	assert.Empty(t, rc.Neighbourhood.Before, "`a` is the first unit in the file")
	require.Len(t, rc.Neighbourhood.After, 1)
	assert.Equal(t, "b", rc.Neighbourhood.After[0].Key)
	require.Len(t, rc.Neighbourhood.After[0].Source, 1)
	require.NotNil(t, rc.Neighbourhood.After[0].Source[0].Text)
	assert.Equal(t, "Banana", rc.Neighbourhood.After[0].Source[0].Text.Text)
}

// reviewSession connects a client named client to a server carrying every
// kapi MCP tool, the review set and apply_edits among them.
func reviewSession(t *testing.T, a *cli.App, client string) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	cli.ApplyMCPToolFactories(server, a)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	c := mcp.NewClient(&mcp.Implementation{Name: client, Version: "test"}, nil)
	session, err := c.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// call calls one tool and decodes its structured result into out, returning
// whether the tool reported an error.
func call(t *testing.T, s *mcp.ClientSession, name string, args map[string]any, out any) bool {
	t.Helper()
	res, err := s.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	if out != nil && res.StructuredContent != nil {
		body, err := json.Marshal(res.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, out), string(body))
	}
	return res.IsError
}

// An agent records its pre-review through apply_edits: a decide operation
// with outcome advise, at the reference and revision review_block reported.
// The score lands on the unit under the agent's name, and the unit stays in
// the queue for a person. An agent's establish is refused, and a pre-review
// of wording that moved since the read is refused as stale; neither records
// anything. A server serving the review set alone (`kapi mcp --tools review`)
// carries the whole loop.
func TestPreReviewIsAnAdviseThroughApplyEdits(t *testing.T) {
	root := writeMCPReviewProject(t)
	a := testApp()
	a.MCPSurface = cli.MCPSurface{Sets: map[string]bool{host.MCPSetReview: true}}
	proj := filepath.Join(root, "kapi.yaml")
	session := reviewSession(t, a, "review-agent")

	var queue ReviewQueueOutput
	require.False(t, call(t, session, "review_queue", map[string]any{"project": proj}, &queue))
	require.Len(t, queue.Pending, 2)
	first := queue.Pending[0]

	var picture ReviewBlockOutput
	require.False(t, call(t, session, "review_block", map[string]any{"project": proj, "at": first.Ref}, &picture))

	type result struct {
		Status string `json:"status"`
		Ops    []struct {
			Status string        `json:"status"`
			Error  *change.Error `json:"error"`
		} `json:"ops"`
	}
	decide := func(outcome, rev string) map[string]any {
		return map[string]any{"project": proj, "ops": []any{map[string]any{
			"op": "decide", "at": picture.Ref, "if_match": rev, "outcome": outcome,
			"score": 72, "reasons": []string{"reads stiffly"},
		}}}
	}

	var refused result
	assert.True(t, call(t, session, "apply_edits", decide("establish", picture.Rev), &refused))
	require.Len(t, refused.Ops, 1)
	require.NotNil(t, refused.Ops[0].Error)
	assert.Equal(t, change.CodeNotPermitted, refused.Ops[0].Error.Code, "an agent never decides")

	var stale result
	assert.True(t, call(t, session, "apply_edits", decide("advise", "r:0000000000000000"), &stale))
	require.Len(t, stale.Ops, 1)
	require.NotNil(t, stale.Ops[0].Error)
	assert.Equal(t, change.CodeStale, stale.Ops[0].Error.Code, "a pre-review binds to the wording that was read")

	var advised result
	require.False(t, call(t, session, "apply_edits", decide("advise", picture.Rev), &advised))
	assert.Equal(t, "applied", advised.Status)

	// The unit stays in the queue for a person, carrying the score under the
	// agent's name.
	require.False(t, call(t, session, "review_queue", map[string]any{"project": proj}, &queue))
	require.Equal(t, 2, queue.Total, "a pre-review decides nothing")
	var scored *ReviewQueueRow
	for i := range queue.Pending {
		if queue.Pending[i].Key == first.Key {
			scored = &queue.Pending[i]
		}
	}
	require.NotNil(t, scored)
	require.NotNil(t, scored.AIScore)
	assert.Equal(t, 72, *scored.AIScore)
	assert.Equal(t, "agent/review-agent", scored.AIModel)
}

// writeMCPTranslateAfterProject scaffolds a project whose translate_after asks for a
// human, so the queue carries source units beside the nb translations.
func writeMCPTranslateAfterProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := t.TempDir()
	recipe := `version: v1
name: rev-source
defaults:
  source_language: en
  target_languages: [nb]
  translate_after: established
collections:
  - name: app
    content:
      - path: en.json
        target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

// review_queue lists one queue across the languages: an agent sees the source
// units it is being asked to look at, marked, with the per-language counts
// beside them.
func TestHandleReviewQueue_ListsSourceUnitsAndFiltersByLanguage(t *testing.T) {
	root := writeMCPTranslateAfterProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	_, out, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj})
	require.NoError(t, err)
	assert.Equal(t, 4, out.Total)
	assert.Equal(t, []cli.ReviewLanguage{
		{Language: "en", Pending: 2, Source: true},
		{Language: "nb", Pending: 2},
	}, out.Languages)
	assert.True(t, out.Pending[0].IsSource, "the source rows lead the queue")
	assert.Equal(t, "en", out.Pending[0].Language)
	assert.Equal(t, "en.json", out.Pending[0].File)
	assert.Equal(t, BlockRef{Doc: "en.json", Block: out.Pending[0].Key}, out.Pending[0].Ref,
		"a source row's reference names no edition")

	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Language: "en"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	for _, it := range out.Pending {
		assert.True(t, it.IsSource)
	}
	assert.Len(t, out.Languages, 2, "the summary still offers every language")

	_, out, err = handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: proj, Language: "nb"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Total)
	for _, it := range out.Pending {
		assert.False(t, it.IsSource)
	}
}

// review_queue offers the source lane even when the default gate holds no source
// units: an agent reading the summary sees the source language at count 0 beside
// the target work, because the handler passes through the queue's Languages set.
func TestHandleReviewQueue_OffersTheSourceLaneWhenSourceIsClean(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	root := t.TempDir()
	recipe := `version: v1
name: rev-clean
defaults:
  source_language: en
  target_languages: [nb]
  translate_after: written
collections:
  - name: app
    content:
      - path: en.json
        target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))

	a := testApp()
	_, out, err := handleReviewQueue(t.Context(), a, ReviewQueueInput{Project: filepath.Join(root, "kapi.yaml")})
	require.NoError(t, err)
	assert.Equal(t, []cli.ReviewLanguage{
		{Language: "en", Pending: 0, Source: true},
		{Language: "nb", Pending: 2},
	}, out.Languages, "the source lane is offered at zero beside the queue-driven targets")
	for _, it := range out.Pending {
		assert.False(t, it.IsSource, "a clean source under the default gate queues no source rows")
	}
}

// review_block answers for a source-language block: the wording, its rung on
// the authoring ladder, and the point governing it. The edition of the
// project's source language names the source too.
func TestHandleReviewBlock_AcceptsASourceLanguageBlock(t *testing.T) {
	root := writeMCPTranslateAfterProject(t)
	a := testApp()
	proj := filepath.Join(root, "kapi.yaml")

	for _, edition := range []string{"", "en"} {
		t.Run("edition "+edition, func(t *testing.T) {
			_, out, err := handleReviewBlock(t.Context(), a, ReviewBlockInput{
				Project: proj, At: BlockRef{Doc: "en.json", Block: "a", Edition: edition},
			})
			require.NoError(t, err)
			assert.Equal(t, BlockRef{Doc: "en.json", Block: "a"}, out.Ref)
			assert.Regexp(t, `^r:[0-9a-f]{16}$`, out.Rev)
			require.NotNil(t, out.Block)
			assert.True(t, out.Block.IsSource)
			assert.Equal(t, "en", out.Block.Language)
			assert.Equal(t, "Apple", out.Block.Source)
			assert.Empty(t, out.Block.Target)
			assert.Equal(t, "written", out.Block.Status)
			require.NotNil(t, out.Block.Context)
			assert.Equal(t, "en.json", out.Block.Context.Point.Path)
			assert.True(t, out.Block.Context.Point.IsSource)
			assert.Equal(t, "a", out.Block.Context.Neighbourhood.Key)
		})
	}
}
