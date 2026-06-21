package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckTextMCP proves the check_text MCP tool returns a kapi.check/v1 Report
// — the verifier half of the author→check→revise loop — flagging a doubled word
// from the default hygiene checkset.
func TestCheckTextMCP(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := &App{SourceLang: "en"}

	_, report, err := a.checkTextMCP(context.Background(), CheckTextInput{Text: "We we shipped it"})
	require.NoError(t, err)
	assert.Equal(t, "kapi.check/v1", report.Schema)
	assert.Equal(t, "text", report.Target.Kind)
	assert.Positive(t, ruleCounts(report)["hygiene.doubled-word"], "doubled word must be flagged: %+v", report.Findings)
}

// TestCheckTextMCP_ForbidPattern proves the forbidden-pattern check threads
// through the MCP tool and trips the gate.
func TestCheckTextMCP_ForbidPattern(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	a := &App{SourceLang: "en"}

	_, report, err := a.checkTextMCP(context.Background(), CheckTextInput{
		Text:     "ship it TODO before launch",
		Forbid:   []string{"(?i)todo"},
		MaxWords: 0,
	})
	require.NoError(t, err)
	assert.Positive(t, ruleCounts(report)["pattern.forbidden-pattern"], "forbidden TODO must be flagged: %+v", report.Findings)
}

// TestCheckFileMCP proves check_file reads a file format-aware and returns a
// Report with per-block locations.
func TestCheckFileMCP(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"body": "This source string is far too long for the configured limit"}`), 0o644))

	a := &App{SourceLang: "en"}
	_, report, err := a.checkFileMCP(context.Background(), CheckFileInput{File: src, MaxChars: 10})
	require.NoError(t, err)
	assert.Equal(t, "file", report.Target.Kind)
	assert.Positive(t, report.Target.Blocks)
	assert.Positive(t, ruleCounts(report)["length.max-chars-exceeded"], "over-long body must be flagged: %+v", report.Findings)
	// The located finding points at the block so an assistant knows where to fix.
	require.NotEmpty(t, report.Findings)
	assert.NotEmpty(t, report.Findings[0].Location.Block)
}
