//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall/js"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/core/workspace/workspacetest"
	"github.com/neokapi/neokapi/host"
)

// memoryFolder is a page remote held in JS memory, as a page's folder remote
// answers: create-only puts that reject with ObjectExists, and get answering
// null for a name the folder does not hold. Each call of the function it
// returns is another handle on the same folder.
const memoryFolder = `
let n = 0;
return function () {
  const files = new Map();
  const id = "folder-" + (++n);
  return function () {
    return {
      location: id,
      list: async (dir) => [...files.keys()].filter((name) => name.startsWith(dir)),
      get: async (name) => (files.has(name) ? files.get(name).slice() : null),
      put: async (objs) => {
        for (const o of objs) {
          const held = files.get(o.name);
          if (held && (held.length !== o.data.length || held.some((b, i) => b !== o.data[i]))) {
            const e = new Error(o.name + " holds other bytes");
            e.name = "ObjectExists";
            throw e;
          }
        }
        for (const o of objs) if (!files.has(o.name)) files.set(o.name, o.data.slice());
      },
    };
  };
};`

func newMemoryFolder() js.Value {
	return js.Global().Get("Function").New(memoryFolder).Invoke()
}

// The page's remote owes the sync engine what every remote does.
func TestPageRemote_Conformance(t *testing.T) {
	folders := newMemoryFolder()
	workspacetest.RunRemoteConformance(t, func(t *testing.T) func(t *testing.T) workspace.Remote {
		folder := folders.Invoke()
		return func(t *testing.T) workspace.Remote {
			r, err := newPageRemote(folder.Invoke())
			require.NoError(t, err)
			return r
		}
	})
}

// A remote without the page contract's methods is refused by name.
func TestPageRemote_RefusesAnObjectWithoutTheMethods(t *testing.T) {
	_, err := newPageRemote(js.Global().Get("Object").New())
	assert.ErrorContains(t, err, "no list method")
	_, err = newPageRemote(js.ValueOf("folder"))
	assert.Error(t, err)
}

// A folder whose permission was withdrawn is unreachable, as an unmounted
// share is natively.
func TestPageRemote_WithdrawnPermissionIsUnreachable(t *testing.T) {
	folder := js.Global().Get("Function").New(`return {
  location: "gone",
  list: async () => { const e = new Error("permission withdrawn"); e.name = "NotAllowedError"; throw e; },
  get: async () => null,
  put: async () => {},
};`).Invoke()
	r, err := newPageRemote(folder)
	require.NoError(t, err)
	_, err = r.List(context.Background(), workspace.RemoteLogDir)
	assert.ErrorIs(t, err, workspace.ErrRemoteUnreachable)
}

// A project's context travels through the page's folder: pushed from the
// engine, and pulled back after a reset forgot it.
func TestSyncContext_PushThenPullAfterReset(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Cleanup(func() {
		_ = resetInTurn(context.Background(), root)
		_ = storage.RemoveAll(root)
	})
	proj := filepath.Join(root, "site")
	require.NoError(t, os.MkdirAll(filepath.Join(proj, "docs"), 0o755))
	recipe := "version: v1\nname: site\ndefaults:\n  source_language: en\n  target_languages: [fr]\n" +
		"collections:\n  - name: docs\n    content:\n      - path: \"docs/*.json\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(proj, "kapi.yaml"), []byte(recipe), 0o644))
	options, err := json.Marshal(map[string]string{"project": proj})
	require.NoError(t, err)
	_, err = serveChange(app.ApplyChangesJSON,
		[]byte(`{"ops":[{"op":"term","action":"upsert","term":"handbook","locale":"en"}]}`), options)
	require.NoError(t, err)

	remote, err := newPageRemote(newMemoryFolder().Invoke().Invoke())
	require.NoError(t, err)
	push := true
	engineMu.Lock()
	out, err := syncContext(ctx, remote, syncOptions{Project: proj, Push: &push})
	engineMu.Unlock()
	require.NoError(t, err)
	pushed := out.(host.ContextSync)
	assert.Nil(t, pushed.Pull, "only the push was asked for")
	require.NotNil(t, pushed.Push)
	assert.Positive(t, pushed.Push.Pushed)
	names, err := remote.List(ctx, workspace.RemoteLogDir)
	require.NoError(t, err)
	assert.NotEmpty(t, names, "the folder holds the log")

	require.NoError(t, resetInTurn(ctx, root))
	require.NoError(t, os.MkdirAll(filepath.Join(proj, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "kapi.yaml"), []byte(recipe), 0o644))
	assert.Zero(t, projectTerms(t, proj), "the reset forgot the term")

	engineMu.Lock()
	out, err = syncContext(ctx, remote, syncOptions{Project: proj})
	engineMu.Unlock()
	require.NoError(t, err)
	both := out.(host.ContextSync)
	require.NotNil(t, both.Pull)
	assert.Equal(t, pushed.Push.Pushed, both.Pull.Merged)
	require.NotNil(t, both.Push)
	assert.Zero(t, both.Push.Pushed, "what was pulled is not pushed back")
	assert.Equal(t, 1, projectTerms(t, proj), "the term is back")
}
