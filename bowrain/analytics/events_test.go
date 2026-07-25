package analytics

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eventConstants parses events.go and returns the string value of every
// exported Event* constant, so the drift gate can never miss a new event.
func eventConstants(t *testing.T) map[string]string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	src := filepath.Join(filepath.Dir(thisFile), "events.go")

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, src, nil, 0)
	require.NoError(t, err)

	events := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Event") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				events[name.Name] = val
			}
		}
	}
	require.NotEmpty(t, events, "no Event* constants found in events.go")
	return events
}

// TestEventReferenceDocDrift asserts that every event constant defined in
// events.go is documented in the analytics event reference. Adding an event
// without updating the doc fails this test (epic 018 drift gate).
func TestEventReferenceDocDrift(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	// bowrain/analytics → repo root is two levels up.
	docPath := filepath.Join(filepath.Dir(thisFile), "..", "..",
		"web", "docs", "contribute", "implementation", "analytics-events.md")
	data, err := os.ReadFile(docPath)
	require.NoError(t, err,
		"analytics event reference doc missing; every event constant must be documented there")
	doc := string(data)

	for constName, event := range eventConstants(t) {
		assert.Contains(t, doc, "`"+event+"`",
			"event %s (%q) is not documented in %s — add it to the reference table",
			constName, event, docPath)
	}
}

// TestEventNamesAreSnakeCaseDomainAction asserts the taxonomy naming rule.
func TestEventNamesAreSnakeCaseDomainAction(t *testing.T) {
	for constName, event := range eventConstants(t) {
		assert.NotEmpty(t, event, constName)
		for _, r := range event {
			isValid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
			assert.True(t, isValid,
				"event %s (%q) must be snake_case (got rune %q)", constName, event, r)
		}
		assert.Contains(t, event, "_",
			"event %s (%q) must be domain_action", constName, event)
	}
}

func TestProps(t *testing.T) {
	assert.Empty(t, Props("", ""))
	assert.Equal(t, map[string]any{PropWorkspaceID: "ws1"}, Props("ws1", ""))
	assert.Equal(t, map[string]any{PropProjectID: "p1"}, Props("", "p1"))
	assert.Equal(t,
		map[string]any{PropWorkspaceID: "ws1", PropProjectID: "p1"},
		Props("ws1", "p1"))
}

func TestDurationBucket(t *testing.T) {
	assert.Equal(t, "lt_1s", DurationBucket(200*time.Millisecond))
	assert.Equal(t, "1s_5s", DurationBucket(2*time.Second))
	assert.Equal(t, "5s_30s", DurationBucket(10*time.Second))
	assert.Equal(t, "30s_2m", DurationBucket(time.Minute))
	assert.Equal(t, "2m_10m", DurationBucket(5*time.Minute))
	assert.Equal(t, "gt_10m", DurationBucket(time.Hour))
}

// TestCreditBucket walks every boundary: the grant-sizing read ("what share of
// first runs fits inside the 200K grant?") is only sound if the edges are exact.
func TestCreditBucket(t *testing.T) {
	cases := []struct {
		credits int64
		want    string
	}{
		{-1, "0"}, {0, "0"},
		{1, "1_1k"}, {1_000, "1_1k"},
		{1_001, "1k_10k"}, {10_000, "1k_10k"},
		{10_001, "10k_50k"}, {50_000, "10k_50k"},
		{50_001, "50k_100k"}, {100_000, "50k_100k"},
		{100_001, "100k_200k"}, {200_000, "100k_200k"},
		{200_001, "200k_500k"}, {500_000, "200k_500k"},
		{500_001, "500k_1m"}, {1_000_000, "500k_1m"},
		{1_000_001, "gt_1m"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, CreditBucket(tc.credits), "credits=%d", tc.credits)
	}
}

func TestPercentBucket(t *testing.T) {
	cases := []struct {
		pct  int
		want string
	}{
		{-5, "0"}, {0, "0"},
		{1, "1_25"}, {25, "1_25"},
		{26, "26_50"}, {50, "26_50"},
		{51, "51_75"}, {75, "51_75"},
		{76, "76_99"}, {99, "76_99"},
		{100, "100"}, {150, "100"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, PercentBucket(tc.pct), "pct=%d", tc.pct)
	}
}

func TestSharePercentBucket(t *testing.T) {
	assert.Equal(t, "0", SharePercentBucket(0, 0), "undefined share reads as 0")
	assert.Equal(t, "0", SharePercentBucket(5, 0), "undefined share reads as 0")
	assert.Equal(t, "0", SharePercentBucket(0, 10), "no TM leverage")
	assert.Equal(t, "100", SharePercentBucket(10, 10), "all work came from TM")
	assert.Equal(t, "100", SharePercentBucket(11, 10), "clamped above the whole")
	assert.Equal(t, "1_25", SharePercentBucket(1, 1000), "a tiny share never reads as none")
	assert.Equal(t, "1_25", SharePercentBucket(25, 100))
	assert.Equal(t, "26_50", SharePercentBucket(1, 2))
	assert.Equal(t, "51_75", SharePercentBucket(3, 4))
	assert.Equal(t, "76_99", SharePercentBucket(9, 10))
	assert.Equal(t, "76_99", SharePercentBucket(999, 1000), "a near-complete share never reads as all")
}

func TestCountBucket(t *testing.T) {
	assert.Equal(t, "0", CountBucket(0))
	assert.Equal(t, "0", CountBucket(-3))
	assert.Equal(t, "1_10", CountBucket(1))
	assert.Equal(t, "1_10", CountBucket(10))
	assert.Equal(t, "11_100", CountBucket(42))
	assert.Equal(t, "101_1000", CountBucket(500))
	assert.Equal(t, "gt_1000", CountBucket(5000))
}
