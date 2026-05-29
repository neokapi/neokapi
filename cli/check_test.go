package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_BilingualFindings runs `kapi check <source> <target>` over a
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
	require.NoError(t, cmd.Flags().Set("target-lang", "de"))
	require.NoError(t, cmd.Flags().Set("dnt", "Acme Cloud"))

	out, err := a.computeCheck(cmd, []string{src, tgt})
	require.NoError(t, err)

	assert.False(t, out.Pass, "gate must fail on critical findings")
	cats := map[string]int{}
	for _, f := range out.Findings {
		cats[f.Category]++
	}
	assert.Positive(t, cats["placeholder"], "should flag the dropped {name} placeholder")
	assert.Positive(t, cats["do-not-translate"], "should flag the translated do-not-translate term")
	assert.GreaterOrEqual(t, out.Summary.Critical, 2)
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
	require.NoError(t, cmd.Flags().Set("target-lang", "de"))
	require.NoError(t, cmd.Flags().Set("dnt", "Acme Cloud"))

	out, err := a.computeCheck(cmd, []string{src, tgt})
	require.NoError(t, err)
	assert.True(t, out.Pass, "faithful target should pass: %+v", out.Findings)
	assert.Empty(t, out.Findings)
}
