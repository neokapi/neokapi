package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
)

// `kapi exec review docs/contacts.md --target-lang nb --provider demo` printed
// nothing and exited 0. It wrote the English source over docs/nb/contacts.md
// through the default output layout. review reads a translation and writes
// findings about it. It pairs a translation file with --target and writes no
// file a run does not name. A run that read no translation exits as a check
// that did not run.
func TestExecReviewNeverWritesOverTheTranslation(t *testing.T) {
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
	const bundle = `{
  "schemaVersion": "1.0",
  "kind": "kapi-bundle",
  "generator": {"id": "test", "version": "1.0"},
  "project": {"id": "review", "sourceLocale": "en"},
  "documents": [{
    "id": "app", "documentType": "json", "path": "app.json",
    "blocks": [
      {"id": "greeting", "translatable": true, "type": "json:value",
       "source": [{"text": "Hello"}], "targets": {"nb": [{"text": "Hei"}]}},
      {"id": "farewell", "translatable": true, "type": "json:value",
       "source": [{"text": "Goodbye"}], "targets": {"nb": [{"text": "Ha det"}]}}
    ]
  }]
}
`
	root := t.TempDir()
	t.Chdir(root)
	src := filepath.Join("docs", "contacts.md")
	tgt := filepath.Join("docs", "nb", "contacts.md")
	kbfIn := filepath.Join("docs", "app.kbf.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(tgt), 0o755))
	require.NoError(t, os.WriteFile(src, []byte(source), 0o644))

	reset := func(t *testing.T) {
		t.Helper()
		require.NoError(t, os.WriteFile(tgt, []byte(translation), 0o644))
		require.NoError(t, os.WriteFile(kbfIn, []byte(bundle), 0o644))
		require.NoError(t, os.RemoveAll("out"))
	}
	run := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		app := &App{SourceLang: "en", Quiet: true}
		app.InitRegistries()
		defer app.Shutdown()
		execCmd := NewToolCommands(app)[0]
		execCmd.SetArgs(append([]string{"review", "--provider", "demo"}, args...))
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
	bytesOf := func(t *testing.T, p string) string {
		t.Helper()
		got, err := os.ReadFile(p)
		require.NoError(t, err)
		return string(got)
	}
	unchanged := func(t *testing.T) {
		t.Helper()
		assert.Equal(t, source, bytesOf(t, src), "the source's bytes are unchanged")
		assert.Equal(t, translation, bytesOf(t, tgt), "the translation's bytes are unchanged")
		assert.Equal(t, bundle, bytesOf(t, kbfIn), "the bundle's bytes are unchanged")
		assert.ElementsMatch(t, []string{src, tgt, kbfIn}, docsTree(t), "the run created no file under docs/")
	}

	t.Run("the source alone says what to pass and writes nothing", func(t *testing.T) {
		reset(t)
		out, err := run(t, src, "--target-lang", "nb")
		require.ErrorIs(t, err, ErrCheckNotRun)
		assert.Equal(t, ExitNotRun, ExitCode(nil, err))
		assert.Contains(t, err.Error(), "review read no nb translation")
		assert.Contains(t, out, "pass its translation with --target <file>")
		unchanged(t)
	})

	t.Run("--target reviews the pair and reports each block", func(t *testing.T) {
		reset(t)
		out, err := run(t, src, "--target-lang", "nb", "--target", tgt)
		require.NoError(t, err)
		assert.Contains(t, out, "review.review", "the run reports what the review found")
		assert.Contains(t, out, "contacts/p", "each reviewed block is named")
		unchanged(t)
	})

	t.Run("--target with --json prints the findings report", func(t *testing.T) {
		reset(t)
		out, err := run(t, src, "--target-lang", "nb", "--target", tgt, "--json")
		require.NoError(t, err)
		assert.Contains(t, out, `"rule": "review.review"`)
		unchanged(t)
	})

	t.Run("a bundle holding its translations is reviewed and left as it was", func(t *testing.T) {
		reset(t)
		out, err := run(t, kbfIn, "--target-lang", "nb")
		require.NoError(t, err)
		assert.Contains(t, out, "review.review")
		assert.Contains(t, out, "greeting")
		unchanged(t)
	})

	t.Run("a bundle is written only where -o names a file", func(t *testing.T) {
		reset(t)
		_, err := run(t, kbfIn, "--target-lang", "nb", "-o", filepath.Join("out", "{name}.{ext}"))
		require.NoError(t, err)
		unchanged(t)
		written, err := filepath.Glob(filepath.Join("out", "*"))
		require.NoError(t, err)
		require.Equal(t, []string{filepath.Join("out", "app.kbf.json")}, written, "the run wrote the one file -o names")
		got := bytesOf(t, written[0])
		assert.Contains(t, got, `"text": "Hei"`, "the translations are kept as they were")
		assert.Contains(t, got, `"text": "Ha det"`, "the translations are kept as they were")
	})
}
