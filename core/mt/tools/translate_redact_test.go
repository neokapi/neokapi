package tools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/redaction"
	coretools "github.com/neokapi/neokapi/core/tools"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
)

// A machine-translation engine reads a redacted block with each withheld
// value as a placeholder element, never the value and never a gap. The engine
// keeps the element, it maps back to the placeholder by position, and
// unredact restores the value.
func TestMTTranslate_ReadsAWithheldValueAsAPlaceholderElement(t *testing.T) {
	redact, err := coretools.NewRedactTool(&coretools.RedactConfig{
		Detectors: []string{coretools.DetectRules},
		Rules:     []redaction.Rule{{Term: "Ada Lovelace", Category: redaction.CategoryPerson}},
	})
	require.NoError(t, err)
	unredact, err := coretools.NewUnredactTool(&coretools.UnredactConfig{})
	require.NoError(t, err)

	mock := newMock("mock")
	mock.translateFn = func(_ context.Context, req mtprovider.TranslateRequest) (*mtprovider.TranslateResponse, error) {
		// An engine that keeps the markup and translates the words around it.
		return &mtprovider.TranslateResponse{Translation: "FR " + req.Source}, nil
	}
	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench})

	b := model.NewBlock("b1", "Write to Ada Lovelace today.")
	b.SourceLocale = model.LocaleEnglish
	b.Translatable = true
	part := processPart(t, redact, &model.Part{Type: model.PartBlock, Resource: b})
	part = processPart(t, tl, part)
	assert.Equal(t, `Write to <span data-type="redaction:person"/> today.`, mock.lastRequest.Source,
		"the engine reads a placeholder element where the value was")
	part = processPart(t, unredact, part)
	assert.Equal(t, "FR Write to Ada Lovelace today.", part.Resource.(*model.Block).TargetText(model.LocaleFrench))
}
