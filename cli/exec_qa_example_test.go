package cli

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
	"github.com/neokapi/neokapi/host"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The help example that names a provider for `kapi exec qa` has to select the
// AI review with --mode ai. qa defaults to the rule checks, which call no
// model. An example that carries only --provider prints rule findings, and a
// reader who copies it takes them for an AI review.
func TestExecQualityCheckHelpExampleRunsTheAIReview(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("KAPI_NO_PROJECT", "1")
	for _, vars := range providerenv.Vars {
		for _, v := range vars {
			t.Setenv(v, "")
		}
	}
	t.Setenv("KAPI_AI_PROVIDER", "")
	t.Setenv("KAPI_AI_MODEL", "")

	const xliff = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.2" version="2.2" srcLang="en" trgLang="fr">
  <file id="app.json" original="app.json">
    <unit id="greeting">
      <segment>
        <source>Hello, World!</source>
        <target>Bonjour  le monde !</target>
      </segment>
    </unit>
  </file>
</xliff>
`
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("app.xliff", []byte(xliff), 0o644))

	// providerExample is the example line that names a provider, with the
	// demo provider in place of the cloud one so the run needs no key.
	var providerExample []string
	for line := range strings.SplitSeq(host.ToolExamples["qa"], "\n") {
		fields := strings.Fields(line)
		if !strings.Contains(line, "--provider") {
			continue
		}
		require.Equal(t, []string{"kapi", "exec", "qa"}, fields[:3], "the example mounts under kapi exec qa")
		require.Nil(t, providerExample, "one qa example names a provider")
		providerExample = fields[3:]
		for i, f := range providerExample {
			if f == "--provider" && i+1 < len(providerExample) {
				providerExample[i+1] = "demo"
			}
		}
	}
	require.NotNil(t, providerExample, "a qa example names a provider")
	assert.Contains(t, strings.Join(providerExample, " "), "--mode ai")

	// calls records the provider of each model call the run makes.
	run := func(t *testing.T, args ...string) (string, []string, error) {
		t.Helper()
		var (
			mu    sync.Mutex
			calls []string
		)
		remove := aiprovider.AddRecorder(func(_ context.Context, ex aiprovider.Exchange) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, ex.Provider)
		})
		defer remove()
		app := &App{SourceLang: "en", Quiet: true}
		app.InitRegistries()
		defer app.Shutdown()
		execCmd := NewToolCommands(app)[0]
		execCmd.SetArgs(append([]string{"qa"}, args...))
		out, err := captureStdout(t, func() error { return execCmd.Execute() })
		mu.Lock()
		defer mu.Unlock()
		return out, calls, err
	}

	t.Run("the example asks the demo provider for a review", func(t *testing.T) {
		out, calls, err := run(t, providerExample...)
		require.NoError(t, err)
		assert.NotEmpty(t, calls, "the AI review calls the model")
		for _, p := range calls {
			assert.Equal(t, "demo", p)
		}
		assert.NotContains(t, out, "qa.double-spaces", "the rule checks did not run")
	})

	t.Run("without --mode the rule checks run and call no model", func(t *testing.T) {
		out, calls, err := run(t, "app.xliff", "--target-lang", "fr", "--provider", "demo")
		require.NoError(t, err)
		assert.Empty(t, calls)
		assert.Contains(t, out, "qa.double-spaces")
	})

	t.Run("--check-terminology is not a flag", func(t *testing.T) {
		_, _, err := run(t, "app.xliff", "--target-lang", "fr", "--check-terminology")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown flag: --check-terminology")
		assert.Equal(t, ExitError, ExitCode(nil, err))
	})
}
