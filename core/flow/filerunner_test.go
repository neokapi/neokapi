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
