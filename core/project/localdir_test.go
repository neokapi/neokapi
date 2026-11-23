package project

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every path to a checkout's own state goes through LocalStateDir, so naming
// LocalDir moves all of it and nothing else.
func TestLocalDirHoldsEveryPathToTheCheckoutsOwnState(t *testing.T) {
	root := t.TempDir()
	l := LayoutAt(root)
	assert.True(t, strings.HasPrefix(l.LocalStateDir(), dataroot.CheckoutsDir()), "without LocalDir the state is under the data root")

	local := filepath.Join(t.TempDir(), "checkout-state")
	l.LocalDir = local
	assert.Equal(t, local, l.LocalStateDir())
	for name, p := range map[string]string{
		"work":       l.WorkDir(),
		"store":      l.StorePath(),
		"cache":      l.CacheDir(),
		"sync":       l.SyncDir(),
		"vault":      l.VaultDir(),
		"filters":    l.LocalFiltersPath(),
		"vault file": l.RedactionVaultPath(),
	} {
		rel, err := filepath.Rel(local, p)
		require.NoError(t, err, name)
		assert.NotContains(t, rel, "..", "%s is under LocalDir", name)
	}
	assert.Equal(t, filepath.Join(root, StateDirName), l.StateDir, "the committed directory stays where it is")
	assert.Equal(t, filepath.Join(root, StateDirName, ProfilesDirName), l.Export().ProfilesDir(), "and so does its source")
}
