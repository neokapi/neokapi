package cli

import (
	"context"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processOne sends a single block part through a tool's Process.
func processOne(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, block *model.Block) {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)
	require.NoError(t, tl.Process(context.Background(), in, out))
	close(out)
	<-out
}

// TestSecureTranslate_InProcessNoLeak proves the in-process workflow's core
// guarantee: redact → ai-translate → unredact restores the originals, and the
// LLM provider never receives the sensitive text.
func TestSecureTranslate_InProcessNoLeak(t *testing.T) {
	const secretA, secretB = "Mr Bean", "King of England"

	redactTool, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules: []redaction.Rule{
			{Term: secretA, Category: "person"},
			{Term: secretB, Category: "role"},
		},
	})
	require.NoError(t, err)

	mock := aiprovider.NewMockProvider()
	translateTool := aitools.NewAITranslateTool(mock, aitools.AITranslateConfig{TargetLocale: "fr"})

	unredactTool, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)

	block := model.NewBlock("b1", secretA+" is the new "+secretB)
	block.SourceLocale = "en"

	processOne(t, redactTool, block)
	// After redaction the source no longer carries the secrets.
	assert.NotContains(t, block.SourceText(), secretA)
	assert.NotContains(t, block.SourceText(), secretB)

	processOne(t, translateTool, block)
	// The provider must never have seen either secret.
	require.NotEmpty(t, mock.TranslateCalls, "translate provider was not called")
	for _, call := range mock.TranslateCalls {
		assert.NotContains(t, call.Source, secretA, "secret leaked to the LLM prompt")
		assert.NotContains(t, call.Source, secretB, "secret leaked to the LLM prompt")
	}

	processOne(t, unredactTool, block)
	// Originals are restored into the translated target.
	target := block.TargetText("fr")
	assert.Contains(t, target, secretA, "person not restored")
	assert.Contains(t, target, secretB, "role not restored")
	// And the in-process secret annotation is gone.
	_, hasAnn := block.Annotations[redaction.SecretAnnotationKey]
	assert.False(t, hasAnn)
}
