package connector

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// googleMock is a stand-in for the four Workspace REST APIs on one host. It
// records the batchUpdate / values bodies so tests can assert write-back.
type googleMock struct {
	srv           *httptest.Server
	docBatch      map[string]any
	sheetBatch    map[string]any
	slideBatch    map[string]any
	gotAuthHeader string
}

func newGoogleMock(t *testing.T) *googleMock {
	t.Helper()
	m := &googleMock{}
	mux := http.NewServeMux()

	// Drive: list editor files.
	mux.HandleFunc("/drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		m.gotAuthHeader = r.Header.Get("Authorization")
		writeJSON(w, driveFileList{Files: []driveFile{
			{ID: "doc1", Name: "Launch Brief", MimeType: gwsMimeDoc, Modified: "2026-05-01T10:00:00Z"},
			{ID: "sheet1", Name: "Strings", MimeType: gwsMimeSheet, Modified: "2026-05-01T10:00:00Z"},
			{ID: "slide1", Name: "Deck", MimeType: gwsMimeSlide, Modified: "2026-05-01T10:00:00Z"},
		}})
	})

	// Docs.
	mux.HandleFunc("/v1/documents/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			m.docBatch = decodeBody(r)
			writeJSON(w, map[string]any{"documentId": "doc1"})
			return
		}
		writeJSON(w, docsDocument{
			Title: "Launch Brief",
			Body: docsBody{Content: []docsStructuralElement{
				{Paragraph: &docsParagraph{Elements: []docsParagraphElement{{TextRun: &docsTextRun{Content: "Hello\n"}}}}},
				{Paragraph: &docsParagraph{Elements: []docsParagraphElement{{TextRun: &docsTextRun{Content: "Sign in to continue\n"}}}}},
				{Paragraph: &docsParagraph{Elements: []docsParagraphElement{{TextRun: &docsTextRun{Content: "  \n"}}}}}, // whitespace-only → skipped
			}},
		})
	})

	// Sheets.
	mux.HandleFunc("/v4/spreadsheets/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			m.sheetBatch = decodeBody(r)
			writeJSON(w, map[string]any{"totalUpdatedCells": 1})
		case strings.Contains(r.URL.Path, "/values/"):
			writeJSON(w, sheetsValueRange{
				Range:  "Sheet1",
				Values: [][]any{{"Welcome"}, {"Dashboard"}, {float64(42)}}, // number ignored
			})
		default:
			writeJSON(w, sheetsSpreadsheet{Sheets: []struct {
				Properties struct {
					Title string `json:"title"`
				} `json:"properties"`
			}{{Properties: struct {
				Title string `json:"title"`
			}{Title: "Sheet1"}}}})
		}
	})

	// Slides.
	mux.HandleFunc("/v1/presentations/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			m.slideBatch = decodeBody(r)
			writeJSON(w, map[string]any{"presentationId": "slide1"})
			return
		}
		writeJSON(w, slidesPresentation{Slides: []struct {
			PageElements []slidesPageElement `json:"pageElements"`
		}{{PageElements: []slidesPageElement{
			{ObjectID: "shape1", Shape: &struct {
				Text *slidesTextContent `json:"text"`
			}{Text: &slidesTextContent{TextElements: []struct {
				TextRun *struct {
					Content string `json:"content"`
				} `json:"textRun"`
			}{{TextRun: &struct {
				Content string `json:"content"`
			}{Content: "Get started"}}}}}},
		}}}})
	})

	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func newGoogleConn(t *testing.T, srvURL string) *GoogleWorkspaceConnector {
	t.Helper()
	c, err := NewGoogleWorkspaceConnector(map[string]string{
		"oauth_access_token": "test-token",
		"base_url":           srvURL,
		"name":               "Test Google",
	})
	require.NoError(t, err)
	return c
}

func TestGoogleFetchExtractsAllKinds(t *testing.T) {
	m := newGoogleMock(t)
	c := newGoogleConn(t, m.srv.URL)

	items, err := c.Fetch(t.Context(), platconn.FetchOptions{})
	require.NoError(t, err)
	require.Len(t, items, 3)

	byKind := map[string]*platconn.ContentItem{}
	for _, it := range items {
		byKind[it.Metadata["gws_kind"]] = it
	}

	// Doc: two real paragraphs (whitespace-only paragraph skipped).
	doc := byKind[gwsKindDoc]
	require.NotNil(t, doc)
	require.Len(t, doc.Blocks, 2)
	assert.Equal(t, "Hello", doc.Blocks[0].SourceText())
	assert.Equal(t, "Sign in to continue", doc.Blocks[1].SourceText())

	// Sheet: two string cells with A1 coordinates (number cell ignored).
	sheet := byKind[gwsKindSheet]
	require.NotNil(t, sheet)
	require.Len(t, sheet.Blocks, 2)
	assert.Equal(t, "Welcome", sheet.Blocks[0].SourceText())
	assert.Equal(t, "Sheet1!A1", sheet.Blocks[0].Properties["gws_cell"])
	assert.Equal(t, "Sheet1!A2", sheet.Blocks[1].Properties["gws_cell"])

	// Slide: one shape run.
	slide := byKind[gwsKindSlide]
	require.NotNil(t, slide)
	require.Len(t, slide.Blocks, 1)
	assert.Equal(t, "Get started", slide.Blocks[0].SourceText())

	assert.Equal(t, "Bearer test-token", m.gotAuthHeader)
}

func TestGooglePublishDocReplaceAllText(t *testing.T) {
	m := newGoogleMock(t)
	c := newGoogleConn(t, m.srv.URL)

	b := model.NewBlock("doc1:doc:0", "Hello")
	b.SetTargetText(model.LocaleFrench, "Bonjour")
	item := &platconn.ContentItem{
		ID:       "doc1",
		Metadata: map[string]string{"gws_kind": gwsKindDoc, "gws_file_id": "doc1"},
		Blocks:   []*model.Block{b},
	}

	err := c.Publish(t.Context(), []*platconn.ContentItem{item}, platconn.PublishOptions{Locales: []model.LocaleID{model.LocaleFrench}})
	require.NoError(t, err)

	require.NotNil(t, m.docBatch)
	reqs, _ := m.docBatch["requests"].([]any)
	require.Len(t, reqs, 1)
	first, _ := reqs[0].(map[string]any)
	rat, _ := first["replaceAllText"].(map[string]any)
	require.NotNil(t, rat)
	contains, _ := rat["containsText"].(map[string]any)
	assert.Equal(t, "Hello", contains["text"])
	assert.Equal(t, "Bonjour", rat["replaceText"])
}

func TestGooglePublishSheetValues(t *testing.T) {
	m := newGoogleMock(t)
	c := newGoogleConn(t, m.srv.URL)

	b := model.NewBlock("sheet1:cell:Sheet1!A1", "Welcome")
	b.Properties = map[string]string{"gws_cell": "Sheet1!A1"}
	b.SetTargetText(model.LocaleFrench, "Bienvenue")
	item := &platconn.ContentItem{
		ID:       "sheet1",
		Metadata: map[string]string{"gws_kind": gwsKindSheet, "gws_file_id": "sheet1"},
		Blocks:   []*model.Block{b},
	}

	err := c.Publish(t.Context(), []*platconn.ContentItem{item}, platconn.PublishOptions{Locales: []model.LocaleID{model.LocaleFrench}})
	require.NoError(t, err)

	require.NotNil(t, m.sheetBatch)
	assert.Equal(t, "RAW", m.sheetBatch["valueInputOption"])
	data, _ := m.sheetBatch["data"].([]any)
	require.Len(t, data, 1)
	vr, _ := data[0].(map[string]any)
	assert.Equal(t, "Sheet1!A1", vr["range"])
}

func TestGooglePublishSlidesReplaceAllText(t *testing.T) {
	m := newGoogleMock(t)
	c := newGoogleConn(t, m.srv.URL)

	b := model.NewBlock("slide1:slide:0", "Get started")
	b.SetTargetText(model.LocaleFrench, "Commencer")
	item := &platconn.ContentItem{
		ID:       "slide1",
		Metadata: map[string]string{"gws_kind": gwsKindSlide, "gws_file_id": "slide1"},
		Blocks:   []*model.Block{b},
	}

	err := c.Publish(t.Context(), []*platconn.ContentItem{item}, platconn.PublishOptions{Locales: []model.LocaleID{model.LocaleFrench}})
	require.NoError(t, err)

	require.NotNil(t, m.slideBatch)
	reqs, _ := m.slideBatch["requests"].([]any)
	require.Len(t, reqs, 1)
}

func TestGoogleListAndStatus(t *testing.T) {
	m := newGoogleMock(t)
	c := newGoogleConn(t, m.srv.URL)

	items, err := c.List(t.Context())
	require.NoError(t, err)
	assert.Len(t, items, 3)

	st, err := c.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, st.ItemCount)
}

func TestGoogleCategoryAndCredentials(t *testing.T) {
	c := newGoogleConn(t, "http://example.invalid")
	assert.Equal(t, platconn.CategoryProductivity, c.Category())
	assert.Equal(t, "google-workspace", c.ID())

	_, err := NewGoogleWorkspaceConnector(map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OAuth credentials")
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func decodeBody(r *http.Request) map[string]any {
	defer r.Body.Close()
	data, _ := io.ReadAll(r.Body)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}
