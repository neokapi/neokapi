//go:build js && wasm

// Command kapi-wasm is a WebAssembly entrypoint that exposes a subset of the
// neokapi framework (format readers/writers + the pseudo-translate tool) to
// the browser. It powers the interactive "playground" on the docs site: a
// user uploads a document (e.g. a .docx) and sees it pseudo-translated
// entirely client-side, with no server round-trip.
//
// The whole pipeline runs in-memory — there is no filesystem in the browser,
// so input arrives as a Uint8Array, output is returned base64-encoded, and
// skeleton reconstruction uses an in-memory skeleton store.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"syscall/js"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/version"
)

// previewLimit caps how many before/after block pairs we return for display.
// The translated document itself always contains every block; this only
// bounds the UI table.
const previewLimit = 200

// pseudoOpts mirrors the tunable knobs of the pseudo-translate tool, decoded
// from the JSON options string passed by JavaScript.
type pseudoOpts struct {
	TargetLang string `json:"targetLang"`
	SourceLang string `json:"sourceLang"`
	Prefix     string `json:"prefix"`
	Suffix     string `json:"suffix"`
	Expansion  int    `json:"expansion"`
}

type blockPreview struct {
	Source string
	Target string
}

type runResult struct {
	Format     string
	Output     []byte
	Blocks     []blockPreview
	BlockCount int
	WordCount  int
}

func main() {
	js.Global().Set("kapiVersion", js.FuncOf(kapiVersion))
	js.Global().Set("kapiListFormats", js.FuncOf(kapiListFormats))
	js.Global().Set("kapiPseudoTranslate", js.FuncOf(kapiPseudoTranslate))

	// Signal readiness so the page can enable the UI only once the module
	// has installed its functions.
	if ready := js.Global().Get("__kapiWasmReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}

	// Keep the Go program alive so the exported functions remain callable.
	select {}
}

func kapiVersion(js.Value, []js.Value) any {
	return version.Version
}

// kapiListFormats returns the format names that have both a reader and a
// writer registered, so the UI can advertise what it accepts.
func kapiListFormats(js.Value, []js.Value) any {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	writers := make(map[registry.FormatID]bool)
	for _, w := range reg.WriterNames() {
		writers[w] = true
	}
	var names []any
	for _, r := range reg.ReaderNames() {
		if writers[r] {
			names = append(names, string(r))
		}
	}
	return names
}

// kapiPseudoTranslate is the main entrypoint. Args:
//
//	[0] filename string   — used for extension-based format detection
//	[1] input    Uint8Array — the raw document bytes
//	[2] optsJSON string   — JSON-encoded pseudoOpts
//
// It returns a JS object: {ok, error?, format?, outputBase64?, blockCount?,
// wordCount?, blocks?}.
func kapiPseudoTranslate(_ js.Value, args []js.Value) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = map[string]any{"ok": false, "error": fmt.Sprintf("internal error: %v", r)}
		}
	}()

	if len(args) < 3 {
		return map[string]any{"ok": false, "error": "expected (filename, bytes, optsJSON)"}
	}

	filename := args[0].String()
	n := args[1].Get("length").Int()
	input := make([]byte, n)
	js.CopyBytesToGo(input, args[1])

	var opts pseudoOpts
	if s := args[2].String(); s != "" {
		if err := json.Unmarshal([]byte(s), &opts); err != nil {
			return map[string]any{"ok": false, "error": "invalid options: " + err.Error()}
		}
	}

	res, err := run(filename, input, opts)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}

	blocks := make([]any, 0, len(res.Blocks))
	for _, b := range res.Blocks {
		blocks = append(blocks, map[string]any{"source": b.Source, "target": b.Target})
	}

	return map[string]any{
		"ok":           true,
		"format":       res.Format,
		"outputBase64": base64.StdEncoding.EncodeToString(res.Output),
		"blockCount":   res.BlockCount,
		"wordCount":    res.WordCount,
		"blocks":       blocks,
	}
}

// run executes the full read → pseudo-translate → write pipeline in memory.
// It mirrors core/flow.FileRunner.RunFileWithReaderWriter but sources its
// input from a byte slice and writes to a buffer, since the browser has no
// filesystem.
func run(filename string, input []byte, opts pseudoOpts) (runResult, error) {
	ctx := context.Background()

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	fmtName, err := reg.DetectByExtension(strings.ToLower(filepath.Ext(filename)))
	if err != nil {
		return runResult{}, fmt.Errorf("detect format for %q: %w", filepath.Base(filename), err)
	}

	reader, err := reg.NewReader(fmtName)
	if err != nil {
		return runResult{}, fmt.Errorf("no reader for %q: %w", fmtName, err)
	}
	writer, err := reg.NewWriter(fmtName)
	if err != nil {
		reader.Close()
		return runResult{}, fmt.Errorf("no writer for %q: %w", fmtName, err)
	}

	// Wire an in-memory skeleton store when both sides support it (required
	// for skeleton-driven formats like OOXML to inject translations — the
	// fallback path copies the original unchanged).
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
			store := format.NewMemorySkeletonStore()
			emitter.SetSkeletonStore(store)
			consumer.SetSkeletonStore(store)
			defer store.Close()
		}
	}

	if opts.SourceLang == "" {
		opts.SourceLang = "en"
	}
	if opts.TargetLang == "" {
		opts.TargetLang = "fr"
	}
	target := model.LocaleID(opts.TargetLang)

	doc := &model.RawDocument{
		URI:          filename,
		SourceLocale: model.LocaleID(opts.SourceLang),
		TargetLocale: target,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(input)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return runResult{}, fmt.Errorf("open %q: %w", filepath.Base(filename), err)
	}

	var parts []*model.Part
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			reader.Close()
			return runResult{}, fmt.Errorf("read %q: %w", filepath.Base(filename), res.Error)
		}
		parts = append(parts, res.Part)
	}
	reader.Close()

	pt := tools.NewPseudoTranslateTool(&tools.PseudoConfig{
		TargetLocale:     target,
		Prefix:           opts.Prefix,
		Suffix:           opts.Suffix,
		ExpansionPercent: opts.Expansion,
	})
	f, err := flow.NewFlow("pseudo").AddTool(pt).Build()
	if err != nil {
		return runResult{}, fmt.Errorf("build flow: %w", err)
	}

	executor := flow.NewExecutor()
	inCh, outCh, wait := executor.ExecuteWithChannels(ctx, f)

	go func() {
		defer close(inCh)
		for _, p := range parts {
			inCh <- p
		}
	}()

	// Collect every processed part so we can both extract a before/after
	// preview and re-feed them to the writer.
	var outParts []*model.Part
	for p := range outCh {
		outParts = append(outParts, p)
	}
	if err := wait(); err != nil {
		return runResult{}, fmt.Errorf("execute flow: %w", err)
	}

	res := runResult{Format: string(fmtName)}
	for _, p := range outParts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok {
			continue
		}
		res.BlockCount++
		res.WordCount += b.WordCount()
		if len(res.Blocks) < previewLimit {
			res.Blocks = append(res.Blocks, blockPreview{
				Source: b.SourceText(),
				Target: b.TargetText(target),
			})
		}
	}

	var out bytes.Buffer
	if err := writer.SetOutputWriter(&out); err != nil {
		return runResult{}, fmt.Errorf("set output: %w", err)
	}
	if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(input)
	}
	if sls, ok := writer.(format.SourceLocaleSetter); ok {
		sls.SetSourceLocale(doc.SourceLocale)
	}
	writer.SetLocale(target)

	wch := make(chan *model.Part)
	go func() {
		defer close(wch)
		for _, p := range outParts {
			wch <- p
		}
	}()
	if err := writer.Write(ctx, wch); err != nil {
		return runResult{}, fmt.Errorf("write output: %w", err)
	}
	if err := writer.Close(); err != nil {
		return runResult{}, fmt.Errorf("close writer: %w", err)
	}

	res.Output = out.Bytes()
	return res, nil
}
