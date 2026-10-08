//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"syscall/js"

	"github.com/neokapi/neokapi/core/workspace"
)

// A page shares a project's context through a remote it holds: an object with
// list, get and put, which the page builds over a folder the person gave it
// access to (packages/engine/src/dirremote.ts, over the File System Access
// API). The folder keeps the layout a `file` backend keeps on disk, so the
// browser and a machine whose recipe names that folder share one context.
// kapiSyncContext pulls from the remote and pushes to it, as `kapi context
// sync` does for the backend a recipe declares.
//
// # Page contract
//
//	remote.location: string                  where the remote is, for messages and the sync state
//	remote.list(dir) => Promise<string[]>     every object name under a layout directory ("log/", "blobs/", "checkpoints/")
//	remote.get(name) => Promise<Uint8Array | null>   null for a name the remote does not hold
//	remote.put([{name, data}]) => Promise<void>      create-only; rejects with name "ObjectExists" for a name held with other bytes
//
// A rejection named NotAllowedError, NotFoundError or SecurityError (the
// folder's permission withdrawn, or the folder gone) reports the remote as
// unreachable.

// pageRemote is a workspace.Remote over a remote the page holds.
type pageRemote struct {
	v    js.Value
	desc workspace.RemoteDescriptor
}

func newPageRemote(v js.Value) (*pageRemote, error) {
	if v.Type() != js.TypeObject {
		return nil, errors.New("kapiSyncContext takes a remote: an object with list, get and put")
	}
	for _, m := range []string{"list", "get", "put"} {
		if v.Get(m).Type() != js.TypeFunction {
			return nil, fmt.Errorf("the remote has no %s method", m)
		}
	}
	location := "the page's folder"
	if l := v.Get("location"); l.Type() == js.TypeString && l.String() != "" {
		location = l.String()
	}
	return &pageRemote{v: v, desc: workspace.RemoteDescriptor{Kind: "folder", Location: location}}, nil
}

func (r *pageRemote) Describe() workspace.RemoteDescriptor { return r.desc }

func (r *pageRemote) List(ctx context.Context, dir string) ([]string, error) {
	switch dir {
	case workspace.RemoteLogDir, workspace.RemoteBlobDir, workspace.RemoteCheckpointsDir:
	default:
		return nil, fmt.Errorf("workspace: %q is not a directory of the context layout", dir)
	}
	res, err := awaitRemote(ctx, r.desc, r.v.Call("list", dir))
	if err != nil {
		return nil, err
	}
	var out []string
	for i := range res.Length() {
		name := res.Index(i).String()
		// Anything a person or a sync client left beside the layout is not an object.
		if strings.HasPrefix(name, dir) && workspace.ValidObjectName(name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func (r *pageRemote) Get(ctx context.Context, name string) ([]byte, error) {
	if !workspace.ValidObjectName(name) {
		return nil, fmt.Errorf("workspace: %q is not a name in the context layout", name)
	}
	res, err := awaitRemote(ctx, r.desc, r.v.Call("get", name))
	if err != nil {
		return nil, err
	}
	if res.IsNull() || res.IsUndefined() {
		return nil, fmt.Errorf("%w: %s", workspace.ErrNoObject, name)
	}
	data := make([]byte, res.Length())
	js.CopyBytesToGo(data, res)
	return data, nil
}

func (r *pageRemote) Put(ctx context.Context, objs ...workspace.Object) error {
	for _, obj := range objs {
		if !workspace.ValidObjectName(obj.Name) {
			return fmt.Errorf("workspace: %q is not a name in the context layout", obj.Name)
		}
	}
	if len(objs) == 0 {
		return nil
	}
	arr := js.Global().Get("Array").New(len(objs))
	for i, obj := range objs {
		data := js.Global().Get("Uint8Array").New(len(obj.Data))
		js.CopyBytesToJS(data, obj.Data)
		o := js.Global().Get("Object").New()
		o.Set("name", obj.Name)
		o.Set("data", data)
		arr.SetIndex(i, o)
	}
	_, err := awaitRemote(ctx, r.desc, r.v.Call("put", arr))
	return err
}

func (r *pageRemote) Close() error { return nil }

// awaitRemote waits for a remote's Promise and reads a rejection as the
// remote's error.
func awaitRemote(ctx context.Context, desc workspace.RemoteDescriptor, promise js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	then := js.FuncOf(func(_ js.Value, args []js.Value) any {
		v := js.Undefined()
		if len(args) > 0 {
			v = args[0]
		}
		ch <- result{v: v}
		return nil
	})
	catch := js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{err: remoteError(desc, args)}
		return nil
	})
	js.Global().Get("Promise").Call("resolve", promise).Call("then", then, catch)
	select {
	case <-ctx.Done():
		// A late settle must not invoke a released function.
		return js.Undefined(), ctx.Err()
	case r := <-ch:
		then.Release()
		catch.Release()
		return r.v, r.err
	}
}

// remoteError reads a remote's rejection.
func remoteError(desc workspace.RemoteDescriptor, args []js.Value) error {
	name, msg := "", "the remote failed"
	if len(args) > 0 && args[0].Type() == js.TypeObject {
		if n := args[0].Get("name"); n.Type() == js.TypeString {
			name = n.String()
		}
		if m := args[0].Get("message"); m.Type() == js.TypeString && m.String() != "" {
			msg = m.String()
		}
	} else if len(args) > 0 && args[0].Truthy() {
		msg = args[0].Call("toString").String()
	}
	switch name {
	case "ObjectExists":
		return fmt.Errorf("%w: %s", workspace.ErrObjectExists, msg)
	case "NotAllowedError", "NotFoundError", "SecurityError":
		return fmt.Errorf("%w: %s: %s", workspace.ErrRemoteUnreachable, desc.Location, msg)
	}
	return fmt.Errorf("workspace: %s: %s", desc.Location, msg)
}

// syncOptions is kapiSyncContext's second argument.
type syncOptions struct {
	// Project is the project to sync: its kapi.yaml, its root, or a path
	// inside it. Omitted is the one discovery finds from the working directory.
	Project string `json:"project"`
	// Pull and Push say which to run: both when neither is set, otherwise
	// the ones set to true.
	Pull *bool `json:"pull"`
	Push *bool `json:"push"`
}

// kapiSyncContext pulls a project's context from a remote the page holds and
// pushes what this engine recorded to it. args[0] is the remote (see the page
// contract above) and args[1] the options as a JSON string, {project, pull,
// push}. It returns a Promise of the report as a JSON string, {pull, push},
// each the report `kapi context sync --json` prints for that half.
func kapiSyncContext(_ js.Value, args []js.Value) any {
	var (
		remote  *pageRemote
		opts    syncOptions
		argErr  error
		rawOpts string
	)
	if len(args) >= 1 {
		remote, argErr = newPageRemote(args[0])
	} else {
		argErr = errors.New("kapiSyncContext takes a remote")
	}
	if len(args) >= 2 && args[1].Type() == js.TypeString {
		rawOpts = args[1].String()
	}
	if argErr == nil && rawOpts != "" {
		if err := json.Unmarshal([]byte(rawOpts), &opts); err != nil {
			argErr = fmt.Errorf("kapiSyncContext options: %w", err)
		}
	}
	return goPromise(func() (any, error) {
		if argErr != nil {
			return nil, argErr
		}
		engineMu.Lock()
		defer engineMu.Unlock()
		if err := ensureApp(); err != nil {
			return nil, err
		}
		res, err := syncContext(context.Background(), remote, opts)
		if err != nil {
			return nil, err
		}
		out, err := json.Marshal(res)
		return string(out), err
	})
}

// syncContext runs a sync through the page's remote. The caller holds engineMu.
func syncContext(ctx context.Context, remote workspace.Remote, opts syncOptions) (any, error) {
	recipe, err := app.RequireMCPCallProject(opts.Project)
	if err != nil {
		return nil, err
	}
	pull, push := true, true
	if opts.Pull != nil || opts.Push != nil {
		pull = opts.Pull != nil && *opts.Pull
		push = opts.Push != nil && *opts.Push
	}
	return app.SyncProjectContextWith(ctx, recipe, remote, pull, push)
}
