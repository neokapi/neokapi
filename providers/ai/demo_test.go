package aiprovider

import (
	"context"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/ai/prompt"
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
	for range 5 {
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

// A placeholder is structure, not prose. The demo used to accent the inside of
// one — `%d` split into `%` and the word `d`, `{0}` into braces around `0` — so
// its own output failed kapi's placeholder check. Nothing that ships in kapi is
// allowed to produce a translation kapi cannot write back.
func TestDemoProvider_PreservesPlaceholders(t *testing.T) {
	p := newTestDemo()

	for _, src := range []string{
		"You have {0} items in your cart",
		"Welcome back, {{name}}",
		"Uploaded %d of %d files",
		"You have used {used} of {total} GB",
		"Could not sync %s: %s",
	} {
		resp, err := p.Translate(context.Background(), TranslateRequest{Source: src, TargetLocale: "de"})
		require.NoError(t, err)

		for _, ph := range regexp.MustCompile(`\{\{[^{}]*\}\}|\{[^{}]*\}|%[a-zA-Z]`).FindAllString(src, -1) {
			assert.Containsf(t, resp.Translation, ph, "placeholder %q lost from %q → %q", ph, src, resp.Translation)
		}
	}
}

// The AI translate tool masks do-not-translate spans with double-bracketed
// sentinels ([[DNT_0]]) and restores them verbatim after generation. A real
// model passes the opaque token through; the demo stub must do the same, or
// kapi's own DNT machinery breaks under `--provider demo`.
func TestDemoProvider_PreservesDNTSentinels(t *testing.T) {
	p := newTestDemo()

	src := "Open [[DNT_0]] and save [[DNT_12]] now"
	resp, err := p.Translate(context.Background(), TranslateRequest{Source: src, TargetLocale: "fr"})
	require.NoError(t, err)
	assert.Contains(t, resp.Translation, "[[DNT_0]]", "sentinel mangled: %q", resp.Translation)
	assert.Contains(t, resp.Translation, "[[DNT_12]]", "sentinel mangled: %q", resp.Translation)
}

// Excluding `{` and `%` from the tokenizer's catch-all class is how placeholders
// stay whole; a stray one that matches no placeholder form must still be emitted,
// not silently dropped on the floor.
func TestDemoProvider_KeepsStrayBracesAndPercents(t *testing.T) {
	p := newTestDemo()

	for _, tc := range []struct{ src, want string }{
		{"Save 50% today", "%"},       // a bare percent is not a placeholder
		{"An unmatched { brace", "{"}, // nor is an unbalanced brace
		{"A }} stray close", "}"},     //
	} {
		resp, err := p.Translate(context.Background(), TranslateRequest{Source: tc.src, TargetLocale: "de"})
		require.NoError(t, err)
		assert.Containsf(t, resp.Translation, tc.want, "%q → %q dropped a character", tc.src, resp.Translation)
	}
}

func TestDemoProvider_BatchTranslations(t *testing.T) {
	p := newTestDemo()

	// Drive the real prompt builder, and carry intent in prompt.Meta the way the
	// translate tool does. The demo provider reads the target locale from that
	// metadata — it must not recover it by parsing the prompt's English.
	pt := prompt.Translate{SourceLocale: "en", TargetLocale: "fr"}
	turns := pt.Batch(prompt.BatchSegments([]string{"Hello", "Save", "world"}))
	ctx := prompt.WithMeta(context.Background(), pt.Meta(prompt.IDTranslateBatch))

	resp, err := p.ChatStructured(ctx, MessagesFromTurns(turns), JSONSchema{Name: "batch_translations"})
	require.NoError(t, err)

	var out struct {
		Translations []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"translations"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &out))
	require.Len(t, out.Translations, 3)
	assert.Equal(t, "s1", out.Translations[0].ID, "the reply must echo the id it was given")
	assert.Equal(t, "⟦fr⟧ Bonjour", out.Translations[0].Text)
	assert.Equal(t, "⟦fr⟧ Enregistrer", out.Translations[1].Text)
}

func TestDemoProvider_NeutralSchema(t *testing.T) {
	// A check-style schema must yield an empty issues array, not fabricated findings.
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
	resp, err := p.ChatStructured(context.Background(), []Message{TextMessage("user", "check this")}, qa)
	require.NoError(t, err)

	var out struct {
		Issues []any `json:"issues"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &out))
	assert.Empty(t, out.Issues)
}

func TestDemoProvider_NoticeOnce(t *testing.T) {
	// Reset the once guard for an isolated assertion.
	noticeOnce = sync.OnceFunc(func() {
		_, _ = io.WriteString(demoNoticeWriter, DemoNotice+"\n")
	})
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

// An ICU plural or select is a brace pair that contains brace pairs. Its
// argument name, keyword and category keywords are program syntax, and a demo
// that accents them writes a message no ICU formatter accepts — into the
// catalog a sample's own page reads. Everything from an opening brace to its
// match passes through untouched.
func TestDemoProvider_PreservesICUMessages(t *testing.T) {
	p := newTestDemo()

	for _, tc := range []struct{ name, src, want string }{
		{
			name: "plural",
			src:  "{count, plural, one {# berth} other {# berths}} at this terminal.",
			want: "{count, plural, one {# berth} other {# berths}}",
		},
		{
			name: "select",
			src:  "{gender, select, male {He} female {She} other {They}} is alongside.",
			want: "{gender, select, male {He} female {She} other {They}}",
		},
		{
			name: "selectordinal",
			src:  "The {n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}} berth is free.",
			want: "{n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}",
		},
		{
			name: "plural nested in select",
			src:  "{g, select, male {{n, plural, one {# berth} other {# berths}}} other {none}} today.",
			want: "{g, select, male {{n, plural, one {# berth} other {# berths}}} other {none}}",
		},
		{
			name: "offset and exact-value branches",
			src:  "{count, plural, offset:1 =0 {Nobody} one {# other} other {# others}} aboard.",
			want: "{count, plural, offset:1 =0 {Nobody} one {# other} other {# others}}",
		},
		{
			name: "quoted literal braces stay literal",
			src:  "Write '{'name'}' to interpolate {name}.",
			want: "{name}",
		},
		{
			name: "a prose apostrophe does not swallow the placeholder",
			src:  "Don't move {vessel} yet.",
			want: "{vessel}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := p.Translate(context.Background(), TranslateRequest{Source: tc.src, TargetLocale: "nl"})
			require.NoError(t, err)
			assert.Containsf(t, resp.Translation, tc.want, "ICU syntax mangled: %q → %q", tc.src, resp.Translation)
		})
	}
}
