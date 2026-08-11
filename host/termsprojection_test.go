package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var projectionStamp = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// authoredConcept builds a concept in the shape the committed bundle carries.
func authoredConcept(id, en, nb string, status model.TermStatus) terms.Concept {
	return terms.Concept{
		ID:     id,
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: en, Locale: "en", Status: status},
			{Text: nb, Locale: "nb", Status: status},
		},
		CreatedAt: projectionStamp,
		UpdatedAt: projectionStamp,
	}
}

// writeAuthoredTerms writes the committed bundle the projection merges into.
func writeAuthoredTerms(t *testing.T, root string, concepts ...terms.Concept) string {
	t.Helper()
	data, err := ktb.Marshal(ktb.FromConcepts(concepts))
	require.NoError(t, err)
	path := filepath.Join(root, project.RelStatePath(ktb.ConventionalName))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

// readAuthoredTerms decodes the committed bundle back.
func readAuthoredTerms(t *testing.T, root string) *ktb.File {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, project.RelStatePath(ktb.ConventionalName)))
	require.NoError(t, err)
	file, err := ktb.Unmarshal(data)
	require.NoError(t, err)
	return file
}

// conceptByID finds a concept in a decoded bundle.
func conceptByID(f *ktb.File, id string) (terms.Concept, bool) {
	for _, c := range f.Concepts {
		if c.ID == id {
			return c, true
		}
	}
	return terms.Concept{}, false
}

func TestProjectReviewedConcepts(t *testing.T) {
	tests := []struct {
		name string
		// authored is the committed bundle before the projection.
		authored []terms.Concept
		// pulled is the workspace snapshot the projection reads.
		pulled      []terms.Concept
		wantWritten bool
		wantAdded   int
		wantUpdated int
		// check inspects the resulting bundle.
		check func(t *testing.T, f *ktb.File)
	}{
		{
			name:     "an approved decision lands",
			authored: []terms.Concept{authoredConcept("term:memory", "memory", "minne", model.TermProposed)},
			pulled: []terms.Concept{{
				ID: "term:memory",
				Terms: []terms.Term{
					{Text: "memory", Locale: "en", Status: model.TermPreferred},
					{Text: "innholdsminne", Locale: "nb", Status: model.TermPreferred},
				},
				UpdatedAt: projectionStamp.Add(24 * time.Hour),
			}},
			wantWritten: true,
			wantUpdated: 1,
			check: func(t *testing.T, f *ktb.File) {
				c, ok := conceptByID(f, "term:memory")
				require.True(t, ok)
				// The decided en term replaced its authored twin, the decided nb
				// term was added, and the authored nb term survived.
				assert.Len(t, c.Terms, 3)
				assert.Equal(t, terms.TermSourceTerminology, c.Source, "the authored source marker survived")
				assert.Equal(t, projectionStamp, c.CreatedAt, "the authored creation stamp survived")
				for _, term := range c.Terms {
					if term.Text == "memory" {
						assert.Equal(t, model.TermPreferred, term.Status)
					}
				}
			},
		},
		{
			name:     "authored concepts the workspace lacks survive",
			authored: []terms.Concept{authoredConcept("term:kept", "faithful", "trofast", model.TermPreferred)},
			pulled: []terms.Concept{{
				ID:    "term:new",
				Terms: []terms.Term{{Text: "voice profile", Locale: "en", Status: model.TermPreferred}},
			}},
			wantWritten: true,
			wantAdded:   1,
			check: func(t *testing.T, f *ktb.File) {
				_, ok := conceptByID(f, "term:kept")
				assert.True(t, ok, "the authored concept was not deleted")
				added, ok := conceptByID(f, "term:new")
				require.True(t, ok)
				assert.Equal(t, terms.TermSourceTerminology, added.Source, "an adopted concept is marked as terminology")
				assert.Len(t, f.Concepts, 2)
			},
		},
		{
			name:     "raw workspace state does not project",
			authored: []terms.Concept{authoredConcept("term:kept", "faithful", "trofast", model.TermPreferred)},
			pulled: []terms.Concept{{
				ID: "term:draft",
				Terms: []terms.Term{
					{Text: "scratchpad", Locale: "en", Status: model.TermProposed},
					{Text: "kladdeblokk", Locale: "nb", Status: model.TermAdmitted},
				},
			}},
			wantWritten: false,
			check: func(t *testing.T, f *ktb.File) {
				_, ok := conceptByID(f, "term:draft")
				assert.False(t, ok, "an ungoverned concept stayed out of git")
				assert.Len(t, f.Concepts, 1)
			},
		},
		{
			name:     "a decision already in git writes nothing",
			authored: []terms.Concept{authoredConcept("term:memory", "memory", "innholdsminne", model.TermPreferred)},
			pulled: []terms.Concept{{
				ID: "term:memory",
				Terms: []terms.Term{
					{Text: "memory", Locale: "en", Status: model.TermPreferred},
					{Text: "innholdsminne", Locale: "nb", Status: model.TermPreferred},
				},
				UpdatedAt: projectionStamp.Add(90 * 24 * time.Hour),
			}},
			wantWritten: false,
			check: func(t *testing.T, f *ktb.File) {
				c, ok := conceptByID(f, "term:memory")
				require.True(t, ok)
				assert.Equal(t, projectionStamp, c.UpdatedAt,
					"a workspace timestamp alone never moves the authored stamp")
			},
		},
		{
			name:     "an ungoverned term beside a governed one rides along",
			authored: []terms.Concept{authoredConcept("term:pair", "terms", "termer", model.TermPreferred)},
			pulled: []terms.Concept{{
				ID: "term:pair",
				Terms: []terms.Term{
					{Text: "terms", Locale: "en", Status: model.TermPreferred},
					{Text: "termbase", Locale: "en", Status: model.TermForbidden},
					{Text: "terminologi", Locale: "nb", Status: model.TermDeprecated},
				},
			}},
			wantWritten: true,
			wantUpdated: 1,
			check: func(t *testing.T, f *ktb.File) {
				c, ok := conceptByID(f, "term:pair")
				require.True(t, ok)
				assert.Len(t, c.Terms, 4)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _, root, recipe := newApplyAssetProject(t)
			writeAuthoredTerms(t, root, tc.authored...)

			res, err := a.ProjectReviewedConcepts(context.Background(), recipe, tc.pulled, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.wantWritten, res.Written)
			assert.Equal(t, tc.wantAdded, res.Added)
			assert.Equal(t, tc.wantUpdated, res.Updated)
			tc.check(t, readAuthoredTerms(t, root))
		})
	}
}

// TestProjectReviewedConcepts_Idempotent: a second projection over the same
// workspace snapshot leaves the file byte-identical. A projection that churned
// would put a change under `.kapi/` with no decision behind it, which is what
// the erasure gate reads as backing for a derived artifact.
func TestProjectReviewedConcepts_Idempotent(t *testing.T) {
	a, _, root, recipe := newApplyAssetProject(t)
	srcPath := writeAuthoredTerms(t, root, authoredConcept("term:memory", "memory", "minne", model.TermProposed))
	pulled := []terms.Concept{{
		ID:         "term:memory",
		Domain:     "product",
		Definition: "The built-in content memory package.",
		Terms: []terms.Term{
			{Text: "memory", Locale: "en", Status: model.TermPreferred},
			{Text: "innholdsminne", Locale: "nb", Status: model.TermPreferred},
		},
		UpdatedAt: projectionStamp.Add(24 * time.Hour),
	}}
	ctx := context.Background()

	first, err := a.ProjectReviewedConcepts(ctx, recipe, pulled, nil)
	require.NoError(t, err)
	require.True(t, first.Written)
	afterFirst, err := os.ReadFile(srcPath)
	require.NoError(t, err)

	second, err := a.ProjectReviewedConcepts(ctx, recipe, pulled, nil)
	require.NoError(t, err)
	assert.False(t, second.Written, "the second projection wrote nothing")
	assert.Equal(t, 1, second.Considered, "it still considered the decision")

	afterSecond, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, string(afterFirst), string(afterSecond), "the file is byte-identical")
}

// TestProjectReviewedConcepts_RoundTrip: what the projection wrote compiles back
// into the store unchanged — the file and the store agree after the merge.
func TestProjectReviewedConcepts_RoundTrip(t *testing.T) {
	a, _, root, recipe := newApplyAssetProject(t)
	writeAuthoredTerms(t, root, authoredConcept("term:kept", "faithful", "trofast", model.TermPreferred))
	ctx := context.Background()

	pulled := []terms.Concept{
		{ID: "term:old", Terms: []terms.Term{{Text: "termbase", Locale: "en", Status: model.TermForbidden}}},
		{ID: "term:new", Terms: []terms.Term{{Text: "terms", Locale: "en", Status: model.TermPreferred}}},
	}
	relations := []terms.ConceptRelation{
		{ID: "rel:old-new", SourceID: "term:old", TargetID: "term:new", RelationType: graph.LabelReplacedBy},
		// An ordinary edge is not a reviewed decision and does not project.
		{ID: "rel:related", SourceID: "term:old", TargetID: "term:kept", RelationType: graph.LabelRelated},
		// A governed edge with an endpoint nothing projected would not compile.
		{ID: "rel:dangling", SourceID: "term:new", TargetID: "term:absent", RelationType: graph.LabelReplacedBy},
	}

	res, err := a.ProjectReviewedConcepts(ctx, recipe, pulled, relations)
	require.NoError(t, err)
	require.True(t, res.Written)
	assert.Equal(t, 2, res.Added)
	assert.Equal(t, 1, res.Relations, "only the governed edge with both endpoints projected")

	file := readAuthoredTerms(t, root)
	require.Len(t, file.Concepts, 3)
	require.Len(t, file.Relations, 1)

	// The projection recompiled the source, so the store holds exactly what the
	// file carries — including the edge.
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	stored, err := db.Terms().Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, stored, 3)
	rels, err := db.Terms().ListRelations(ctx, nil)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	assert.Equal(t, "rel:old-new", rels[0].ID)

	// Re-exporting the store reproduces the same document: the file is the
	// store's lossless form, so the round trip is closed.
	var buf []byte
	{
		f, cerr := os.CreateTemp(t.TempDir(), "export-*.json")
		require.NoError(t, cerr)
		require.NoError(t, ExportKTB(ctx, db.Terms(), f))
		require.NoError(t, f.Close())
		buf, err = os.ReadFile(f.Name())
		require.NoError(t, err)
	}
	exported, err := ktb.Unmarshal(buf)
	require.NoError(t, err)
	assert.Len(t, exported.Concepts, 3)
	assert.Len(t, exported.Relations, 1)
}

// TestProjectReviewedConcepts_NoDecisionsTouchesNothing: a workspace with no
// reviewed terminology leaves the recipe unbound and creates no bundle.
func TestProjectReviewedConcepts_NoDecisionsTouchesNothing(t *testing.T) {
	a, _, root, recipe := newApplyAssetProject(t)

	res, err := a.ProjectReviewedConcepts(context.Background(), recipe, []terms.Concept{{
		ID:    "term:draft",
		Terms: []terms.Term{{Text: "scratchpad", Locale: "en", Status: model.TermProposed}},
	}}, nil)
	require.NoError(t, err)
	assert.False(t, res.Written)
	assert.Equal(t, 0, res.Considered)

	assert.NoFileExists(t, filepath.Join(root, project.RelStatePath(ktb.ConventionalName)))
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Empty(t, proj.Defaults.TermsSource, "no decision bound a terms source")
}
