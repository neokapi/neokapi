package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/registry"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/require"
)

// Local copies of the cobra-side test fixtures (cli package) for the tests
// that moved into host with the runtime/service extraction.

func newTestApp() *App {
	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)
	aitools.RegisterAll(toolReg)
	return &App{ToolReg: toolReg}
}

func writeToolboxFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written along with fn's return value.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String(), runErr
}

// ruleCounts tallies a report's diagnostics by their stable rule id — the
// contract an AI/CI keys off.
func ruleCounts(r check.Report) map[string]int {
	m := map[string]int{}
	for _, d := range r.Findings {
		m[d.Rule]++
	}
	return m
}
