package backend

import (
	"testing"

	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The desktop leg of the locale parity contract: a project that declared its
// locales in POSIX style reports the canonical tags in the app, as it does at
// the verb and through the host layer.
func TestFaceParity_DesktopReportsCanonicalLocales(t *testing.T) {
	p := facetest.WritePosix(t)
	app := NewApp()
	tab, err := app.OpenProject(p.Recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	_, err = app.RunExtract(tab.ID)
	require.NoError(t, err)

	status, err := app.GetProjectStatus(tab.ID)
	require.NoError(t, err)
	require.NotEmpty(t, status.Collections, "the fixture declares a collection")

	var seen []string
	for _, c := range status.Collections {
		seen = append(seen, c.TargetLanguages...)
	}
	require.NotEmpty(t, seen, "the fixture declares a target")
	for _, l := range seen {
		assert.Equal(t, string(facetest.PosixTargetLocale), l,
			"the app reports the canonical tag for a POSIX-declared target")
	}
}
