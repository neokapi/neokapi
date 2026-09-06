package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	bstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockIDInPrompt matches the "Block (id: b000):" line the entity-extract
// prompt writes for each block it submits, which is how the fake model below
// learns which blocks a request covers.
var blockIDInPrompt = regexp.MustCompile(`Block \(id: ([^)]+)\):`)

// extractionModel is a stand-in OpenAI endpoint that answers an extraction
// request with one entity and one term candidate per block, and counts how many
// requests each block was sent in. The count is the measurement: an extraction
// that sends a block to the model twice pays for it twice.
type extractionModel struct {
	mu       sync.Mutex
	requests int
	perBlock map[string]int
}

func newExtractionModel(t *testing.T) (*httptest.Server, *extractionModel) {
	t.Helper()
	m := &extractionModel{perBlock: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ids := blockIDsIn(string(body))

		m.mu.Lock()
		m.requests++
		for _, blockID := range ids {
			m.perBlock[blockID]++
		}
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(extractionReply(ids))
	}))
	t.Cleanup(srv.Close)
	return srv, m
}

// blockIDsIn reads the block ids out of a request body.
func blockIDsIn(body string) []string {
	var ids []string
	for _, match := range blockIDInPrompt.FindAllStringSubmatch(body, -1) {
		ids = append(ids, match[1])
	}
	return ids
}

// extractionReply is one structured extraction result: for each block, the
// entity "Acme" at offset 0 and the term candidate "widget" after it. Both
// offsets sit inside the fixture's text.
func extractionReply(ids []string) []byte {
	blocks := make([]map[string]any, 0, len(ids))
	for _, blockID := range ids {
		blocks = append(blocks, map[string]any{
			"block_id": blockID,
			"entities": []map[string]any{{
				"text": "Acme", "type": "organization", "dnt": true,
				"offset": 0, "length": 4, "confidence": 0.9,
			}},
			"term_candidates": []map[string]any{{
				"text": "widget", "definition": "a thing", "category": "technical",
				"translatability": "consistent", "confidence": 0.8,
				"offset": 5, "length": 6,
			}},
		})
	}
	content, _ := json.Marshal(map[string]any{"blocks": blocks})
	reply, _ := json.Marshal(map[string]any{
		"model": "test-model",
		"choices": []map[string]any{{
			"finish_reason": "stop",
			"message":       map[string]any{"role": "assistant", "content": string(content)},
		}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
	})
	return reply
}

// countingExtractionStore is the lease bookkeeping an extraction run needs,
// with every renewal granted.
type countingExtractionStore struct {
	ExtractionJobStore
	job *ExtractionJob
}

func (s *countingExtractionStore) GetExtractionJob(context.Context, string) (*ExtractionJob, error) {
	return s.job, nil
}

func (s *countingExtractionStore) ClaimExtractionJob(context.Context, string) (bool, int64, error) {
	return true, 1, nil
}

func (s *countingExtractionStore) RenewLease(context.Context, string, int64) (bool, error) {
	return true, nil
}

func (s *countingExtractionStore) UpdateExtractionJobProgress(context.Context, string, int, int, int) error {
	return nil
}

func (s *countingExtractionStore) CompleteExtractionJob(context.Context, string, int64) (bool, error) {
	return true, nil
}

// recordingReviewQueue keeps the review items an extraction created, so a test
// can state what the run produced rather than only how much it spent.
type recordingReviewQueue struct {
	mu    sync.Mutex
	items []*ReviewQueueItem
}

func (q *recordingReviewQueue) CreateReviewItem(_ context.Context, item *ReviewQueueItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
	return nil
}

func (q *recordingReviewQueue) IsTermRejected(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func (q *recordingReviewQueue) typed(kind string) []*ReviewQueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []*ReviewQueueItem
	for _, item := range q.items {
		if item.Type == kind {
			out = append(out, item)
		}
	}
	return out
}

// newExtractionFixture stores one item of blocks whose text carries the entity
// and the term the fake model reports.
func newExtractionFixture(t *testing.T, blocks int) (bstore.ContentStore, string) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(t.TempDir() + "/content.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	projectID := "extract-" + id.New()
	ctx := context.Background()
	require.NoError(t, cs.CreateProject(ctx, &bstore.Project{
		ID: projectID, Name: "Extract", DefaultSourceLanguage: "en",
	}))
	bs := make([]*model.Block, 0, blocks)
	for i := range blocks {
		b := &model.Block{ID: fmt.Sprintf("b%03d", i), Translatable: true}
		b.SetSourceText(fmt.Sprintf("Acme widget number %d.", i))
		bs = append(bs, b)
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", bs))
	return cs, projectID
}

// extractionRun is what one extraction job spent and produced.
type extractionRun struct {
	content   bstore.ContentStore
	projectID string
	// blockIDs are the ids the store assigned, which are the ids the prompt
	// carries and the fake model answers under.
	blockIDs []string
	spent    *extractionModel
	queue    *recordingReviewQueue
}

// runExtraction drives one extraction job against the fake model.
func runExtraction(t *testing.T, blocks int) extractionRun {
	t.Helper()
	ctx := context.Background()
	cs, projectID := newExtractionFixture(t, blocks)
	srv, spent := newExtractionModel(t)
	queue := &recordingReviewQueue{}
	store := &countingExtractionStore{job: &ExtractionJob{
		ID: id.New(), WorkspaceSlug: "ws", ProjectID: projectID,
		ItemName: "en.json", Status: ExtractionStatusQueued,
	}}
	deps := &ExtractionWorkerDeps{
		ExtractionJobStore: store,
		ContentStore:       cs,
		ReviewQueueCreator: queue,
		Platform: &PlatformProviderConfig{
			Provider: "openai", APIKey: "k", Model: "test-model", BaseURL: srv.URL,
		},
	}

	stored, err := cs.GetBlocks(ctx, bstore.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "en.json",
	})
	require.NoError(t, err)
	require.Len(t, stored, blocks)
	ids := make([]string, 0, len(stored))
	for _, sb := range stored {
		ids = append(ids, sb.Block.ID)
	}

	require.NoError(t, processExtractionJob(ctx, deps, store.job.ID))
	return extractionRun{content: cs, projectID: projectID, blockIDs: ids, spent: spent, queue: queue}
}

// An extraction sends each block to the model once. A second unchunked pass
// over the whole item doubles what the job spends, and the count here is what
// holds the pipeline to one pass.
func TestExtractionWorker_SendsEachBlockToTheModelOnce(t *testing.T) {
	// 120 blocks over a progress chunk of 50 and a batch of 10: three chunks,
	// twelve batches, and one request per batch.
	run := runExtraction(t, 120)

	run.spent.mu.Lock()
	defer run.spent.mu.Unlock()
	require.Len(t, run.spent.perBlock, 120, "not every block reached the model")
	for _, blockID := range run.blockIDs {
		assert.Equal(t, 1, run.spent.perBlock[blockID], "block %s was sent to the model more than once", blockID)
	}
	assert.Equal(t, 12, run.spent.requests, "one request per batch of ten, over three chunks")
}

// The annotations stored back are the chunk loop's own, one span per finding.
// A second pass over blocks the chunk loop already annotated appends its spans
// to theirs, storing every entity and every term candidate twice under a span
// id that collides with its twin.
func TestExtractionWorker_StoresOneSpanPerFinding(t *testing.T) {
	run := runExtraction(t, 60)

	stored, err := run.content.GetBlocks(context.Background(), bstore.BlockQuery{
		ProjectID: run.projectID, Stream: "main", ItemName: "en.json",
	})
	require.NoError(t, err)
	require.Len(t, stored, 60)

	for _, sb := range stored {
		entities := sb.Block.OverlayOf(model.OverlayEntity)
		require.NotNil(t, entities, "block %s carries no entity overlay", sb.Block.ID)
		require.Len(t, entities.Spans, 1, "block %s stored its entity more than once", sb.Block.ID)
		entity, ok := entities.Spans[0].Value.(*model.EntityAnnotation)
		require.True(t, ok)
		assert.Equal(t, "Acme", entity.Text)
		assert.Equal(t, model.EntityOrganization, entity.Type)

		terms := sb.Block.OverlayOf(model.OverlayTermCandidate)
		require.NotNil(t, terms, "block %s carries no term-candidate overlay", sb.Block.ID)
		require.Len(t, terms.Spans, 1, "block %s stored its term candidate more than once", sb.Block.ID)
		term, ok := terms.Spans[0].Value.(*model.TermCandidateAnnotation)
		require.True(t, ok)
		assert.Equal(t, "widget", term.Text)
		assert.Equal(t, model.Translatability("consistent"), term.Translatability)
	}
}

// The review items an extraction creates: one per finding, from the chunk
// loop's output.
func TestExtractionWorker_CreatesOneReviewItemPerFinding(t *testing.T) {
	run := runExtraction(t, 60)

	assert.Len(t, run.queue.typed("entity_review"), 60)
	assert.Len(t, run.queue.typed("term_candidate"), 60)
}
