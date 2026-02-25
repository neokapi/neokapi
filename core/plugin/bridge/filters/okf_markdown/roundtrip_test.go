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

	// Known failing files from Java's RoundTripMarkdownIT:
	// - HTML block/table/list handling issues in Okapi's markdown filter
	// - subfilter cast errors on HTML-in-markdown content
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_markdown/*.md", mimeType, nil,
		"test-html-block-newline.md",
		"html_list_original.md",
		"html_table_changed.md",
		"admonitions.md",
		"html_list_changed.md",
		"html-table-w-empty-lines.md",
		"html_table1_original.md",
		// Subfilter handling issues (HTML-in-markdown)
		"DirectShape.md",
		"example1.md",
		"example3.md",
		"html-cdata-sample-uppercased.md",
		"html-cdata-sample.md",
		"img_w_alt_attr_original.md",
		"min_math_original.md",
		"quotes-after-html-in-table.md",
		"regressing_test_single_page.md",
		"sample_html_combo.md",
		"ul-in-table.md",
	)
}
