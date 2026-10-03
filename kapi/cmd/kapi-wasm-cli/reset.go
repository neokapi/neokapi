//go:build js && wasm

package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"syscall/js"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/host"
)

// kapiReset starts a directory of the page's file system over as a fresh tab
// would find it, before the host clears the directory's files. args[0] is the
// directory. Every project at or below it is forgotten in the workspace, its
// stores closed (App.ForgetProjectsUnder), so a project seeded there again
// begins with an empty context; then every database at or below it goes, since
// the page's file system never sees them. The workspace's own databases stay,
// holding every other project. A reset waits for a command or a change call
// that is running, and one that starts after it waits for it (engineMu).
//
// It returns a Promise that resolves to null, or to the error's message.
func kapiReset(_ js.Value, args []js.Value) any {
	dir := ""
	if len(args) >= 1 && args[0].Type() == js.TypeString {
		dir = args[0].String()
	}
	executor := js.FuncOf(func(_ js.Value, p []js.Value) any {
		resolve := p[0]
		go func() {
			if err := resetInTurn(context.Background(), dir); err != nil {
				resolve.Invoke(err.Error())
				return
			}
			resolve.Invoke(js.Null())
		}()
		return js.Undefined()
	})
	return js.Global().Get("Promise").New(executor)
}

// resetInTurn starts dir over once no command or call is running (engineMu).
func resetInTurn(ctx context.Context, dir string) error {
	engineMu.Lock()
	defer engineMu.Unlock()
	return resetDir(ctx, dir)
}

func resetDir(ctx context.Context, dir string) error {
	if dir == "" {
		return errors.New("kapiReset: name the directory to start over")
	}
	if err := app.ForgetProjectsUnder(ctx, dir); err != nil {
		return err
	}
	paths, err := storage.List(dir)
	if err != nil {
		return err
	}
	data := filepath.Clean(host.DataDir()) + string(filepath.Separator)
	var errs []error
	for _, p := range paths {
		if strings.HasPrefix(p, data) {
			continue
		}
		errs = append(errs, storage.Remove(p))
	}
	return errors.Join(errs...)
}
