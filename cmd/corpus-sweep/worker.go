package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/safeio"
)

// workerResult is the one-line JSON the worker emits on stdout. The driver
// parses it as the authoritative classification when the subprocess exits
// cleanly; a missing line (the worker died before emitting) is the driver's
// CRASH signal.
type workerResult struct {
	Format string `json:"format"`
	File   string `json:"file"`
	Class  string `json:"class"`
	Detail string `json:"detail,omitempty"`
}

// runWorkerArgs parses the worker flags (--format, --file) and runs the
// single-file classification. It is invoked both by main (real subprocess,
// after the `--worker` selector) and by the test TestMain helper (the test
// binary re-exec'd as a worker). It always returns 0 when it produced a result
// line — process-fatal failures are the driver's concern, not an exit code the
// worker chooses.
func runWorkerArgs(args []string) int {
	fs := flag.NewFlagSet("corpus-sweep --worker", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "", "format id to parse the file as")
	file := fs.String("file", "", "path to the corpus file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "corpus-sweep worker: %v\n", err)
		return 2
	}
	if *format == "" || *file == "" {
		fmt.Fprintln(os.Stderr, "corpus-sweep worker: --format and --file are required")
		return 2
	}
	return runWorker(*format, *file)
}

// runWorker reads one file and classifies it read→write→read, emitting a single
// JSON line on stdout.
func runWorker(formatID, file string) int {
	budget := safeio.DefaultBudget()
	res := workerResult{Format: formatID, File: file}

	// Byte-budget guard before reading the whole file into memory: an
	// over-budget file is a graceful reject, not an OOM risk for the worker.
	if fi, err := os.Stat(file); err == nil && budget.MaxBytes > 0 && fi.Size() > budget.MaxBytes {
		res.Class = string(ExpectedReject)
		res.Detail = fmt.Sprintf("file size %d exceeds byte budget %d", fi.Size(), budget.MaxBytes)
		emitResult(res)
		return 0
	}

	data, err := os.ReadFile(file)
	if err != nil {
		// A missing/unreadable file is a harness error; surface it as CRASH
		// with detail so it is visible rather than silently dropped.
		res.Class = string(Crash)
		res.Detail = fmt.Sprintf("read file: %v", err)
		emitResult(res)
		return 0
	}

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	cls, detail := classify(reg, formatID, data, budget)
	res.Class = string(cls)
	res.Detail = detail
	emitResult(res)
	return 0
}

// emitResult writes the worker result as a single JSON line on stdout.
func emitResult(res workerResult) {
	b, err := json.Marshal(res)
	if err != nil {
		// Should never happen for this flat struct; fall back to a fixed line.
		fmt.Fprintf(os.Stdout, `{"format":%q,"file":%q,"class":"CRASH","detail":"marshal result failed"}`+"\n", res.Format, res.File)
		return
	}
	fmt.Fprintln(os.Stdout, string(b))
}
