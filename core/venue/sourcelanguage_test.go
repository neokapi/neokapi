package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

func TestSourceLanguageMismatch(t *testing.T) {
	for _, tc := range []struct {
		pushed, project model.LocaleID
		refused         bool
	}{
		{"en-US", "en-US", false},
		{"en-US", "en_us", false},
		{"", "en-US", false},
		{"en-US", "", false},
		{"en-US", "en", true},
		{"en", "fr", true},
	} {
		why := SourceLanguageMismatch(tc.pushed, tc.project)
		if !tc.refused {
			assert.Empty(t, why, "%s on %s", tc.pushed, tc.project)
			continue
		}
		assert.Contains(t, why, string(tc.pushed), "names the recipe's language")
		assert.Contains(t, why, string(tc.project), "names the project's language")
		assert.Contains(t, why, "set defaults.source_language to "+string(tc.project))
	}
}
