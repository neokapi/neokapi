package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFormatMappings(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    []FormatMapping
		wantErr bool
	}{
		{
			name:  "single mapping",
			input: []string{"*.docx=okf_openxml:test"},
			want:  []FormatMapping{{Pattern: "*.docx", Format: "okf_openxml:test"}},
		},
		{
			name:  "multiple mappings",
			input: []string{"*.docx=okf_openxml:test", "*.md=okf_markdown@0.38"},
			want: []FormatMapping{
				{Pattern: "*.docx", Format: "okf_openxml:test"},
				{Pattern: "*.md", Format: "okf_markdown@0.38"},
			},
		},
		{
			name:  "version and preset",
			input: []string{"*.html=okf_html@1.46.0:wellFormed"},
			want:  []FormatMapping{{Pattern: "*.html", Format: "okf_html@1.46.0:wellFormed"}},
		},
		{
			name:  "empty input",
			input: nil,
			want:  []FormatMapping{},
		},
		{
			name:    "missing equals",
			input:   []string{"*.docx"},
			wantErr: true,
		},
		{
			name:    "empty pattern",
			input:   []string{"=okf_openxml"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFormatMappings(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchFormatMapping(t *testing.T) {
	mappings := []FormatMapping{
		{Pattern: "*.docx", Format: "okf_openxml:test"},
		{Pattern: "*.md", Format: "okf_markdown@0.38"},
		{Pattern: "report-*.txt", Format: "plaintext"},
	}

	tests := []struct {
		filePath string
		want     string
	}{
		{"/home/user/doc.docx", "okf_openxml:test"},
		{"/home/user/README.md", "okf_markdown@0.38"},
		{"/tmp/report-2024.txt", "plaintext"},
		{"/tmp/notes.txt", ""}, // no match
		{"/tmp/data.json", ""}, // no match
		{"relative/path/file.docx", "okf_openxml:test"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := matchFormatMapping(tt.filePath, mappings)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchFormatMappingEmpty(t *testing.T) {
	assert.Equal(t, "", matchFormatMapping("/some/file.docx", nil))
	assert.Equal(t, "", matchFormatMapping("/some/file.docx", []FormatMapping{}))
}
