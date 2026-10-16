package preset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFormatRef(t *testing.T) {
	cases := []struct {
		in   string
		want FormatRef
		reg  string
	}{
		{"okf_openxml", FormatRef{Name: "okf_openxml"}, "okf_openxml"},
		{"okf_openxml@0.38", FormatRef{Name: "okf_openxml", Version: "0.38"}, "okf_openxml@0.38"},
		{"okf_openxml:wellFormed", FormatRef{Name: "okf_openxml", Preset: "wellFormed"}, "okf_openxml"},
		{"okf_openxml@0.38:wellFormed", FormatRef{Name: "okf_openxml", Version: "0.38", Preset: "wellFormed"}, "okf_openxml@0.38"},
		{"html@okapi-bridge", FormatRef{Name: "html", Engine: "okapi-bridge"}, "html"},
		{"html@native", FormatRef{Name: "html", Engine: "native"}, "html"},
		{"html@okapi-bridge@1.47.0:strict", FormatRef{Name: "html", Engine: "okapi-bridge", Version: "1.47.0", Preset: "strict"}, "html@1.47.0"},
		{"html@1.47.0@okapi-bridge", FormatRef{Name: "html", Engine: "okapi-bridge", Version: "1.47.0"}, "html@1.47.0"},
		{"@odd", FormatRef{Name: "@odd"}, "@odd"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := ParseFormatRef(tc.in)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.reg, got.RegistryName())
			assert.Equal(t, tc.want.Engine != "", got.HasEngine())
		})
	}
}

// String renders what ParseFormatRef reads, in the canonical order.
func TestFormatRef_StringRoundTrips(t *testing.T) {
	for _, s := range []string{"json", "okf_openxml@0.38", "okf_html:strict", "html@okapi-bridge", "html@okapi-bridge@1.47.0:strict"} {
		assert.Equal(t, s, ParseFormatRef(s).String())
	}
	assert.Equal(t, "html@okapi-bridge@1.47.0", ParseFormatRef("html@1.47.0@okapi-bridge").String())
}
