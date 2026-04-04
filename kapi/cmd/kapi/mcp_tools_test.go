package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testApp() *cli.App {
	a := &cli.App{}
	a.FormatReg = registry.NewFormatRegistry()
	formats.RegisterAll(a.FormatReg)
	return a
}

func TestHandleListFormats(t *testing.T) {
	a := testApp()
	_, out, err := handleListFormats(a)
	require.NoError(t, err)
	assert.NotEmpty(t, out.Formats)
	assert.Equal(t, len(out.Formats), out.Total)

	// Verify json format is present.
	var found bool
	for _, e := range out.Formats {
		if e.Name == "json" {
			found = true
			assert.True(t, e.HasReader)
			assert.True(t, e.HasWriter)
			assert.Equal(t, "built-in", e.Source)
			break
		}
	}
	assert.True(t, found, "json format should be in the list")
}

func TestHandleDetectFormat(t *testing.T) {
	a := testApp()

	tests := []struct {
		name       string
		path       string
		wantFormat string
		wantErr    bool
	}{
		{"json file", "test.json", "json", false},
		{"html file", "page.html", "html", false},
		{"no extension", "noext", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, out, err := handleDetectFormat(a, DetectFormatInput{Path: tt.path})
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantFormat, out.Format)
			}
		})
	}
}

func TestHandleExtractContent(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	// Use the JSON testdata fixture.
	fixturePath := filepath.Join("..", "..", "..", "core", "formats", "json", "testdata", "simple.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("test fixture not found:", fixturePath)
	}

	_, out, err := handleExtractContent(ctx, a, ExtractContentInput{
		Path:       fixturePath,
		SourceLang: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "json", out.Format)
	assert.NotEmpty(t, out.Blocks)
	assert.Greater(t, out.WordCount, 0)

	// Verify we have the expected blocks from simple.json.
	var texts []string
	for _, b := range out.Blocks {
		texts = append(texts, b.SourceText)
		assert.Greater(t, b.WordCount, 0)
		assert.NotEmpty(t, b.ID)
	}
	assert.Contains(t, texts, "Hello World")
}

func TestHandleWordCount(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	fixturePath := filepath.Join("..", "..", "..", "core", "formats", "json", "testdata", "simple.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("test fixture not found:", fixturePath)
	}

	_, out, err := handleWordCount(ctx, a, WordCountInput{
		Path:       fixturePath,
		SourceLang: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "json", out.Format)
	assert.Greater(t, out.WordCount, 0)
	assert.Greater(t, out.BlockCount, 0)
}

func TestHandleListFlows(t *testing.T) {
	_, out, err := handleListFlows()
	require.NoError(t, err)
	assert.NotEmpty(t, out.Flows)
	assert.Equal(t, len(out.Flows), out.Total)

	var names []string
	for _, f := range out.Flows {
		names = append(names, f.Name)
	}
	assert.Contains(t, names, "pseudo-translate")
	assert.Contains(t, names, "qa-check")
	assert.Contains(t, names, "ai-translate")
}

func TestHandleListTools(t *testing.T) {
	_, out, err := handleListTools()
	require.NoError(t, err)
	assert.NotEmpty(t, out.Tools)
	assert.Equal(t, len(out.Tools), out.Total)

	var names []string
	for _, tool := range out.Tools {
		names = append(names, tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.NotEmpty(t, tool.Source)
	}
	assert.Contains(t, names, "word-count")
	assert.Contains(t, names, "pseudo-translate")
}

func TestHandlePseudoTranslate(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	fixturePath := filepath.Join("..", "..", "..", "core", "formats", "json", "testdata", "simple.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("test fixture not found:", fixturePath)
	}

	// Use a temp dir for output.
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output_qps.json")

	_, out, err := handlePseudoTranslate(ctx, a, PseudoTranslateInput{
		Path:       fixturePath,
		OutputPath: outputPath,
	})
	require.NoError(t, err)
	assert.Equal(t, "pseudo-translate", out.FlowName)
	assert.Equal(t, fixturePath, out.InputPath)
	assert.Equal(t, outputPath, out.OutputPath)

	// Verify output file was written.
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

func TestHandleRunFlow(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	fixturePath := filepath.Join("..", "..", "..", "core", "formats", "json", "testdata", "simple.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("test fixture not found:", fixturePath)
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.json")

	_, out, err := handleRunFlow(ctx, a, RunFlowInput{
		FlowName:   "pseudo-translate",
		Path:       fixturePath,
		TargetLang: "qps",
		OutputPath: outputPath,
	})
	require.NoError(t, err)
	assert.Equal(t, "pseudo-translate", out.FlowName)
	assert.Equal(t, outputPath, out.OutputPath)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

func TestHandleRunFlowUnknown(t *testing.T) {
	a := testApp()
	ctx := t.Context()

	fixturePath := filepath.Join("..", "..", "..", "core", "formats", "json", "testdata", "simple.json")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("test fixture not found:", fixturePath)
	}

	_, _, err := handleRunFlow(ctx, a, RunFlowInput{
		FlowName:   "nonexistent",
		Path:       fixturePath,
		TargetLang: "fr",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flow")
}
