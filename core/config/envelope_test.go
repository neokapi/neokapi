package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKind_IsValid(t *testing.T) {
	tests := []struct {
		kind Kind
		want bool
	}{
		{KindFormatConfig, true},
		{KindFormatPreset, true},
		{KindFlowDefinition, true},
		{KindProjectConfig, true},
		{Kind("Unknown"), false},
		{Kind(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.kind.IsValid())
		})
	}
}

func TestValidKinds(t *testing.T) {
	kinds := ValidKinds()
	assert.Len(t, kinds, 4)
	assert.Contains(t, kinds, KindFormatConfig)
	assert.Contains(t, kinds, KindFormatPreset)
	assert.Contains(t, kinds, KindFlowDefinition)
	assert.Contains(t, kinds, KindProjectConfig)
}

func TestParseAPIVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    APIVersion
		wantErr bool
	}{
		{
			name:  "gokapi html v1",
			input: "gokapi/html-v1",
			want:  APIVersion{Namespace: "gokapi", Resource: "html", Version: 1},
		},
		{
			name:  "okapi json v2",
			input: "okapi/json-v2",
			want:  APIVersion{Namespace: "okapi", Resource: "json", Version: 2},
		},
		{
			name:  "gokapi project v1",
			input: "gokapi/project-v1",
			want:  APIVersion{Namespace: "gokapi", Resource: "project", Version: 1},
		},
		{
			name:  "gokapi flow v1",
			input: "gokapi/flow-v1",
			want:  APIVersion{Namespace: "gokapi", Resource: "flow", Version: 1},
		},
		{
			name:  "gokapi preset v1",
			input: "gokapi/preset-v1",
			want:  APIVersion{Namespace: "gokapi", Resource: "preset", Version: 1},
		},
		{
			name:  "gokapi markdown v1",
			input: "gokapi/markdown-v1",
			want:  APIVersion{Namespace: "gokapi", Resource: "markdown", Version: 1},
		},
		{
			name:  "high version number",
			input: "gokapi/html-v42",
			want:  APIVersion{Namespace: "gokapi", Resource: "html", Version: 42},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "no slash",
			input:   "gokapi-html-v1",
			wantErr: true,
		},
		{
			name:    "no version suffix",
			input:   "gokapi/html",
			wantErr: true,
		},
		{
			name:    "empty namespace",
			input:   "/html-v1",
			wantErr: true,
		},
		{
			name:    "empty resource",
			input:   "gokapi/-v1",
			wantErr: true,
		},
		{
			name:    "zero version",
			input:   "gokapi/html-v0",
			wantErr: true,
		},
		{
			name:    "negative version",
			input:   "gokapi/html-v-1",
			wantErr: true,
		},
		{
			name:    "non-numeric version",
			input:   "gokapi/html-vabc",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAPIVersion(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAPIVersion_String(t *testing.T) {
	av := APIVersion{Namespace: "gokapi", Resource: "html", Version: 1}
	assert.Equal(t, "gokapi/html-v1", av.String())
}

func TestAPIVersion_ResourceKey(t *testing.T) {
	av := APIVersion{Namespace: "gokapi", Resource: "html", Version: 1}
	assert.Equal(t, "gokapi/html", av.ResourceKey())
}
