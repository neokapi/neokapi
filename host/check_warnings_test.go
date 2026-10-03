package host

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The voice profile a check reports warnings about comes from the project's
// voice store, so a fixture states its warning in the profile and reads it in.
// The warning the store can carry is one about a VALUE, since a store holds a
// decoded profile: an unfamiliar tone is kept, rendered into the guide as
// written, and reported.

// unfamiliarProfileTone gives the scoped fixture's voice profile a formality
// outside the usual set and reads the profile into the store, which is where
// every project-resolved face loads it from.
func unfamiliarProfileTone(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".kapi", "voice.yaml")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	unfamiliar := strings.Replace(string(body), "name: Service\n",
		"name: Service\ntone:\n  formality: brisk\n", 1)
	require.NotEqual(t, string(body), unfamiliar, "the fixture's profile is where the tone goes")
	require.NoError(t, os.WriteFile(path, []byte(unfamiliar), 0o600))
	readProjectContext(t, root)
}

// writeReadyPages writes one clean page per channel directory named.
func writeReadyPages(t *testing.T, root string, channels ...string) []string {
	t.Helper()
	files := make([]string, 0, len(channels))
	for _, channel := range channels {
		file := filepath.Join(root, channel, "page"+strconv.Itoa(len(files))+".json")
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
		require.NoError(t, os.WriteFile(file, []byte(`{"body":"Ready to go."}`), 0o600))
		files = append(files, file)
	}
	return files
}

// assertOneUnfamiliarTone: the profile governing every file in the fixture is
// one profile, so its warning is reported once, and it names the store the
// profile was loaded from.
func assertOneUnfamiliarTone(t *testing.T, warnings []check.Warning) {
	t.Helper()
	assertOneUnfamiliarToneFrom(t, warnings, "store:service")
}

// assertOneUnfamiliarToneFrom is assertOneUnfamiliarTone for a face that names
// the profile itself, where the warning is about the file the caller pointed at.
func assertOneUnfamiliarToneFrom(t *testing.T, warnings []check.Warning, source string) {
	t.Helper()
	require.Len(t, warnings, 1, "one profile governs every file, and its warning is reported once")
	w := warnings[0]
	assert.Equal(t, coreprofile.CodeUnfamiliarValue, w.Code)
	assert.Equal(t, "tone.formality", w.Key)
	assert.Equal(t, source, w.Source)
	assert.Contains(t, w.Message, `"brisk"`)
}
