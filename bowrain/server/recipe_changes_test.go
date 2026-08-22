package server

import (
	"encoding/json"
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// channelRefServer is a server holding one project whose collections sit at
// three different kinds of point, which is what composing a `channel:` has to
// read.
func channelRefServer(t *testing.T) (*Server, string) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "refs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	ctx := t.Context()
	proj := &platstore.Project{
		Name:                  "refs",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"nb"},
		WorkspaceID:           "ws-1",
	}
	require.NoError(t, cs.CreateProject(ctx, proj))

	for _, col := range []*platstore.Collection{
		// Fully placed: both structural axes already resolved.
		{ID: "c-docs", Name: "docs", Context: map[string]string{
			project.ProductAxis: "cloud", project.ChannelAxis: "guides",
		}},
		// A product with no surface named.
		{ID: "c-web", Name: "web", Context: map[string]string{project.ProductAxis: "cloud"}},
		// The default point: nowhere in particular yet.
		{ID: "c-loose", Name: "loose"},
	} {
		col.ProjectID = proj.ID
		col.Kind = platstore.CollectionUploaded
		col.Stream = "main"
		col.Owner = venue.ContextOwnerRecipe
		require.NoError(t, cs.CreateCollection(ctx, col))
	}

	return &Server{ContentStore: cs}, proj.ID
}

// TestChannelRefKeepsTheHalfNotBeingApproved: both halves of `channel:` are
// needed and only one is approved, so the other comes from where the collection
// sits today — the point the server holds because a push reconciled it.
func TestChannelRefKeepsTheHalfNotBeingApproved(t *testing.T) {
	s, projectID := channelRefServer(t)

	got, err := s.channelRefFor(t.Context(), projectID, "docs", project.ProductAxis, "desktop")
	require.NoError(t, err)
	assert.Equal(t, "desktop/guides", got, "the surface it ships on is unchanged")

	got, err = s.channelRefFor(t.Context(), projectID, "docs", project.ChannelAxis, "reference")
	require.NoError(t, err)
	assert.Equal(t, "cloud/reference", got, "the product it belongs to is unchanged")
}

// A product with no channel is a coherent point, but `channel:` has no spelling
// for it, so the collection's own name is the surface.
func TestChannelRefNamesTheSurfaceAfterTheCollection(t *testing.T) {
	s, projectID := channelRefServer(t)

	got, err := s.channelRefFor(t.Context(), projectID, "web", project.ProductAxis, "cloud")
	require.NoError(t, err)
	assert.Equal(t, "cloud/web", got)
}

// A channel is a surface OF a product, so approving one for a collection that
// sits at no product is refused rather than guessed: inventing a product would
// put the collection somewhere nobody chose.
func TestChannelRefRefusesAChannelWithNoProduct(t *testing.T) {
	s, projectID := channelRefServer(t)

	_, err := s.channelRefFor(t.Context(), projectID, "loose", project.ChannelAxis, "docs")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approve the product for it first")
}

// A collection the venue does not hold is refused here rather than on someone's
// laptop days later, and the error lists the ones it does.
func TestChannelRefRefusesAnUnknownCollection(t *testing.T) {
	s, projectID := channelRefServer(t)

	_, err := s.channelRefFor(t.Context(), projectID, "nope", project.ProductAxis, "cloud")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no collection named "nope"`)
	assert.Contains(t, err.Error(), "docs")
}

// The composed value is one `project.SetField` accepts, which is the whole
// point of composing it here: the two ends of the round trip agree on what a
// recipe may say.
func TestComposedChannelRefIsSettable(t *testing.T) {
	s, projectID := channelRefServer(t)

	ref, err := s.channelRefFor(t.Context(), projectID, "docs", project.ProductAxis, "desktop")
	require.NoError(t, err)

	proj := &project.KapiProject{
		Version:     "v1",
		Name:        "refs",
		Collections: []project.Collection{{Name: "docs", Path: "docs/**/*.md"}},
	}
	changed, err := project.SetField(proj, "collections.docs.channel", mustJSON(t, ref))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, "desktop/guides", proj.Collections[0].Channel)

	resolved, err := proj.ResolveChannel(proj.Collections[0].Channel)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		project.ProductAxis: "desktop", project.ChannelAxis: "guides",
	}, resolved.Coordinates(), "and the recipe derives the axes that were approved")
}

// mustJSON encodes a recipe value the way a pending change carries it.
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}
