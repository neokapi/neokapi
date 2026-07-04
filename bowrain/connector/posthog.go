package connector

// PostHog locale-demand connector (phase 0, read-only).
//
// Unlike the content connectors in this package (WordPress, Git, File, …),
// PostHog is not a source of translatable content, so it is not registered in
// the IntegrationConnector registry. It is an analytics source: the server
// queries the PostHog HogQL API for language-demand signals ($browser_language
// x $geoip_country_code on $pageview events) and serves them to the
// Locale-demand page. Its per-project configuration and personal API key are
// stored via the existing connector-secret pattern (an encrypted
// Collection.ConnectorConfig; see bowrain/server/handlers_posthog.go).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Well-known PostHog Cloud hosts. A custom self-hosted URL is also accepted.
const (
	PostHogHostUS = "us"
	PostHogHostEU = "eu"

	posthogUSBaseURL = "https://us.posthog.com"
	posthogEUBaseURL = "https://eu.posthog.com"
)

// DefaultPostHogPathLocalePattern extracts a served-locale prefix from
// $pathname: a leading /xx/ or /xx-XX/ segment. The first capture group is
// the locale.
const DefaultPostHogPathLocalePattern = `^/([a-z]{2}(-[A-Z]{2})?)/`

// posthogRowLimit bounds every HogQL query; demand aggregation never needs
// unbounded result sets.
const posthogRowLimit = 5000

// posthogQueryTimeout bounds a single HogQL query round-trip.
const posthogQueryTimeout = 15 * time.Second

// PostHogErrorCode classifies a PostHog API failure so the connect card can
// tell the user what to fix.
type PostHogErrorCode string

const (
	// PostHogErrBadHost — the host is unreachable or not a PostHog API.
	PostHogErrBadHost PostHogErrorCode = "bad_host"
	// PostHogErrBadKey — the personal API key was rejected (401/403).
	PostHogErrBadKey PostHogErrorCode = "bad_key"
	// PostHogErrBadProject — the PostHog project does not exist or the key
	// cannot see it (404).
	PostHogErrBadProject PostHogErrorCode = "bad_project"
	// PostHogErrQuery — the query failed or returned a malformed response.
	PostHogErrQuery PostHogErrorCode = "query_error"
)

// PostHogError is a classified PostHog API failure.
type PostHogError struct {
	Code    PostHogErrorCode
	Status  int
	Message string
}

func (e *PostHogError) Error() string {
	return fmt.Sprintf("posthog: %s (%s)", e.Message, e.Code)
}

// IsPostHogAuthError reports whether err is a PostHogError that indicates a
// configuration problem (bad host, key, or project) rather than a transient
// query failure.
func IsPostHogAuthError(err error) bool {
	var pe *PostHogError
	if !errors.As(err, &pe) {
		return false
	}
	return pe.Code == PostHogErrBadHost || pe.Code == PostHogErrBadKey || pe.Code == PostHogErrBadProject
}

// ResolvePostHogHost maps the host config value ("us", "eu", or a custom
// https URL for self-hosted instances) to an API base URL.
func ResolvePostHogHost(host string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case PostHogHostUS:
		return posthogUSBaseURL, nil
	case PostHogHostEU:
		return posthogEUBaseURL, nil
	case "":
		return "", errors.New("posthog host is required")
	}
	u, err := url.Parse(strings.TrimSpace(host))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("posthog host must be %q, %q, or an http(s) URL", PostHogHostUS, PostHogHostEU)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// PostHogHostLabel returns a short display label for the configured host
// (e.g. "us.posthog.com"), for provenance footers.
func PostHogHostLabel(host string) string {
	base, err := ResolvePostHogHost(host)
	if err != nil {
		return host
	}
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		return u.Host
	}
	return base
}

// ValidatePostHogPathPattern checks a served-locale path pattern: it must be
// a valid regexp with at least one capture group, and must not contain quote
// or backslash-escape characters that could break out of the HogQL string
// literal it is embedded in.
func ValidatePostHogPathPattern(pattern string) error {
	if strings.ContainsAny(pattern, `'"\`) {
		return errors.New("path locale pattern must not contain quotes or backslashes")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid path locale pattern: %w", err)
	}
	if re.NumSubexp() < 1 {
		return errors.New("path locale pattern needs a capture group for the locale")
	}
	return nil
}

// PostHogClient talks to the PostHog query API for one project.
type PostHogClient struct {
	baseURL   string
	projectID string
	apiKey    string
	client    *http.Client
}

// NewPostHogClient builds a client from the connector config values.
func NewPostHogClient(host, projectID, apiKey string) (*PostHogClient, error) {
	base, err := ResolvePostHogHost(host)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, errors.New("posthog project id is required")
	}
	if apiKey == "" {
		return nil, errors.New("posthog personal API key is required")
	}
	return &PostHogClient{
		baseURL:   base,
		projectID: strings.TrimSpace(projectID),
		apiKey:    apiKey,
		client:    &http.Client{Timeout: posthogQueryTimeout},
	}, nil
}

// hogQLResponse is the subset of the PostHog query API response we consume.
type hogQLResponse struct {
	Results [][]any `json:"results"`
}

// Query runs a HogQL query (kind: "HogQLQuery") against
// POST {host}/api/projects/{id}/query and returns the result rows.
func (c *PostHogClient) Query(ctx context.Context, hogql string) ([][]any, error) {
	ctx, cancel := context.WithTimeout(ctx, posthogQueryTimeout)
	defer cancel()

	body, err := json.Marshal(map[string]any{
		"query": map[string]any{"kind": "HogQLQuery", "query": hogql},
	})
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/api/projects/%s/query", c.baseURL, url.PathEscape(c.projectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, &PostHogError{Code: PostHogErrBadHost, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &PostHogError{Code: PostHogErrBadHost, Message: fmt.Sprintf("cannot reach %s: %s", c.baseURL, err)}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, &PostHogError{Code: PostHogErrQuery, Status: resp.StatusCode, Message: err.Error()}
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, &PostHogError{Code: PostHogErrBadKey, Status: resp.StatusCode, Message: "personal API key was rejected"}
	case resp.StatusCode == http.StatusNotFound:
		return nil, &PostHogError{Code: PostHogErrBadProject, Status: resp.StatusCode, Message: fmt.Sprintf("project %s not found (or the key cannot access it)", c.projectID)}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, &PostHogError{Code: PostHogErrQuery, Status: resp.StatusCode, Message: fmt.Sprintf("query failed with status %d: %s", resp.StatusCode, truncate(string(data), 200))}
	}

	var parsed hogQLResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, &PostHogError{Code: PostHogErrQuery, Status: resp.StatusCode, Message: "malformed HogQL response: " + err.Error()}
	}
	return parsed.Results, nil
}

// TestConnection runs the cheapest possible HogQL query to verify host, key,
// and project in one round-trip.
func (c *PostHogClient) TestConnection(ctx context.Context) error {
	_, err := c.Query(ctx, "SELECT 1")
	return err
}

// ---------------------------------------------------------------------------
// Demand snapshot
// ---------------------------------------------------------------------------

// PostHogLanguageSessions is a (language tag, sessions) pair.
type PostHogLanguageSessions struct {
	Tag      string `json:"tag"`
	Sessions int64  `json:"sessions"`
}

// PostHogCountryDemand is demand observed from one country.
type PostHogCountryDemand struct {
	CountryCode string                    `json:"country_code"`
	Sessions    int64                     `json:"sessions"`
	Languages   []PostHogLanguageSessions `json:"languages"`
}

// PostHogTrendPoint is one week of a language's demand trend.
type PostHogTrendPoint struct {
	WeekStart string `json:"week_start"`
	Sessions  int64  `json:"sessions"`
}

// PostHogLanguageDemand aggregates one language across countries.
type PostHogLanguageDemand struct {
	Tag      string              `json:"tag"`
	Sessions int64               `json:"sessions"`
	Trend    []PostHogTrendPoint `json:"trend,omitempty"`
}

// PostHogDemandSnapshot is the demand payload assembled from the HogQL
// queries. Warnings carry per-query failures: the snapshot is best-effort
// partial data, never all-or-nothing (except for auth-class errors).
type PostHogDemandSnapshot struct {
	Range               string                  `json:"range"`
	GeneratedAt         time.Time               `json:"generated_at"`
	TotalSessions       int64                   `json:"total_sessions"`
	Countries           []PostHogCountryDemand  `json:"countries"`
	Languages           []PostHogLanguageDemand `json:"languages"`
	ServedLocaleHitRate *float64                `json:"served_locale_hit_rate,omitempty"`
	Warnings            []string                `json:"warnings,omitempty"`
}

// rangeToDays maps the demand ranges to day windows.
var rangeToDays = map[string]int{"7d": 7, "30d": 30, "90d": 90}

// PostHogRangeDays returns the day window for a demand range ("7d"|"30d"|"90d").
func PostHogRangeDays(r string) (int, bool) {
	d, ok := rangeToDays[r]
	return d, ok
}

// FetchDemand runs the three demand queries and assembles a snapshot.
// pathPattern is the served-locale extraction regex (first capture group =
// locale prefix); it must already be validated. HogQL failures on individual
// queries degrade to warnings + partial data; auth-class errors abort.
func (c *PostHogClient) FetchDemand(ctx context.Context, demandRange string, pathPattern string) (*PostHogDemandSnapshot, error) {
	days, ok := PostHogRangeDays(demandRange)
	if !ok {
		return nil, fmt.Errorf("invalid range %q", demandRange)
	}
	if pathPattern == "" {
		pathPattern = DefaultPostHogPathLocalePattern
	}
	if err := ValidatePostHogPathPattern(pathPattern); err != nil {
		return nil, err
	}

	snap := &PostHogDemandSnapshot{
		Range:       demandRange,
		GeneratedAt: time.Now().UTC(),
	}

	// Query 1 — sessions by country x browser language.
	demandQuery := fmt.Sprintf(
		"SELECT properties.$geoip_country_code AS country, properties.$browser_language AS lang, count() AS sessions "+
			"FROM events WHERE event = '$pageview' AND timestamp > now() - INTERVAL %d DAY "+
			"GROUP BY country, lang ORDER BY sessions DESC LIMIT %d", days, posthogRowLimit)
	rows, err := c.Query(ctx, demandQuery)
	if err != nil {
		if IsPostHogAuthError(err) {
			return nil, err
		}
		snap.Warnings = append(snap.Warnings, "demand query failed: "+err.Error())
	} else {
		buildDemandBreakdown(snap, rows)
	}

	// Query 2 — 12-week trend per language.
	trendQuery := fmt.Sprintf(
		"SELECT toStartOfWeek(timestamp) AS week, properties.$browser_language AS lang, count() AS sessions "+
			"FROM events WHERE event = '$pageview' AND timestamp > now() - INTERVAL 12 WEEK "+
			"GROUP BY week, lang ORDER BY week ASC LIMIT %d", posthogRowLimit)
	trendRows, err := c.Query(ctx, trendQuery)
	if err != nil {
		if IsPostHogAuthError(err) {
			return nil, err
		}
		snap.Warnings = append(snap.Warnings, "trend query failed: "+err.Error())
	} else {
		attachTrends(snap, trendRows)
	}

	// Query 3 — served-locale outcome: does the path's locale prefix match
	// the wanted language base?
	servedQuery := fmt.Sprintf(
		"SELECT properties.$browser_language AS lang, extract(properties.$pathname, '%s') AS served, count() AS sessions "+
			"FROM events WHERE event = '$pageview' AND timestamp > now() - INTERVAL %d DAY "+
			"GROUP BY lang, served LIMIT %d", pathPattern, days, posthogRowLimit)
	servedRows, err := c.Query(ctx, servedQuery)
	if err != nil {
		if IsPostHogAuthError(err) {
			return nil, err
		}
		snap.Warnings = append(snap.Warnings, "served-locale query failed: "+err.Error())
	} else if rate, derived := servedLocaleHitRate(servedRows); derived {
		snap.ServedLocaleHitRate = &rate
	}

	return snap, nil
}

// buildDemandBreakdown fills totals, per-country, and per-language aggregates
// from (country, lang, sessions) rows.
func buildDemandBreakdown(snap *PostHogDemandSnapshot, rows [][]any) {
	type countryAgg struct {
		sessions  int64
		languages map[string]int64
	}
	countries := map[string]*countryAgg{}
	languages := map[string]int64{}

	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		country := strings.ToUpper(strings.TrimSpace(rowString(row[0])))
		if country == "" {
			country = "unknown"
		}
		lang := NormalizePostHogLangTag(rowString(row[1]))
		sessions := rowInt(row[2])
		if sessions <= 0 {
			continue
		}

		agg := countries[country]
		if agg == nil {
			agg = &countryAgg{languages: map[string]int64{}}
			countries[country] = agg
		}
		agg.sessions += sessions
		agg.languages[lang] += sessions
		languages[lang] += sessions
		snap.TotalSessions += sessions
	}

	for code, agg := range countries {
		cd := PostHogCountryDemand{CountryCode: code, Sessions: agg.sessions}
		for tag, n := range agg.languages {
			cd.Languages = append(cd.Languages, PostHogLanguageSessions{Tag: tag, Sessions: n})
		}
		sort.Slice(cd.Languages, func(i, j int) bool {
			if cd.Languages[i].Sessions != cd.Languages[j].Sessions {
				return cd.Languages[i].Sessions > cd.Languages[j].Sessions
			}
			return cd.Languages[i].Tag < cd.Languages[j].Tag
		})
		snap.Countries = append(snap.Countries, cd)
	}
	sort.Slice(snap.Countries, func(i, j int) bool {
		if snap.Countries[i].Sessions != snap.Countries[j].Sessions {
			return snap.Countries[i].Sessions > snap.Countries[j].Sessions
		}
		return snap.Countries[i].CountryCode < snap.Countries[j].CountryCode
	})

	for tag, n := range languages {
		snap.Languages = append(snap.Languages, PostHogLanguageDemand{Tag: tag, Sessions: n})
	}
	sort.Slice(snap.Languages, func(i, j int) bool {
		if snap.Languages[i].Sessions != snap.Languages[j].Sessions {
			return snap.Languages[i].Sessions > snap.Languages[j].Sessions
		}
		return snap.Languages[i].Tag < snap.Languages[j].Tag
	})
}

// attachTrends fills per-language weekly series from (week, lang, sessions)
// rows, zero-filling weeks a language has no sessions in. Weeks come from the
// query's toStartOfWeek buckets; only the trailing 12 are kept.
func attachTrends(snap *PostHogDemandSnapshot, rows [][]any) {
	perWeek := map[string]map[string]int64{} // lang -> week -> sessions
	weekSet := map[string]struct{}{}

	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		week := rowString(row[0])
		if len(week) > 10 {
			week = week[:10] // "2026-06-01T00:00:00Z" -> "2026-06-01"
		}
		if week == "" {
			continue
		}
		lang := NormalizePostHogLangTag(rowString(row[1]))
		sessions := rowInt(row[2])
		if perWeek[lang] == nil {
			perWeek[lang] = map[string]int64{}
		}
		perWeek[lang][week] += sessions
		weekSet[week] = struct{}{}
	}
	if len(weekSet) == 0 {
		return
	}

	weeks := make([]string, 0, len(weekSet))
	for w := range weekSet {
		weeks = append(weeks, w)
	}
	sort.Strings(weeks)
	if len(weeks) > 12 {
		weeks = weeks[len(weeks)-12:]
	}

	// Attach series to languages already present from the demand breakdown;
	// if that query failed, synthesize the language list from the trend data.
	if len(snap.Languages) == 0 {
		for tag := range perWeek {
			snap.Languages = append(snap.Languages, PostHogLanguageDemand{Tag: tag})
		}
		sort.Slice(snap.Languages, func(i, j int) bool { return snap.Languages[i].Tag < snap.Languages[j].Tag })
	}
	for i := range snap.Languages {
		byWeek := perWeek[snap.Languages[i].Tag]
		trend := make([]PostHogTrendPoint, 0, len(weeks))
		for _, w := range weeks {
			trend = append(trend, PostHogTrendPoint{WeekStart: w, Sessions: byWeek[w]})
		}
		snap.Languages[i].Trend = trend
	}
}

// servedLocaleHitRate computes the share of pageviews whose wanted-language
// base matches the served path-locale prefix, from (lang, served, sessions)
// rows. Not derivable when no row carries a served prefix.
func servedLocaleHitRate(rows [][]any) (float64, bool) {
	var total, hits int64
	anyServed := false
	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		lang := NormalizePostHogLangTag(rowString(row[0]))
		served := NormalizePostHogLangTag(rowString(row[1]))
		sessions := rowInt(row[2])
		if sessions <= 0 {
			continue
		}
		total += sessions
		if served != "unknown" {
			anyServed = true
			if langBase(lang) != "" && langBase(lang) == langBase(served) {
				hits += sessions
			}
		}
	}
	if total == 0 || !anyServed {
		return 0, false
	}
	return float64(hits) / float64(total), true
}

// NormalizePostHogLangTag canonicalizes a raw $browser_language value:
// lowercase language, UPPERCASE region ("pt-br" -> "pt-BR"), underscores
// treated as hyphens, Accept-Language q-junk stripped ("en-US,en;q=0.9" ->
// "en-US"), empty -> "unknown".
func NormalizePostHogLangTag(raw string) string {
	tag := strings.TrimSpace(raw)
	// Strip Accept-Language style lists and quality parameters.
	if i := strings.IndexAny(tag, ",;"); i >= 0 {
		tag = tag[:i]
	}
	tag = strings.TrimSpace(strings.ReplaceAll(tag, "_", "-"))
	if tag == "" || tag == "*" {
		return "unknown"
	}
	parts := strings.Split(tag, "-")
	out := make([]string, 0, len(parts))
	for i, p := range parts {
		if p == "" {
			continue
		}
		switch {
		case i == 0:
			out = append(out, strings.ToLower(p))
		case len(p) == 2: // region subtag
			out = append(out, strings.ToUpper(p))
		case len(p) == 4: // script subtag, e.g. Hans
			out = append(out, strings.ToUpper(p[:1])+strings.ToLower(p[1:]))
		default:
			out = append(out, strings.ToLower(p))
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return strings.Join(out, "-")
}

// langBase returns the primary-language subtag of a normalized tag.
func langBase(tag string) string {
	if tag == "unknown" {
		return ""
	}
	base, _, _ := strings.Cut(tag, "-")
	return base
}

func rowString(v any) string {
	s, _ := v.(string)
	return s
}

func rowInt(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
