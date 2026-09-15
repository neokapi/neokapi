package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/registry"
)

// Opening or editing one review unit names its file. When no reader for the
// file's format is installed the request fails and names the plugin to install;
// the readable unit beside it opens as usual.
func TestReviewUnitRequestsWithNoReaderNameThePlugin(t *testing.T) {
	isolateCheckPlugins(t)
	app := NewApp()
	tabID := unreadCheckProject(t, app, `{"greeting":"Hello world"}`, true)

	_, err := app.GetReviewUnit(tabID, "fr", "pkg/doc.fr.idml", "hello")
	require.ErrorIs(t, err, registry.ErrUnknownFormat)
	assert.Contains(t, err.Error(), "kapi plugins install okapi-bridge")

	err = app.UpdateReviewTarget(tabID, "fr", "pkg/doc.fr.idml", "hello", "Bonjour")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kapi plugins install okapi-bridge")

	unit, err := app.GetReviewUnit(tabID, "fr", "locales/fr.json", "greeting")
	require.NoError(t, err)
	assert.NotEmpty(t, unit.Target, "the readable unit is read")
}
