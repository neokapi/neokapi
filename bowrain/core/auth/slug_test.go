package auth

import "testing"

func TestSuggestSlug(t *testing.T) {
	tests := []struct {
		email string
		want  string
	}{
		{"asgeirf@gmail.com", "asgeirf"},
		{"asgeir.f@gmail.com", "asgeir-f"},
		{"a.b.c@example.com", "a-b-c"},
		{"first_last@example.com", "first-last"},
		{"name+tag@example.com", "name-tag"},
		{"-leading-hyphen@example.com", "leading-hyphen"},
		{"trailing-hyphen-@example.com", "trailing-hyphen"},
		{"multiple..dots@example.com", "multiple-dots"},
		{"plus+tag.dotted@example.com", "plus-tag-dotted"},
		{"NoLowerCase@example.com", "nolowercase"},
		{"", ""},
		{"@example.com", ""},
	}
	for _, tt := range tests {
		got := SuggestSlug(tt.email)
		if got != tt.want {
			t.Errorf("SuggestSlug(%q) = %q, want %q", tt.email, got, tt.want)
		}
	}
}

func TestValidateWorkspaceSlug(t *testing.T) {
	for _, slug := range []string{"asgeirf", "my-team", "ab", "my-team-2"} {
		if err := ValidateWorkspaceSlug(slug); err != nil {
			t.Errorf("ValidateWorkspaceSlug(%q) unexpected error: %v", slug, err)
		}
	}
	bad := []string{"a", "-leading", "trailing-", "double--hyphen", "UPPER", "with space", "auth", "admin"}
	for _, slug := range bad {
		if err := ValidateWorkspaceSlug(slug); err == nil {
			t.Errorf("ValidateWorkspaceSlug(%q) expected error, got nil", slug)
		}
	}
}
