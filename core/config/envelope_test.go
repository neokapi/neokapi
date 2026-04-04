package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKind_IsValid(t *testing.T) {
	tests := []struct {
		kind Kind
		want bool
	}{
		{KindFormatPreset, true},
		{KindFlowDefinition, true},
		{KindProjectConfig, true},
		{FormatConfigKind("html"), true},
		{FormatConfigKind("json"), true},
		{OkapiFilterConfigKind("html"), true},
		{Kind("Unknown"), false},
		{Kind(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.kind.IsValid())
		})
	}
}

func TestFormatConfigKind(t *testing.T) {
	assert.Equal(t, Kind("HtmlFormatConfig"), FormatConfigKind("html"))
	assert.Equal(t, Kind("JsonFormatConfig"), FormatConfigKind("json"))
	assert.Equal(t, Kind("XmlFormatConfig"), FormatConfigKind("xml"))
	assert.Equal(t, Kind("Xliff2FormatConfig"), FormatConfigKind("xliff2"))
	assert.Equal(t, Kind("PlaintextFormatConfig"), FormatConfigKind("plaintext"))
}

func TestOkapiFilterConfigKind(t *testing.T) {
	assert.Equal(t, Kind("OkfHtmlFilterConfig"), OkapiFilterConfigKind("html"))
	assert.Equal(t, Kind("OkfJsonFilterConfig"), OkapiFilterConfigKind("json"))
	assert.Equal(t, Kind("OkfXmlFilterConfig"), OkapiFilterConfigKind("xml"))
}

func TestIsFormatConfigKind(t *testing.T) {
	assert.True(t, IsFormatConfigKind(FormatConfigKind("html")))
	assert.True(t, IsFormatConfigKind(OkapiFilterConfigKind("html")))
	assert.False(t, IsFormatConfigKind(KindProjectConfig))
	assert.False(t, IsFormatConfigKind(KindFlowDefinition))
	assert.False(t, IsFormatConfigKind(Kind("Unknown")))
}

func TestParseAPIVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "v1", input: "v1", want: 1},
		{name: "v2", input: "v2", want: 2},
		{name: "v42", input: "v42", want: 42},
		{name: "empty", input: "", wantErr: true},
		{name: "no v prefix", input: "1", wantErr: true},
		{name: "v0", input: "v0", wantErr: true},
		{name: "v-1", input: "v-1", wantErr: true},
		{name: "vabc", input: "vabc", wantErr: true},
		{name: "old slash format", input: "neokapi/html/v1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAPIVersion(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatAPIVersion(t *testing.T) {
	assert.Equal(t, "v1", FormatAPIVersion(1))
	assert.Equal(t, "v42", FormatAPIVersion(42))
}
