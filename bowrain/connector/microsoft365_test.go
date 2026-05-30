package connector

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/sample.docx
var sampleDocx []byte

// graphMock stands in for Microsoft Graph: it lists one .docx in the OneDrive
// root, serves its bytes on /content, and captures the PUT upload.
type graphMock struct {
	srv      *httptest.Server
	uploaded []byte
	gotAuth  string
}

func newGraphMock(t *testing.T) *graphMock {
	t.Helper()
	m := &graphMock{}
	mux := http.NewServeMux()

	mux.HandleFunc("/me/drive/root/children", func(w http.ResponseWriter, r *http.Request) {
		m.gotAuth = r.Header.Get("Authorization")
		writeJSON(w, graphChildren{Value: []graphDriveItem{
			{ID: "item1", Name: "brief.docx", ETag: "\"v1\"", Modified: "2026-05-01T09:00:00Z",
				File: &struct {
					MimeType string `json:"mimeType"`
				}{MimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"}},
			{ID: "skip1", Name: "notes.txt"}, // no File facet / non-office → filtered out
		}})
	})

	mux.HandleFunc("/me/drive/items/item1/content", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(sampleDocx)
		case http.MethodPut:
			data, _ := io.ReadAll(r.Body)
			m.uploaded = data
			w.WriteHeader(http.StatusOK)
			writeJSON(w, graphDriveItem{ID: "item1", Name: "brief.docx"})
		}
	})

	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func newM365Conn(t *testing.T, srvURL string) *Microsoft365Connector {
	t.Helper()
	c, err := NewMicrosoft365Connector(map[string]string{
		"oauth_access_token": "graph-token",
		"base_url":           srvURL,
		"name":               "Test M365",
	})
	require.NoError(t, err)
	return c
}

func TestM365FetchParsesDocx(t *testing.T) {
	m := newGraphMock(t)
	c := newM365Conn(t, m.srv.URL)

	items, err := c.Fetch(t.Context(), platconn.FetchOptions{})
	require.NoError(t, err)
	require.Len(t, items, 1) // .txt filtered out

	item := items[0]
	assert.Equal(t, "brief.docx", item.Name)
	assert.Equal(t, "openxml", item.Format)
	assert.Equal(t, "item1", item.Metadata["ms_item_id"])
	require.NotEmpty(t, item.Blocks)

	var allText []string
	for _, b := range item.Blocks {
		allText = append(allText, b.SourceText())
	}
	assert.Contains(t, strings.Join(allText, " | "), "Hello, World!")
	assert.Equal(t, "Bearer graph-token", m.gotAuth)
}

func TestM365PublishRoundTripsDocx(t *testing.T) {
	m := newGraphMock(t)
	c := newM365Conn(t, m.srv.URL)

	items, err := c.Fetch(t.Context(), platconn.FetchOptions{})
	require.NoError(t, err)
	require.Len(t, items, 1)

	// Translate the "Hello, World!" block; leave the rest untouched.
	var translated bool
	for _, b := range items[0].Blocks {
		if b.SourceText() == "Hello, World!" {
			b.SetTargetText(model.LocaleFrench, "Bonjour le monde")
			translated = true
		}
	}
	require.True(t, translated, "fixture must contain a 'Hello, World!' block")

	err = c.Publish(t.Context(), items, platconn.PublishOptions{Locales: []model.LocaleID{model.LocaleFrench}})
	require.NoError(t, err)

	require.NotEmpty(t, m.uploaded, "expected an uploaded document")
	require.True(t, bytes.HasPrefix(m.uploaded, []byte("PK")), "uploaded bytes must be a valid OOXML (zip) archive")

	// Re-parse the uploaded archive and confirm the translation was spliced in.
	reparsed := extractDocxText(t, m.uploaded)
	assert.Contains(t, reparsed, "Bonjour le monde")
	assert.NotContains(t, reparsed, "Hello, World!")
}

func TestM365ListAndStatus(t *testing.T) {
	m := newGraphMock(t)
	c := newM365Conn(t, m.srv.URL)

	items, err := c.List(t.Context())
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "brief.docx", items[0].Name)

	st, err := c.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, st.ItemCount)
}

func TestM365CategoryAndAuthAliases(t *testing.T) {
	c := newM365Conn(t, "http://example.invalid")
	assert.Equal(t, platconn.CategoryProductivity, c.Category())
	assert.Equal(t, "microsoft365", c.ID())

	// tenant_id/client_id/client_secret derive the Entra token endpoint.
	aliased := withMicrosoftAuthAliases(map[string]string{
		"tenant_id":     "contoso.onmicrosoft.com",
		"client_id":     "app-123",
		"client_secret": "secret",
	})
	assert.Equal(t, "app-123", aliased["oauth_client_id"])
	assert.Contains(t, aliased["oauth_token_url"], "contoso.onmicrosoft.com")
	assert.Contains(t, aliased["oauth_token_url"], "oauth2/v2.0/token")

	_, err := NewMicrosoft365Connector(map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials")
}

// extractDocxText re-reads a .docx via the native openxml reader and returns
// its concatenated source text.
func extractDocxText(t *testing.T, data []byte) string {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	reader, err := reg.NewReader("openxml")
	require.NoError(t, err)
	defer reader.Close()

	doc := &model.RawDocument{URI: "out.docx", FormatID: "openxml", Reader: io.NopCloser(bytes.NewReader(data))}
	require.NoError(t, reader.Open(context.Background(), doc))

	var sb strings.Builder
	for pr := range reader.Read(context.Background()) {
		require.NoError(t, pr.Error)
		if pr.Part != nil && pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				sb.WriteString(b.SourceText())
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}
