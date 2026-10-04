package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// -o names the .kpz an ad-hoc extract writes. Inside a project the bilingual
// files go to --out-dir, so an -o that names anything else is refused rather
// than ignored, and nothing is written.
func TestExtract_RefusesAnOutputThatIsNotAWorkspace(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"fr-FR"})
	writeJSONSource(t, real, "src/locales/en/messages.json", `{"greeting": "Hello, world."}`)

	_, err = runExtractCmd(t, recipe, "-o", "fresh.xliff")
	require.Error(t, err)
	assert.Equal(t, ExitUsage, ExitCode(nil, err))
	assert.Contains(t, err.Error(), "--out-dir")
	_, statErr := os.Stat(filepath.Join(real, "out"))
	assert.True(t, os.IsNotExist(statErr), "nothing is written")
}
