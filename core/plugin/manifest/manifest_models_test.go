package manifest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// base returns a minimal valid manifest the model-section tests extend.
func base() *Manifest {
	return &Manifest{ManifestVersion: "1", Plugin: "llm", Version: "0.1.1", Binary: "kapi-llm"}
}

const goodSHA = "2d8c8a2bcc30e8ded7f636967c2a58a346116583356dd933720b005fc88079c4"

func validModel() ModelAsset {
	return ModelAsset{
		ID:      "gemma-4-e2b",
		Version: "1",
		Default: true,
		License: "Gemma",
		Files: []ModelFile{
			{Path: "decoder_model_merged_q4.onnx", URL: "https://h/resolve/abc123/onnx/decoder_model_merged_q4.onnx", SHA256: goodSHA, Size: 647599},
			{Path: "tokenizer.json", URL: "https://h/resolve/abc123/tokenizer.json", SHA256: goodSHA},
		},
	}
}

func TestModelsValidate_OK(t *testing.T) {
	m := base()
	m.Models = []ModelAsset{validModel()}
	require.NoError(t, m.Validate())
}

func TestModelsValidate_Errors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(a *ModelAsset)
		substr string
	}{
		{"missing id", func(a *ModelAsset) { a.ID = "" }, "id is required"},
		{"bad id", func(a *ModelAsset) { a.ID = "Gemma_BAD" }, "invalid id"},
		{"missing version", func(a *ModelAsset) { a.Version = "" }, "version is required"},
		{"no files", func(a *ModelAsset) { a.Files = nil }, "at least one file"},
		{"file no path", func(a *ModelAsset) { a.Files[0].Path = "" }, "path is required"},
		{"file dir path", func(a *ModelAsset) { a.Files[0].Path = "onnx/x.onnx" }, "bare basename"},
		{"file backslash path", func(a *ModelAsset) { a.Files[0].Path = "onnx\\x.onnx" }, "bare basename"},
		{"file no url", func(a *ModelAsset) { a.Files[0].URL = "" }, "url is required"},
		{"file no sha", func(a *ModelAsset) { a.Files[0].SHA256 = "" }, "sha256 is required"},
		{"file short sha", func(a *ModelAsset) { a.Files[0].SHA256 = "abc" }, "sha256 is required"},
		{"dup file path", func(a *ModelAsset) { a.Files[1].Path = a.Files[0].Path }, "duplicate file path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			a := validModel()
			tc.mutate(&a)
			m.Models = []ModelAsset{a}
			err := m.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.substr)
		})
	}
}

func TestModelsValidate_DuplicateIDAndMultiDefault(t *testing.T) {
	m := base()
	a, b := validModel(), validModel()
	m.Models = []ModelAsset{a, b}
	require.ErrorContains(t, m.Validate(), "duplicate model id")

	b.ID = "gemma-4-e4b" // distinct id, but both default
	m.Models = []ModelAsset{a, b}
	require.ErrorContains(t, m.Validate(), "at most one model may be marked default")
}

func TestDefaultModelAndLookup(t *testing.T) {
	m := base()
	// No models → no default.
	_, ok := m.DefaultModel()
	assert.False(t, ok)

	// Single undeclared-default model is unambiguously the default.
	single := validModel()
	single.Default = false
	m.Models = []ModelAsset{single}
	got, ok := m.DefaultModel()
	require.True(t, ok)
	assert.Equal(t, "gemma-4-e2b", got.ID)

	// Explicit default among several wins.
	a := validModel()
	a.Default = false
	a.ID = "small"
	b := validModel()
	b.Default = true
	b.ID = "big"
	m.Models = []ModelAsset{a, b}
	got, ok = m.DefaultModel()
	require.True(t, ok)
	assert.Equal(t, "big", got.ID)

	byID, ok := m.Model("small")
	require.True(t, ok)
	assert.Equal(t, "small", byID.ID)
	_, ok = m.Model("nope")
	assert.False(t, ok)
}

// A models section parses end-to-end through Parse (JSON → validated Manifest).
func TestParseWithModels(t *testing.T) {
	raw := `{
      "manifest_version": "1",
      "plugin": "llm",
      "version": "0.1.1",
      "binary": "kapi-llm",
      "models": [{
        "id": "gemma-4-e2b",
        "version": "1",
        "default": true,
        "license": "Gemma",
        "files": [
          {"path": "decoder.onnx", "url": "https://h/resolve/abc/decoder.onnx", "sha256": "` + goodSHA + `", "size": 10}
        ]
      }]
    }`
	m, err := Parse([]byte(raw))
	require.NoError(t, err)
	require.Len(t, m.Models, 1)
	assert.True(t, strings.HasPrefix(m.Models[0].Files[0].URL, "https://"))
	d, ok := m.DefaultModel()
	require.True(t, ok)
	assert.Equal(t, "gemma-4-e2b", d.ID)
}
