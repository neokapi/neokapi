package addin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/brand"
)

// googleDocMock serves the single Drive file + Docs document a scoped add-on
// reads, and captures the documents.batchUpdate write-back.
type googleDocMock struct {
	srv      *httptest.Server
	docBatch map[string]any
}

func newGoogleDocMock(t *testing.T) *googleDocMock {
	t.Helper()
	m := &googleDocMock{}
	mux := http.NewServeMux()

	// Drive metadata for the scoped file id.
	mux.HandleFunc("/drive/v3/files/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "doc1", "name": "Launch Brief",
			"mimeType": "application/vnd.google-apps.document", "modifiedTime": "2026-05-01T10:00:00Z",
		})
	})

	mux.HandleFunc("/v1/documents/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, ":batchUpdate") {
			defer r.Body.Close()
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &m.docBatch)
			_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "doc1"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"title": "Launch Brief",
			"body": map[string]any{"content": []any{
				map[string]any{"paragraph": map[string]any{"elements": []any{
					map[string]any{"textRun": map[string]any{"content": "Please utilize the dashboard.\n"}},
				}}},
			}},
		})
	})

	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func googleAddinService(t *testing.T, baseURL string) *Service {
	t.Helper()
	s := New()
	s.LoadProfile = func(string) (*brand.VoiceProfile, error) { return testProfile(), nil }
	s.GoogleBaseURL = baseURL
	s.PublicURL = "https://addin.example.com"
	return s
}

func googleServer(t *testing.T, s *Service) *echo.Echo {
	t.Helper()
	e := echo.New()
	s.RegisterGoogleRoutes(e.Group(""))
	return e
}

func postEvent(t *testing.T, e *echo.Echo, path string, ev GoogleEvent) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(ev)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(buf)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func docEvent(scope bool) GoogleEvent {
	var ev GoogleEvent
	ev.CommonEventObject.HostApp = "DOCS"
	ev.AuthorizationEventObject.UserOAuthToken = "user-token"
	ev.Docs = &googleEditorContext{ID: "doc1", Title: "Launch Brief", AddonHasFileScopePermission: scope}
	return ev
}

func TestGoogleHomepageNeedsScope(t *testing.T) {
	s := googleAddinService(t, "")
	e := googleServer(t, s)
	rec := postEvent(t, e, "/google/homepage", docEvent(false))
	require.Equal(t, http.StatusOK, rec.Code)

	// Grant-access card pushed; the button targets /google/authorize.
	body := rec.Body.String()
	assert.Contains(t, body, "Grant access")
	assert.Contains(t, body, "https://addin.example.com/google/authorize")
	assert.Contains(t, body, "pushCard")
}

func TestGoogleHomepageWithScope(t *testing.T) {
	s := googleAddinService(t, "")
	e := googleServer(t, s)
	rec := postEvent(t, e, "/google/homepage", docEvent(true))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Translate")
	assert.Contains(t, body, "https://addin.example.com/google/scan")
	assert.Contains(t, body, "targetLang")
}

func TestGoogleAuthorizeReturnsScopeRequest(t *testing.T) {
	s := googleAddinService(t, "")
	e := googleServer(t, s)
	rec := postEvent(t, e, "/google/authorize", docEvent(false))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "requestFileScopeForActiveDocument")
}

func TestGoogleScanRendersFindings(t *testing.T) {
	m := newGoogleDocMock(t)
	s := googleAddinService(t, m.srv.URL)
	e := googleServer(t, s)

	rec := postEvent(t, e, "/google/scan", docEvent(true))
	require.Equal(t, http.StatusOK, rec.Code)

	var res clickResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.Len(t, res.RenderActions.Action.Navigations, 1)
	card := res.RenderActions.Action.Navigations[0].UpdateCard
	require.NotNil(t, card)
	require.NotNil(t, res.RenderActions.Action.Notification)
	assert.Equal(t, "Scan complete.", res.RenderActions.Action.Notification.Text)

	// The forbidden term in the document text should surface as a finding.
	body := rec.Body.String()
	assert.Contains(t, body, "Brand voice")
	assert.Contains(t, strings.ToLower(body), "utilize")
}

func TestGoogleTranslateWritesBack(t *testing.T) {
	m := newGoogleDocMock(t)
	s := googleAddinService(t, m.srv.URL)
	e := googleServer(t, s)

	ev := docEvent(true)
	ev.CommonEventObject.FormInputs = map[string]googleFormInput{
		"targetLang": {StringInputs: struct {
			Value []string `json:"value"`
		}{Value: []string{"fr"}}},
	}

	rec := postEvent(t, e, "/google/translate", ev)
	require.Equal(t, http.StatusOK, rec.Code)

	var res clickResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.NotNil(t, res.RenderActions.Action.Notification)
	assert.Contains(t, res.RenderActions.Action.Notification.Text, "fr")

	// A documents.batchUpdate write-back must have reached the mock.
	require.NotNil(t, m.docBatch, "expected a translation write-back")
	reqs, _ := m.docBatch["requests"].([]any)
	assert.NotEmpty(t, reqs)
}
