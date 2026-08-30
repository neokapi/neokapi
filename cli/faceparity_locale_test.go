package cli

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CLI leg of the locale parity contract: `kapi status` on a project that
// declared its locales in POSIX style prints the canonical tags.
func TestFaceParity_CLIStatusReportsCanonicalLocales(t *testing.T) {
	facetest.WritePosix(t)

	out := runVerb(t, &App{}, NewStatusCmd, "status", "--json")
	var status host.StatusOutput
	require.NoError(t, json.Unmarshal([]byte(out), &status))
	require.NotEmpty(t, status.Locales, "the fixture declares a target")

	for _, lc := range status.Locales {
		assert.Equal(t, string(facetest.PosixTargetLocale), lc.Locale,
			"the verb reports the canonical tag for a POSIX-declared target")
	}
}
