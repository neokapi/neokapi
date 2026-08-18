package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A sidecar's file name is the binding: applyNativeDoc overlays it onto the
// entry carrying that id. A name the registry does not carry therefore documents
// nothing, and dropping it in silence is how a tool rename left two dossiers
// unread while the tools they were written for shipped the registry's bare
// metadata — and the gap report, measuring the entry rather than the file, said
// the entry had no overview without saying why.

func TestLoadNativeDocs_RefusesASidecarThatDocumentsNothing(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		file    string
		entries []Entry
		wantErr []string
	}{
		{
			name:    "a tool sidecar named for a retired id",
			kind:    KindTool,
			file:    "tools/brand-voice-check.yaml",
			entries: []Entry{{ID: "voice-check", Kind: KindTool}},
			wantErr: []string{"brand-voice-check.yaml", "documents nothing", "built-in tool", `"brand-voice-check"`},
		},
		{
			name:    "a format sidecar named for a removed format",
			kind:    KindFormat,
			file:    "formats/dtd.yaml",
			entries: []Entry{{ID: "xml", Kind: KindFormat}},
			wantErr: []string{"dtd.yaml", "built-in format", `"dtd"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, tt.file, "description: Documents a thing that is not there.\n")

			_, err := loadNativeDocs(dir, tt.kind, tt.entries)
			require.Error(t, err)
			for _, want := range tt.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestLoadNativeDocs_ReadsASidecarThatMatchesAnEntry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tools/voice-check.yaml", "description: Judge text against a voice profile.\n")

	docs, err := loadNativeDocs(dir, KindTool, []Entry{{ID: "voice-check", Kind: KindTool}})
	require.NoError(t, err)
	require.Contains(t, docs, "voice-check")
	assert.Equal(t, "Judge text against a voice profile.", docs["voice-check"].Description)
}

// A tree with no sidecars for a kind is not an error — it is a kind nobody has
// authored documentation for yet.
func TestLoadNativeDocs_MissingDirectoryIsEmpty(t *testing.T) {
	docs, err := loadNativeDocs(t.TempDir(), KindFormat, []Entry{{ID: "json", Kind: KindFormat}})
	require.NoError(t, err)
	assert.Empty(t, docs)
}
