package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
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
