package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsContextFileNamesEveryKindOfContextFile(t *testing.T) {
	for _, rel := range []string{
		"context/voice.yaml",
		"context/terms.json",
		"seeds/web.terms.json",
		"context/memory/memory.json",
		"seeds/docs.memory.json",
		"context/state/d-0d589ceffe194564.jsonl",
		".kapi/state/d-0d589ceffe194564-2.jsonl",
		"context/context.kpz",
	} {
		assert.True(t, IsContextFile(rel), rel)
	}
	for _, rel := range []string{
		"web/en/home.json",
		"src/en/strings.yaml",
		"logs/state/events.jsonl",
		"state/d-notes.jsonl",
	} {
		assert.False(t, IsContextFile(rel), rel)
	}
}

func TestContextFileFilterLeavesOutAFileBackendInsideTheProject(t *testing.T) {
	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\ncontext:\n  backend: file\n  path: shared\n"), 0o644))

	isContext := ContextFileFilter(recipe)
	assert.True(t, isContext("shared/anything.bin"))
	assert.True(t, isContext("context/voice.yaml"))
	assert.False(t, isContext("sharedness/home.json"))
	assert.False(t, isContext("web/en/home.json"))
}
