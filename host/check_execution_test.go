package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateCheckExecution points a test or a benchmark at throwaway
// configuration, cache and plugin directories, and switches discovery off.
// The data root needs nothing here: TestMain clears KAPI_DATA_DIR, so the
// binary's own temporary root answers (host.DataDir).
func isolateCheckExecution(tb testing.TB) {
	tb.Helper()
	dir := tb.TempDir()
	tb.Setenv("KAPI_NO_PROJECT", "1")
	tb.Setenv("KAPI_CONFIG_DIR", filepath.Join(dir, "config"))
	tb.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	tb.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	tb.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	tb.Setenv("KAPI_PLUGINS_DIR", filepath.Join(dir, "plugins"))
}

func executionCommand(t *testing.T) *EnvCommand {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "check")
	return cmd
}

func TestCheckExecutionNoFailDoesNotHideMissingPlugin(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"title":"Hello world"}`), 0o644))
	profilePath := filepath.Join(dir, "voice.yaml")
	require.NoError(t, os.WriteFile(profilePath, []byte("id: test\nname: Test\nexamples:\n  - after: Hello world\n"), 0o644))
	cmd := executionCommand(t)
	cmd.Flags().Bool("voice", true, "")
	cmd.Flags().Bool("no-fail", true, "")
	cmd.Flags().String("profile-file", profilePath, "")
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := (&App{SourceLang: "en"}).RunCheck(cmd, []string{src})
	require.Error(t, err)
	assert.Empty(t, out.String())
}

func TestCheckExecutionGuidanceScope(t *testing.T) {
	isolateCheckExecution(t)
	p := &profile.VoiceProfile{ID: "test", Constraints: []profile.Constraint{
		{ID: "service-facts", Kind: profile.ConstraintGuidance, Statement: "Use only approved service descriptions."},
	}}
	for _, scoped := range []bool{false, true} {
		p.Constraints[0].Scope = profile.ConstraintScope{}
		if scoped {
			p.Constraints[0].Scope.Channel = "help"
		}
		execution := newCheckExecution()
		_, err := (&App{SourceLang: "en"}).collectFileDiagnostics(t.Context(), nil, "sample.json", checkRunOptions{profile: p, execution: execution})
		require.NoError(t, err)
		found := false
		for _, run := range execution.Analyzers {
			if run.ID == "voice.guidance" {
				found = true
				assert.Equal(t, check.AnalyzerUnsupported, run.Status)
				assert.False(t, run.Required)
				assert.NotEmpty(t, run.Reason)
			}
		}
		assert.Equal(t, !scoped, found)
	}
}
