package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end over the generated `kapi exec <check>` command: a check tool writes
// its verdict into a stand-off annotation no writer serializes, so before the
// findings collector was wired an exec run of one printed nothing and exited 0 —
// a do-not-translate check that reported reassurance (#1476). This drives the
// real cobra command so the *wiring* is what's under test, not the collector.

const dntXLIFF = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="nb" datatype="plaintext" original="app">
    <body>
      <trans-unit id="save">
        <source>Save now with Acme Cloud</source>
        <target>Lagre nå med Toppskyen</target>
      </trans-unit>
    </body>
  </file>
</xliff>
`

// runExecDNT runs `kapi exec dnt-check` over a fixture and returns stdout.
func runExecDNT(t *testing.T, target string, args ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.xlf")
	require.NoError(t, os.WriteFile(path, []byte(target), 0o644))

	// A fully-wired App: the run reads a real XLIFF through the real format
	// registry, so what is under test is the command, not a stub.
	app := &App{SourceLang: "en", Quiet: true}
	app.InitRegistries()
	execCmd := NewToolCommands(app)[0]
	require.NotNil(t, execChildren(t, app)["dnt-check"], "expected `exec dnt-check`")
	execCmd.SetArgs(append([]string{"dnt-check", path}, args...))

	out, err := captureStdout(t, func() error { return execCmd.Execute() })
	require.NoError(t, err)
	return out
}

func TestExecCheckReportsItsFindings(t *testing.T) {
	out := runExecDNT(t, dntXLIFF, "--target-lang", "nb", "--terms", "Acme Cloud")
	assert.Contains(t, out, "CRITICAL",
		"an exec run of a check must report what it found, not exit 0 in silence")
	assert.Contains(t, out, "Acme Cloud")
	assert.Contains(t, out, "1 finding(s) (1 critical")
}

// And a clean run says so — "printed nothing" and "found nothing" must not look
// the same from the outside.
func TestExecCheckSaysWhenThereIsNothingToReport(t *testing.T) {
	out := runExecDNT(t, dntXLIFF, "--target-lang", "nb", "--terms", "Nonexistent")
	assert.Contains(t, out, "No findings.")
	assert.NotContains(t, out, "CRITICAL")
}

// A check that also writes a file must still report. `qa` declares WritesOutput,
// and the collector used to be fed only from the branch taken when a run
// produced *no* output file — so every `exec qa` run reported "No findings."
// whatever it found, which is worse than silence: it is a clean bill of health
// for content nobody checked.
func TestExecCheckThatWritesOutputStillReports(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.xlf")
	require.NoError(t, os.WriteFile(path, []byte(doubledSpaceXLIFF), 0o644))

	app := &App{SourceLang: "en", Quiet: true}
	app.InitRegistries()
	execCmd := NewToolCommands(app)[0]
	execCmd.SetArgs([]string{"qa", path, "--target-lang", "nb", "-o", filepath.Join(dir, "out.xlf")})

	out, err := captureStdout(t, func() error { return execCmd.Execute() })
	require.NoError(t, err)
	assert.Contains(t, out, "double-spaces",
		"a check tool that also writes a file must still report its findings")
	assert.FileExists(t, filepath.Join(dir, "out.xlf"), "and the file is still written")
}

const doubledSpaceXLIFF = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="nb" datatype="plaintext" original="bad">
    <body>
      <trans-unit id="doubled">
        <source>Save now.</source>
        <target>Lagre  nå.</target>
      </trans-unit>
    </body>
  </file>
</xliff>
`
