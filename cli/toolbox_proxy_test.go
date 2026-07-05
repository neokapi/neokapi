package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// and behaves like kdiff (so -q is brief, not kapi's --quiet global).
func TestDiffProxyDetached(t *testing.T) {
	app := newToolboxApp(t)
	c := findCmd(NewToolboxProxies(app), "diff")
	require.NotNil(t, c)
	assert.True(t, c.Hidden)
	assert.True(t, c.DisableFlagParsing)

	dir := t.TempDir()
	a := writeToolboxFile(t, dir, "a.json", `{"k":"one"}`)
	b := writeToolboxFile(t, dir, "b.json", `{"k":"two"}`)
	out, err := captureStdout(t, func() error {
		c.SetArgs([]string{"-q", a, b})
		return c.Execute()
	})
	require.ErrorIs(t, err, ErrSilentExit, "-q is brief (diff status), not kapi's --quiet")
	assert.Contains(t, out, "differ")
}

func TestBusyboxRoot(t *testing.T) {
	app := newToolboxApp(t)
	for _, name := range []string{"kgrep", "ksed", "kcat", "/usr/local/bin/kgrep", "kgrep.exe"} {
		root := BusyboxRoot(app, name)
		require.NotNil(t, root, "prog %q should map to a toolbox root", name)
	}
	assert.Nil(t, BusyboxRoot(app, "kapi"))
	assert.Nil(t, BusyboxRoot(app, "something-else"))
}

func TestBusyboxRootKdiff(t *testing.T) {
	app := newToolboxApp(t)
	for _, name := range []string{"kdiff", "/usr/local/bin/kdiff", "kdiff.exe"} {
		require.NotNil(t, BusyboxRoot(app, name), "prog %q should map to a toolbox root", name)
	}
}
