package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file the project's ignore rules match takes no collection's voice, so its
// guide gives the project's voice. A sibling the rules leave alone gives the
// voice of the collection that claims it.
func TestVoiceGuideGivesAnIgnoredFileTheProjectVoice(t *testing.T) {
	root := writeCommentVoiceProject(t, false)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapiignore"), []byte("config/app.yaml\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "other.yaml"), []byte("# The farewell.\nfarewell: Goodbye\n"), 0o644))
	t.Chdir(root)

	assert.Contains(t, voiceGuide(t, filepath.Join("config", "app.yaml")), "Voice Guide: project", "the ignored file")
	assert.Contains(t, voiceGuide(t, filepath.Join("config", "other.yaml")), "Voice Guide: site", "its sibling")
}
