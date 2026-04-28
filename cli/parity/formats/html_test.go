//go:build parity

package formats

import (
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/html"
)

func TestParityHTML(t *testing.T) {
	const filterClass = "okf_html"

	cases := []struct {
		name  string
		input string
	}{
		{
			name:  "minimal",
			input: `<html><body><p>Hello world.</p></body></html>`,
		},
		{
			name:  "inline-codes",
			input: `<html><body><p>Click <a href="/x">here</a> to continue.</p></body></html>`,
		},
		{
			name:  "two-paragraphs",
			input: `<html><body><p>First.</p><p>Second.</p></body></html>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.input)

			native := parity.RunNative(t, parity.NativeRequest{
				NewReader:  func() format.DataFormatReader { return html.NewReader() },
				InputBytes: input,
				MimeType:   "text/html",
				URI:        "test.html",
			})
			bridge := parity.RunBridge(t, parity.BridgeRequest{
				FilterClass: parity.FilterFQCN(filterClass),
				InputBytes:  input,
				MimeType:    "text/html",
			})

			parity.CompareBlockText(t, native, bridge)

			parity.Report(t, parity.Outcome{
				Kind: "format",
				ID:   filterClass,
				Name: t.Name(),
				Mode: "head-to-head",
			})
		})
	}
}
