//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi up --provider demo` drafts with the demo provider in a project whose
// recipe names no flow, with no provider set anywhere else: no environment
// variable, no saved credential and no app config.
func TestUpProviderFlagReachesTheBuiltInDefault(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	source, err := os.ReadFile(filepath.Join(testdata, "messages_en.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "messages_en.json"), source, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(`version: v1
name: up-provider-flag-e2e
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - name: app
    path: src/messages_en.json
    target: src/messages_{lang}.json
`), 0o644))

	cmd := exec.Command(kapiBin, "up", "-p", "kapi.yaml", "--provider", "demo")
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), isoEnv...),
		"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kapi up --provider demo failed:\n%s", out)

	got, err := os.ReadFile(filepath.Join(dir, "src", "messages_fr.json"))
	require.NoError(t, err, "kapi up wrote the fr target:\n%s", out)
	assert.Contains(t, string(got), "⟦fr⟧", "the demo provider drafted the fr target")
}
