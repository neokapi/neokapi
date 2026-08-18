package main

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
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

// The source-side checkers are the sidecars with nothing to overlay onto: a
// checker off the registry has no entry, so the file-name binding has no entry
// to bind to. Naming the set in code makes the binding hold in both directions
// rather than exempting the files from it, so neither a retired checker's
// dossier nor an undocumented checker survives a regeneration.
func TestVerifyCheckDocs(t *testing.T) {
	ids := []string{"content-lint", "length-check"}

	tests := []struct {
		name    string
		files   []string
		wantErr []string
	}{
		{
			name:  "a dossier for every check",
			files: []string{"checks/content-lint.yaml", "checks/length-check.yaml"},
		},
		{
			name:    "a dossier for a check that no longer exists",
			files:   []string{"checks/content-lint.yaml", "checks/length-check.yaml", "checks/repetition-analysis.yaml"},
			wantErr: []string{"repetition-analysis.yaml", "documents nothing", "source-side check", `"repetition-analysis"`},
		},
		{
			name:    "a check nobody has documented",
			files:   []string{"checks/content-lint.yaml"},
			wantErr: []string{"length-check", "no dossier", filepath.Join("checks", "length-check.yaml")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				writeFile(t, dir, f, "description: A check that reports something.\n")
			}

			err := verifyCheckDocs(dir, ids)
			if len(tt.wantErr) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range tt.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

// The repository's own tree satisfies the binding, so the guard is a live
// statement about it rather than a rule only its unit tests obey.
func TestVerifyCheckDocs_TheRepositoryIsDocumented(t *testing.T) {
	require.NoError(t, verifyCheckDocs("nativedocs", check.SourceCheckIDs()))
}
