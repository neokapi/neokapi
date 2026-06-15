package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExpandInputsSkipsJunkFiles verifies that editor lock/metadata stubs
// (Office "~$…" owner files, macOS "._…" AppleDouble files) are dropped
// whether they are named explicitly / via shell glob expansion or discovered
// during a recursive walk — and that skipping them is silent (no onSkip
// callback, so the exit status stays 0).
func TestExpandInputsSkipsJunkFiles(t *testing.T) {
	dir := t.TempDir()
	good := writeToolboxFile(t, dir, "report.docx", "irrelevant")
	officeLock := writeToolboxFile(t, dir, "~$report.docx", "lock stub")
	appleDouble := writeToolboxFile(t, dir, "._report.docx", "metadata stub")

	t.Run("explicit args (shell-expanded glob)", func(t *testing.T) {
		var skipped []string
		files, err := expandInputs([]string{good, officeLock, appleDouble}, false, func(p string, _ error) {
			skipped = append(skipped, p)
		})
		require.NoError(t, err)
		assert.Equal(t, []string{good}, files, "only the real document survives")
		assert.Empty(t, skipped, "junk files are dropped silently, not reported as skips")
	})

	t.Run("recursive walk", func(t *testing.T) {
		files, err := expandInputs([]string{dir}, true, func(string, error) {
			t.Fatal("recursive walk must not report junk as a skip")
		})
		require.NoError(t, err)
		assert.Equal(t, []string{good}, files)
	})

	t.Run("directories are unaffected", func(t *testing.T) {
		// A directory argument without -r is still reported as a skip.
		var skipped []string
		files, err := expandInputs([]string{dir}, false, func(p string, _ error) {
			skipped = append(skipped, p)
		})
		require.NoError(t, err)
		assert.Empty(t, files)
		assert.Equal(t, []string{dir}, skipped)
	})
}

// TestRunCatContinuesPastBadFile reproduces the real `kcat ~/Downloads/*`
// scenario: a glob that mixes an Office "~$" lock file, a file that cannot be
// parsed as its detected format, and good content. kcat must skip the lock
// file silently, report the unparseable file to stderr and carry on, still
// print every good file, and exit 2 (trouble) because one file errored.
func TestRunCatContinuesPastBadFile(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()

	lock := writeToolboxFile(t, dir, "~$5. ANS.pptx", "not a zip")          // junk → skipped
	bad := writeToolboxFile(t, dir, "broken.docx", "this is not a zip file") // openxml parse error
	good1 := writeToolboxFile(t, dir, "a.json", `{"k":"Alpha"}`)
	good2 := writeToolboxFile(t, dir, "b.json", `{"k":"Beta"}`)

	// Order mirrors a shell glob (lexicographic): the bad file is not last, so a
	// fail-fast implementation would drop good2.
	args := []string{good1, bad, good2, lock}

	out, err := captureStdout(t, func() error {
		return app.runCat(context.Background(), &cobra.Command{}, args, catOptions{})
	})

	assert.Contains(t, out, "Alpha", "good file before the bad one is printed")
	assert.Contains(t, out, "Beta", "good file after the bad one is still printed (no fail-fast)")

	require.Error(t, err, "an unparseable file must surface as an error")
	assert.Equal(t, ExitUsage, ExitCode(nil, err), "a parse error maps to exit 2 (trouble)")
	assert.ErrorIs(t, err, ErrSilentExit, "trouble suppresses the summary message")
}

// TestRunCatGlobWithOnlyJunkAndGood confirms that a lock file alone does not
// poison the run: with only junk + good input, kcat exits 0.
func TestRunCatGlobWithOnlyJunkAndGood(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	lock := writeToolboxFile(t, dir, "~$doc.docx", "lock stub")
	good := writeToolboxFile(t, dir, "en.json", `{"k":"Hello"}`)

	out, err := captureStdout(t, func() error {
		return app.runCat(context.Background(), &cobra.Command{}, []string{lock, good}, catOptions{})
	})
	require.NoError(t, err, "skipping a junk file is not an error — exit 0")
	assert.Equal(t, "Hello\n", out)
}
