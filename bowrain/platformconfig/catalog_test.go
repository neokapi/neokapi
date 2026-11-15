package platformconfig

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// bareModelID strips the inference-profile scope ("eu.", "global.") and the
// vendor segment ("anthropic.", "openai.") from a Bedrock profile id, leaving
// the model id the framework catalog matches against.
func bareModelID(profile string) string {
	_, rest, _ := strings.Cut(profile, ".")
	_, model, _ := strings.Cut(rest, ".")
	return model
}

// Every model the platform offers must be one the framework catalog knows, so
// the platform list and providers/ai/models.json are refreshed together: a
// retired model dropped from the catalog fails here until its row goes too.
func TestBedrockCatalogResolvesInTheModelCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range BedrockCatalog {
		assert.Falsef(t, seen[m.ID], "duplicate platform model %q", m.ID)
		seen[m.ID] = true

		entry, ok := aiprovider.ModelForID(bareModelID(m.ID))
		if assert.Truef(t, ok, "platform model %q has no entry in providers/ai/models.json", m.ID) {
			assert.NotZerof(t, entry.MaxOutputTokens, "platform model %q resolves to %q, which has no output ceiling", m.ID, entry.ID)
		}
	}
}

func TestBedrockCatalogOffersTheOpenAIModels(t *testing.T) {
	for _, id := range []string{"global.openai.gpt-6.1-terra", "global.openai.gpt-5.6-terra"} {
		require.Truef(t, InCatalog("bedrock", id), "%q missing from the platform catalog", id)
		entry, ok := aiprovider.ModelForID(bareModelID(id))
		require.True(t, ok)
		assert.Equal(t, aiprovider.OpenAI, entry.Provider)
	}
}
