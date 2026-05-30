package addin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/brand"
)

// testProfile is a small, deterministic brand voice profile so checks/terms
// don't depend on the contents of a built-in pack.
func testProfile() *brand.VoiceProfile {
	return &brand.VoiceProfile{
		ID:   "test",
		Name: "Test Brand",
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms:  []brand.TermRule{{Term: "utilize", Replacement: "use", Severity: "minor"}},
			PreferredTerms:  []brand.TermRule{{Term: "sign in", Note: `not "log in"`}},
			CompetitorTerms: []brand.TermRule{{Term: "Globex", Replacement: "our platform", Severity: "major"}},
		},
	}
}

func testService() *Service {
	s := New()
	s.LoadProfile = func(string) (*brand.VoiceProfile, error) { return testProfile(), nil }
	return s
}

func TestServiceCheck(t *testing.T) {
	s := testService()
	res, err := s.Check(context.Background(), CheckRequest{Text: "Please utilize the dashboard."})
	require.NoError(t, err)
	assert.Equal(t, "Test Brand", res.Profile)
	require.NotEmpty(t, res.Findings, "forbidden term 'utilize' should produce a finding")
	assert.Less(t, res.Score, 100)

	var sawUtilize bool
	for _, f := range res.Findings {
		if strings.Contains(strings.ToLower(f.Message+f.OriginalText), "utilize") {
			sawUtilize = true
		}
	}
	assert.True(t, sawUtilize, "a finding should reference the forbidden term")
}

func TestServiceCheckClean(t *testing.T) {
	s := testService()
	res, err := s.Check(context.Background(), CheckRequest{Text: "Sign in to your dashboard."})
	require.NoError(t, err)
	assert.Equal(t, 100, res.Score)
	assert.Empty(t, res.Findings)
}

func TestServiceTerms(t *testing.T) {
	s := testService()
	res, err := s.Terms(context.Background(), TermsRequest{Text: "We utilize Globex daily; please sign in."})
	require.NoError(t, err)

	byTerm := map[string]TermHit{}
	for _, m := range res.Matches {
		byTerm[m.Term] = m
	}
	require.Contains(t, byTerm, "utilize")
	assert.Equal(t, "forbidden", byTerm["utilize"].Status)
	assert.Equal(t, "use", byTerm["utilize"].Replacement)
	require.Contains(t, byTerm, "Globex")
	assert.Equal(t, "competitor", byTerm["Globex"].Status)
	require.Contains(t, byTerm, "sign in")
	assert.Equal(t, "preferred", byTerm["sign in"].Status)
}

func TestServiceTranslate(t *testing.T) {
	s := testService() // default (demo) provider
	res, err := s.Translate(context.Background(), TranslateRequest{
		Text:         "Hello world",
		TargetLocale: "fr",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res.Translation)
	assert.Equal(t, "fr", res.TargetLocale)
	assert.Equal(t, "en", res.SourceLocale)
	assert.Equal(t, "demo", res.Provider)
}

func TestServiceTranslateRequiresTarget(t *testing.T) {
	s := testService()
	_, err := s.Translate(context.Background(), TranslateRequest{Text: "Hello"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target_locale")
}

// --- REST handler tests (the Office task pane contract) ---

func restServer(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	testService().RegisterRoutes(e.Group("/addin"))
	return e
}

func postJSON(t *testing.T, e *echo.Echo, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(buf)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestRESTCheck(t *testing.T) {
	e := restServer(t)
	rec := postJSON(t, e, "/addin/check", CheckRequest{Text: "Please utilize this."})
	require.Equal(t, http.StatusOK, rec.Code)
	var res CheckResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.NotEmpty(t, res.Findings)
}

func TestRESTTerms(t *testing.T) {
	e := restServer(t)
	rec := postJSON(t, e, "/addin/terms", TermsRequest{Text: "We utilize things."})
	require.Equal(t, http.StatusOK, rec.Code)
	var res TermsResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.Len(t, res.Matches, 1)
	assert.Equal(t, "utilize", res.Matches[0].Term)
}

func TestRESTTranslate(t *testing.T) {
	e := restServer(t)
	rec := postJSON(t, e, "/addin/translate", TranslateRequest{Text: "Hello", TargetLocale: "de"})
	require.Equal(t, http.StatusOK, rec.Code)
	var res TranslateResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.NotEmpty(t, res.Translation)
	assert.Equal(t, "de", res.TargetLocale)
}

func TestRESTTranslateBadRequest(t *testing.T) {
	e := restServer(t)
	rec := postJSON(t, e, "/addin/translate", TranslateRequest{Text: "Hello"}) // no target
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
