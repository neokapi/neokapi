//go:build js && wasm

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"syscall/js"

	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
)

// labInspect reads a file through the kapi format reader and returns its full
// content tree (Layers → Groups → Blocks → Runs / Data / Media) as JSON — the
// structured backbone of the docs "Anatomy" explorer. Where kapiPreview returns
// a flat list of block text for the files panel, labInspect exposes the whole
// content model so a learner can see how a reader decomposes a document.
//
// It returns a Promise: os.ReadFile and the reader are async under js/wasm and
// would deadlock if run synchronously inside the JS callback. The result is
// {ok:true, format, json, bytes} where json is a stringified ContentTree (the
// page JSON.parses it) — passing a pre-marshaled string honors model.Run's
// custom JSON encoding rather than relying on syscall/js struct conversion.
func labInspect(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return errorResult("labInspect expects a path")
	}
	path := args[0].String()
	executor := js.FuncOf(func(_ js.Value, p []js.Value) any {
		resolve := p[0]
		go func() { resolve.Invoke(doInspect(path)) }()
		return js.Undefined()
	})
	return js.Global().Get("Promise").New(executor)
}

func doInspect(path string) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = errorResult("internal error inspecting file")
		}
	}()

	data, err := os.ReadFile(path)
	if err != nil {
		return errorResult(err.Error())
	}

	// Content-aware detection: an extension claimed by more than one format
	// (notably .xlf/.xliff → XLIFF 1.2 and 2.x) must be disambiguated by
	// sniffing the bytes, otherwise the deterministic extension tie-break
	// would hand a 2.x document to the 1.2 reader (which can't parse <unit>
	// and yields zero blocks). DetectFile sniffs among the candidates and
	// falls back to the extension pick when sniffing is inconclusive.
	fmtName, err := app.FormatReg.DetectFile(path, nil)
	if err != nil {
		return errorResult("unsupported format for " + filepath.Base(path))
	}
	reader, err := app.FormatReg.NewReader(fmtName)
	if err != nil {
		return errorResult(err.Error())
	}

	ctx := context.Background()
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return errorResult(err.Error())
	}

	var parts []*model.Part
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			reader.Close()
			return errorResult(res.Error.Error())
		}
		if res.Part != nil {
			parts = append(parts, res.Part)
		}
	}
	reader.Close()

	tree := editor.BuildContentTree(parts, string(fmtName))
	treeJSON, err := json.Marshal(tree)
	if err != nil {
		return errorResult(err.Error())
	}

	return map[string]any{
		"ok":     true,
		"format": string(fmtName),
		"json":   string(treeJSON),
		"bytes":  len(data),
	}
}
