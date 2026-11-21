package plaintext_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestWriteTargetWithPlaceholderRun(t *testing.T) {
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Bonjour "}},
		{Ph: &model.PlaceholderRun{ID: "1", Data: "{name}"}},
		{Text: &model.TextRun{Text: ", bienvenue"}},
	}
	out := skeletonRoundtripWithConfig(t, "Hello {name}, welcome\nSecond line\n", nil, "fr", func(parts []*model.Part) {
		for _, p := range parts {
			if b, ok := p.Resource.(*model.Block); ok && b.SourceText() == "Hello {name}, welcome" {
				b.SetTargetRuns("fr", runs)
			}
		}
	})
	assert.Equal(t, "Bonjour {name}, bienvenue\nSecond line\n", out)
}
