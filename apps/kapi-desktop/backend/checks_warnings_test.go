package backend

import (
	"os"
	"path/filepath"
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunChecksReportsProfileWarningsWithoutChangingTheVerdict gives the panel
// the profile setupCheckProject writes, with one misspelt key added. The load
// drops the key, so the run is the same run, and the panel carries the key as a
// warning beside a verdict and score it leaves alone.
func TestRunChecksReportsProfileWarningsWithoutChangingTheVerdict(t *testing.T) {
	app := NewApp()
	tabID, src := setupCheckProject(t, app, `{"greeting":"Hello world"}`)
	clean, err := app.RunChecks(tabID, ProjectFilter{})
	require.NoError(t, err)
	require.Empty(t, clean.Warnings)

	profile := filepath.Join(filepath.Dir(filepath.Dir(src)), "voice.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(`id: house
name: House Style
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: major
  preffered_terms:
    - term: dashboard
`), 0o644))
	warned, err := app.RunChecks(tabID, ProjectFilter{})
	require.NoError(t, err)

	require.Len(t, warned.Warnings, 1)
	w := warned.Warnings[0]
	assert.Equal(t, coreprofile.CodeUnknownKey, w.Code)
	assert.Equal(t, "vocabulary.preffered_terms", w.Key)
	assert.Equal(t, "voice.yaml", filepath.Base(w.Source))

	assert.Equal(t, "passed", warned.Verdict)
	assert.Equal(t, clean.Verdict, warned.Verdict)
	assert.Equal(t, clean.Pass, warned.Pass)
	assert.Equal(t, clean.Score, warned.Score)
	assert.Equal(t, clean.Files, warned.Files)
}
