package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePostHog serves the PostHog query API shape: POST /api/projects/{id}/query
// with an Authorization: Bearer header. It routes HogQL text to canned result
// sets and counts query calls.
type fakePostHog struct {
	srv        *httptest.Server
	queryCalls atomic.Int64

	apiKey    string
	projectID string

	// malformedFor, when non-empty, returns invalid JSON for queries whose
	// text contains this substring.
	malformedFor string
}

func newFakePostHog(t *testing.T) *fakePostHog {
	t.Helper()
	f := &fakePostHog{apiKey: "phx_test_key_1234", projectID: "12345"}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/{pid}/query", func(w http.ResponseWriter, r *http.Request) {
		f.queryCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+f.apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"detail":"Invalid personal API key."}`))
			return
		}
		if r.PathValue("pid") != f.projectID {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"Project not found."}`))
			return
		}
		var req struct {
			Query struct {
				Kind  string `json:"kind"`
				Query string `json:"query"`
			} `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query.Kind != "HogQLQuery" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"expected a HogQLQuery"}`))
			return
		}

		q := req.Query.Query
		if f.malformedFor != "" && strings.Contains(q, f.malformedFor) {
			_, _ = w.Write([]byte(`{"results": not-json`))
			return
		}

		var results [][]any
		switch {
		case strings.Contains(q, "SELECT 1"):
			results = [][]any{{float64(1)}}
		case strings.Contains(q, "toStartOfWeek"):
			results = [][]any{
				{"2026-06-15", "pt-br", float64(40)},
				{"2026-06-22", "pt-br", float64(60)},
				{"2026-06-22", "en-US", float64(90)},
			}
		case strings.Contains(q, "extract("):
			// (wanted lang, served path prefix, sessions)
			results = [][]any{
				{"pt-BR", "pt-BR", float64(60)},     // hit
				{"pt-BR", "", float64(40)},          // served default (miss)
				{"en-US", "en", float64(80)},        // hit (base match)
				{"en-US,en;q=0.9", "", float64(20)}, // miss
			}
		default: // country x lang demand
			results = [][]any{
				{"BR", "pt-br", float64(100)},
				{"US", "en-US", float64(90)},
				{"US", "en-US,en;q=0.9", float64(20)},
				{"", "", float64(10)},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"columns": []string{"a", "b", "c"},
			"results": results,
		})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func TestResolvePostHogHost(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"us", "https://us.posthog.com", true},
		{"EU", "https://eu.posthog.com", true},
		{"https://ph.example.com/", "https://ph.example.com", true},
		{"", "", false},
		{"not a url", "", false},
		{"ftp://ph.example.com", "", false},
	}
	for _, tc := range cases {
		got, err := ResolvePostHogHost(tc.in)
		if tc.ok {
			require.NoError(t, err, tc.in)
			assert.Equal(t, tc.want, got, tc.in)
		} else {
			assert.Error(t, err, tc.in)
		}
	}
	assert.Equal(t, "us.posthog.com", PostHogHostLabel("us"))
	assert.Equal(t, "ph.example.com", PostHogHostLabel("https://ph.example.com"))
}

func TestNormalizePostHogLangTag(t *testing.T) {
	cases := map[string]string{
		"en-US":          "en-US",
		"EN-us":          "en-US",
		"pt_br":          "pt-BR",
		"en-US,en;q=0.9": "en-US",
		"fr;q=0.8":       "fr",
		"zh-hans":        "zh-Hans",
		"zh-HANS-cn":     "zh-Hans-CN",
		"":               "unknown",
		"  ":             "unknown",
		"*":              "unknown",
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizePostHogLangTag(in), "input %q", in)
	}
}

func TestValidatePostHogPathPattern(t *testing.T) {
	assert.NoError(t, ValidatePostHogPathPattern(DefaultPostHogPathLocalePattern))
	assert.Error(t, ValidatePostHogPathPattern("^/(fr|de"), "unbalanced regex")
	assert.Error(t, ValidatePostHogPathPattern("^/fr/"), "no capture group")
	assert.Error(t, ValidatePostHogPathPattern(`^/([a-z]{2})' OR '1'='1`), "quote injection")
}

func TestPostHogTestConnection(t *testing.T) {
	f := newFakePostHog(t)

	c, err := NewPostHogClient(f.srv.URL, "12345", "phx_test_key_1234")
	require.NoError(t, err)
	require.NoError(t, c.TestConnection(t.Context()))

	// Bad key → classified as bad_key (401).
	c, err = NewPostHogClient(f.srv.URL, "12345", "phx_wrong")
	require.NoError(t, err)
	err = c.TestConnection(t.Context())
	var pe *PostHogError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, PostHogErrBadKey, pe.Code)
	assert.True(t, IsPostHogAuthError(err))

	// Bad project → classified as bad_project (404).
	c, err = NewPostHogClient(f.srv.URL, "99999", "phx_test_key_1234")
	require.NoError(t, err)
	err = c.TestConnection(t.Context())
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, PostHogErrBadProject, pe.Code)

	// Unreachable host → classified as bad_host.
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	c, err = NewPostHogClient(closed.URL, "12345", "phx_test_key_1234")
	require.NoError(t, err)
	err = c.TestConnection(t.Context())
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, PostHogErrBadHost, pe.Code)
}

func TestPostHogFetchDemandHappyPath(t *testing.T) {
	f := newFakePostHog(t)
	c, err := NewPostHogClient(f.srv.URL, "12345", "phx_test_key_1234")
	require.NoError(t, err)

	snap, err := c.FetchDemand(t.Context(), "30d", "")
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.Empty(t, snap.Warnings)
	assert.Equal(t, "30d", snap.Range)
	assert.EqualValues(t, 3, f.queryCalls.Load(), "demand + trend + served queries")

	// Totals and per-country breakdown, normalized tags, empty→unknown.
	assert.EqualValues(t, 220, snap.TotalSessions)
	require.Len(t, snap.Countries, 3)
	assert.Equal(t, "US", snap.Countries[0].CountryCode)
	assert.EqualValues(t, 110, snap.Countries[0].Sessions)
	// en-US and its q-junk variant fold into one normalized tag.
	require.Len(t, snap.Countries[0].Languages, 1)
	assert.Equal(t, "en-US", snap.Countries[0].Languages[0].Tag)
	assert.EqualValues(t, 110, snap.Countries[0].Languages[0].Sessions)
	assert.Equal(t, "BR", snap.Countries[1].CountryCode)
	assert.Equal(t, "pt-BR", snap.Countries[1].Languages[0].Tag)
	assert.Equal(t, "unknown", snap.Countries[2].CountryCode)
	assert.Equal(t, "unknown", snap.Countries[2].Languages[0].Tag)

	// Language aggregates, sorted by demand.
	require.GreaterOrEqual(t, len(snap.Languages), 2)
	assert.Equal(t, "en-US", snap.Languages[0].Tag)
	assert.EqualValues(t, 110, snap.Languages[0].Sessions)
	assert.Equal(t, "pt-BR", snap.Languages[1].Tag)

	// Weekly trend attached per language, zero-filled across weeks.
	require.Len(t, snap.Languages[0].Trend, 2)
	assert.Equal(t, "2026-06-15", snap.Languages[0].Trend[0].WeekStart)
	assert.EqualValues(t, 0, snap.Languages[0].Trend[0].Sessions, "en-US had no week-1 sessions")
	assert.EqualValues(t, 90, snap.Languages[0].Trend[1].Sessions)
	assert.EqualValues(t, 40, snap.Languages[1].Trend[0].Sessions)
	assert.EqualValues(t, 60, snap.Languages[1].Trend[1].Sessions)

	// Served-locale hit rate: (60 + 80) / 200.
	require.NotNil(t, snap.ServedLocaleHitRate)
	assert.InDelta(t, 0.7, *snap.ServedLocaleHitRate, 0.0001)
}

func TestPostHogFetchDemandBadKeyAborts(t *testing.T) {
	f := newFakePostHog(t)
	c, err := NewPostHogClient(f.srv.URL, "12345", "phx_wrong")
	require.NoError(t, err)

	snap, err := c.FetchDemand(t.Context(), "7d", "")
	assert.Nil(t, snap)
	var pe *PostHogError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, PostHogErrBadKey, pe.Code)
}

func TestPostHogFetchDemandMalformedResponseIsPartial(t *testing.T) {
	f := newFakePostHog(t)
	f.malformedFor = "toStartOfWeek" // trend query returns broken JSON

	c, err := NewPostHogClient(f.srv.URL, "12345", "phx_test_key_1234")
	require.NoError(t, err)

	snap, err := c.FetchDemand(t.Context(), "90d", "")
	require.NoError(t, err, "HogQL failures degrade to partial data")
	require.NotNil(t, snap)

	// Demand + served queries still populated the snapshot.
	assert.EqualValues(t, 220, snap.TotalSessions)
	require.NotNil(t, snap.ServedLocaleHitRate)

	// The trend failure surfaced as a warning; no trend series attached.
	require.Len(t, snap.Warnings, 1)
	assert.Contains(t, snap.Warnings[0], "trend query failed")
	assert.Contains(t, snap.Warnings[0], "malformed")
	for _, l := range snap.Languages {
		assert.Empty(t, l.Trend)
	}
}

func TestPostHogFetchDemandInvalidRange(t *testing.T) {
	f := newFakePostHog(t)
	c, err := NewPostHogClient(f.srv.URL, "12345", "phx_test_key_1234")
	require.NoError(t, err)
	_, err = c.FetchDemand(t.Context(), "365d", "")
	assert.Error(t, err)
	assert.Zero(t, f.queryCalls.Load(), "invalid range must not hit the API")
}
