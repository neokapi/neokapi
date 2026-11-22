package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlatformAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		env      map[string]string
		want     string
	}{
		{
			name:     "provider variable wins over the generic one",
			provider: "openai",
			env:      map[string]string{"OPENAI_API_KEY": "sk-openai", "BOWRAIN_PLATFORM_API_KEY": "generic"},
			want:     "sk-openai",
		},
		{
			name:     "generic variable when the provider variable is unset",
			provider: "openai",
			env:      map[string]string{"BOWRAIN_PLATFORM_API_KEY": "generic"},
			want:     "generic",
		},
		{
			name:     "placeholder generic key counts as unset",
			provider: "openai",
			env:      map[string]string{"OPENAI_API_KEY": "sk-openai", "BOWRAIN_PLATFORM_API_KEY": "CHANGEME"},
			want:     "sk-openai",
		},
		{
			name:     "placeholder alone resolves to no key",
			provider: "openai",
			env:      map[string]string{"BOWRAIN_PLATFORM_API_KEY": "CHANGEME"},
			want:     "",
		},
		{
			name:     "placeholder in the provider variable falls through",
			provider: "openai",
			env:      map[string]string{"OPENAI_API_KEY": "CHANGEME", "BOWRAIN_PLATFORM_API_KEY": "generic"},
			want:     "generic",
		},
		{
			name:     "whitespace-only values count as unset",
			provider: "anthropic",
			env:      map[string]string{"ANTHROPIC_API_KEY": "  ", "BOWRAIN_PLATFORM_API_KEY": "generic"},
			want:     "generic",
		},
		{
			name:     "gemini falls back to GOOGLE_API_KEY",
			provider: "gemini",
			env:      map[string]string{"GOOGLE_API_KEY": "g-key"},
			want:     "g-key",
		},
		{
			name:     "provider id is case-insensitive",
			provider: "OpenAI",
			env:      map[string]string{"OPENAI_API_KEY": "sk-openai"},
			want:     "sk-openai",
		},
		{
			name:     "keyless provider with the placeholder gets no key",
			provider: "bedrock",
			env:      map[string]string{"BOWRAIN_PLATFORM_API_KEY": "CHANGEME", "OPENAI_API_KEY": "sk-openai"},
			want:     "",
		},
		{
			name:     "another provider's variable is never used",
			provider: "anthropic",
			env:      map[string]string{"OPENAI_API_KEY": "sk-openai"},
			want:     "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := platformAPIKey(tc.provider, func(k string) string { return tc.env[k] })
			assert.Equal(t, tc.want, got)
		})
	}
}
