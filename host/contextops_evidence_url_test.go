package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// Evidence URLs are offered as links on every surface that shows them, and an
// agent records them, so only a web page reaches the log.
func TestEvidenceURLMustBeAWebPage(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-evidence-url")

	for _, bad := range []string{"javascript:alert(1)", "data:text/html,hi", "file:///etc/passwd", "https://"} {
		_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
			Actor: agentIn("s1"), Project: recipeOf(root),
			Text:     "The product page says so.",
			Evidence: []contextop.Evidence{{URL: bad}},
		})
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "not a web page")

		_, err = app.RecordContextCorrection(t.Context(), ContextCorrectRequest{
			Actor: agentIn("s1"), Project: recipeOf(root),
			From: "Quick cast", To: "Quickcast",
			Evidence: []contextop.Evidence{{URL: bad}},
		})
		require.Error(t, err, bad)
	}

	noted, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: agentIn("s1"), Project: recipeOf(root),
		Text:     "The product page says so.",
		Evidence: []contextop.Evidence{{URL: "https://example.com/product"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/product", noted.Evidence[0].URL)
}
