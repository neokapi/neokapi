//go:build integration

package bridge

import (
	"bytes"
	"context"
	"io"
	"log"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These integration tests verify that Okapi bridge step tools work end-to-end.
// They require:
//   - NEOKAPI_BRIDGE_JAR set to the path of the bridge JAR
//   - Java available on PATH
//
// Test patterns are adapted from Okapi Framework's own integration tests:
//   - MultistepPipelineTestIT.searchAndReplacePipeline
//   - SearchAndReplaceTest.replaceSourceCharacter
//   - MultistepPipelineTestIT.segmentationPipeline

// setupBridgeRegistry creates a BridgeRegistry and warms up a bridge.
func setupBridgeRegistry(t *testing.T) (*BridgeRegistry, BridgeConfig) {
	t.Helper()
	jarPath := skipIfNoJAR(t)

	cfg := BridgeConfig{
		Command: "java",
		Args:    []string{"-jar", jarPath},
	}

	reg := NewBridgeRegistry(1, 1, log.Default())
	require.NoError(t, reg.Warmup(cfg))
	t.Cleanup(func() { reg.Shutdown() })

	return reg, cfg
}

// readHTMLParts reads an HTML string through the bridge and returns the parts.
func readHTMLParts(t *testing.T, reg *BridgeRegistry, cfg BridgeConfig, html string) []*model.Part {
	t.Helper()
	reader := NewBridgeFormatReader(reg, cfg,
		"net.sf.okapi.filters.html.HtmlFilter",
		format.FormatSignature{})

	doc := &model.RawDocument{
		URI:          "test.html",
		SourceLocale: "en",
		TargetLocale: "fr",
		Encoding:     "UTF-8",
		MimeType:     "text/html",
		Reader:       io.NopCloser(bytes.NewReader([]byte(html))),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
	}
	require.NoError(t, reader.Close())
	return parts
}

// TestIntegrationSearchAndReplaceStep runs the SearchAndReplaceStep through
// the bridge's ProcessStep RPC. Adapted from Okapi's SearchAndReplaceTest
// and MultistepPipelineTestIT.searchAndReplacePipeline.
//
// Pipeline: HTML reader → SearchAndReplaceStep → verify parts flow through.
func TestIntegrationSearchAndReplaceStep(t *testing.T) {
	reg, cfg := setupBridgeRegistry(t)

	// 1. Read HTML through bridge to get Parts.
	html := `<html><body><p>The Okapi Framework is great.</p><p>Use the Okapi Framework for localization.</p></body></html>`
	parts := readHTMLParts(t, reg, cfg, html)
	require.NotEmpty(t, parts)

	// 2. Create the step tool: replace "Okapi Framework" → "neokapi"
	// SearchAndReplace rules use Okapi's StringParameters serialization:
	// count=N, use0/search0/replace0, use1/search1/replace1, etc.
	stepTool := NewBridgeStepTool(reg, cfg,
		"net.sf.okapi.steps.searchandreplace.SearchAndReplaceStep",
		"search-and-replace",
		"Search and Replace",
		nil,
	)
	stepTool.SetLocales("en", "fr")
	stepTool.SetStepParams(map[string]any{
		"source":   true,
		"target":   false,
		"regEx":    false,
		"count":    1,
		"use0":     "true",
		"search0":  "Okapi Framework",
		"replace0": "neokapi",
	})

	// 3. Run parts through the step.
	in := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	out := make(chan *model.Part, len(parts)*2)
	ctx := context.Background()
	err := stepTool.Process(ctx, in, out)
	close(out)
	require.NoError(t, err)

	// 4. Collect output parts — verify the step processed without errors
	// and returned the correct number of parts.
	var outputParts []*model.Part
	for p := range out {
		outputParts = append(outputParts, p)
	}
	assert.Equal(t, len(parts), len(outputParts),
		"step should return the same number of parts")
}

// TestIntegrationSearchAndReplaceRegex tests regex-based search and replace,
// adapted from Okapi's SearchAndReplaceTest.replaceSourceCharacter.
func TestIntegrationSearchAndReplaceRegex(t *testing.T) {
	reg, cfg := setupBridgeRegistry(t)

	html := `<html><body><p>{nb}{tab}{em} some text {en}{emsp}{ensp}</p></body></html>`
	parts := readHTMLParts(t, reg, cfg, html)

	stepTool := NewBridgeStepTool(reg, cfg,
		"net.sf.okapi.steps.searchandreplace.SearchAndReplaceStep",
		"search-and-replace",
		"Search and Replace",
		nil,
	)
	stepTool.SetLocales("en", "fr")
	stepTool.SetStepParams(map[string]any{
		"source":   true,
		"target":   false,
		"regEx":    true,
		"count":    1,
		"use0":     "true",
		"search0":  "\\{nb\\}|\\{tab\\}|\\{em\\}|\\{en\\}|\\{emsp\\}|\\{ensp\\}",
		"replace0": "",
	})

	in := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	out := make(chan *model.Part, len(parts)*2)
	err := stepTool.Process(context.Background(), in, out)
	close(out)
	require.NoError(t, err)

	for p := range out {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			for _, seg := range block.Source {
				text := seg.Text()
				if text != "" {
					assert.NotContains(t, text, "{nb}")
					assert.NotContains(t, text, "{tab}")
					assert.NotContains(t, text, "{em}")
					assert.Contains(t, text, "some text")
				}
			}
		}
	}
}

// TestIntegrationStepProcessesWithoutError verifies that a configured step
// processes all parts through the bridge without errors.
func TestIntegrationStepProcessesWithoutError(t *testing.T) {
	reg, cfg := setupBridgeRegistry(t)

	html := `<html><body><p>Hello World</p></body></html>`
	parts := readHTMLParts(t, reg, cfg, html)

	stepTool := NewBridgeStepTool(reg, cfg,
		"net.sf.okapi.steps.searchandreplace.SearchAndReplaceStep",
		"search-and-replace",
		"Search and Replace",
		nil,
	)
	stepTool.SetLocales("en", "fr")
	stepTool.SetStepParams(map[string]any{
		"source":   false,
		"target":   true,
		"regEx":    false,
		"count":    1,
		"use0":     "true",
		"search0":  "World",
		"replace0": "Monde",
	})

	in := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	out := make(chan *model.Part, len(parts)*2)
	err := stepTool.Process(context.Background(), in, out)
	close(out)
	require.NoError(t, err)

	var outputParts []*model.Part
	for p := range out {
		outputParts = append(outputParts, p)
	}
	assert.Equal(t, len(parts), len(outputParts),
		"step should return the same number of parts")
}
