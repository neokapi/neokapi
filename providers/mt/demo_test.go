package mtprovider

import (
	"context"
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
	return NewDemoProvider()
}

func TestDemoMTProvider_Registered(t *testing.T) {
	p, err := NewProvider(Demo)
	require.NoError(t, err)
	assert.Equal(t, Demo, p.Name())

	_, err = NewProvider("nope")
	assert.Error(t, err)
}

func TestDemoMTProvider_TranslateLexicon(t *testing.T) {
	p := newTestDemo()
	cases := []struct {
		target model.LocaleID
		source string
		want   string
	}{
		{"fr", "Save", "⟦fr⟧ Enregistrer"},
		{"fr", "Hello world", "⟦fr⟧ Bonjour worldé"},
		{"es-419", "Welcome", "⟦es⟧ Bienvenido"},
		{"de", "Settings", "⟦de⟧ Einstellungen"},
		{"ja", "Save", "⟦ja⟧ Save~"},
	}
	for _, tc := range cases {
		resp, err := p.Translate(context.Background(), TranslateRequest{
			Source:       tc.source,
			TargetLocale: tc.target,
		})
		require.NoError(t, err)
		assert.Equal(t, tc.want, resp.Translation)
	}
}

func TestDemoMTProvider_Deterministic(t *testing.T) {
	p := newTestDemo()
	req := TranslateRequest{Source: "Hello, save the file or cancel.", TargetLocale: "de"}
	first, err := p.Translate(context.Background(), req)
	require.NoError(t, err)
	for range 5 {
		got, err := p.Translate(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, first.Translation, got.Translation)
	}
}

func TestDemoMTProvider_PreservesHTML(t *testing.T) {
	p := newTestDemo()
	resp, err := p.Translate(context.Background(), TranslateRequest{
		Source:       "Hello <a href=\"x\">world</a>",
		TargetLocale: "fr",
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Translation, `<a href="x">`)
	assert.Contains(t, resp.Translation, "</a>")
}

func TestDemoMTProvider_NoticeOnce(t *testing.T) {
	demoNoticeOnce = sync.Once{}
	var buf strings.Builder
	SetDemoNoticeWriter(&buf)
	defer SetDemoNoticeWriter(io.Discard)

	p := NewDemoProvider()
	_, _ = p.Translate(context.Background(), TranslateRequest{Source: "Hi", TargetLocale: "fr"})
	_, _ = p.Translate(context.Background(), TranslateRequest{Source: "Bye", TargetLocale: "fr"})

	assert.Equal(t, 1, strings.Count(buf.String(), DemoNotice))
	assert.Contains(t, buf.String(), "not a real machine-translation engine")
}

func TestDemoMTToolConfig(t *testing.T) {
	c := &DemoToolConfig{}
	assert.Equal(t, "demo-translate", c.ToolName())
	require.Error(t, c.Validate()) // missing target locale
	c.TargetLocale = "fr"
	require.NoError(t, c.Validate())
	c.Reset()
	assert.True(t, c.TargetLocale.IsEmpty())
}
