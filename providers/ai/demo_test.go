package aiprovider

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDemo() *DemoProvider {
	SetDemoNoticeWriter(io.Discard)
	return NewDemoProvider(Config{})
}

func TestDemoProvider_Registered(t *testing.T) {
	p, err := NewProvider(Demo, Config{})
	require.NoError(t, err)
	assert.Equal(t, Demo, p.Name())

	names := ProviderNames()
	assert.Contains(t, names, string(Demo))
}

func TestDemoProvider_TranslateLexicon(t *testing.T) {
	p := newTestDemo()
	cases := []struct {
		target model.LocaleID
		source string
		want   string
	}{
		{"fr", "Save", "⟦fr⟧ Enregistrer"},
		{"fr", "Hello world", "⟦fr⟧ Bonjour worldé"},
		{"es-ES", "Welcome", "⟦es⟧ Bienvenido"},
		{"de-DE", "Settings", "⟦de⟧ Einstellungen"},
		{"ja", "Save", "⟦ja⟧ Save~"},
	}
	for _, tc := range cases {
		resp, err := p.Translate(context.Background(), TranslateRequest{
			Source:       tc.source,
			TargetLocale: tc.target,
		})
		require.NoError(t, err)
		assert.Equal(t, tc.want, resp.Translation)
		assert.Equal(t, DemoModelName, resp.Model)
	}
}

func TestDemoProvider_Deterministic(t *testing.T) {
	p := newTestDemo()
	req := TranslateRequest{Source: "Hello, save the file or cancel.", TargetLocale: "fr"}
	first, err := p.Translate(context.Background(), req)
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		got, err := p.Translate(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, first.Translation, got.Translation)
	}
}

func TestDemoProvider_PreservesMarkup(t *testing.T) {
	p := newTestDemo()
	resp, err := p.Translate(context.Background(), TranslateRequest{
		Source:       "Hello <b>world</b>!",
		TargetLocale: "fr",
	})
	require.NoError(t, err)
	// Tags and punctuation must survive verbatim.
	assert.Contains(t, resp.Translation, "<b>")
	assert.Contains(t, resp.Translation, "</b>")
	assert.Contains(t, resp.Translation, "!")
}

func TestDemoProvider_BatchTranslations(t *testing.T) {
	p := newTestDemo()
	prompt := "Translate each numbered segment from en to fr.\n\n[1] Hello\n[2] Save\n[3] world\n"
	resp, err := p.ChatStructured(context.Background(), []Message{{Role: "user", Content: prompt}}, JSONSchema{Name: "batch_translations"})
	require.NoError(t, err)

	var out struct {
		Translations []struct {
			Index int    `json:"index"`
			Text  string `json:"text"`
		} `json:"translations"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &out))
	require.Len(t, out.Translations, 3)
	assert.Equal(t, 1, out.Translations[0].Index)
	assert.Equal(t, "⟦fr⟧ Bonjour", out.Translations[0].Text)
	assert.Equal(t, "⟦fr⟧ Enregistrer", out.Translations[1].Text)
}

func TestDemoProvider_NeutralSchema(t *testing.T) {
	// QA-style schema must yield an empty issues array, not fabricated findings.
	p := newTestDemo()
	qa := JSONSchema{
		Name: "qa_check",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"issues": map[string]any{"type": "array"},
			},
		},
	}
	resp, err := p.ChatStructured(context.Background(), []Message{{Role: "user", Content: "check this"}}, qa)
	require.NoError(t, err)

	var out struct {
		Issues []any `json:"issues"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &out))
	assert.Empty(t, out.Issues)
}

func TestDemoProvider_NoticeOnce(t *testing.T) {
	// Reset the once guard for an isolated assertion.
	demoNoticeOnce = sync.Once{}
	var buf strings.Builder
	SetDemoNoticeWriter(&buf)
	defer SetDemoNoticeWriter(io.Discard)

	p := NewDemoProvider(Config{})
	_, _ = p.Translate(context.Background(), TranslateRequest{Source: "Hi", TargetLocale: "fr"})
	_, _ = p.Translate(context.Background(), TranslateRequest{Source: "Bye", TargetLocale: "fr"})

	assert.Equal(t, 1, strings.Count(buf.String(), DemoNotice))
	assert.Contains(t, buf.String(), "not a real language model")
}

func TestDemoProvider_ImplementsStreaming(t *testing.T) {
	var _ StreamingLLMProvider = newTestDemo()
}
