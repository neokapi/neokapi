//go:build !js

package host

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckReportsProfileWarningsOnceAndLeavesTheOutcome(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	files := writeReadyPages(t, root, "child", "adult", "child")
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")

	clean, err := app.ComputeCheck(cmd, files)
	require.NoError(t, err)
	require.Empty(t, clean.Warnings)

	unfamiliarProfileTone(t, root)
	warned, err := app.ComputeCheck(cmd, files)
	require.NoError(t, err)
	assertOneUnfamiliarTone(t, warned.Warnings)

	assert.Equal(t, check.VerdictPassed, warned.Verdict)
	assert.Equal(t, clean.Verdict, warned.Verdict)
	assert.Equal(t, clean.Pass, warned.Pass)
	assert.Equal(t, clean.DidNotRun, warned.DidNotRun)
	assert.Equal(t, clean.Summary, warned.Summary)
	assert.Equal(t, clean.Summary, warned.Summary)
	assert.Equal(t, clean.Findings, warned.Findings)

	var text bytes.Buffer
	require.NoError(t, (checkReport{warned}).FormatText(&text))
	out := text.String()
	assert.Contains(t, out, "Configuration warnings (1):")
	assert.Contains(t, out, "tone.formality")
	assert.Less(t, strings.Index(out, "configured checks"), strings.Index(out, "Configuration warnings"),
		"the warnings follow the verdict, apart from the findings table")

	body, err := json.Marshal(checkReport{warned})
	require.NoError(t, err)
	assert.Contains(t, string(body), `"warnings":[{"code":"voice.unfamiliar_value"`)
}

func TestMCPCheckToolsReportProfileWarnings(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	file := writeReadyPages(t, root, "child")[0]
	unfamiliarProfileTone(t, root)

	_, fileReport, err := app.checkFileMCP(t.Context(), checkFileInput{File: file})
	require.NoError(t, err)
	_, draft, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Ready.", ContextPath: "child/next.json"})
	require.NoError(t, err)
	_, override, err := app.checkTextMCP(t.Context(), checkTextInput{
		Text: "Ready.", ProfileFile: filepath.Join(root, ".kapi", "voice.yaml"),
	})
	require.NoError(t, err)

	// The two project-resolved faces load the profile the store holds. The
	// third names a file, and the warning is about the file it was handed.
	for name, report := range map[string]check.Report{
		"check_file":              fileReport,
		"check_text context_path": draft,
	} {
		t.Run(name, func(t *testing.T) { assertOneUnfamiliarTone(t, report.Warnings) })
	}
	t.Run("check_text profile_file", func(t *testing.T) {
		assertOneUnfamiliarToneFrom(t, override.Warnings,
			displayRelative(filepath.Join(root, ".kapi", "voice.yaml")))
	})
}

func TestShipCheckReportsProfileWarningsOnce(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	writeReadyPages(t, root, "child", "adult")
	run := func() verifyOutput {
		t.Helper()
		cmd := NewEnvCommand(t.Context(), "verify")
		AddProjectFlag(cmd)
		AddVerifyFlags(cmd)
		require.NoError(t, cmd.Flags().Set(projectFlagName, filepath.Join(root, "kapi.yaml")))
		out, err := app.computeVerify(cmd, nil)
		require.NoError(t, err)
		return out
	}

	clean := run()
	require.Empty(t, clean.Warnings)
	unfamiliarProfileTone(t, root)
	warned := run()
	assertOneUnfamiliarTone(t, warned.Warnings)

	assert.Equal(t, clean.Verdict, warned.Verdict)
	assert.Equal(t, clean.Pass, warned.Pass)
	assert.Equal(t, clean.Summary, warned.Summary)
	require.Len(t, warned.Gates, len(clean.Gates))
	for i := range clean.Gates {
		assert.Equal(t, clean.Gates[i].Verdict, warned.Gates[i].Verdict, clean.Gates[i].Gate)
		assert.Equal(t, clean.Gates[i].Findings, warned.Gates[i].Findings, clean.Gates[i].Gate)
	}

	var text bytes.Buffer
	require.NoError(t, warned.FormatText(&text))
	assert.Contains(t, text.String(), "Configuration warnings (1):")
	body, err := json.Marshal(warned)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"warnings":[{"code":"voice.unfamiliar_value"`)
}
