package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionHas(t *testing.T) {
	t.Run("single bit", func(t *testing.T) {
		p := PermViewContent | PermTranslate
		assert.True(t, p.Has(PermViewContent))
		assert.True(t, p.Has(PermTranslate))
		assert.False(t, p.Has(PermReview))
	})

	t.Run("multiple bits", func(t *testing.T) {
		p := PermViewContent | PermTranslate | PermReview
		assert.True(t, p.Has(PermTranslate|PermReview))
		assert.False(t, p.Has(PermTranslate|PermManageTM))
	})

	t.Run("PermAll has every permission", func(t *testing.T) {
		assert.True(t, PermAll.Has(PermViewContent))
		assert.True(t, PermAll.Has(PermManageAssets))
		assert.True(t, PermAll.Has(PermViewContent|PermManageProject|PermRunFlows))
	})

	t.Run("zero has nothing", func(t *testing.T) {
		var p Permission
		assert.True(t, p.Has(0), "zero requires zero")
		assert.False(t, p.Has(PermViewContent))
	})
}

func TestPermissionLanguageScoped(t *testing.T) {
	scoped := []Permission{PermTranslate, PermReview}
	for _, p := range scoped {
		assert.True(t, p.LanguageScoped(), "%s should be language-scoped", p)
	}

	notScoped := []Permission{
		PermViewContent, PermEditSource, PermManageTerms, PermManageTM,
		PermRunFlows, PermManageFiles, PermManageStreams, PermManageConnectors,
		PermManageAutomation, PermManageMembers, PermManageProject,
		PermManageBrand, PermManageAssets, PermAuditRead, PermRollbackChanges,
	}
	for _, p := range notScoped {
		assert.False(t, p.LanguageScoped(), "%s should not be language-scoped", p)
	}
}

func TestPermissionStrings(t *testing.T) {
	tests := []struct {
		name string
		perm Permission
		want []string
	}{
		{"zero", 0, nil},
		{"single", PermRunFlows, []string{"run_flows"}},
		{"two bits", PermViewContent | PermTranslate, []string{"view_content", "translate"}},
		{"all", PermAll, []string{
			"view_content", "edit_source", "translate", "review",
			"manage_terms", "manage_tm", "run_flows", "manage_files",
			"manage_streams", "manage_connectors", "manage_automation",
			"manage_members", "manage_project", "manage_brand", "manage_assets",
			"audit_read", "rollback_changes",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.perm.Strings())
		})
	}
}

func TestPermissionString(t *testing.T) {
	tests := []struct {
		name string
		perm Permission
		want string
	}{
		{"zero", 0, ""},
		{"single", PermReview, "review"},
		{"multiple", PermEditSource | PermManageMembers, "edit_source,manage_members"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.perm.String())
		})
	}
}

func TestParsePermission(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Permission
	}{
		{"view_content", "view_content", PermViewContent},
		{"translate", "translate", PermTranslate},
		{"manage_assets", "manage_assets", PermManageAssets},
		{"unknown", "fly", 0},
		{"empty", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ParsePermission(tt.input))
		})
	}
}

func TestParsePermissions(t *testing.T) {
	t.Run("multiple names", func(t *testing.T) {
		got := ParsePermissions([]string{"translate", "review", "manage_tm"})
		want := PermTranslate | PermReview | PermManageTM
		assert.Equal(t, want, got)
	})

	t.Run("unknown names ignored", func(t *testing.T) {
		got := ParsePermissions([]string{"translate", "bogus"})
		assert.Equal(t, PermTranslate, got)
	})

	t.Run("empty slice", func(t *testing.T) {
		assert.Equal(t, Permission(0), ParsePermissions(nil))
	})
}

func TestPermAll(t *testing.T) {
	all := []Permission{
		PermViewContent, PermEditSource, PermTranslate, PermReview,
		PermManageTerms, PermManageTM, PermRunFlows, PermManageFiles,
		PermManageStreams, PermManageConnectors, PermManageAutomation,
		PermManageMembers, PermManageProject, PermManageBrand, PermManageAssets,
		PermAuditRead, PermRollbackChanges,
	}
	require.Len(t, all, 17, "expected 17 individual permissions")
	for _, p := range all {
		assert.True(t, PermAll.Has(p), "PermAll should include %s", p)
	}

	// PermAll should equal the union of all individual permissions.
	var combined Permission
	for _, p := range all {
		combined |= p
	}
	assert.Equal(t, PermAll, combined)
}
