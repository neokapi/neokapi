package aiprovider

import "testing"

func TestProviderForModel(t *testing.T) {
	cases := []struct {
		model    string
		want     ProviderID
		wantOK   bool
		scenario string
	}{
		{"claude-sonnet-4-20250514", Anthropic, true, "anthropic by claude prefix"},
		{"claude-3-5-haiku", Anthropic, true, "anthropic short name"},
		{"gpt-4o", OpenAI, true, "openai by gpt prefix"},
		{"o3-mini", OpenAI, true, "openai o3 reasoning model"},
		{"gemini-3-flash-preview", Gemini, true, "gemini by prefix"},
		{"gemma3:4b", Ollama, true, "ollama tag (gemma is local, not gemini)"},
		{"llama3.2:3b", Ollama, true, "ollama default tag"},
		{"aya-expanse:8b", Ollama, true, "ollama recommended tag"},
		{"GPT-4O", OpenAI, true, "case-insensitive"},
		{"", "", false, "empty"},
		{"some-unknown-model", "", false, "no confident inference"},
	}
	for _, c := range cases {
		t.Run(c.scenario, func(t *testing.T) {
			got, ok := ProviderForModel(c.model)
			if ok != c.wantOK || got != c.want {
				t.Fatalf("ProviderForModel(%q) = (%q, %v), want (%q, %v)", c.model, got, ok, c.want, c.wantOK)
			}
		})
	}
}
