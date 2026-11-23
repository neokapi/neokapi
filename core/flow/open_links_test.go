//go:build unix

package flow

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An input beneath the project is read only when it resolves inside it: the
// walk lists, and the read is where the decision is made again. A file a
// person names outside the project is read as named.
func TestOpenInputStaysInsideTheProject(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	secret := filepath.Join(outside, "secret.json")
	require.NoError(t, os.WriteFile(secret, []byte(`{"s":"outside"}`), 0o644))
	link := filepath.Join(root, "src", "fr.json")
	require.NoError(t, os.Symlink(secret, link))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "vendor")))

	_, err := openBudgetedFile(root, link)
	require.ErrorContains(t, err, "leads outside the root")
	_, err = openBudgetedFile(root, filepath.Join(root, "vendor", "secret.json"))
	require.ErrorContains(t, err, "leads outside the root")

	// The same input spelled through another name for the project directory
	// is still beneath it, and still refused: the decision follows the
	// directory's identity, not the spelling.
	alias := filepath.Join(base, "alias")
	require.NoError(t, os.Symlink(root, alias))
	_, err = openBudgetedFile(root, filepath.Join(alias, "src", "fr.json"))
	require.ErrorContains(t, err, "leads outside the root")
	_, err = openBudgetedFile(alias, link)
	require.ErrorContains(t, err, "leads outside the root")

	src, err := openBudgetedFile(root, secret)
	require.NoError(t, err, "a path outside the project, as named, is read")
	b, err := io.ReadAll(src)
	require.NoError(t, err)
	require.NoError(t, src.Close())
	assert.JSONEq(t, `{"s":"outside"}`, string(b))
}
