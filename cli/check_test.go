package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ruleCounts tallies a report's diagnostics by their stable rule id — the
// contract an AI/CI keys off.
func ruleCounts(r check.Report) map[string]int {
	m := map[string]int{}
	for _, d := range r.Findings {
		m[d.Rule]++
	}
	return m
}

// TestCheck_BilingualFindings runs `kapi check <source> --target <target>` over a
// JSON pair where the target drops a placeholder and translates a
// do-not-translate term, and asserts the gate fails with both findings.
func TestCheck_BilingualFindings(t *testing.T) {
	// Isolate from the in-repo dogfood project and any $TMPDIR recipe pollution.
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	tgt := filepath.Join(dir, "app.de.json")
	require.NoError(t, os.WriteFile(src, []byte(`{
  "greeting": "Hello {name}, open Acme Cloud",
  "bye": "Goodbye"
}
`), 0o644))
	// Target drops {name} (critical placeholder) and translates "Acme Cloud"
	// (critical do-not-translate).
	require.NoError(t, os.WriteFile(tgt, []byte(`{
  "greeting": "Hallo, öffne Akme-Wolke",
  "bye": "Tschüss"
}
`), 0o644))

	a := &App{SourceLang: "en"}
	cmd := a.NewCheckCmd()
	require.NoError(t, cmd.Flags().Set("target", tgt))
	require.NoError(t, cmd.Flags().Set("target-lang", "de"))
	require.NoError(t, cmd.Flags().Set("dnt", "Acme Cloud"))

	out, err := a.computeCheck(cmd, []string{src})
	require.NoError(t, err)

	assert.False(t, out.Pass, "gate must fail on critical findings")
	counts := ruleCounts(out)
	assert.Positive(t, counts["placeholder.placeholder"], "should flag the dropped {name} placeholder: %+v", out.Findings)
	assert.Positive(t, counts["dnt.do-not-translate"], "should flag the translated do-not-translate term: %+v", out.Findings)
	assert.GreaterOrEqual(t, out.Summary.Critical, 2)
}

// TestCheck_MonolingualSourceChecks runs `kapi check <source>` with no target
// and source-side limits (--max-chars, --forbid), asserting the length and
// forbidden-pattern findings surface for the single file.
func TestCheck_MonolingualSourceChecks(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{
  "title": "Short",
  "body": "This is a fairly long source string TODO rewrite it before launch"
}
`), 0o644))

	a := &App{SourceLang: "en"}
	cmd := a.NewCheckCmd()
	require.NoError(t, cmd.Flags().Set("max-chars", "20"))
	require.NoError(t, cmd.Flags().Set("forbid", "(?i)todo"))

	out, err := a.computeCheck(cmd, []string{src})
	require.NoError(t, err)

	counts := ruleCounts(out)
	assert.Positive(t, counts["length.max-chars-exceeded"], "should flag the over-long body: %+v", out.Findings)
	assert.Positive(t, counts["pattern.forbidden-pattern"], "should flag the TODO marker in source: %+v", out.Findings)
	// "Short" (5 chars) stays under the limit and is clean: exactly one length finding.
	assert.Equal(t, 1, counts["length.max-chars-exceeded"])
}

// TestCheck_MonolingualCleanSourcePasses confirms a single clean file with
// source-side limits passes and exits with no findings.
func TestCheck_MonolingualCleanSourcePasses(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"title": "Crisp copy"}`), 0o644))

	a := &App{SourceLang: "en"}
	cmd := a.NewCheckCmd()
	require.NoError(t, cmd.Flags().Set("max-chars", "200"))
	require.NoError(t, cmd.Flags().Set("forbid", "(?i)todo"))

	out, err := a.computeCheck(cmd, []string{src})
	require.NoError(t, err)
	assert.True(t, out.Pass, "clean source should pass: %+v", out.Findings)
	assert.Empty(t, out.Findings)
}

// TestCheck_HygieneAlwaysRuns proves the content-lint hygiene checker is part of
// the default checkset (no flags): doubled words surface as a hygiene finding.
func TestCheck_HygieneAlwaysRuns(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"body": "We we shipped it"}`), 0o644))

	a := &App{SourceLang: "en"}
	cmd := a.NewCheckCmd()
	out, err := a.computeCheck(cmd, []string{src})
	require.NoError(t, err)
	assert.Positive(t, ruleCounts(out)["hygiene.doubled-word"], "the doubled word must be flagged by default: %+v", out.Findings)
}

// TestCheck_MonolingualGateOnMajor confirms that source-side findings (which are
// SeverityMajor, not critical) clear the default critical-only gate but fail once
// the caller tightens it with --max-major 0 — the way teams actually gate on
// source quality.
func TestCheck_MonolingualGateOnMajor(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"body": "This source string is far too long for the limit"}`), 0o644))

	// Default gate is critical-only: a major length finding still passes.
	a := &App{SourceLang: "en"}
	def := a.NewCheckCmd()
	require.NoError(t, def.Flags().Set("max-chars", "10"))
	defOut, err := a.computeCheck(def, []string{src})
	require.NoError(t, err)
	assert.Positive(t, defOut.Summary.Major, "the over-long body should be a major finding: %+v", defOut.Findings)
	assert.Zero(t, defOut.Summary.Critical)
	assert.True(t, defOut.Pass, "the default critical-only gate passes on a major finding")

	// --max-major 0 tightens the gate: the same major finding now fails it.
	gated := a.NewCheckCmd()
	require.NoError(t, gated.Flags().Set("max-chars", "10"))
	require.NoError(t, gated.Flags().Set("max-major", "0"))
	gatedOut, err := a.computeCheck(gated, []string{src})
	require.NoError(t, err)
	assert.False(t, gatedOut.Pass, "--max-major 0 must gate on the major length finding")
}

// TestCheck_MonolingualInvalidForbidPattern confirms a bad --forbid regex is an
// operational error rather than a silent no-op.
func TestCheck_MonolingualInvalidForbidPattern(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"title": "Copy"}`), 0o644))

	a := &App{SourceLang: "en"}
	cmd := a.NewCheckCmd()
	require.NoError(t, cmd.Flags().Set("forbid", "[invalid"))

	_, err := a.computeCheck(cmd, []string{src})
	require.Error(t, err)
}

// TestCheck_CleanTargetPasses runs the same checks over a faithful target and
// asserts a clean pass.
func TestCheck_CleanTargetPasses(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	tgt := filepath.Join(dir, "app.de.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting": "Hello {name}, open Acme Cloud"}`), 0o644))
	require.NoError(t, os.WriteFile(tgt, []byte(`{"greeting": "Hallo {name}, öffne Acme Cloud"}`), 0o644))

	a := &App{SourceLang: "en"}
	cmd := a.NewCheckCmd()
	require.NoError(t, cmd.Flags().Set("target", tgt))
	require.NoError(t, cmd.Flags().Set("target-lang", "de"))
	require.NoError(t, cmd.Flags().Set("dnt", "Acme Cloud"))

	out, err := a.computeCheck(cmd, []string{src})
	require.NoError(t, err)
	assert.True(t, out.Pass, "faithful target should pass: %+v", out.Findings)
	assert.Empty(t, out.Findings)
}
