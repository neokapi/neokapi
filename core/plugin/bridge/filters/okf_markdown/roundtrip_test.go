//go:build integration

package okf_markdown

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte("# Hello World\n\nThis is a paragraph.\n")
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.md", mimeType, nil)
}

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Known failing: Okapi MarkdownFilter whitespace/structure normalization.
	// - HTML block files: Okapi normalizes newlines in HTML blocks, producing
	//   slightly different whitespace on re-read (documented Okapi limitation).
	// - example1/example3: Okapi HTML sub-filter normalizes indentation within
	//   embedded HTML content (e.g. 4 spaces → 5 spaces).
	// - deployconfigure-reality: Okapi merges adjacent Data parts in complex
	//   markdown with HTML comments, producing 3 fewer parts on re-read.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_markdown/*.md", mimeType, nil,
		"test-html-block-newline.md",
		"html_list_original.md",
		"html_table_changed.md",
		"admonitions.md",
		"html_list_changed.md",
		"html-table-w-empty-lines.md",
		"html_table1_original.md",
		"example1.md",
		"example3.md",
		"deployconfigure-reality.md",
	)
}
