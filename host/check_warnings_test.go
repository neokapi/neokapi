package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The voice profile a check reports warnings about comes from the project's
// voice store, so a fixture states its warning in the profile and reads it in.
// The warning the store can carry is one about a VALUE, since a store holds a
// decoded profile: an unfamiliar tone is kept, rendered into the guide as
// written, and reported.

// unfamiliarProfileTone gives the scoped fixture's voice profile a formality
// outside the usual set and reads the profile into the store, which is where
// every project-resolved face loads it from.
func unfamiliarProfileTone(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".kapi", "voice.yaml")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	unfamiliar := strings.Replace(string(body), "name: Service\n",
		"name: Service\ntone:\n  formality: brisk\n", 1)
	require.NotEqual(t, string(body), unfamiliar, "the fixture's profile is where the tone goes")
	require.NoError(t, os.WriteFile(path, []byte(unfamiliar), 0o600))
	readProjectContext(t, root)
}

// writeReadyPages writes one clean page per channel directory named.
func writeReadyPages(t *testing.T, root string, channels ...string) []string {
	t.Helper()
	files := make([]string, 0, len(channels))
	for _, channel := range channels {
		file := filepath.Join(root, channel, "page"+strconv.Itoa(len(files))+".json")
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
		require.NoError(t, os.WriteFile(file, []byte(`{"body":"Ready to go."}`), 0o600))
		files = append(files, file)
	}
	return files
}

// assertOneUnfamiliarTone: the profile governing every file in the fixture is
// one profile, so its warning is reported once, and it names the store the
// profile was loaded from.
func assertOneUnfamiliarTone(t *testing.T, warnings []check.Warning) {
	t.Helper()
	assertOneUnfamiliarToneFrom(t, warnings, "store:service")
}

// assertOneUnfamiliarToneFrom is assertOneUnfamiliarTone for a face that names
// the profile itself, where the warning is about the file the caller pointed at.
func assertOneUnfamiliarToneFrom(t *testing.T, warnings []check.Warning, source string) {
	t.Helper()
	require.Len(t, warnings, 1, "one profile governs every file, and its warning is reported once")
	w := warnings[0]
	assert.Equal(t, coreprofile.CodeUnfamiliarValue, w.Code)
	assert.Equal(t, "tone.formality", w.Key)
	assert.Equal(t, source, w.Source)
	assert.Contains(t, w.Message, `"brisk"`)
}

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
