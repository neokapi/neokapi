package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// These tests pin how the check endpoints choose what to run and what to read:
// the checker set comes from the point the item sits at, so a project governing
// two products by two voices judges each by the voice that governs it (the
// selection host/check.go makes per file), and the stream, block id, item and
// locale come from the request.

// seedCheckPoint stores a collection bound to a voice profile, an item inside
// it, and one block whose source carries the term under test.
func seedCheckPoint(t *testing.T, cs platstore.ContentStore, projectID, collection, profileID, itemName, source string) {
	t.Helper()
	ctx := t.Context()

	col := &platstore.Collection{
		ProjectID:       projectID,
		Name:            collection,
		Kind:            platstore.CollectionUploaded,
		ItemLabel:       "page",
		Stream:          "main",
		ConnectorConfig: map[string]string{coreprofile.PropertyProfileID: profileID},
	}
	require.NoError(t, cs.CreateCollection(ctx, col))
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &platstore.Item{
		Name: itemName, Format: "txt", ItemType: "file", CollectionID: col.ID,
	}))

	b := &model.Block{ID: "b-" + collection, Translatable: true}
	b.SetSourceText(source)
	b.SetTargetText("fr", "Le logiciel bon marché est prêt")
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", itemName, []*model.Block{b}))
}

// TestChecksAtPoint_GovernanceSelectsTheCheckers proves the checker set is
// chosen where the content sits: the same source text raises a vocabulary
// finding under the voice governing one collection and passes clean under the
// voice governing another.
func TestChecksAtPoint_GovernanceSelectsTheCheckers(t *testing.T) {
	ctx := t.Context()
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "two-points")

	srv.VoiceStore = &editorFakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{
		"bp-strict": {
			ID:   "bp-strict",
			Name: "Strict",
			Vocabulary: coreprofile.VocabularyRules{
				ForbiddenTerms: []coreprofile.TermRule{{Term: "cheap", Replacement: "affordable"}},
			},
		},
		"bp-open": {ID: "bp-open", Name: "Open"},
	}}

	const source = "the cheap software is ready"
	seedCheckPoint(t, cs, proj.ID, "strict-docs", "bp-strict", "strict.txt", source)
	seedCheckPoint(t, cs, proj.ID, "open-docs", "bp-open", "open.txt", source)

	strict := srv.checksAtPoint(ctx, proj.ID, "main", "strict.txt", originTestWS, "acme", "fr")
	open := srv.checksAtPoint(ctx, proj.ID, "main", "open.txt", originTestWS, "acme", "fr")

	require.NotNil(t, strict.Voice)
	require.NotNil(t, open.Voice)
	assert.Equal(t, "bp-strict", strict.Voice.ID, "the voice bound on this item's collection governs it")
	assert.Equal(t, "bp-open", open.Voice.ID)

	block := &model.Block{ID: "b1", Translatable: true}
	block.SetSourceText(source)
	block.SetTargetText("fr", "Le logiciel bon marché est prêt")

	strictIssues := runChecksOnBlock(ctx, block, strict)
	openIssues := runChecksOnBlock(ctx, block, open)

	assert.True(t, hasIssueContaining(strictIssues, "cheap"),
		"the vocabulary governing this point is part of its checker set: %+v", strictIssues)
	assert.False(t, hasIssueContaining(openIssues, "cheap"),
		"a point whose voice forbids nothing must not inherit another point's rules: %+v", openIssues)
}

// TestChecksAtPoint_UngovernedPointKeepsTheStandardSet: an item in no
// collection, in a workspace with no voice bound, is still checked. Governance
// adds checkers; it is not a precondition for having any.
func TestChecksAtPoint_UngovernedPointKeepsTheStandardSet(t *testing.T) {
	ctx := t.Context()
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "ungoverned")
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "loose.txt", Format: "txt", ItemType: "file",
	}))

	checks := srv.checksAtPoint(ctx, proj.ID, "main", "loose.txt", originTestWS, "acme", "fr")
	assert.Nil(t, checks.Voice, "nothing is bound here")
	assert.Equal(t, model.LocaleID("en"), checks.SourceLocale)

	block := model.NewBlock("b1", "Hello world")
	block.SetTargetText("fr", "Bonjour  le monde")
	assert.True(t, hasIssueOfType(runChecksOnBlock(ctx, block, checks), "double-spaces"),
		"the standard per-locale checks run whatever governs the point")
}

// TestChecksAtPoint_ProtectedTermsJoinTheSet: a project that declares
// do-not-translate terms has them checked in the editor, in the same pass, and
// not only after a batch job has run.
func TestChecksAtPoint_ProtectedTermsJoinTheSet(t *testing.T) {
	ctx := t.Context()
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "protected")
	proj.Properties = map[string]string{"dnt_terms": "Kapi"}
	require.NoError(t, cs.UpdateProject(ctx, proj))

	checks := srv.checksAtPoint(ctx, proj.ID, "main", "", originTestWS, "acme", "fr")
	require.Equal(t, []string{"Kapi"}, checks.DNT)

	block := model.NewBlock("b1", "Kapi is ready")
	block.SetTargetText("fr", "Le truc est prêt")
	issues := runChecksOnBlock(ctx, block, checks)
	assert.True(t, hasIssueContaining(issues, "Kapi"),
		"a protected term dropped from the target is reported: %+v", issues)
}

// TestHandleCheckBlock_HonoursTheRequestedStream: the endpoint read "main"
// whatever stream the request named, so a block on a branch was reported as
// missing while a same-named block on main was checked in its place. The stream
// comes from the request, and the block id and locale from the body the editor
// actually posts.
func TestHandleCheckBlock_HonoursTheRequestedStream(t *testing.T) {
	ctx := t.Context()
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "streamed")
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "v2", &platstore.Item{
		Name: "hello.txt", Format: "txt", ItemType: "file",
	}))
	b := &model.Block{ID: "only-on-v2", Translatable: true}
	b.SetSourceText("Hello world")
	b.SetTargetText("fr", "Bonjour  le monde")
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "v2", "hello.txt", []*model.Block{b}))
	// The store mints the id it stores under; ask it what the block is called.
	stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: "v2", ItemName: "hello.txt"})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	blockID := stored[0].Block.ID

	e := echo.New()
	body := `{"block_id":"` + blockID + `","locale":"fr"}`

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost,
		"/api/v1/acme/"+proj.ID+"/actions/v2/qa-check-block", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	c := originCtx(e, r, rec, proj.ID, [2]string{"ref", "v2"})
	require.NoError(t, srv.HandleCheckBlock(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var issues []CheckIssueResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &issues))
	assert.True(t, hasIssueOfType(issues, "double-spaces"),
		"the block on the requested stream is the one checked: %s", rec.Body.String())

	// The same request against main finds nothing to check, rather than
	// silently answering about another stream's block.
	rec = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost,
		"/api/v1/acme/"+proj.ID+"/actions/main/qa-check-block", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	c = originCtx(e, r, rec, proj.ID, [2]string{"ref", "main"})
	require.NoError(t, srv.HandleCheckBlock(c))
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestHandleCheckFile_ReadsTheBodyTheEditorPosts: the item and the locale
// arrive in the request body, and the checks run over the named stream.
func TestHandleCheckFile_ReadsTheBodyTheEditorPosts(t *testing.T) {
	ctx := t.Context()
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "file-checks")
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "hello.txt", Format: "txt", ItemType: "file",
	}))
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello world")
	b.SetTargetText("fr", "Bonjour  le monde")
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "hello.txt", []*model.Block{b}))

	e := echo.New()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost,
		"/api/v1/acme/"+proj.ID+"/actions/main/qa-check",
		strings.NewReader(`{"item":"hello.txt","locale":"fr"}`))
	r.Header.Set("Content-Type", "application/json")
	c := originCtx(e, r, rec, proj.ID, [2]string{"ref", "main"})
	require.NoError(t, srv.HandleCheckFile(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var results []FileCheckResultResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &results))
	require.Len(t, results, 1)
	assert.NotEmpty(t, results[0].BlockID)
	assert.True(t, hasIssueOfType(results[0].Issues, "double-spaces"), rec.Body.String())
}

func hasIssueOfType(issues []CheckIssueResponse, category string) bool {
	for _, i := range issues {
		if i.Type == category {
			return true
		}
	}
	return false
}

func hasIssueContaining(issues []CheckIssueResponse, text string) bool {
	for _, i := range issues {
		if strings.Contains(i.Message, text) || strings.Contains(i.OriginalText, text) {
			return true
		}
	}
	return false
}
