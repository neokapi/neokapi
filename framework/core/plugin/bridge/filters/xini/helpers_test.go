//go:build integration

package xini

import (
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.xini.XINIFilter"
const rainbowkitFilterClass = "net.sf.okapi.filters.xini.rainbowkit.XINIRainbowkitFilter"
const mimeType = "text/xml"

// readXINI reads a XINI file from testdata and returns parts.
func readXINI(t *testing.T, relPath string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xini/"+relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
}

// readXINIString parses a XINI string with custom filter params and returns parts.
func readXINIString(t *testing.T, content string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, content, "test.xini", mimeType, params)
}

// readXINIDefault reads a XINI file from testdata with default params.
func readXINIDefault(t *testing.T, relPath string) []*model.Part {
	t.Helper()
	return readXINI(t, relPath, nil)
}

// fileRoundtrip roundtrips a XINI testdata file and returns the output string.
func fileRoundtrip(t *testing.T, relPath string, params map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_xini/"+relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, params)
	return string(result.Output)
}

// snippetRoundtrip roundtrips a XINI string and returns the output string.
func snippetRoundtrip(t *testing.T, content string, params map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(content), "test.xini", mimeType, params)
	return string(result.Output)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// findBlockContaining finds a block whose source text contains the given substring.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// countPartsByType counts parts of a given type.
func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}

// groupStarts returns all GroupStart parts from a part list.
func groupStarts(parts []*model.Part) []*model.GroupStart {
	var result []*model.GroupStart
	for _, p := range parts {
		if p.Type == model.PartGroupStart {
			if gs, ok := p.Resource.(*model.GroupStart); ok {
				result = append(result, gs)
			}
		}
	}
	return result
}

// groupEnds returns all GroupEnd parts from a part list.
func groupEnds(parts []*model.Part) []*model.GroupEnd {
	var result []*model.GroupEnd
	for _, p := range parts {
		if p.Type == model.PartGroupEnd {
			if ge, ok := p.Resource.(*model.GroupEnd); ok {
				result = append(result, ge)
			}
		}
	}
	return result
}

// spanCount counts the total spans across all source segments of a block.
func spanCount(b *model.Block) int {
	n := 0
	for _, seg := range b.Source {
		if seg.Content != nil {
			n += len(seg.Content.Spans)
		}
	}
	return n
}

// hasSpanOfType checks if any source segment of a block has a span of the given type.
func hasSpanOfType(b *model.Block, st model.SpanType) bool {
	for _, seg := range b.Source {
		if seg.Content != nil {
			for _, s := range seg.Content.Spans {
				if s.SpanType == st {
					return true
				}
			}
		}
	}
	return false
}
