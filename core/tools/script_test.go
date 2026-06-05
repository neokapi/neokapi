package tools_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processAllParts sends parts through a tool and collects all output parts.
func processAllParts(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, parts []*model.Part) []*model.Part {
	t.Helper()
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts)*2)
	for _, p := range parts {
		in <- p
	}
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)
	require.NoError(t, err)

	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}
	return results
}

func TestScriptPassThrough(t *testing.T) {
	t.Parallel()
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: "emit(part)"})

	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello World", resultBlock.SourceText())
}

func TestScriptFilterByTextLength(t *testing.T) {
	t.Parallel()
	code := `
		if (part.type === "block" && part.block.source[0].content.text.length > 5) {
			emit(part);
		} else {
			skip();
		}
	`
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: code})

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hi")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "Hello World")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu3", "Bye")},
	}

	results := processAllParts(t, tl, parts)
	require.Len(t, results, 1)

	resultBlock := results[0].Resource.(*model.Block)
	assert.Equal(t, "Hello World", resultBlock.SourceText())
}

func TestScriptModifyTargetText(t *testing.T) {
	t.Parallel()
	code := `
		if (part.type === "block") {
			part.block.targets["fr"] = [{content: {text: "Bonjour"}}];
			emit(part);
		}
	`
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: code})

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour", resultBlock.TargetText("fr"))
}

func TestScriptModifySourceTextInPlace(t *testing.T) {
	t.Parallel()
	// In-place edits to source text must round-trip, not only whole-array
	// reassignment — source/targets are exposed as native JS arrays so nested
	// mutations reflect on readback. Source mutation is opt-in per the
	// immutability model (AD-006), so the script declares AllowSourceMutation.
	code := `
		if (part.type === "block") {
			part.block.source[0].content.text = part.block.source[0].content.text.toUpperCase();
			emit(part);
		}
	`
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: code, AllowSourceMutation: true})

	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "HELLO WORLD", resultBlock.SourceText())
}

func TestScriptFunctionFormReturnEmits(t *testing.T) {
	t.Parallel()
	// A process(part) function is detected and called per Part; returning the
	// part emits it (with edits applied). JSDoc on the param is a comment, so
	// goja runs the body fine. Source mutation is opt-in (AD-006).
	code := `
		/** @param {Part} part */
		function process(part) {
			if (part.type === "block") {
				part.block.source[0].content.text = part.block.source[0].content.text.toUpperCase();
			}
			return part;
		}
	`
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: code, AllowSourceMutation: true})
	block := model.NewBlock("tu1", "hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)
	assert.Equal(t, "HELLO", result.Resource.(*model.Block).SourceText())
}

func TestScriptFunctionFormReturnNullSkips(t *testing.T) {
	t.Parallel()
	code := `
		function process(part) {
			if (part.type === "block" && part.block.source[0].content.text.length <= 5) return null;
			return part;
		}
	`
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: code})
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hi")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "Hello World")},
	}
	results := processAllParts(t, tl, parts)
	require.Len(t, results, 1)
	assert.Equal(t, "Hello World", results[0].Resource.(*model.Block).SourceText())
}

func TestScriptFunctionFormEmitSkipInside(t *testing.T) {
	t.Parallel()
	// emit()/skip() still work inside process(); the return value is optional.
	code := `
		function process(part) {
			if (part.type === "block") { skip(); return; }
			emit(part);
		}
	`
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: code})
	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)
	assert.Nil(t, <-out)
}

func TestScriptFunctionFormReturnArrayEmitsMany(t *testing.T) {
	t.Parallel()
	code := `function process(part) { return [part, part]; }`
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: code})
	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	results := processAllParts(t, tl, []*model.Part{part})
	assert.Len(t, results, 2)
}

func TestScriptSkipDropsParts(t *testing.T) {
	t.Parallel()
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: "skip()"})

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)
	require.NoError(t, err)

	// No parts should be emitted.
	result := <-out
	assert.Nil(t, result)
}

func TestScriptLog(t *testing.T) {
	t.Parallel()
	// Redirect stderr to capture log output.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: `log("test message"); emit(part);`})
	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	processPart(t, tl, part)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	assert.Contains(t, buf.String(), "test message")
}

func TestScriptEmptyPassesThrough(t *testing.T) {
	t.Parallel()
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: ""})

	// Empty code + empty scriptfile should fail validation, but
	// for the process test we use a script with only whitespace.
	tl2 := tools.NewScriptTool(&tools.ScriptConfig{Code: "  "})

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	// The tool with whitespace-only code should pass through.
	result := processPart(t, tl2, part)
	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello", resultBlock.SourceText())

	// Verify config validation catches empty code.
	err := tl.Config().Validate()
	require.Error(t, err)
}

func TestScriptInvalidCodeReturnsError(t *testing.T) {
	t.Parallel()
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: "this is not valid javascript %%% {"})

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compile error")
}

func TestScriptFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "test.js")
	err := os.WriteFile(scriptPath, []byte(`
		if (part.type === "block") {
			part.block.targets["de"] = [{content: {text: "Hallo Welt"}}];
			emit(part);
		}
	`), 0644)
	require.NoError(t, err)

	tl := tools.NewScriptTool(&tools.ScriptConfig{ScriptFile: scriptPath})

	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hallo Welt", resultBlock.TargetText("de"))
}

func TestScriptDefaultPassThrough(t *testing.T) {
	t.Parallel()
	// Script that does not call emit or skip should pass through by default.
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: "var x = 1;"})

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello", resultBlock.SourceText())
}

func TestScriptNonBlockPartTypes(t *testing.T) {
	t.Parallel()
	tl := tools.NewScriptTool(&tools.ScriptConfig{Code: `
		if (part.type === "data") {
			emit(part);
		} else if (part.type === "layer-start") {
			emit(part);
		} else {
			skip();
		}
	`})

	layer := &model.Layer{ID: "doc1", Name: "test.html"}
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: layer},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")},
	}

	results := processAllParts(t, tl, parts)
	require.Len(t, results, 2)
	assert.Equal(t, model.PartLayerStart, results[0].Type)
	assert.Equal(t, model.PartData, results[1].Type)
}

func TestScriptConfigValidation(t *testing.T) {
	t.Parallel()
	// Inline mode with no code: error.
	cfg := &tools.ScriptConfig{Source: "inline"}
	require.Error(t, cfg.Validate())

	// Inline mode with code: ok.
	cfg = &tools.ScriptConfig{Source: "inline", Code: "emit(part)"}
	require.NoError(t, cfg.Validate())

	// Default source (empty) with code: ok (defaults to inline).
	cfg = &tools.ScriptConfig{Code: "emit(part)"}
	require.NoError(t, cfg.Validate())

	// Default source (empty) with no code: error.
	cfg = &tools.ScriptConfig{}
	require.Error(t, cfg.Validate())

	// File mode with file: ok.
	cfg = &tools.ScriptConfig{Source: "file", ScriptFile: "test.js"}
	require.NoError(t, cfg.Validate())

	// File mode with no file: error.
	cfg = &tools.ScriptConfig{Source: "file"}
	require.Error(t, cfg.Validate())

	// Reset clears values and sets source to inline.
	cfg.Reset()
	assert.Equal(t, "inline", cfg.Source)
	assert.Empty(t, cfg.Code)
	assert.Empty(t, cfg.ScriptFile)
}
