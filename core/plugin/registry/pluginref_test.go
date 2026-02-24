package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePluginRef(t *testing.T) {
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
			ref := ParsePluginRef(tt.input)
			assert.Equal(t, tt.name, ref.Name)
			assert.Equal(t, tt.version, ref.Version)
		})
	}
}

func TestPluginRefString(t *testing.T) {
	assert.Equal(t, "okapi@1.46.0", PluginRef{Name: "okapi", Version: "1.46.0"}.String())
	assert.Equal(t, "okapi", PluginRef{Name: "okapi"}.String())
}

func TestPluginRefIsVersioned(t *testing.T) {
	assert.True(t, PluginRef{Name: "okapi", Version: "1.0.0"}.IsVersioned())
	assert.False(t, PluginRef{Name: "okapi"}.IsVersioned())
}

func TestParseFormatRef(t *testing.T) {
	tests := []struct {
		input   string
		name    string
		version string
		preset  string
	}{
		{"okapi-html", "okapi-html", "", ""},
		{"okapi-html@1.46.0", "okapi-html", "1.46.0", ""},
		{"csv", "csv", "", ""},
		{"okf_html@wellFormed", "okf_html", "", "wellFormed"},
		{"okf_html@1.46.0", "okf_html", "1.46.0", ""},
		{"okf_html@strict-mode", "okf_html", "", "strict-mode"},
		{"json", "json", "", ""},
		{"json@1.0", "json", "1.0", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ref := ParseFormatRef(tt.input)
			assert.Equal(t, tt.name, ref.Name)
			assert.Equal(t, tt.version, ref.Version)
			assert.Equal(t, tt.preset, ref.Preset)
		})
	}
}

func TestFormatRefString(t *testing.T) {
	assert.Equal(t, "okapi-html@1.46.0", FormatRef{Name: "okapi-html", Version: "1.46.0"}.String())
	assert.Equal(t, "okapi-html", FormatRef{Name: "okapi-html"}.String())
	assert.Equal(t, "okf_html@wellFormed", FormatRef{Name: "okf_html", Preset: "wellFormed"}.String())
}

func TestFormatRefIsVersioned(t *testing.T) {
	assert.True(t, FormatRef{Name: "okapi-html", Version: "1.0.0"}.IsVersioned())
	assert.False(t, FormatRef{Name: "okapi-html"}.IsVersioned())
}

func TestFormatRefIsPreset(t *testing.T) {
	assert.True(t, FormatRef{Name: "okf_html", Preset: "wellFormed"}.IsPreset())
	assert.False(t, FormatRef{Name: "okf_html", Version: "1.0.0"}.IsPreset())
	assert.False(t, FormatRef{Name: "json"}.IsPreset())
}

func TestCompareSemver(t *testing.T) {
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
			assert.Equal(t, tt.want, CompareSemver(tt.a, tt.b))
		})
	}
}

func TestLatestVersion(t *testing.T) {
	assert.Equal(t, "", LatestVersion(nil))
	assert.Equal(t, "", LatestVersion([]string{}))
	assert.Equal(t, "1.0.0", LatestVersion([]string{"1.0.0"}))
	assert.Equal(t, "1.47.0", LatestVersion([]string{"1.46.0", "1.47.0", "1.45.0"}))
	assert.Equal(t, "2.0.0", LatestVersion([]string{"1.0.0", "2.0.0", "1.5.0"}))
}
