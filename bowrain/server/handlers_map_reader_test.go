package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/host/screens"
	"github.com/neokapi/neokapi/terms"
)

// mapParityDir is the fixture the desktop's TestMapReaderParity writes: the
// sample samples/audience-context as the desktop reads it, and the Map and
// the Reader it draws.
var mapParityDir = filepath.Join("..", "..", "host", "screens", "testdata", "map-parity")

// mapParityPushed is pushed.json.
type mapParityPushed struct {
	SourceLanguage string `json:"source_language"`
	Collections    []struct {
		Name        string                    `json:"name"`
		Voice       *coreprofile.VoiceProfile `json:"voice"`
		CarriedFrom string                    `json:"carried_from"`
		Carried     []coreprofile.TermRule    `json:"carried"`
	} `json:"collections"`
	Items []struct {
		Name       string `json:"name"`
		Format     string `json:"format"`
		Collection string `json:"collection"`
		Blocks     []struct {
			ID           string      `json:"id"`
			Translatable bool        `json:"translatable"`
			Runs         []model.Run `json:"runs"`
		} `json:"blocks"`
	} `json:"items"`
	Concepts []terms.Concept `json:"concepts"`
}

// readGolden decodes one of the fixture's files into out.
func readGolden(t *testing.T, name string, out any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(mapParityDir, name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, out))
}

// mapReaderSQLiteServer serves the Map, Reader and context object routes over
// a SQLite content store, signed in as caller, with the voices and terms the
// parity project binds.
func mapReaderSQLiteServer(t *testing.T, caller *decideCaller) (*Server, *echo.Echo, *sqlitestore.SQLiteStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	srv.wsStores.termsFactory = func() terms.Store { return &testTermStore{terms.NewInMemoryStore()} }
	e := echo.New()
	g := e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", caller.userID)
			c.Set("workspace_id", "ws")
			c.Set("project_permissions", caller.perms)
			c.Set("project_coordinates", caller.reach)
			return next(c)
		}
	})
	srv.registerContextObjectRoutes(g)
	srv.registerMapReaderRoutes(g)
	return srv, e, cs
}

// seedMapParity stores the parity project as a push leaves it: each
// collection bound to the voice at its point, each file's blocks, and the
// workspace's terms.
func seedMapParity(t *testing.T, srv *Server, cs platstore.ContentStore) string {
	t.Helper()
	ctx := t.Context()
	var pushed mapParityPushed
	readGolden(t, "pushed.json", &pushed)

	p := &platstore.Project{
		Name: "audience-context", DefaultSourceLanguage: model.LocaleID(pushed.SourceLanguage),
		WorkspaceID: "ws", Properties: map[string]string{},
	}
	require.NoError(t, cs.CreateProject(ctx, p))

	voices := &editorFakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{}}
	srv.VoiceStore = voices
	collectionIDs := map[string]string{}
	for _, c := range pushed.Collections {
		profile := *c.Voice
		profile.ID = "voice-" + c.Name
		voices.profiles[profile.ID] = (&profile).Carry(c.CarriedFrom, c.Carried)
		col := &platstore.Collection{
			ProjectID: p.ID, Name: c.Name, Kind: platstore.CollectionUploaded, ItemLabel: "file",
			Stream: "main", Owner: "recipe",
			ConnectorConfig: map[string]string{coreprofile.PropertyProfileID: profile.ID},
		}
		require.NoError(t, cs.CreateCollection(ctx, col))
		collectionIDs[c.Name] = col.ID
	}
	for _, it := range pushed.Items {
		require.NoError(t, cs.StoreItem(ctx, p.ID, "main", &platstore.Item{
			Name: it.Name, Format: it.Format, ItemType: "file", CollectionID: collectionIDs[it.Collection],
		}))
		var blocks []*model.Block
		var order []string
		for _, b := range it.Blocks {
			block := &model.Block{ID: b.ID, Translatable: b.Translatable}
			block.SetSourceRuns(b.Runs)
			blocks = append(blocks, block)
			order = append(order, b.ID)
		}
		require.NoError(t, cs.StoreBlocksForItem(ctx, p.ID, "main", it.Name, blocks))
		// A push records the file's order beside its blocks.
		require.NoError(t, cs.SetBlockOrder(ctx, p.ID, "main", it.Name, order))
	}

	tb, err := srv.wsStores.getTerms(ctx, "ws")
	require.NoError(t, err)
	for _, c := range pushed.Concepts {
		require.NoError(t, tb.AddConcept(ctx, c))
	}
	return p.ID
}

// mapReaderGet requests one route of the project and decodes its JSON into
// out, returning the status.
func mapReaderGet(t *testing.T, e *echo.Echo, method, pid, route string, body, out any) int {
	t.Helper()
	return decideRoute(t, e, method, pid, route, body, out)
}

// TestMapReaderParity holds the server's Map and Reader to the desktop's for
// the same project: the same blocks under the same voice and terms answer
// with the same JSON, apart from what the server cannot know (paths on disk,
// the workspace key of a project no push has bound, and when the files were
// read and checked) and the desktop's note about the profile a file sits
// under, which the context answer writes and the server does not.
func TestMapReaderParity(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: platauth.PermViewContent}
	srv, e, cs := mapReaderSQLiteServer(t, caller)
	pid := seedMapParity(t, srv, cs)

	// Before a project-wide check, the Map counts no findings.
	var before screens.MapSummaryDTO
	require.Equal(t, http.StatusOK, mapReaderGet(t, e, http.MethodGet, pid, "/map/main", nil, &before))
	assert.Nil(t, before.Findings, "a project not checked yet counts no findings")
	assert.Empty(t, before.CheckedAt)

	res, err := jobs.ProjectCheck{
		Binding: srv.checkBinding("ws"), Tallies: cs, ProjectID: pid, Stream: "main", WorkspaceID: "ws",
	}.Run(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 4, res.Items)

	var got screens.MapSummaryDTO
	require.Equal(t, http.StatusOK, mapReaderGet(t, e, http.MethodGet, pid, "/map/main", nil, &got))
	assert.NotEmpty(t, got.CheckedAt, "the Map reports when the check ran")
	for _, row := range got.FileRows {
		assert.Equal(t, row.Relative, row.Path, "the server knows a file by its item name")
	}
	var wantMap screens.MapSummaryDTO
	readGolden(t, "map-summary.json", &wantMap)
	gotMap := got
	gotMap.ProjectKey, gotMap.ReadAt, gotMap.CheckedAt = "", "", ""
	gotMap.FileRows = make([]screens.MapFileDTO, len(got.FileRows))
	for i, r := range got.FileRows {
		r.Path = ""
		gotMap.FileRows[i] = r
	}
	assertSameJSON(t, wantMap, gotMap, "the Map")

	var reader screens.ReaderFileDTO
	require.Equal(t, http.StatusOK, mapReaderGet(t, e, http.MethodGet, pid,
		"/reader/main?item="+url.QueryEscape("site/locales/en/child.json"), nil, &reader))
	var wantReader screens.ReaderFileDTO
	readGolden(t, "reader-file.json", &wantReader)
	assert.Empty(t, reader.Abs)
	assert.Empty(t, reader.Refused, "the Reader writes nothing")
	assert.Empty(t, reader.Attention)
	wantReader.Attention = reader.Attention
	assertSameJSON(t, wantReader, reader, "the Reader")

	// The Map's tally for the file is the Reader's count.
	for _, row := range got.FileRows {
		if row.Relative == reader.Path {
			assert.Equal(t, reader.Findings, row.Findings)
		}
	}
}

// assertSameJSON compares two values as the JSON they encode to.
func assertSameJSON(t *testing.T, want, got any, what string) {
	t.Helper()
	w, err := json.MarshalIndent(want, "", "  ")
	require.NoError(t, err)
	g, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, string(w), string(g), "%s differs from the desktop's", what)
}

// TestReader_UnknownItemAndMissingItem: the Reader names one item, and an
// item the stream does not hold is not found.
func TestReader_UnknownItemAndMissingItem(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: platauth.PermViewContent}
	srv, e, cs := mapReaderSQLiteServer(t, caller)
	pid := seedMapParity(t, srv, cs)
	var errBody ErrorResponse
	assert.Equal(t, http.StatusBadRequest, mapReaderGet(t, e, http.MethodGet, pid, "/reader/main", nil, &errBody))
	assert.Equal(t, http.StatusNotFound, mapReaderGet(t, e, http.MethodGet, pid, "/reader/main?item=nothing.json", nil, &errBody))
	assert.Equal(t, http.StatusNotFound, mapReaderGet(t, e, http.MethodGet, "no-such-project", "/map/main", nil, &errBody))

	caller.perms = 0
	assert.Equal(t, http.StatusForbidden, mapReaderGet(t, e, http.MethodGet, pid, "/map/main", nil, &errBody))
	assert.Equal(t, http.StatusForbidden, mapReaderGet(t, e, http.MethodGet, pid, "/reader/main?item=x", nil, &errBody))
}

// TestChecksAtSourceLocale: checking an item at the project's source
// language runs the source-side checks alone. The block has no translation
// in that language, and no check reports an empty one; the voice's finding
// on the source stands.
func TestChecksAtSourceLocale(t *testing.T) {
	ctx := t.Context()
	srv, cs, _ := newOriginTestServer(t)
	srv.wsStores.termsFactory = func() terms.Store { return &testTermStore{terms.NewInMemoryStore()} }
	proj := seedOriginProject(t, cs, "source-locale")
	seedCheckStream(t, cs, proj.ID, "main")
	srv.VoiceStore = &editorFakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{
		"bp-strict": (&coreprofile.VoiceProfile{ID: "bp-strict", Name: "Strict"}).Carry("voice file",
			[]coreprofile.TermRule{{Term: "cheap", Replacement: "affordable"}}),
	}}
	seedCheckPoint(t, cs, proj.ID, "docs", "bp-strict", "docs.txt", "the cheap software is ready")

	checks, err := srv.checksAtPoint(ctx, proj.ID, "main", "docs.txt", originTestWS, "acme", proj.DefaultSourceLanguage)
	require.NoError(t, err)
	assert.True(t, checks.OfSource())
	stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: "main", ItemName: "docs.txt"})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	sb := stored[0]
	issues, err := runChecksOnBlock(ctx, sb.Block, checks)
	require.NoError(t, err)
	require.Len(t, issues, 1, "the word rule's finding, and no empty translation")
	assert.Equal(t, "cheap", issues[0].OriginalText)

	// At a target language the standard checks run as before.
	fr, err := srv.checksAtPoint(ctx, proj.ID, "main", "docs.txt", originTestWS, "acme", "fr")
	require.NoError(t, err)
	assert.False(t, fr.OfSource())
	untranslated := model.NewBlock("untranslated", "Some text to translate.")
	standard, err := jobs.StandardFindings(ctx, untranslated, fr)
	require.NoError(t, err)
	require.Len(t, standard, 1, "a block with no French translation is reported at French")
	assert.Equal(t, "empty-target", standard[0].Category)
	none, err := jobs.StandardFindings(ctx, untranslated, checks)
	require.NoError(t, err)
	assert.Empty(t, none, "and not at the source language")
}

// TestCheckProject_WithoutJobSystem: a server with no job queue says so.
func TestCheckProject_WithoutJobSystem(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: platauth.PermViewContent}
	srv, e, cs := mapReaderSQLiteServer(t, caller)
	pid := seedMapParity(t, srv, cs)
	var errBody ErrorResponse
	assert.Equal(t, http.StatusServiceUnavailable, mapReaderGet(t, e, http.MethodPost, pid, "/map/main/check", nil, &errBody))
}

// mapReaderPgServer is a server over the PostgreSQL stores with a job queue
// and source proposals, signed in as caller.
func mapReaderPgServer(t *testing.T, caller *decideCaller) (*Server, *echo.Echo, *jobs.ChannelQueue) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := jobs.NewJobStore(db)
	require.NoError(t, err)
	q := jobs.NewChannelQueue(4)
	t.Cleanup(func() { _ = q.Close() })
	srv := &Server{
		ContentStore: cs, JobStore: js, JobQueue: q,
		SourceProposalStore: bstore.NewSourceProposalStore(db.DB),
		wsStores:            newWorkspaceStores(),
	}
	srv.wsStores.termsFactory = func() terms.Store { return &testTermStore{terms.NewInMemoryStore()} }
	e := echo.New()
	g := e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", caller.userID)
			c.Set("workspace_id", "ws")
			c.Set("project_permissions", caller.perms)
			return next(c)
		}
	})
	srv.registerMapReaderRoutes(g)
	return srv, e, q
}

// TestCheckProject_EnqueuesAJob: the Map's check is a project-wide check
// job of the stream, requested by the member, and enqueued.
func TestCheckProject_EnqueuesAJob(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: platauth.PermViewContent}
	srv, e, q := mapReaderPgServer(t, caller)
	p := &platstore.Project{Name: "check", DefaultSourceLanguage: "en", WorkspaceID: "ws", Properties: map[string]string{}}
	require.NoError(t, srv.ContentStore.CreateProject(t.Context(), p))

	var started projectCheckStarted
	require.Equal(t, http.StatusAccepted, mapReaderGet(t, e, http.MethodPost, p.ID, "/map/main/check", nil, &started))
	require.NotEmpty(t, started.JobID)
	job, err := srv.JobStore.GetJob(t.Context(), started.JobID)
	require.NoError(t, err)
	assert.True(t, job.IsProjectCheck())
	assert.Equal(t, "main", job.Stream)
	assert.Equal(t, p.ID, job.ProjectID)
	assert.Equal(t, "member-a", job.CreatedBy)
	queued, ack, _, err := q.Dequeue(t.Context())
	require.NoError(t, err)
	ack()
	assert.Equal(t, started.JobID, queued)
}

// TestReaderFix_FilesASourceProposal: a fix from the Reader is filed as a
// source proposal of the block's new text, one per block, and writes no
// block. A finding with no fix, or one no longer reported, is refused with
// the reason. Filing needs the permission a source proposal needs.
func TestReaderFix_FilesASourceProposal(t *testing.T) {
	ctx := t.Context()
	caller := &decideCaller{userID: "member-a", perms: platauth.PermViewContent | platauth.PermReview}
	srv, e, _ := mapReaderPgServer(t, caller)
	srv.VoiceStore = &editorFakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{
		"bp": {ID: "bp", Name: "Acme", Style: coreprofile.StyleRules{
			ProhibitedPatterns: []coreprofile.Pattern{{Regex: `\bLocalizely\b`, Description: "Name no competitor"}},
		}},
	}}
	p := &platstore.Project{
		Name: "fix", DefaultSourceLanguage: "en", WorkspaceID: "ws",
		Properties: map[string]string{coreprofile.PropertyProfileID: "bp"},
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, p))
	tb, err := srv.wsStores.getTerms(ctx, "ws")
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: "c-affordable", Terms: []terms.Term{
		{Text: "affordable", Locale: "en", Status: model.TermPreferred},
		{Text: "cheap", Locale: "en", Status: model.TermForbidden},
	}}))
	require.NoError(t, srv.ContentStore.StoreItem(ctx, p.ID, "main", &platstore.Item{Name: "docs/a.md", Format: "markdown", ItemType: "file"}))
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(ctx, p.ID, "main", "docs/a.md", []*model.Block{
		model.NewBlock("a-1", "The cheap plan is ready."),
		model.NewBlock("a-2", "Faster than Localizely, and cheap."),
	}))
	require.NoError(t, srv.ContentStore.SetBlockOrder(ctx, p.ID, "main", "docs/a.md", []string{"a-1", "a-2"}))

	item := "?item=" + url.QueryEscape("docs/a.md")
	var r screens.ReaderFileDTO
	require.Equal(t, http.StatusOK, mapReaderGet(t, e, http.MethodGet, p.ID, "/reader/main"+item, nil, &r))
	require.Equal(t, 3, r.Findings)
	require.Equal(t, 2, r.Fixable)
	assert.Equal(t, "failed", r.Verdict)
	var fixable, unfixable screens.ReaderFindingDTO
	for _, b := range r.Blocks {
		for _, f := range b.Findings {
			switch {
			case f.Fixable && b.Runs[0].Text.Text == "The cheap plan is ready.":
				fixable = f
			case !f.Fixable:
				unfixable = f
			}
		}
	}
	require.NotEmpty(t, fixable.ID)
	require.NotEmpty(t, unfixable.ID)

	var one ReaderFixResult
	require.Equal(t, http.StatusOK, mapReaderGet(t, e, http.MethodPost, p.ID, "/reader/main/fix",
		readerFixRequest{Item: "docs/a.md", FindingID: fixable.ID}, &one))
	require.Len(t, one.Proposals, 1)
	assert.Empty(t, one.Refused)
	proposal, err := srv.SourceProposalStore.Get(ctx, one.Proposals[0])
	require.NoError(t, err)
	assert.Equal(t, "The cheap plan is ready.", proposal.OriginalSource)
	assert.Equal(t, "The affordable plan is ready.", proposal.ProposedSource)
	assert.Equal(t, bstore.SourceProposalTextFix, proposal.Kind)
	assert.Equal(t, "member-a", proposal.FinderUser)
	// The Reader names the block as the format's reader did; the proposal
	// names the stored block.
	assert.Equal(t, "a-1", fixable.BlockID)
	storedBlocks, err := srv.ContentStore.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "docs/a.md"})
	require.NoError(t, err)
	storedID := map[string]string{}
	for _, sb := range storedBlocks {
		storedID[sb.SourceID] = sb.Block.ID
	}
	assert.Equal(t, storedID["a-1"], proposal.BlockID)

	// The block is as it was: the proposal waits for a source owner.
	var again screens.ReaderFileDTO
	require.Equal(t, http.StatusOK, mapReaderGet(t, e, http.MethodGet, p.ID, "/reader/main"+item, nil, &again))
	assert.Equal(t, 3, again.Findings)

	var refused ReaderFixResult
	require.Equal(t, http.StatusOK, mapReaderGet(t, e, http.MethodPost, p.ID, "/reader/main/fix",
		readerFixRequest{Item: "docs/a.md", FindingID: unfixable.ID}, &refused))
	assert.Empty(t, refused.Proposals)
	require.Len(t, refused.Refused, 1)
	assert.Equal(t, "This finding has no fix.", refused.Refused[0].Reason)
	require.Equal(t, http.StatusOK, mapReaderGet(t, e, http.MethodPost, p.ID, "/reader/main/fix",
		readerFixRequest{Item: "docs/a.md", FindingID: "gone-1"}, &refused))
	assert.Equal(t, "This finding is no longer reported.", refused.Refused[0].Reason)

	// The whole file: one proposal per block with a fix.
	var all ReaderFixResult
	require.Equal(t, http.StatusOK, mapReaderGet(t, e, http.MethodPost, p.ID, "/reader/main/fix-file",
		readerFixRequest{Item: "docs/a.md"}, &all))
	require.Len(t, all.Proposals, 2)
	second, err := srv.SourceProposalStore.Get(ctx, all.Proposals[1])
	require.NoError(t, err)
	assert.Equal(t, "Faster than Localizely, and affordable.", second.ProposedSource)

	caller.perms = platauth.PermViewContent
	var errBody ErrorResponse
	assert.Equal(t, http.StatusForbidden, mapReaderGet(t, e, http.MethodPost, p.ID, "/reader/main/fix",
		readerFixRequest{Item: "docs/a.md", FindingID: fixable.ID}, &errBody))
}
