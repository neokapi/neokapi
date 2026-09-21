package backend

import (
	"strings"
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunChecksReportsProfileWarningsWithoutChangingTheVerdict gives the panel
// the profile setupCheckProject writes, with a tone register outside the usual
// set. The value is kept as written, so the run is the same run, and the panel
// carries the register as a warning beside a verdict and score it leaves alone.
//
// The warning names the profile where the run resolved it, which is the
// project's store.
func TestRunChecksReportsProfileWarningsWithoutChangingTheVerdict(t *testing.T) {
	clean := NewApp()
	cleanTab, _ := setupCheckProjectVoiced(t, clean, `{"greeting":"Hello world"}`, houseVoiceYAML)
	cleanRes, err := clean.RunChecks(cleanTab, ProjectFilter{})
	require.NoError(t, err)
	require.Empty(t, cleanRes.Warnings)

	warnedApp := NewApp()
	warnedTab, _ := setupCheckProjectVoiced(t, warnedApp, `{"greeting":"Hello world"}`,
		houseVoiceYAML+"tone:\n  formality: sardonic\n")
	warned, err := warnedApp.RunChecks(warnedTab, ProjectFilter{})
	require.NoError(t, err)

	require.Len(t, warned.Warnings, 1)
	w := warned.Warnings[0]
	assert.Equal(t, coreprofile.CodeUnfamiliarValue, w.Code)
	assert.Equal(t, "tone.formality", w.Key)
	assert.True(t, strings.HasPrefix(w.Source, "store:"),
		"the warning names the store the profile was resolved from, got %q", w.Source)

	assert.Equal(t, "passed", warned.Verdict)
	assert.Equal(t, cleanRes.Verdict, warned.Verdict)
	assert.Equal(t, cleanRes.Pass, warned.Pass)
	assert.Equal(t, cleanRes.Score, warned.Score)
	require.Len(t, warned.Files, len(cleanRes.Files))
	for i := range warned.Files {
		assert.Equal(t, cleanRes.Files[i].Findings, warned.Files[i].Findings)
	}
}
