//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"sync"
	"syscall/js"

	"github.com/neokapi/neokapi/host/config"
)

// The change contract (core/change) in the browser: kapiRead, kapiApply and
// kapiDescribe hand a page the same change service kapi inspect and kapi
// apply use, without going through a command line. Each takes the contract's
// own JSON as a string, a read request ({doc, blocks, editions, cursor,
// limit}), a kapi.change/v1 change set or a describe request ({format, doc}),
// and an optional second string, the call's options ({project, actor}). Each
// returns a Promise of a JSON string: the read page, the
// kapi.change-result/v1 result or the format's description. A request the
// contract refuses, a change set that does not decode included, resolves to a
// kapi.change-result/v1 whose error says why. The Promise rejects only for a
// failure the contract has no code for, such as a project path that names no
// project (host.ReadChangesJSON and its siblings).
//
// A call acts in the project its options name, or the one discovery finds
// from the working directory, as a command given no -p does. Inside a project
// an apply is recorded in the browser's workspace log as a content.edit
// operation, the record kapi apply writes natively.

// browserChangeOrigin names this surface in the record of a change.
const browserChangeOrigin = "browser"

// changeMu orders the calls, which share the App: one call reads or writes
// at a time.
var changeMu sync.Mutex

func kapiRead(_ js.Value, args []js.Value) any {
	return changeCall("kapiRead", args, app.ReadChangesJSON)
}

func kapiApply(_ js.Value, args []js.Value) any {
	return changeCall("kapiApply", args, app.ApplyChangesJSON)
}

func kapiDescribe(_ js.Value, args []js.Value) any {
	return changeCall("kapiDescribe", args, app.DescribeChangesJSON)
}

// changeServe is one of the host's JSON entry points of the change service.
type changeServe func(ctx context.Context, origin string, request, options []byte) ([]byte, error)

// changeCall runs serve on the call's arguments in a goroutine and returns a
// Promise of its answer. The work cannot run inside the callback: the
// engine's file system calls are asynchronous and wait on the JS event loop
// (see kapiRun).
func changeCall(name string, args []js.Value, serve changeServe) any {
	request, options, argErr := changeArgs(name, args)
	executor := js.FuncOf(func(_ js.Value, p []js.Value) any {
		resolve, reject := p[0], p[1]
		if argErr != nil {
			reject.Invoke(js.Global().Get("TypeError").New(argErr.Error()))
			return js.Undefined()
		}
		go func() {
			out, err := serveChange(serve, request, options)
			if err != nil {
				reject.Invoke(js.Global().Get("Error").New(name + ": " + err.Error()))
				return
			}
			resolve.Invoke(string(out))
		}()
		return js.Undefined()
	})
	// The Promise constructor calls the executor before it returns.
	defer executor.Release()
	return js.Global().Get("Promise").New(executor)
}

// changeArgs reads a call's request and its optional options, each a JSON
// string.
func changeArgs(name string, args []js.Value) (request, options []byte, err error) {
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return nil, nil, fmt.Errorf("%s takes its request as a JSON string", name)
	}
	request = []byte(args[0].String())
	if len(args) >= 2 {
		switch args[1].Type() {
		case js.TypeString:
			options = []byte(args[1].String())
		case js.TypeUndefined, js.TypeNull:
		default:
			return nil, nil, fmt.Errorf("%s takes its options as a JSON string", name)
		}
	}
	return request, options, nil
}

// serveChange answers one call, one call at a time. A call takes its project
// from its options and its source language from that project, not from the
// flags a command gave the App. A host that runs a command and a call at once
// orders them itself, as the lab runtime does.
func serveChange(serve changeServe, request, options []byte) (out []byte, err error) {
	changeMu.Lock()
	defer changeMu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("internal error: %v", r)
		}
	}()
	if app.Config == nil {
		// No command has run yet, so nothing has finished initializing the App.
		app.Config = config.NewAppConfig()
		if err := app.Init(); err != nil {
			return nil, err
		}
		forceDemoProviders(app)
	}
	return serve(context.Background(), browserChangeOrigin, request, options)
}
