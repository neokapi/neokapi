package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
)

// `kapi exec qa docs/contacts.md --mode ai --target-lang nb` printed "pass its
// translation with --target <file>" on a command that took no --target, then
// wrote the English source over docs/nb/contacts.md through the default
// output layout. qa reads a translation and writes only findings: it pairs a
// translation file with --target, it writes no file a run does not name, and
// a run that compared no translation exits as a check that did not run.
func TestExecQANeverWritesOverTheTranslation(t *testing.T) {
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

	const source = "# Contacts\n\nEmail the support team.\n\nCall us on weekdays.\n"
	const translation = "# Kontakter\n\nSend e-post til kundestøtte.\n\nRing oss på hverdager.\n"
	root := t.TempDir()
	t.Chdir(root)
	src := filepath.Join("docs", "contacts.md")
	tgt := filepath.Join("docs", "nb", "contacts.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(tgt), 0o755))
	require.NoError(t, os.WriteFile(src, []byte(source), 0o644))

	reset := func(t *testing.T) {
		t.Helper()
		require.NoError(t, os.WriteFile(tgt, []byte(translation), 0o644))
	}
	run := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		app := &App{SourceLang: "en", Quiet: true}
		app.InitRegistries()
		defer app.Shutdown()
		execCmd := NewToolCommands(app)[0]
		execCmd.SetArgs(append([]string{"qa"}, args...))
		return captureStdout(t, func() error { return execCmd.Execute() })
	}
	// docsTree is every file under docs/, so a run that wrote one anywhere in
	// the target layout shows up as an extra entry.
	docsTree := func(t *testing.T) []string {
		t.Helper()
		var files []string
		require.NoError(t, filepath.WalkDir("docs", func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files = append(files, p)
			}
			return err
		}))
		return files
	}
	unchanged := func(t *testing.T) {
		t.Helper()
		got, err := os.ReadFile(tgt)
		require.NoError(t, err)
		assert.Equal(t, translation, string(got), "the translation's bytes are unchanged")
		assert.ElementsMatch(t, []string{src, tgt}, docsTree(t), "the run created no file")
	}

	t.Run("ai mode with no --target says what to pass and writes nothing", func(t *testing.T) {
		reset(t)
		out, err := run(t, src, "--mode", "ai", "--target-lang", "nb", "--provider", "demo")
		require.ErrorIs(t, err, ErrCheckNotRun)
		assert.Equal(t, ExitNotRun, ExitCode(nil, err))
		assert.Contains(t, err.Error(), "--target <file>")
		assert.Contains(t, out, "pass its translation with --target <file>")
		unchanged(t)
	})

	t.Run("ai mode with --target compares the pair", func(t *testing.T) {
		reset(t)
		_, err := run(t, src, "--mode", "ai", "--target-lang", "nb", "--provider", "demo", "--target", tgt)
		require.NoError(t, err)
		unchanged(t)
	})

	t.Run("rules mode with --target-lang writes nothing under the target layout", func(t *testing.T) {
		reset(t)
		out, err := run(t, src, "--mode", "rules", "--target-lang", "nb")
		require.NoError(t, err)
		assert.Contains(t, out, "qa.empty-target", "the rules report each block that holds no translation")
		unchanged(t)
	})

	t.Run("rules mode with --target compares the pair", func(t *testing.T) {
		reset(t)
		out, err := run(t, src, "--mode", "rules", "--target-lang", "nb", "--target", tgt)
		require.NoError(t, err)
		assert.NotContains(t, out, "qa.empty-target", "every block has its translation")
		unchanged(t)
	})

	t.Run("--target writes no file, so -o is refused", func(t *testing.T) {
		reset(t)
		_, err := run(t, src, "--target-lang", "nb", "--target", tgt, "-o", filepath.Join("out", "{name}{ext}"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writes no file")
		unchanged(t)
		assert.NoDirExists(t, "out")
	})
}
