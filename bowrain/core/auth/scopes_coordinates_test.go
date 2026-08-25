package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseScopeCoordinates(t *testing.T) {
	tests := []struct {
		name       string
		scope      string
		wantAction ScopeAction
		wantProj   string
		wantLangs  []string
		wantCoords CoordinateFilter
		wantErr    bool
	}{
		{
			name:       "action with a region",
			scope:      "review@brand=acme",
			wantAction: ScopeReview,
			wantCoords: CoordinateFilter{"brand": "acme"},
		},
		{
			name:       "languages and a region",
			scope:      "review:de@brand=acme",
			wantAction: ScopeReview,
			wantLangs:  []string{"de"},
			wantCoords: CoordinateFilter{"brand": "acme"},
		},
		{
			name:       "project, action, languages and a region",
			scope:      "project:p-7:review:de,fr@brand=acme,channel=support",
			wantAction: ScopeReview,
			wantProj:   "p-7",
			wantLangs:  []string{"de", "fr"},
			wantCoords: CoordinateFilter{"brand": "acme", "channel": "support"},
		},
		{
			// The point of regions on tokens: a pipeline pushing one brand's
			// content structurally cannot propose into another.
			name:       "a machine scope narrowed to one brand",
			scope:      "contribute@brand=acme",
			wantAction: ScopeContribute,
			wantCoords: CoordinateFilter{"brand": "acme"},
		},
		{
			name:       "wildcard with a region keeps the region",
			scope:      "*@brand=acme",
			wantAction: ScopeAll,
			wantCoords: CoordinateFilter{"brand": "acme"},
		},
		{
			name:       "no region is the whole space",
			scope:      "review:de",
			wantAction: ScopeReview,
			wantLangs:  []string{"de"},
			wantCoords: nil,
		},
		{name: "empty region", scope: "review@", wantErr: true},
		{name: "malformed region", scope: "review@brand", wantErr: true},
		{name: "region without an action", scope: "@brand=acme", wantErr: true},
		{name: "unknown action with a region", scope: "bogus@brand=acme", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScope(tt.scope)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantAction, got.Action)
			assert.Equal(t, tt.wantProj, got.ProjectID)
			assert.Equal(t, tt.wantLangs, got.Languages)
			assert.Equal(t, tt.wantCoords, got.Coordinates)
		})
	}
}

func TestParseScopesUnionsRegions(t *testing.T) {
	// A token holding two regions holds both. This deliberately differs from
	// Languages, which intersects: intersecting two disjoint regions would leave
	// a token that can act nowhere while plainly having been granted two places.
	got, err := ParseScopes(`["review@brand=acme","review@brand=other"]`)
	require.NoError(t, err)
	assert.False(t, got.Coordinates.Unconstrained())
	assert.True(t, got.Coordinates.Reaches(map[string]string{"brand": "acme"}))
	assert.True(t, got.Coordinates.Reaches(map[string]string{"brand": "other"}))
	assert.False(t, got.Coordinates.Reaches(map[string]string{"brand": "third"}))
}

func TestParseScopesUnboundedScopeOpensTheSpace(t *testing.T) {
	// A token that may act anywhere is not narrowed by also being told it may
	// act in acme.
	got, err := ParseScopes(`["review@brand=acme","translate"]`)
	require.NoError(t, err)
	assert.True(t, got.Coordinates.Unconstrained())
}

func TestParseScopesFullAccessNeedsToBeUnbounded(t *testing.T) {
	t.Run("bare wildcard is full access", func(t *testing.T) {
		got, err := ParseScopes(`["*"]`)
		require.NoError(t, err)
		assert.True(t, got.IsFullAccess)
		assert.True(t, got.Coordinates.Unconstrained())
	})

	t.Run("wildcard in a region is not full access", func(t *testing.T) {
		// IsFullAccess makes ScopeRestrictionMiddleware skip narrowing
		// altogether, so treating "*@brand=acme" as full access would drop the
		// region rather than apply it — every permission over the whole space.
		got, err := ParseScopes(`["*@brand=acme"]`)
		require.NoError(t, err)
		assert.False(t, got.IsFullAccess)
		assert.Equal(t, PermAll, got.Permissions)
		assert.True(t, got.Coordinates.Reaches(map[string]string{"brand": "acme"}))
		assert.False(t, got.Coordinates.Reaches(map[string]string{"brand": "other"}))
	})
}

func TestValidateScopesAcceptsRegions(t *testing.T) {
	require.NoError(t, ValidateScopes(`["project:p-1:review:de@brand=acme"]`))
	require.Error(t, ValidateScopes(`["review@brand"]`))
}

func TestParseScopeUnchangedWithoutRegions(t *testing.T) {
	// Every scope string issued before regions existed must parse exactly as it
	// did, with the whole space in reach.
	for _, s := range []string{"*", "read", "translate:fr,de", "contribute", "project:p-1:manage", "project:p-1:translate:fr"} {
		got, err := ParseScope(s)
		require.NoError(t, err, "scope %q", s)
		assert.True(t, got.Coordinates.Unconstrained(), "scope %q", s)
	}
}
