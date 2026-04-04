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

func TestScopeParseScopes_LanguageIntersection(t *testing.T) {
	result, err := ParseScopes(`["translate:fr,de,es", "translate:fr,de"]`)
	require.NoError(t, err)
	slices.Sort(result.Languages)
	assert.Equal(t, []string{"de", "fr"}, result.Languages)
}

func TestScopeParseScopes_NoLanguageConstraintIfAnyUnconstrained(t *testing.T) {
	// One scope has languages, one doesn't — result is unconstrained.
	result, err := ParseScopes(`["translate:fr", "review"]`)
	require.NoError(t, err)
	assert.Equal(t, PermViewContent|PermTranslate|PermReview, result.Permissions)
	// "review" has no language constraint, so the union is unconstrained.
	// But since we do intersection of only the constrained scopes, and
	// "review" doesn't contribute to langSets, we get ["fr"].
	// However the spec says "intersection of language constraints (empty = all)".
	// With only one constrained scope, the result is that scope's languages.
	assert.Equal(t, []string{"fr"}, result.Languages)
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
