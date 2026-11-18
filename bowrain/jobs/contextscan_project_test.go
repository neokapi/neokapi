package jobs

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	fwterms "github.com/neokapi/neokapi/terms"
)

// TestContextScanWorker_ProjectCandidates scans a project of two products:
// the sample reaches both collections, every candidate is counted over the
// whole project, and each is filed as a proposed concept of the project,
// scoped to the product its uses share. A term a person has rejected is not
// filed again.
func TestContextScanWorker_ProjectCandidates(t *testing.T) {
	f := newContextScanFixture(t)
	ctx := t.Context()
	cs, err := bstore.NewPostgresStoreFromDB(f.db)
	require.NoError(t, err)
	const (
		projectID = "proj-scan"
		wsID      = "ws-demo"
	)
	f.deps.ContentStore = cs
	f.deps.TermsResolver = TermsResolverFunc(func(slug string) (fwterms.Terminology, error) {
		return sqlterms.NewPostgresStoreFromDB(f.db, slug)
	})
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID: projectID, Name: "Scan Test", DefaultSourceLanguage: "en-US", WorkspaceID: wsID,
		Properties: map[string]string{termcandidate.ProjectPropRejected: "acmesync"},
	}))
	type col struct {
		name    string
		product string
		lines   []string
	}
	cols := []col{
		{"neokapi-docs", "neokapi", []string{
			"Kapifiles describe a Flowgraph. Every Flowgraph runs on Kapifiles.",
			"A Flowgraph has steps. Kapifiles name them. Acmesync is a rival.",
		}},
		{"bowrain-app", "bowrain", []string{
			"Open the Dashpanel to see Kapifiles. The Dashpanel lists your Kapifiles.",
			"Each Dashpanel row is a project. Acmesync appears nowhere else.",
		}},
	}
	for _, c := range cols {
		collection := &store.Collection{
			ProjectID: projectID, Name: c.name, Kind: store.CollectionConnected, Owner: venue.ContextOwnerRecipe,
			Context: map[string]string{"brand": "acme", "product": c.product},
		}
		require.NoError(t, cs.CreateCollection(ctx, collection))
		for i := range 3 {
			name := fmt.Sprintf("%s/page-%d.md", c.name, i)
			require.NoError(t, cs.StoreItem(ctx, projectID, "main", &store.Item{
				Name: name, Format: "markdown", ItemType: "file", CollectionID: collection.ID,
			}))
			blocks := make([]*model.Block, len(c.lines))
			for j, line := range c.lines {
				blocks[j] = model.NewBlock(fmt.Sprintf("%s-%d-%d", c.name, i, j), line)
			}
			require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", name, blocks))
		}
	}

	job := f.seedJob(t, wsID, ContextScanRequest{Project: projectID, ProfileName: "Scan Voice"})
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))
	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, ContextScanStatusCompleted, got.Status, "error: %s", got.Error)

	var result ContextScanResult
	require.NoError(t, json.Unmarshal(got.Result, &result))
	require.Len(t, result.Sources, 1)
	assert.Equal(t, "project", result.Sources[0].Kind)
	assert.Equal(t, "Scan Test", result.Sources[0].Label)

	reach := map[string]CandidateReach{}
	for _, a := range result.Artefacts {
		for _, r := range a.Reach {
			reach[r.Term] = r
		}
	}
	require.Contains(t, reach, "Kapifiles", "candidates: %v", result.Terms())
	assert.Equal(t, 9, reach["Kapifiles"].Blocks, "counted over every block, not the sample")
	assert.Empty(t, reach["Kapifiles"].At, "used in both products: held across the project")
	require.Contains(t, reach, "Dashpanel")
	assert.Equal(t, 6, reach["Dashpanel"].Blocks)
	assert.Equal(t, map[string]string{"product": "bowrain"}, reach["Dashpanel"].At)
	assert.Equal(t, []CollectionReach{{Name: "bowrain-app", Blocks: 6}}, reach["Dashpanel"].Collections)

	tb, err := sqlterms.NewPostgresStoreFromDB(f.db, "acme")
	require.NoError(t, err)
	concepts, err := tb.Concepts(ctx)
	require.NoError(t, err)
	filed := map[string]fwterms.Concept{}
	for _, cp := range concepts {
		filed[termcandidate.SourceText(cp, "en")] = cp
	}
	require.Contains(t, filed, "Dashpanel")
	dash := filed["Dashpanel"]
	assert.Equal(t, projectID, termcandidate.ProjectOf(dash))
	assert.True(t, termcandidate.IsCandidate(dash, "en"))
	assert.Equal(t, map[string]string{"product": "bowrain"}, dash.Coordinates())
	assert.Equal(t, termcandidate.ScanIdentity, dash.Properties[termcandidate.PropProposedBy])
	r, ok := termcandidate.ReachOf(dash)
	require.True(t, ok)
	assert.Equal(t, 6, r.Blocks)
	assert.NotContains(t, filed, "Acmesync", "a rejected term is not proposed again")

	// A second scan files nothing twice.
	again := f.seedJob(t, wsID, ContextScanRequest{Project: projectID, ProfileName: "Scan Voice"})
	require.NoError(t, processContextScanJob(ctx, f.deps, again.ID))
	after, err := tb.Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, after, len(concepts))
}

// TestContextScanWorker_ProjectOutsideWorkspace refuses a project another
// workspace holds, as a skipped source.
func TestContextScanWorker_ProjectOutsideWorkspace(t *testing.T) {
	f := newContextScanFixture(t)
	ctx := t.Context()
	cs, err := bstore.NewPostgresStoreFromDB(f.db)
	require.NoError(t, err)
	f.deps.ContentStore = cs
	require.NoError(t, cs.CreateProject(ctx, &store.Project{ID: "other", Name: "Other", DefaultSourceLanguage: "en", WorkspaceID: "ws-other"}))

	job := f.seedJob(t, "ws-demo", ContextScanRequest{Project: "other"})
	require.Error(t, processContextScanJob(ctx, f.deps, job.ID))
	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusFailed, got.Status)
	assert.Contains(t, got.Error, "the project could not be read")
}

// TestLoadScanProject_FailsClosed refuses a project of another workspace, one
// naming no workspace, and a scan carrying no workspace id.
func TestLoadScanProject_FailsClosed(t *testing.T) {
	f := newContextScanFixture(t)
	ctx := t.Context()
	cs, err := bstore.NewPostgresStoreFromDB(f.db)
	require.NoError(t, err)
	require.NoError(t, cs.CreateProject(ctx, &store.Project{ID: "mine", Name: "Mine", DefaultSourceLanguage: "en", WorkspaceID: "ws-a"}))
	require.NoError(t, cs.CreateProject(ctx, &store.Project{ID: "theirs", Name: "Theirs", DefaultSourceLanguage: "en", WorkspaceID: "ws-b"}))
	require.NoError(t, cs.CreateProject(ctx, &store.Project{ID: "nobody", Name: "Nobody", DefaultSourceLanguage: "en"}))

	_, err = loadScanProject(ctx, cs, "ws-a", "mine")
	require.NoError(t, err)
	for _, tc := range []struct{ ws, project string }{
		{"ws-a", "theirs"},
		{"ws-a", "nobody"},
		{"", "mine"},
		{"", "nobody"},
		{"ws-a", "missing"},
	} {
		_, err := loadScanProject(ctx, cs, tc.ws, tc.project)
		assert.Error(t, err, "workspace %q, project %q", tc.ws, tc.project)
	}
}
