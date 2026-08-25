package server

import (
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// custodyFixture stands up a workspace with one project and the four member
// shapes that matter: a bounded custodian, a second bounded custodian
// elsewhere, a bounded reviewer (custody of nothing — reviewing is volume
// work), and blanket authority.
func custodyFixture(t *testing.T) (srv *Server, wsID string, acme, other, reviewer, blanket string) {
	t.Helper()
	srv, _, wsID = newWorkflowTestServer(t)
	projID := createWorkflowProject(t, srv, wsID, map[string]string{})

	acme = addProjectMemberAt(t, srv, wsID, projID, "project-admin", nil,
		platauth.CoordinateFilter{"brand": "acme"})
	other = addProjectMemberAt(t, srv, wsID, projID, "project-admin", nil,
		platauth.CoordinateFilter{"brand": "other"})
	reviewer = addProjectMemberAt(t, srv, wsID, projID, "reviewer", nil,
		platauth.CoordinateFilter{"brand": "acme"})
	blanket = addProjectMember(t, srv, wsID, projID, "project-admin", nil)
	return srv, wsID, acme, other, reviewer, blanket
}

func custodyIDs(list []ContextProfileCustodian) []string {
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.UserID)
	}
	return out
}

func TestAttachCustody(t *testing.T) {
	srv, wsID, acme, other, reviewer, blanket := custodyFixture(t)
	ctx := t.Context()

	projects, err := srv.Services.Project.ListProjects(ctx)
	require.NoError(t, err)

	profiles := []ContextProfile{
		{Slug: defaultProfileSlug, IsDefault: true},
		{Slug: "brand~acme", Declared: true, Coordinates: map[string]string{"brand": "acme"}},
		{Slug: "brand~acme.channel~support", Declared: true,
			Coordinates: map[string]string{"brand": "acme", "channel": "support"}},
		{Slug: "brand~third", Declared: true, Coordinates: map[string]string{"brand": "third"}},
		{Slug: "voice~v-unbound"}, // a voice bound to no point
	}
	srv.attachCustody(ctx, wsID, projects, profiles)

	t.Run("a point with a custodian is covered", func(t *testing.T) {
		c := profiles[1].Custody
		require.NotNil(t, c)
		assert.True(t, c.Covered)
		assert.Equal(t, []string{acme}, custodyIDs(c.Custodians))
	})

	t.Run("custody reaches every point inside the region", func(t *testing.T) {
		c := profiles[2].Custody
		require.NotNil(t, c)
		assert.True(t, c.Covered)
		assert.Equal(t, []string{acme}, custodyIDs(c.Custodians),
			"brand=acme reaches acme/support, and brand=other does not")
	})

	t.Run("a point nobody holds is uncovered", func(t *testing.T) {
		c := profiles[3].Custody
		require.NotNil(t, c)
		assert.False(t, c.Covered)
		assert.Empty(t, c.Custodians)
		assert.NotEmpty(t, c.Fallback, "an uncovered point still reads as falling back to someone")
	})

	t.Run("the default point is a real place that can be uncovered", func(t *testing.T) {
		// Content that declared no coordinates sits here, and a custodian of
		// acme does not hold it.
		c := profiles[0].Custody
		require.NotNil(t, c)
		assert.False(t, c.Covered)
		assert.NotContains(t, custodyIDs(c.Custodians), acme)
		assert.NotContains(t, custodyIDs(c.Custodians), other)
	})

	t.Run("blanket authority is fallback, never coverage", func(t *testing.T) {
		// The question the report answers is which content has nobody who knows
		// it. Someone who can approve everything is the fallback.
		for _, i := range []int{0, 1, 3} {
			assert.Contains(t, custodyIDs(profiles[i].Custody.Fallback), blanket)
			assert.NotContains(t, custodyIDs(profiles[i].Custody.Custodians), blanket)
		}
	})

	t.Run("a bounded reviewer is not custody", func(t *testing.T) {
		// Reviewing is volume work; authoring a rule is not.
		c := profiles[1].Custody
		assert.NotContains(t, custodyIDs(c.Custodians), reviewer)
		assert.NotContains(t, custodyIDs(c.Fallback), reviewer)
	})

	t.Run("a voice bound to no point has no custody", func(t *testing.T) {
		assert.Nil(t, profiles[4].Custody, "an unbound voice is not a place and cannot be uncovered")
	})
}

func TestAttachCustodyIgnoresOtherWorkspaces(t *testing.T) {
	srv, wsID, acme, _, _, _ := custodyFixture(t)
	ctx := t.Context()

	projects, err := srv.Services.Project.ListProjects(ctx)
	require.NoError(t, err)

	profiles := []ContextProfile{
		{Slug: "brand~acme", Declared: true, Coordinates: map[string]string{"brand": "acme"}},
	}
	srv.attachCustody(ctx, "some-other-workspace", projects, profiles)

	require.NotNil(t, profiles[0].Custody)
	assert.False(t, profiles[0].Custody.Covered)
	assert.NotContains(t, custodyIDs(profiles[0].Custody.Custodians), acme)

	// And the workspace that does own it still reads as covered.
	profiles[0].Custody = nil
	srv.attachCustody(ctx, wsID, projects, profiles)
	require.NotNil(t, profiles[0].Custody)
	assert.True(t, profiles[0].Custody.Covered)
}
