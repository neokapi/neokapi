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

// misspellProfileKey adds a key the profile model does not define to the scoped
// fixture's voice profile, beside the channel vocabulary that works. The
// lenient load drops it, so a check runs exactly as it did before.
func misspellProfileKey(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".kapi", "voice.yaml")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	misspelt := strings.Replace(string(body), "  child:\n    vocabulary:\n",
		"  child:\n    vocabulary_rules:\n      forbidden_terms:\n        - term: leverage\n    vocabulary:\n", 1)
	require.NotEqual(t, string(body), misspelt, "the fixture's child channel is where the key goes")
	require.NoError(t, os.WriteFile(path, []byte(misspelt), 0o600))
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

func assertOneUnknownKey(t *testing.T, warnings []check.Warning) {
	t.Helper()
	require.Len(t, warnings, 1, "one profile governs every file, and its warning is reported once")
	w := warnings[0]
	assert.Equal(t, coreprofile.CodeUnknownKey, w.Code)
	assert.Equal(t, "channels.child.vocabulary_rules", w.Key)
	assert.True(t, strings.HasSuffix(w.Source, filepath.Join(".kapi", "voice.yaml")), w.Source)
	assert.Contains(t, w.Message, `"vocabulary_rules"`)
}

func TestCheckReportsProfileWarningsOnceAndLeavesTheOutcome(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	files := writeReadyPages(t, root, "child", "adult", "child")
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")

	clean, err := app.ComputeCheck(cmd, files)
	require.NoError(t, err)
	require.Empty(t, clean.Warnings)

	misspellProfileKey(t, root)
	warned, err := app.ComputeCheck(cmd, files)
	require.NoError(t, err)
	assertOneUnknownKey(t, warned.Warnings)

	assert.Equal(t, check.VerdictPassed, warned.Verdict)
	assert.Equal(t, clean.Verdict, warned.Verdict)
	assert.Equal(t, clean.Pass, warned.Pass)
	assert.Equal(t, clean.DidNotRun, warned.DidNotRun)
	assert.Equal(t, clean.Summary, warned.Summary)
	assert.Equal(t, clean.Gate, warned.Gate)
	assert.Equal(t, clean.Findings, warned.Findings)

	var text bytes.Buffer
	require.NoError(t, (checkReport{warned}).FormatText(&text))
	out := text.String()
	assert.Contains(t, out, "Configuration warnings (1):")
	assert.Contains(t, out, "channels.child.vocabulary_rules")
	assert.Less(t, strings.Index(out, "configured checks"), strings.Index(out, "Configuration warnings"),
		"the warnings follow the verdict, apart from the findings table")

	body, err := json.Marshal(checkReport{warned})
	require.NoError(t, err)
	assert.Contains(t, string(body), `"warnings":[{"code":"voice.unknown_key"`)
}

func TestMCPCheckToolsReportProfileWarnings(t *testing.T) {
	app, root := scopedTextCheckFixture(t)
	file := writeReadyPages(t, root, "child")[0]
	misspellProfileKey(t, root)

	_, fileReport, err := app.checkFileMCP(t.Context(), checkFileInput{File: file})
	require.NoError(t, err)
	_, draft, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Ready.", ContextPath: "child/next.json"})
	require.NoError(t, err)
	_, override, err := app.checkTextMCP(t.Context(), checkTextInput{
		Text: "Ready.", ProfileFile: filepath.Join(root, ".kapi", "voice.yaml"),
	})
	require.NoError(t, err)

	for name, report := range map[string]check.Report{
		"check_file":              fileReport,
		"check_text context_path": draft,
		"check_text profile_file": override,
	} {
		t.Run(name, func(t *testing.T) { assertOneUnknownKey(t, report.Warnings) })
	}
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
	misspellProfileKey(t, root)
	warned := run()
	assertOneUnknownKey(t, warned.Warnings)

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
	assert.Contains(t, string(body), `"warnings":[{"code":"voice.unknown_key"`)
}
