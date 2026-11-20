package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime/pprof"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// The scale test seeds one project at the size of a large documentation
// project and times the review plan and the review inbox against it. It runs
// only when BOWRAIN_PLAN_SCALE is set, because seeding takes a while:
//
//	BOWRAIN_PLAN_SCALE=1 go test ./server/ -run TestReviewPlan_Scale -v
//
// BOWRAIN_PLAN_SCALE_BLOCKS overrides the block count (70000), and
// BOWRAIN_PLAN_SCALE_PROFILE names a file for a CPU profile of the timed
// calls.

const (
	planScaleCollections = 10
	planScaleItemBlocks  = 50
	planScaleConcepts    = 300
)

var planScaleLocales = []string{"nb", "de", "fr", "sv", "da"}

// The plan and the inbox count in SQL. Reading a project's blocks whole costs
// a large project seconds, and once ran the server out of memory, so neither
// may read a block at all.
func TestReviewPlan_ReadsNoBlocks(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	counter := &countingStore{ContentStore: s.ContentStore}
	s.ContentStore = counter

	rec := deckCall(t, s, s.HandleGetReviewPlan, wsID, projID, "", owner, platauth.PermAll, nil, url.Values{})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	plan := decodeJSON[reviewPlanResponse](t, rec)
	require.Len(t, plan.Languages, 1)
	require.Positive(t, plan.Languages[0].Waiting+plan.Languages[0].Held, "the project has translations to count")

	inbox := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), inbox)
	c.Set("user_id", owner)
	require.NoError(t, s.HandleReviewInbox(c))
	require.Equal(t, http.StatusOK, inbox.Code, inbox.Body.String())
	require.NotEmpty(t, decodeJSON[reviewInboxResponse](t, inbox).Projects)

	assert.Zero(t, counter.getBlocks.Load(), "the plan and the inbox read no block")
}

func TestReviewPlan_Scale(t *testing.T) {
	if os.Getenv("BOWRAIN_PLAN_SCALE") == "" {
		t.Skip("set BOWRAIN_PLAN_SCALE to seed a large project and time the plan")
	}
	blocks := 70000
	if n, err := strconv.Atoi(os.Getenv("BOWRAIN_PLAN_SCALE_BLOCKS")); err == nil && n > 0 {
		blocks = n
	}
	s, wsID, owner, db := newPlanScaleHarness(t)
	projID := seedPlanScaleProject(t, s, db, wsID, blocks)

	plan := func() time.Duration {
		start := time.Now()
		rec := deckCall(t, s, s.HandleGetReviewPlan, wsID, projID, "", owner, platauth.PermAll, nil, url.Values{})
		d := time.Since(start)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return d
	}
	inbox := func() time.Duration {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		c.Set("user_id", owner)
		start := time.Now()
		require.NoError(t, s.HandleReviewInbox(c))
		d := time.Since(start)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return d
	}
	// Warm the database and the server's caches.
	plan()
	inbox()

	if path := os.Getenv("BOWRAIN_PLAN_SCALE_PROFILE"); path != "" {
		f, err := os.Create(path)
		require.NoError(t, err)
		require.NoError(t, pprof.StartCPUProfile(f))
		defer func() {
			pprof.StopCPUProfile()
			_ = f.Close()
		}()
	}
	const runs = 5
	var plans, inboxes []time.Duration
	for range runs {
		plans = append(plans, plan())
		inboxes = append(inboxes, inbox())
	}
	slices.Sort(plans)
	slices.Sort(inboxes)
	t.Logf("%d blocks, %d languages: plan median %v (min %v, max %v); inbox median %v (min %v, max %v)",
		blocks, len(planScaleLocales),
		plans[runs/2], plans[0], plans[runs-1], inboxes[runs/2], inboxes[0], inboxes[runs-1])
}

// newPlanScaleHarness is the recheck harness with the workspace terms store on
// PostgreSQL, as production runs it.
func newPlanScaleHarness(t *testing.T) (*Server, string, string, *storage.PgDB) {
	t.Helper()
	s, wsID, owner := newRecheckHarness(t)
	db := s.ContentStore.(interface{ DB() *storage.PgDB }).DB()
	s.wsStores.termsFactory = nil
	s.wsStores.pgDB = db
	return s, wsID, owner, db
}

// seedPlanScaleProject seeds a project with the given number of source blocks in
// planScaleCollections collections, a translation of each in every scale
// language (most established, a share waiting, most of those classified),
// a check tally per item, a terms store of planScaleConcepts concepts, and a
// bound context key.
func seedPlanScaleProject(t *testing.T, s *Server, db *storage.PgDB, wsID string, blocks int) string {
	t.Helper()
	ctx := context.Background()
	targets := make([]model.LocaleID, 0, len(planScaleLocales))
	for _, l := range planScaleLocales {
		targets = append(targets, model.LocaleID(l))
	}
	proj := &platstore.Project{
		Name: "Scale", WorkspaceID: wsID, DefaultSourceLanguage: "en",
		TargetLanguages: targets,
		Properties:      map[string]string{"translate_after": "none"},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))
	for i := range planScaleCollections {
		c := &platstore.Collection{
			ID: fmt.Sprintf("col-%02d", i), ProjectID: proj.ID, Name: fmt.Sprintf("docs-%02d", i),
			Kind: platstore.CollectionKind("uploaded"), ItemLabel: "page",
			Context: map[string]string{"product": fmt.Sprintf("p%d", i%3)},
		}
		require.NoError(t, s.ContentStore.CreateCollection(ctx, c))
	}
	cos, ok := s.contextObjects()
	require.True(t, ok)
	_, err := cos.BindContextKey(ctx, proj.ID, "prj_scaleaaaaaaaaaaaaaaaa")
	require.NoError(t, err)

	items := (blocks + planScaleItemBlocks - 1) / planScaleItemBlocks
	policy := deckPolicyKey(s.deckPolicy())
	exec := func(q string, args ...any) {
		t.Helper()
		_, err := db.DB.ExecContext(ctx, q, args...)
		require.NoError(t, err, q)
	}
	exec(`INSERT INTO items (project_id, stream, id, name, collection_id, format, item_type)
		SELECT $1, 'main', 'it-' || g, 'docs/page-' || lpad(g::text, 5, '0') || '.md',
			'col-' || lpad((g % $3)::text, 2, '0'), 'markdown', 'file'
		FROM generate_series(0, $2 - 1) g`, proj.ID, items, planScaleCollections)
	exec(`INSERT INTO blocks (project_id, stream, id, item_id, item_name, position, source_id,
			source_json, source_revision, translatable, type, properties)
		SELECT $1, 'main', 'b-' || g, 'it-' || (g / $3), 'docs/page-' || lpad((g / $3)::text, 5, '0') || '.md',
			g % $3, 's' || (g % $3), '[{"text":"Source sentence ' || g || '"}]', 'rev-' || (g % ($2 * 9 / 10)),
			true, 'paragraph', '{"component":"c' || (g % 3) || '"}'
		FROM generate_series(0, $2 - 1) g`, proj.ID, blocks, planScaleItemBlocks)
	for li, loc := range planScaleLocales {
		// One translation in twelve waits for review, the rest are established.
		exec(`INSERT INTO translations (project_id, stream, block_id, locale, text, target_json, updated_at)
			SELECT $1, 'main', 'b-' || g, $3, 'Target ' || $3 || ' ' || g,
				CASE WHEN (g + $4) % 12 = 0 THEN '{"status":"draft"}'::jsonb ELSE '{"status":"established"}'::jsonb END,
				TIMESTAMPTZ '2026-11-01 00:00:00+00'
			FROM generate_series(0, $2 - 1) g`, proj.ID, blocks, loc, li)
		// Four in five waiting translations are classified, one in ten of
		// them failing a check.
		exec(`INSERT INTO review_deck_facts (project_id, stream, block_id, locale, target_updated_at,
				source_revision, policy, group_rank, deck_group, findings, failing, rules, computed_at)
			SELECT $1, 'main', 'b-' || g, $3, TIMESTAMPTZ '2026-11-01 00:00:00+00',
				'rev-' || (g % ($2 * 9 / 10)), $5,
				CASE WHEN g % 10 = 0 THEN 2 ELSE g % 2 END,
				CASE WHEN g % 10 = 0 THEN 'desk' WHEN g % 2 = 0 THEN 'fast' ELSE 'closer' END,
				CASE WHEN g % 10 = 0 THEN 1 ELSE 0 END, CASE WHEN g % 10 = 0 THEN 1 ELSE 0 END,
				CASE WHEN g % 10 = 0 THEN jsonb_build_array(jsonb_build_object(
					'key', 'terms.vocabulary:t' || (g % 7), 'rule', 'terms.vocabulary', 'count', 1, 'fails', true))
				ELSE '[]'::jsonb END, NOW()
			FROM generate_series(0, $2 - 1) g WHERE (g + $4) % 12 = 0 AND g % 5 <> 0`, proj.ID, blocks, loc, li, policy)
	}
	exec(`INSERT INTO item_check_tallies (project_id, stream, item_id, findings, rules)
		SELECT $1, 'main', 'it-' || g, g % 4, CASE WHEN g % 4 = 0 THEN '[]'::jsonb ELSE jsonb_build_array(jsonb_build_object(
			'key', 'voice.pattern:p' || (g % 5), 'rule', 'voice.pattern', 'count', g % 4)) END
		FROM generate_series(0, $2 - 1) g`, proj.ID, items)
	exec(`ANALYZE`)

	ws, err := s.AuthStore.GetWorkspace(ctx, wsID)
	require.NoError(t, err)
	tb, err := s.wsStores.getTerms(ctx, ws.Slug)
	require.NoError(t, err)
	for i := range planScaleConcepts {
		c := terms.Concept{ID: fmt.Sprintf("c-%03d", i), Definition: "A concept of the scale project.",
			Terms: []terms.Term{{Text: fmt.Sprintf("term%03d", i), Locale: "en", Status: model.TermPreferred}}}
		for li, loc := range planScaleLocales {
			if (i+li)%3 != 0 {
				c.Terms = append(c.Terms, terms.Term{Text: fmt.Sprintf("%s-term%03d", loc, i), Locale: model.LocaleID(loc), Status: model.TermPreferred})
			}
		}
		require.NoError(t, tb.AddConcept(ctx, c))
	}
	return proj.ID
}
