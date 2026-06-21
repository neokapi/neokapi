package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultModelName(t *testing.T) {
	assert.Equal(t, "gemma-4-e2b", DefaultModelName())
}

func TestLookup(t *testing.T) {
	s, ok := Lookup("")
	require.True(t, ok)
	assert.Equal(t, "gemma-4-e2b", s.Name, "empty name resolves to default")

	s, ok = Lookup("gemma-4-e2b")
	require.True(t, ok)
	assert.True(t, s.Default)
	assert.NotEmpty(t, s.Embed.RepoPath)
	assert.NotEmpty(t, s.Decoder.RepoPath)
	// v0.1.x is text-only: vision/audio encoders are not used yet.
	assert.Empty(t, s.Vision.RepoPath)
	assert.Empty(t, s.Audio.RepoPath)

	_, ok = Lookup("nope")
	assert.False(t, ok)
}

func TestFileBase(t *testing.T) {
	assert.Equal(t, "decoder_model_merged_q4.onnx", File{RepoPath: "onnx/decoder_model_merged_q4.onnx"}.Base())
	assert.Equal(t, "tokenizer.json", File{RepoPath: "tokenizer.json"}.Base())
}

func TestAllFilesCoversComponentsDataAndConfigs(t *testing.T) {
	s, _ := Lookup("gemma-4-e2b")
	files := s.allFiles()
	var bases []string
	for _, f := range files {
		bases = append(bases, f.Base())
	}
	// Text-only: embed + decoder (2 graphs) + 2 data siblings + tokenizer
	// + config + generation_config = 7.
	assert.Len(t, files, 7)
	assert.Contains(t, bases, "embed_tokens_q4.onnx")
	assert.Contains(t, bases, "decoder_model_merged_q4.onnx_data")
	assert.Contains(t, bases, "tokenizer.json")
	assert.Contains(t, bases, "generation_config.json")
	assert.NotContains(t, bases, "vision_encoder_q4.onnx")
}

// stageModel writes a non-empty placeholder for every file the model needs.
func stageModel(t *testing.T, dir string) {
	t.Helper()
	s, _ := Lookup("gemma-4-e2b")
	for _, f := range s.allFiles() {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f.Base()), []byte("x"), 0o644))
	}
}

func TestResolveInDirMapsStagedFiles(t *testing.T) {
	dir := t.TempDir()
	stageModel(t, dir)

	paths, err := ResolveInDir("gemma-4-e2b", dir)
	require.NoError(t, err)
	assert.Equal(t, dir, paths.Dir)
	assert.Equal(t, filepath.Join(dir, "embed_tokens_q4.onnx"), paths.Embed)
	assert.Equal(t, filepath.Join(dir, "decoder_model_merged_q4.onnx"), paths.Decoder)
	assert.Equal(t, filepath.Join(dir, "tokenizer.json"), paths.Tokenizer)
	assert.Equal(t, filepath.Join(dir, "config.json"), paths.Config)
	assert.Equal(t, filepath.Join(dir, "generation_config.json"), paths.GenerationConfig)
	// Text-only model: no vision/audio graphs.
	assert.Empty(t, paths.Vision)
	assert.Empty(t, paths.Audio)
}

func TestResolveInDirEmptyDir(t *testing.T) {
	_, err := ResolveInDir("gemma-4-e2b", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no model directory")
}

func TestResolveInDirUnknownModel(t *testing.T) {
	_, err := ResolveInDir("nope", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown model")
}

func TestResolveInDirMissingFile(t *testing.T) {
	dir := t.TempDir()
	stageModel(t, dir)
	// Remove one required file: resolution must fail loudly, not silently.
	require.NoError(t, os.Remove(filepath.Join(dir, "decoder_model_merged_q4.onnx_data")))

	_, err := ResolveInDir("gemma-4-e2b", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoder_model_merged_q4.onnx_data")
	assert.Contains(t, err.Error(), "kapi models pull")
}

func TestResolveInDirEmptyFileRejected(t *testing.T) {
	dir := t.TempDir()
	stageModel(t, dir)
	// A zero-byte file is treated as absent (a truncated/partial stage).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tokenizer.json"), nil, 0o644))

	_, err := ResolveInDir("gemma-4-e2b", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tokenizer.json")
}
