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
		"web", "docs", "contribute", "notes-internal", "analytics-events.md")
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

func TestCountBucket(t *testing.T) {
	assert.Equal(t, "0", CountBucket(0))
	assert.Equal(t, "0", CountBucket(-3))
	assert.Equal(t, "1_10", CountBucket(1))
	assert.Equal(t, "1_10", CountBucket(10))
	assert.Equal(t, "11_100", CountBucket(42))
	assert.Equal(t, "101_1000", CountBucket(500))
	assert.Equal(t, "gt_1000", CountBucket(5000))
}
