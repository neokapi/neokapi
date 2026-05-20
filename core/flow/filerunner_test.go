package flow_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileRunner_PseudoTranslate(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	require.NoError(t, os.WriteFile(inputPath, []byte(`{"greeting": "Hello World"}`), 0o644))

	outputPath := filepath.Join(dir, "output", "qps", "input.json")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	})

	err = runner.RunFile(context.Background(), "pseudo-translate", []tool.Tool{pseudoTool}, inputPath, outputPath, "qps")
	require.NoError(t, err)

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, output)
	assert.NotEqual(t, `{"greeting": "Hello World"}`, string(output))
}

// TestFileRunner_BufferedOutputFlushesFully asserts the filerunner-site
// output buffer (#608, S4) is fully flushed to disk — every block's
// pseudo-translated value lands in the output with no truncation, even
// when total output far exceeds the buffer. A skeleton-driven JSON write
// emits many small writes (one per skeleton run), exercising the buffer.
func TestFileRunner_BufferedOutputFlushesFully(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")

	// Build a JSON object with enough distinct entries that the output
	// comfortably exceeds the 64 KiB output buffer.
	var sb strings.Builder
	sb.WriteString("{\n")
	const n = 4000
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",\n")
		}
		fmt.Fprintf(&sb, "  %q: %q", fmt.Sprintf("key%05d", i), fmt.Sprintf("Value number %d here", i))
	}
	sb.WriteString("\n}\n")
	require.NoError(t, os.WriteFile(inputPath, []byte(sb.String()), 0o644))

	outputPath := filepath.Join(dir, "out", "input.json")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	})
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudoTool}, inputPath, outputPath, "qps"))

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Greater(t, len(output), 64*1024, "test must exceed the output buffer to be meaningful")

	// Output must be valid JSON with all keys present (no truncation).
	var got map[string]string
	require.NoError(t, json.Unmarshal(output, &got), "flushed output must be complete, valid JSON")
	assert.Len(t, got, n, "every key must survive to the flushed output")
	// First and last entries present and pseudo-translated (accented).
	require.Contains(t, got, "key00000")
	require.Contains(t, got, fmt.Sprintf("key%05d", n-1))
	assert.NotEqual(t, "Value number 0 here", got["key00000"], "value should be pseudo-translated")
}

// TestFileRunner_EmitOnCloseWriterFlushes covers writers (like the KLF
// jsx writer) that emit their entire payload in Close() rather than
// Write(). The filerunner must close the writer before flushing the
// output buffer, otherwise the file would be empty.
func TestFileRunner_EmitOnCloseWriterFlushes(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.klf")
	klfFile := &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "test", Version: "0"},
		Project:       klf.ProjectInfo{ID: "p", SourceLocale: "en-US"},
		Documents: []klf.Document{{
			ID:           "doc1",
			DocumentType: klf.DocumentTypeJSX,
			Path:         "a.json",
			Blocks: []klf.Block{{
				ID:           "b1",
				Translatable: true,
				Source:       []klf.Run{{Text: &klf.TextRun{Text: "Hello World"}}},
			}},
		}},
	}
	klfBytes, err := klf.Marshal(klfFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(inputPath, klfBytes, 0o644))

	outputPath := filepath.Join(dir, "out", "input.klf")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	})
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudoTool}, inputPath, outputPath, "qps"))

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, output, "emit-on-Close writer output must be flushed, not truncated to empty")
	assert.Contains(t, string(output), "documents", "KLF payload must be present in the flushed file")
}

// erroringTool returns an error after passing through n parts. Used to
// exercise the filerunner's concurrent error-propagation path.
type erroringTool struct {
	*tool.BaseTool
	afterN int
}

func newErroringTool(afterN int) *erroringTool {
	return &erroringTool{BaseTool: &tool.BaseTool{ToolName: "boom"}, afterN: afterN}
}

func (e *erroringTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	seen := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-in:
			if !ok {
				return nil
			}
			if seen >= e.afterN {
				return errBoom
			}
			seen++
			select {
			case out <- p:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

var errBoom = fmt.Errorf("boom: simulated tool failure")

// TestFileRunner_ToolErrorAbortsWithoutPartialOutput verifies that a tool
// error aborts the run, surfaces the error, and leaves NO output file at
// the destination — matching the pre-S1 contract even though the
// pipeline now streams concurrently (#608, S1).
func TestFileRunner_ToolErrorAbortsWithoutPartialOutput(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	require.NoError(t, os.WriteFile(inputPath,
		[]byte(`{"a":"one","b":"two","c":"three","d":"four"}`), 0o644))

	outputPath := filepath.Join(dir, "out", "input.json")

	err := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	}).RunFile(context.Background(), "boom", []tool.Tool{newErroringTool(2)},
		inputPath, outputPath, "qps")

	require.Error(t, err, "tool error must propagate")
	assert.ErrorIs(t, err, errBoom)

	_, statErr := os.Stat(outputPath)
	assert.True(t, os.IsNotExist(statErr),
		"a tool error must leave no output file at the destination; got statErr=%v", statErr)

	// No leftover temp files in the output dir either.
	entries, _ := os.ReadDir(filepath.Dir(outputPath))
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".kapi-out-", "temp file must be cleaned up on error")
	}
}

// TestFileRunner_ContextCancellationAborts verifies that cancelling the
// context aborts the run promptly without deadlock and without producing
// a destination file.
func TestFileRunner_ContextCancellationAborts(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")

	var sb strings.Builder
	sb.WriteString("{\n")
	for i := 0; i < 2000; i++ {
		if i > 0 {
			sb.WriteString(",\n")
		}
		fmt.Fprintf(&sb, "  %q: %q", fmt.Sprintf("k%05d", i), "value here")
	}
	sb.WriteString("\n}\n")
	require.NoError(t, os.WriteFile(inputPath, []byte(sb.String()), 0o644))

	outputPath := filepath.Join(dir, "out", "input.json")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the run starts so the pipeline observes Done

	runErr := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	}).RunFile(ctx, "pseudo-translate", []tool.Tool{pseudoTool},
		inputPath, outputPath, "qps")

	require.Error(t, runErr, "a cancelled context must abort the run")
	assert.ErrorIs(t, runErr, context.Canceled)

	_, statErr := os.Stat(outputPath)
	assert.True(t, os.IsNotExist(statErr),
		"a cancelled run must leave no output file; got statErr=%v", statErr)
}
