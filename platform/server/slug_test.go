package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		slug    string
		wantErr bool
	}{
		{"acme", false},
		{"my-app", false},
		{"a1", false},
		{"my-cool-project-2024", false},
		{"ab", false}, // min length

		// Invalid: too short
		{"a", true},
		{"", true},

		// Invalid: uppercase
		{"Acme", true},
		{"MY-APP", true},

		// Invalid: consecutive hyphens
		{"my--app", true},

		// Invalid: leading/trailing hyphen
		{"-acme", true},
		{"acme-", true},
		{"-", true},

		// Invalid: special characters
		{"my_app", true},
		{"my.app", true},
		{"my app", true},
		{"my/app", true},

		// Invalid: too long (65 chars)
		{"a2345678901234567890123456789012345678901234567890123456789012345", true},

		// Valid: 64 chars (max)
		{"a234567890123456789012345678901234567890123456789012345678901234", false},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			err := ValidateSlug(tt.slug)
			if tt.wantErr {
				assert.Error(t, err, "slug %q should be invalid", tt.slug)
			} else {
				assert.NoError(t, err, "slug %q should be valid", tt.slug)
			}
		})
	}
}

func TestValidateWorkspaceSlug(t *testing.T) {
	// Valid workspace slug.
	assert.NoError(t, ValidateWorkspaceSlug("acme"))
	assert.NoError(t, ValidateWorkspaceSlug("my-team"))

	// Reserved names.
	for _, reserved := range []string{"auth", "admin", "health", "workspaces", "projects"} {
		err := ValidateWorkspaceSlug(reserved)
		assert.Error(t, err, "workspace slug %q should be reserved", reserved)
		assert.Contains(t, err.Error(), "reserved")
	}
}

func TestValidateProjectSlug(t *testing.T) {
	// Valid project slug.
	assert.NoError(t, ValidateProjectSlug("my-app"))
	assert.NoError(t, ValidateProjectSlug("frontend"))

	// Reserved names.
	for _, reserved := range []string{"blocks", "items", "sync", "streams", "tags", "members", "settings"} {
		err := ValidateProjectSlug(reserved)
		assert.Error(t, err, "project slug %q should be reserved", reserved)
		assert.Contains(t, err.Error(), "reserved")
	}
}
