package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/observe"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
)

type formatTracer struct{ spans []formatSpan }

type formatSpan struct{ op, name string }

func (f *formatTracer) Start(ctx context.Context, op, name string) (context.Context, observe.Span) {
	f.spans = append(f.spans, formatSpan{op: op, name: name})
	return ctx, nopSpan{}
}

type nopSpan struct{}

func (nopSpan) SetAttr(string, string) {}
func (nopSpan) End(error)              {}

func (f *formatTracer) ops(op string) []string {
	var out []string
	for _, s := range f.spans {
		if s.op == op {
			out = append(out, s.name)
		}
	}
	return out
}

// Reading and writing a document each produce a span named for the format.
//
// Asserted through a real run rather than by calling the reader directly: the
// point is that the spans are wired into the path a file actually takes, and a
// unit test of the reader would pass with the call sites uninstrumented.
//
// The span names are the literal wire strings, not the package constants —
// what a backend groups by is the string, so that is what is pinned.
func TestFileRun_ProducesFormatSpans(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	runner := flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: reg, SourceLocale: "en-US"})

	dir := t.TempDir()
	in := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "out", "notes.md")
	require.NoError(t, os.WriteFile(in, []byte("# Title\n\nA paragraph.\n"), 0o644))

	rd, err := reg.NewReader("markdown")
	require.NoError(t, err)
	wr, err := reg.NewWriter("markdown")
	require.NoError(t, err)

	tr := &formatTracer{}
	ctx := observe.WithTracer(context.Background(), tr)

	require.NoError(t, runner.RunFileWithReaderWriter(ctx, "noop",
		[]tool.Tool{&tool.BaseTool{ToolName: "noop"}}, in, out, "", rd, wr))

	assert.Contains(t, tr.ops("format.read"), "markdown", "the read is measured")
	assert.Contains(t, tr.ops("format.write"), "markdown", "and so is the write")
}

// A run with no tracer registered behaves identically — the default for every
// embedded kapi run.
func TestFileRun_WithoutATracer(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	runner := flow.NewFileRunner(flow.FileRunnerConfig{FormatReg: reg, SourceLocale: "en-US"})

	dir := t.TempDir()
	in := filepath.Join(dir, "notes.md")
	out := filepath.Join(dir, "out", "notes.md")
	require.NoError(t, os.WriteFile(in, []byte("# Title\n\nA paragraph.\n"), 0o644))

	rd, err := reg.NewReader("markdown")
	require.NoError(t, err)
	wr, err := reg.NewWriter("markdown")
	require.NoError(t, err)

	require.NoError(t, runner.RunFileWithReaderWriter(context.Background(), "noop",
		[]tool.Tool{&tool.BaseTool{ToolName: "noop"}}, in, out, "", rd, wr))
	assert.FileExists(t, out)
}
