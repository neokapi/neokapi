package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoDiffProxy: the file differ's kapi-side spelling is kdiff only —
// there is no hidden `kapi diff` proxy, freeing the verb for the bowrain
// plugin's local-vs-server sync diff.
func TestNoDiffProxy(t *testing.T) {
	app := newToolboxApp(t)
	assert.Nil(t, findCmd(NewToolboxProxies(app), "diff"),
		"`kapi diff` must not be occupied by a toolbox proxy")
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
