package auth

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeParseScope_Actions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  ScopeAction
		perms Permission
	}{
		{"wildcard", "*", ScopeAll, PermAll},
		{"read", "read", ScopeRead, PermViewContent},
		{"translate", "translate", ScopeTranslate, PermViewContent | PermTranslate},
		{"review", "review", ScopeReview, PermViewContent | PermTranslate | PermReview},
		{"manage", "manage", ScopeManage, PermAll &^ (PermManageProject | PermManageMembers)},
		{"admin", "admin", ScopeAdmin, PermAll},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScope(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Action)
			assert.Equal(t, tt.perms, got.Permissions)
			assert.Empty(t, got.Languages)
			assert.Empty(t, got.ProjectID)
		})
	}
}

func TestScopeParseScope_Languages(t *testing.T) {
	got, err := ParseScope("translate:fr,de")
	require.NoError(t, err)
	assert.Equal(t, ScopeTranslate, got.Action)
	assert.Equal(t, PermViewContent|PermTranslate, got.Permissions)
	slices.Sort(got.Languages)
	assert.Equal(t, []string{"de", "fr"}, got.Languages)
	assert.Empty(t, got.ProjectID)
}

func TestScopeParseScope_ProjectScoped(t *testing.T) {
	got, err := ParseScope("project:proj-123:translate")
	require.NoError(t, err)
	assert.Equal(t, ScopeTranslate, got.Action)
	assert.Equal(t, PermViewContent|PermTranslate, got.Permissions)
	assert.Equal(t, "proj-123", got.ProjectID)
	assert.Empty(t, got.Languages)
}

func TestScopeParseScope_ProjectScopedWithLanguages(t *testing.T) {
	got, err := ParseScope("project:proj-123:translate:fr,de")
	require.NoError(t, err)
	assert.Equal(t, ScopeTranslate, got.Action)
	assert.Equal(t, PermViewContent|PermTranslate, got.Permissions)
	assert.Equal(t, "proj-123", got.ProjectID)
	slices.Sort(got.Languages)
	assert.Equal(t, []string{"de", "fr"}, got.Languages)
}

func TestScopeParseScope_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"unknown action", "delete"},
		{"too many segments", "read:fr:extra"},
		{"project missing id", "project::translate"},
		{"project missing action", "project:id"},
		{"project too many segments", "project:id:translate:fr:extra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseScope(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestScopeParseScopes_Wildcard(t *testing.T) {
	result, err := ParseScopes(`["*"]`)
	require.NoError(t, err)
	assert.True(t, result.IsFullAccess)
	assert.Equal(t, PermAll, result.Permissions)
	assert.Empty(t, result.Languages)
	assert.Empty(t, result.ProjectIDs)
}

func TestScopeParseScopes_Union(t *testing.T) {
	result, err := ParseScopes(`["read", "translate"]`)
	require.NoError(t, err)
	assert.False(t, result.IsFullAccess)
	assert.Equal(t, PermViewContent|PermTranslate, result.Permissions)
}

func TestScopeParseScopes_LanguagesUnion(t *testing.T) {
	// Each scope is a grant, and grants add. This list plainly names es, so the
	// token holds it; intersecting used to drop it.
	result, err := ParseScopes(`["translate:fr,de,es", "translate:fr,de"]`)
	require.NoError(t, err)
	assert.Equal(t, []string{"de", "es", "fr"}, result.Languages, "sorted, so logs and callers are stable")
}

func TestScopeParseScopes_DisjointLanguagesNoLongerOpenEverything(t *testing.T) {
	// The bug this fixes: intersecting two disjoint scopes gave the empty set,
	// and empty means ALL — so a token granted exactly two locales came out able
	// to act in every one.
	result, err := ParseScopes(`["translate:fr", "review:de"]`)
	require.NoError(t, err)
	assert.Equal(t, []string{"de", "fr"}, result.Languages)
	assert.NotEmpty(t, result.Languages, "a token granted two locales must not resolve to all of them")
}

func TestScopeParseScopes_SilenceDoesNotWiden(t *testing.T) {
	// "translate:fr" plus "review" cannot be expressed exactly by one flattened
	// language list, so one of the two acts is misrepresented either way.
	// Under-granting review is the safe direction on an authorization surface,
	// and it is also what this returned before the union landed.
	result, err := ParseScopes(`["translate:fr", "review"]`)
	require.NoError(t, err)
	assert.Equal(t, PermViewContent|PermTranslate|PermReview, result.Permissions)
	assert.Equal(t, []string{"fr"}, result.Languages)
}

func TestScopeParseScopes_UnconstrainedOnlyWhenNobodyNamedOne(t *testing.T) {
	result, err := ParseScopes(`["translate", "review"]`)
	require.NoError(t, err)
	assert.Nil(t, result.Languages, "no scope named a language, so every language is in scope")
}

func TestScopeParseScopes_ProjectIDs(t *testing.T) {
	result, err := ParseScopes(`["project:proj-1:read", "project:proj-2:translate"]`)
	require.NoError(t, err)
	slices.Sort(result.ProjectIDs)
	assert.Equal(t, []string{"proj-1", "proj-2"}, result.ProjectIDs)
	assert.Equal(t, PermViewContent|PermTranslate, result.Permissions)
}

func TestScopeParseScopes_InvalidJSON(t *testing.T) {
	_, err := ParseScopes(`not json`)
	assert.Error(t, err)
}

func TestScopeParseScopes_EmptyArray(t *testing.T) {
	_, err := ParseScopes(`[]`)
	assert.Error(t, err)
}

func TestScopeParseScopes_InvalidScope(t *testing.T) {
	_, err := ParseScopes(`["delete"]`)
	assert.Error(t, err)
}

func TestScopeParseScopes_BackwardCompatDefault(t *testing.T) {
	// Default scopes column value for unrestricted tokens.
	result, err := ParseScopes(`["*"]`)
	require.NoError(t, err)
	assert.True(t, result.IsFullAccess)
	assert.Equal(t, PermAll, result.Permissions)
}

func TestValidateScopes_Valid(t *testing.T) {
	tests := []string{
		`["*"]`,
		`["read"]`,
		`["translate:fr,de"]`,
		`["read", "translate:fr"]`,
		`["project:proj-1:translate:fr,de"]`,
	}
	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			assert.NoError(t, ValidateScopes(tt))
		})
	}
}

func TestValidateScopes_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"bad json", `not json`},
		{"empty array", `[]`},
		{"unknown action", `["delete"]`},
		{"bad scope format", `["read:fr:extra"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, ValidateScopes(tt.input))
		})
	}
}

func TestScopeManageExcludesProjectAndMembers(t *testing.T) {
	got, err := ParseScope("manage")
	require.NoError(t, err)
	assert.True(t, got.Permissions.Has(PermViewContent))
	assert.True(t, got.Permissions.Has(PermTranslate))
	assert.True(t, got.Permissions.Has(PermReview))
	assert.True(t, got.Permissions.Has(PermRunFlows))
	assert.True(t, got.Permissions.Has(PermManageFiles))
	assert.True(t, got.Permissions.Has(PermManageConnectors))
	assert.False(t, got.Permissions.Has(PermManageProject))
	assert.False(t, got.Permissions.Has(PermManageMembers))
}
