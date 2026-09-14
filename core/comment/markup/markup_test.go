package markup

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunsTrimLinesAndProtectMarkup(t *testing.T) {
	runs := Runs("\n    Spans two lines,\n    <string name=\"old\">Retired</string>\n\n    and ends here.\n  ")
	assert.Equal(t, "Spans two lines,\n\n\nand ends here.", model.RunsText(runs))
	var placeholders []string
	for _, r := range runs {
		require.True(t, r.Valid())
		if r.Ph != nil {
			placeholders = append(placeholders, r.Ph.SubType+" "+r.Ph.Data)
		}
	}
	assert.Equal(t, []string{`markup <string name="old">Retired</string>`}, placeholders)
}

func TestRunsKeepInlineTagsAsProse(t *testing.T) {
	assert.Equal(t, "Use <b> for bold.", model.RunsText(Runs(" Use <b> for bold. ")))
	assert.Equal(t, "<3 this", model.RunsText(Runs("<3 this")))
	assert.Empty(t, Runs("   "))
}

func TestDirectiveForms(t *testing.T) {
	forms := []DirectiveForm{PrettierIgnore, FormatterToggle, Suppress, ReSharper}
	for body, want := range map[string]string{
		" prettier-ignore ":                             "prettier-ignore",
		"prettier-ignore-start":                         "prettier-ignore",
		"prettier-ignore-end":                           "prettier-ignore",
		"prettier-ignore-attribute (click)":             "prettier-ignore",
		"@formatter:off":                                "formatter",
		"@formatter:on":                                 "formatter",
		"suppress AndroidLintUnusedResources":           "suppress",
		"suppress HtmlUnknownTag, HtmlUnknownAttribute": "suppress",
		"noinspection XmlUnusedNamespaceDeclaration":    "suppress",
		"ReSharper disable once MarkupAttributeTypo":    "ReSharper",
		"ReSharper restore MarkupTextTypo":              "ReSharper",
	} {
		t.Run(body, func(t *testing.T) {
			form, ok := Classify(body, forms)
			require.True(t, ok)
			assert.Equal(t, want, form)
		})
	}
	for _, prose := range []string{
		"prettier-ignored text reads as prose",
		"suppress the banner on small screens",
		"suppress warnings",
		"noinspection needed",
		"ReSharperish",
		"@formatter is a name",
	} {
		t.Run("prose "+prose, func(t *testing.T) {
			_, ok := Classify(prose, forms)
			assert.False(t, ok)
		})
	}
}
