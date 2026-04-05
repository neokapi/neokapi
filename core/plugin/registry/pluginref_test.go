package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePluginRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		name    string
		version string
	}{
		{"okapi", "okapi", ""},
		{"okapi@1.46.0", "okapi", "1.46.0"},
		{"my-tool@2.0.0", "my-tool", "2.0.0"},
		{"@invalid", "@invalid", ""}, // no name before @, index is 0 so not split
		{"name@", "name", ""},        // trailing @, split but empty version
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			ref := ParsePluginRef(tt.input)
			assert.Equal(t, tt.name, ref.Name)
			assert.Equal(t, tt.version, ref.Version)
		})
	}
}

func TestPluginRefString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "okapi@1.46.0", PluginRef{Name: "okapi", Version: "1.46.0"}.String())
	assert.Equal(t, "okapi", PluginRef{Name: "okapi"}.String())
}

func TestPluginRefIsVersioned(t *testing.T) {
	t.Parallel()
	assert.True(t, PluginRef{Name: "okapi", Version: "1.0.0"}.IsVersioned())
	assert.False(t, PluginRef{Name: "okapi"}.IsVersioned())
}

func TestParseFormatRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		name    string
		version string
		preset  string
	}{
		{"okapi-html", "okapi-html", "", ""},
		{"okapi-html@1.46.0", "okapi-html", "1.46.0", ""},
		{"csv", "csv", "", ""},
		{"okf_html:wellFormed", "okf_html", "", "wellFormed"},
		{"okf_html@1.46.0", "okf_html", "1.46.0", ""},
		{"okf_html:strict-mode", "okf_html", "", "strict-mode"},
		{"json", "json", "", ""},
		{"json@1.0", "json", "1.0", ""},
		{"okf_openxml@0.38:wellFormed", "okf_openxml", "0.38", "wellFormed"},
		{"okf_openxml@0.38", "okf_openxml", "0.38", ""},
		{"okf_openxml:wellFormed", "okf_openxml", "", "wellFormed"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			ref := ParseFormatRef(tt.input)
			assert.Equal(t, tt.name, ref.Name)
			assert.Equal(t, tt.version, ref.Version)
			assert.Equal(t, tt.preset, ref.Preset)
		})
	}
}

func TestFormatRefString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "okapi-html@1.46.0", FormatRef{Name: "okapi-html", Version: "1.46.0"}.String())
	assert.Equal(t, "okapi-html", FormatRef{Name: "okapi-html"}.String())
	assert.Equal(t, "okf_html:wellFormed", FormatRef{Name: "okf_html", Preset: "wellFormed"}.String())
	assert.Equal(t, "okf_openxml@0.38:wellFormed", FormatRef{Name: "okf_openxml", Version: "0.38", Preset: "wellFormed"}.String())
}

func TestFormatRefRegistryName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "okf_html", FormatRef{Name: "okf_html"}.RegistryName())
	assert.Equal(t, "okf_html@1.46.0", FormatRef{Name: "okf_html", Version: "1.46.0"}.RegistryName())
	assert.Equal(t, "okf_html", FormatRef{Name: "okf_html", Preset: "wellFormed"}.RegistryName())
	assert.Equal(t, "okf_html@1.46.0", FormatRef{Name: "okf_html", Version: "1.46.0", Preset: "wellFormed"}.RegistryName())
}

func TestFormatRefIsVersioned(t *testing.T) {
	t.Parallel()
	assert.True(t, FormatRef{Name: "okapi-html", Version: "1.0.0"}.IsVersioned())
	assert.False(t, FormatRef{Name: "okapi-html"}.IsVersioned())
}

func TestFormatRefIsPreset(t *testing.T) {
	t.Parallel()
	assert.True(t, FormatRef{Name: "okf_html", Preset: "wellFormed"}.IsPreset())
	assert.False(t, FormatRef{Name: "okf_html", Version: "1.0.0"}.IsPreset())
	assert.False(t, FormatRef{Name: "json"}.IsPreset())
}

func TestCompareSemver(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.1", "1.0.0", 1},
		{"1.46.0", "1.47.0", -1},
		{"1.47.0", "1.46.0", 1},
		{"2.0.0", "1.99.99", 1},
		{"0.0.1", "0.0.2", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, CompareSemver(tt.a, tt.b))
		})
	}
}

func TestSemverRange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		rangeStr string
		version  string
		want     bool
	}{
		// Exact.
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"1.2.3", "1.2.2", false},

		// Star / any.
		{"*", "0.0.1", true},
		{"*", "99.99.99", true},
		{"", "1.0.0", true},

		// Caret (compatible).
		{"^1.2.3", "1.2.3", true},
		{"^1.2.3", "1.9.9", true},
		{"^1.2.3", "1.2.2", false},
		{"^1.2.3", "2.0.0", false},
		{"^1.0.0", "1.99.99", true},
		{"^0.2.3", "0.2.5", true},
		{"^0.2.3", "0.3.0", false}, // ^0.x treats minor as major

		// Tilde (patch-level).
		{"~1.2.3", "1.2.3", true},
		{"~1.2.3", "1.2.9", true},
		{"~1.2.3", "1.3.0", false},
		{"~1.2.3", "1.2.2", false},

		// Greater/equal.
		{">=1.2.3", "1.2.3", true},
		{">=1.2.3", "2.0.0", true},
		{">=1.2.3", "1.2.2", false},

		// Greater than.
		{">1.2.3", "1.2.4", true},
		{">1.2.3", "1.2.3", false},

		// Less/equal.
		{"<=1.2.3", "1.2.3", true},
		{"<=1.2.3", "1.2.2", true},
		{"<=1.2.3", "1.2.4", false},

		// Less than.
		{"<1.2.3", "1.2.2", true},
		{"<1.2.3", "1.2.3", false},
	}

	for _, tt := range tests {
		t.Run(tt.rangeStr+"_"+tt.version, func(t *testing.T) {
			t.Parallel()
			r := ParseSemverRange(tt.rangeStr)
			assert.Equal(t, tt.want, r.Match(tt.version), "ParseSemverRange(%q).Match(%q)", tt.rangeStr, tt.version)
		})
	}
}

func TestSemverRangeString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "*", ParseSemverRange("*").String())
	assert.Equal(t, "1.2.3", ParseSemverRange("1.2.3").String())
	assert.Equal(t, "^1.2.3", ParseSemverRange("^1.2.3").String())
	assert.Equal(t, "~1.2.3", ParseSemverRange("~1.2.3").String())
	assert.Equal(t, ">=1.2.3", ParseSemverRange(">=1.2.3").String())
}

func TestLatestVersion(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", LatestVersion(nil))
	assert.Equal(t, "", LatestVersion([]string{}))
	assert.Equal(t, "1.0.0", LatestVersion([]string{"1.0.0"}))
	assert.Equal(t, "1.47.0", LatestVersion([]string{"1.46.0", "1.47.0", "1.45.0"}))
	assert.Equal(t, "2.0.0", LatestVersion([]string{"1.0.0", "2.0.0", "1.5.0"}))
}
