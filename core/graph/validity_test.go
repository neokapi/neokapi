package graph

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidityMatches(t *testing.T) {
	ref := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	before := ref.Add(-24 * time.Hour)
	after := ref.Add(24 * time.Hour)

	tests := []struct {
		name     string
		validity *Validity
		scope    Scope
		want     bool
	}{
		{
			name:     "nil validity always matches",
			validity: nil,
			scope:    ScopeAt(ref),
			want:     true,
		},
		{
			name:     "empty validity matches",
			validity: &Validity{},
			scope:    ScopeAt(ref),
			want:     true,
		},
		{
			name:     "valid_from before scope time",
			validity: &Validity{ValidFrom: &before},
			scope:    ScopeAt(ref),
			want:     true,
		},
		{
			name:     "valid_from equals scope time",
			validity: &Validity{ValidFrom: &ref},
			scope:    ScopeAt(ref),
			want:     true,
		},
		{
			name:     "valid_from after scope time",
			validity: &Validity{ValidFrom: &after},
			scope:    ScopeAt(ref),
			want:     false,
		},
		{
			name:     "valid_to after scope time",
			validity: &Validity{ValidTo: &after},
			scope:    ScopeAt(ref),
			want:     true,
		},
		{
			name:     "valid_to equals scope time (exclusive)",
			validity: &Validity{ValidTo: &ref},
			scope:    ScopeAt(ref),
			want:     false,
		},
		{
			name:     "valid_to before scope time",
			validity: &Validity{ValidTo: &before},
			scope:    ScopeAt(ref),
			want:     false,
		},
		{
			name:     "within time range",
			validity: &Validity{ValidFrom: &before, ValidTo: &after},
			scope:    ScopeAt(ref),
			want:     true,
		},
		{
			name:     "tags match",
			validity: &Validity{Tags: map[string]string{"market": "us", "channel": "blog"}},
			scope:    Scope{At: ref, Tags: map[string]string{"market": "us"}},
			want:     true,
		},
		{
			name:     "tags mismatch value",
			validity: &Validity{Tags: map[string]string{"market": "us"}},
			scope:    Scope{At: ref, Tags: map[string]string{"market": "eu"}},
			want:     false,
		},
		{
			name:     "tags missing key in validity",
			validity: &Validity{Tags: map[string]string{"market": "us"}},
			scope:    Scope{At: ref, Tags: map[string]string{"channel": "blog"}},
			want:     false,
		},
		{
			name:     "scope has no tags matches validity with tags",
			validity: &Validity{Tags: map[string]string{"market": "us"}},
			scope:    ScopeAt(ref),
			want:     true,
		},
		{
			name: "time and tags combined match",
			validity: &Validity{
				ValidFrom: &before,
				ValidTo:   &after,
				Tags:      map[string]string{"market": "us"},
			},
			scope: Scope{At: ref, Tags: map[string]string{"market": "us"}},
			want:  true,
		},
		{
			name: "time matches but tags fail",
			validity: &Validity{
				ValidFrom: &before,
				ValidTo:   &after,
				Tags:      map[string]string{"market": "us"},
			},
			scope: Scope{At: ref, Tags: map[string]string{"market": "eu"}},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.validity.Matches(tt.scope))
		})
	}
}

func TestValidityIsExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	tests := []struct {
		name     string
		validity *Validity
		want     bool
	}{
		{"nil validity", nil, false},
		{"no valid_to", &Validity{}, false},
		{"valid_to in past", &Validity{ValidTo: &past}, true},
		{"valid_to in future", &Validity{ValidTo: &future}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.validity.IsExpired())
		})
	}
}

func TestValidityIsActive(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	farPast := time.Now().Add(-2 * time.Hour)

	tests := []struct {
		name     string
		validity *Validity
		want     bool
	}{
		{"nil validity", nil, true},
		{"currently valid", &Validity{ValidFrom: &past, ValidTo: &future}, true},
		{"expired", &Validity{ValidFrom: &farPast, ValidTo: &past}, false},
		{"not yet valid", &Validity{ValidFrom: &future}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.validity.IsActive())
		})
	}
}

func TestScopeConstructors(t *testing.T) {
	t.Run("Now", func(t *testing.T) {
		s := Now()
		assert.WithinDuration(t, time.Now(), s.At, time.Second)
		assert.Nil(t, s.Tags)
	})

	t.Run("ScopeAt", func(t *testing.T) {
		ref := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		s := ScopeAt(ref)
		assert.True(t, ref.Equal(s.At))
		assert.Nil(t, s.Tags)
	})

	t.Run("ScopeWithTags", func(t *testing.T) {
		tags := map[string]string{"market": "us"}
		s := ScopeWithTags(tags)
		assert.WithinDuration(t, time.Now(), s.At, time.Second)
		assert.Equal(t, tags, s.Tags)
	})
}
