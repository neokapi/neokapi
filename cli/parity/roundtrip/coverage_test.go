//go:build parity

package roundtrip_test

import (
	"testing"

	"github.com/neokapi/neokapi/cli/parity/roundtrip"
	"github.com/neokapi/neokapi/core/registry"
)

// Each entry exercises one format end-to-end through every available
// engine. Adding a new format means adding one row plus, if the
// bridge filter expects different parameter names, a translator in
// cli/parity/formats/ — same as the spec runner.
//
// Inputs are intentionally small but multi-block so the comparison
// distinguishes order, count, and per-block content.
type fixture struct {
	name        string
	formatID    registry.FormatID
	filterClass string
	filename    string
	body        []byte
	skip        []string
}

func TestRoundTrip_Coverage(t *testing.T) {
	cases := []fixture{
		{
			name:        "html_paragraphs",
			formatID:    "html",
			filterClass: "okf_html",
			filename:    "doc.html",
			body: []byte(
				"<!doctype html>\n" +
					"<html><body>\n" +
					"<p>First paragraph.</p>\n" +
					"<p>Second one.</p>\n" +
					"</body></html>\n"),
		},
		{
			name:        "po_two_entries",
			formatID:    "po",
			filterClass: "okf_po",
			filename:    "messages.po",
			body: []byte(
				`msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"

msgid "Hello"
msgstr ""

msgid "Goodbye"
msgstr ""
`),
		},
		{
			name:        "properties_two_keys",
			formatID:    "properties",
			filterClass: "okf_properties",
			filename:    "messages.properties",
			body: []byte(
				`greeting=Hello world
farewell=Goodbye now
`),
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			roundtrip.RunThreeWay(t, roundtrip.Case{
				Name:     c.name,
				FormatID: c.formatID,
				Input: roundtrip.Input{
					Bytes:    c.body,
					Filename: c.filename,
				},
				ExpectedSkipped: c.skip,
			},
				&roundtrip.NativeEngine{FormatID: c.formatID},
				&roundtrip.BridgeEngine{FilterClass: c.filterClass},
				&roundtrip.TikalEngine{},
			)
		})
	}
}
