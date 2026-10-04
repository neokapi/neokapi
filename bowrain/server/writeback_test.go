package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readThenChange is a content store on which the stored content changes right
// after a caller's first block read, the way a push applied by the worker lands
// between a server run's read and its write.
type readThenChange struct {
	platstore.ContentStore
	once   sync.Once
	change func(ctx context.Context)
}

func (s *readThenChange) GetBlocks(ctx context.Context, q platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	blocks, err := s.ContentStore.GetBlocks(ctx, q)
	s.once.Do(func() { s.change(ctx) })
	return blocks, err
}

// seedWriteBackItem creates a project with one item, en.json, holding the given
// source strings.
func seedWriteBackItem(t *testing.T, cs platstore.ContentStore, projectID string, sources ...string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: projectID, Name: projectID, DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages: []model.LocaleID{model.LocaleFrench},
	}))
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &platstore.Item{ProjectID: projectID, Name: "en.json", Format: "json"}))
	blocks := make([]*model.Block, 0, len(sources))
	for i, src := range sources {
		b := model.NewBlock("k"+string(rune('a'+i)), src)
		b.Translatable = true
		blocks = append(blocks, b)
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", blocks))
}

// requireNothingStored fails for every block the project still holds.
func requireNothingStored(t *testing.T, cs platstore.ContentStore, projectID string) {
	t.Helper()
	all, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	for _, sb := range all {
		t.Errorf("block %s (%q) is stored again after its item was removed, under item %q",
			sb.Block.ID, sb.Block.SourceText(), sb.ItemName)
	}
}

// A server run's source settlement reads blocks a batch at a time and writes
// back the ones whose status moved. When a push removes an item between the
// read and the write, the write-back has nothing to land on and must store
// nothing: storing the blocks anyway left rows with no item, which the pull
// served as blocks no file can hold.
func TestSettleSource_AnItemRemovedDuringSettlementStaysRemoved(t *testing.T) {
	cs, err := bstore.NewPostgresStoreFromDB(pgtest.NewTestDB(t))
	require.NoError(t, err)
	const projectID = "settle-writeback-removed"
	seedWriteBackItem(t, cs, projectID, "A well-formed sentence.", "Another well-formed sentence.")

	racing := &readThenChange{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, projectID, "main", "en.json"))
	}}
	s := &Server{ContentStore: racing}
	s.convergence = newConvergenceOrchestrator(s)

	_, err = s.convergence.settleSource(t.Context(), projectID)
	require.NoError(t, err)
	requireNothingStored(t, cs, projectID)
}

// readThenEdit is a PostgreSQL content store on which a person edits right
// after a caller's first block read, keeping the store's held writes.
type readThenEdit struct {
	*bstore.PostgresStore
	once   sync.Once
	change func(ctx context.Context)
}

func (s *readThenEdit) GetBlocks(ctx context.Context, q platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	blocks, err := s.PostgresStore.GetBlocks(ctx, q)
	s.once.Do(func() { s.change(ctx) })
	return blocks, err
}

// Settlement judges the source a batch read and stamps what it found. A person
// rewriting a translation of a block in that batch meanwhile keeps the
// rewrite: the stamp lands only on a row holding the content the batch read.
func TestSettleSource_KeepsATranslationRewrittenDuringSettlement(t *testing.T) {
	cs, err := bstore.NewPostgresStoreFromDB(pgtest.NewTestDB(t))
	require.NoError(t, err)
	const projectID = "settle-writeback-rewritten"
	seedWriteBackItem(t, cs, projectID, "A well-formed sentence.", "Another well-formed sentence.")
	all, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	require.Len(t, all, 2)
	rewritten, untouched := all[0].Block.ID, all[1].Block.ID
	writeTargetText(t, cs, projectID, rewritten, "fr", "Une phrase.")

	store := &readThenEdit{PostgresStore: cs}
	s := &Server{ContentStore: store}
	store.change = func(context.Context) {
		editAsPerson(t, s, projectID, rewritten, "fr", textContent("Une phrase bien formée."))
	}
	s.convergence = newConvergenceOrchestrator(s)
	_, err = s.convergence.settleSource(t.Context(), projectID)
	require.NoError(t, err)

	got, err := cs.GetBlock(t.Context(), projectID, "main", rewritten)
	require.NoError(t, err)
	assert.Equal(t, "Une phrase bien formée.", got.Block.TargetText("fr"), "the person's rewrite stays")
	settled, err := cs.GetBlock(t.Context(), projectID, "main", untouched)
	require.NoError(t, err)
	assert.Equal(t, model.SourceStatusWritten, sourceStatusOf(settled.Block), "the untouched block is settled")
}

// Pseudo-translation reads an item's blocks and commits the targets it drafts
// through the stream's change service. When the item is removed in between,
// nothing is stored.
func TestPseudoTranslate_AnItemRemovedDuringTheRunStaysRemoved(t *testing.T) {
	cs, err := bstore.NewPostgresStoreFromDB(pgtest.NewTestDB(t))
	require.NoError(t, err)
	const projectID = "pseudo-writeback-removed"
	seedWriteBackItem(t, cs, projectID, "Hello", "Goodbye")

	racing := &readThenChange{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, projectID, "main", "en.json"))
	}}
	_, err = editorPseudoTranslate(t.Context(), racing, commitFor(t, cs, projectID), projectID, "main", "en.json", "fr")
	require.NoError(t, err)
	requireNothingStored(t, cs, projectID)
}

// Pseudo-translation commits its drafts as the tool: each lands as a draft
// with the tool's provenance, and a translation a person wrote after the run
// read the block keeps the person's wording.
func TestPseudoTranslate_CommitsDraftsAndKeepsAPersonsLaterEdit(t *testing.T) {
	cs, err := bstore.NewPostgresStoreFromDB(pgtest.NewTestDB(t))
	require.NoError(t, err)
	const projectID = "pseudo-drafts"
	seedWriteBackItem(t, cs, projectID, "Hello", "Goodbye")
	stored, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	ids := map[string]string{}
	for _, sb := range stored {
		ids[sb.Block.SourceText()] = sb.Block.ID
	}

	racing := &readThenChange{ContentStore: cs, change: func(ctx context.Context) {
		_, err := cs.UpdateBlock(ctx, projectID, "main", ids["Goodbye"], func(sb *venue.StoredBlock) error {
			sb.Block.SetTargetText(model.LocaleFrench, "Au revoir")
			return nil
		})
		assert.NoError(t, err)
	}}
	_, err = editorPseudoTranslate(t.Context(), racing, commitFor(t, cs, projectID), projectID, "main", "en.json", "fr")
	require.NoError(t, err)

	hello, err := cs.GetBlock(t.Context(), projectID, "main", ids["Hello"])
	require.NoError(t, err)
	require.True(t, holdsTarget(hello.Block, model.LocaleFrench))
	assert.NotEqual(t, "Hello", hello.Block.TargetText(model.LocaleFrench), "the pseudo-translation landed")
	assert.Equal(t, model.TargetStatusDraft, targetStatusOf(t, hello.Block, model.LocaleFrench))
	goodbye, err := cs.GetBlock(t.Context(), projectID, "main", ids["Goodbye"])
	require.NoError(t, err)
	assert.Equal(t, "Au revoir", goodbye.Block.TargetText(model.LocaleFrench), "the person's later wording stays")
}

// Source settlement stamps each block it checks and writes the stamp back. The
// producer's next push decides what to upload by comparing its record hashes
// with the ones the venue's tree holds, so the settlement must leave those
// standing: when it moved them, every settled block read as missing and each
// push after a server run uploaded the whole corpus again.
func TestSettleSource_LeavesThePushedRecordHashesStanding(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)
	items := []pushBlockItem{
		{ID: "b1", Text: "A well-formed sentence.", ItemName: "en.json"},
		{ID: "b2", Text: "Another well-formed sentence.", ItemName: "en.json"},
	}
	pushBlocks(t, srv, e, authHeader, pid, items)

	// missing counts the blocks a producer holding items would upload.
	missing := func() int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/tree", nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var tree struct {
			Items []struct {
				Record []string `json:"record"`
			} `json:"items"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tree))
		held := map[string]bool{}
		for _, item := range tree.Items {
			for _, r := range item.Record {
				held[r] = true
			}
		}
		n := 0
		for _, item := range items {
			b := &model.Block{ID: item.ID, Translatable: true}
			b.SetSourceText(item.Text)
			if !held[venue.RecordHash(b, "en")] {
				n++
			}
		}
		return n
	}
	require.Zero(t, missing(), "the venue holds the pushed content as it was pushed")

	o := srv.convergence
	if o == nil {
		o = newConvergenceOrchestrator(srv)
	}
	res, err := o.settleSource(t.Context(), pid)
	require.NoError(t, err)
	require.Equal(t, len(items), res.Total, "settlement checked every pushed block")
	assert.Zero(t, missing(), "after settlement the next push has nothing to upload")
}
