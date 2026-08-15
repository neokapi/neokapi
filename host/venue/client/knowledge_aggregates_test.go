package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConceptStatusCounts(t *testing.T) {
	c, got := knowledgeServer(t, `{"total":42,"by_status":{"preferred":10,"approved":20,"admitted":0,"proposed":5,"deprecated":0,"forbidden":7}}`)

	counts, err := c.ConceptStatusCounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/concepts/status-counts", got.path)
	assert.Equal(t, 42, counts.Total)
	assert.Equal(t, 10, counts.ByStatus["preferred"])
	assert.Equal(t, 7, counts.ByStatus["forbidden"])
}

func TestConceptLocaleCoverage(t *testing.T) {
	c, got := knowledgeServer(t, `{"total":4,"locales":[
		{"locale":"en-US","present":3,"total":4,"pct":75},
		{"locale":"de-DE","present":1,"total":4,"pct":25}
	]}`)

	report, err := c.ConceptLocaleCoverage(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/concepts/locale-coverage", got.path)
	assert.Equal(t, 4, report.Total)
	require.Len(t, report.Locales, 2)
	assert.Equal(t, LocaleCoverage{Locale: "en-US", Present: 3, Total: 4, Pct: 75}, report.Locales[0])
}

func TestListConceptsSortParam(t *testing.T) {
	c, got := knowledgeServer(t, `{"concepts":[],"total_count":0}`)

	_, err := c.ListConcepts(context.Background(), ListConceptsParams{Sort: SortConceptsUpdatedAt})
	require.NoError(t, err)
	assert.Equal(t, "updated_at", got.query.Get("sort"))
}

func TestChangesetCounts(t *testing.T) {
	c, got := knowledgeServer(t, `{"total":3,"by_status":{"draft":1,"in_review":2,"approved":0,"merged":0,"abandoned":0,"superseded":0}}`)

	counts, err := c.ChangesetCounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/changesets/counts", got.path)
	assert.Equal(t, 3, counts.Total)
	assert.Equal(t, 2, counts.ByStatus["in_review"])
}

// TestListChangesetsCarriesOpsCountAndImpact: the list answers "N changes" and
// the stored blast radius without a detail call per row.
func TestListChangesetsCarriesOpsCountAndImpact(t *testing.T) {
	c, _ := knowledgeServer(t, `[{
		"id":"cs1","workspace_id":"ws1","name":"Rename CTA","status":"in_review","created_by":"alice",
		"created_at":"2026-06-01T10:00:00Z","updated_at":"2026-06-01T11:00:00Z",
		"ops_count":3,
		"impact_summary":{"total_blocks":900,"affected_blocks":12,"new_violations":7,"resolved":0,
			"words":340,"projects":2,"computed_at":"2026-06-01T11:05:00Z"}
	}]`)

	sets, err := c.ListChangesets(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, sets, 1)
	assert.Equal(t, 3, sets[0].OpsCount)
	require.NotNil(t, sets[0].Impact)
	assert.Equal(t, 12, sets[0].Impact.AffectedBlocks)
	assert.Equal(t, 2, sets[0].Impact.Projects)
}

func TestBlastRadiusStoredAndFresh(t *testing.T) {
	body := `{"total_blocks":900,"affected_blocks":12,"stored":true,"computed_at":"2026-06-01T11:05:00Z"}`

	c, got := knowledgeServer(t, body)
	impact, err := c.GetChangesetBlastRadius(context.Background(), "cs1")
	require.NoError(t, err)
	assert.Empty(t, got.query.Get("fresh"))
	assert.True(t, impact.Stored)
	require.NotNil(t, impact.ComputedAt)

	c, got = knowledgeServer(t, body)
	_, err = c.RefreshChangesetBlastRadius(context.Background(), "cs1")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/acme/changesets/cs1/blast-radius", got.path)
	assert.Equal(t, "1", got.query.Get("fresh"))
}
