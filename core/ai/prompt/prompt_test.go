package prompt_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/ai/prompt"
	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestTranslatePromptBuild(t *testing.T) {
	p := &prompt.TranslatePrompt{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}

	sys, user := p.Build("Hello World")
	assert.Contains(t, sys, "professional translator")
	assert.Contains(t, sys, "en")
	assert.Contains(t, sys, "fr")
	assert.Contains(t, user, "Hello World")
}

func TestTranslatePromptWithGlossary(t *testing.T) {
	p := &prompt.TranslatePrompt{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleGerman,
		Glossary:     map[string]string{"computer": "Rechner"},
	}

	_, user := p.Build("Use the computer")
	assert.Contains(t, user, "computer")
	assert.Contains(t, user, "Rechner")
	assert.Contains(t, user, "Glossary")
}

func TestTranslatePromptWithContext(t *testing.T) {
	p := &prompt.TranslatePrompt{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Context:      "Software UI button label",
	}

	_, user := p.Build("Submit")
	assert.Contains(t, user, "Context: Software UI button label")
}

func TestTranslatePromptWithFormat(t *testing.T) {
	p := &prompt.TranslatePrompt{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Format:       "html",
	}

	sys, _ := p.Build("<b>Hello</b>")
	assert.Contains(t, sys, "html")
}

func TestTranslatePromptBuildBatch(t *testing.T) {
	p := &prompt.TranslatePrompt{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}

	sys, user := p.BuildBatch([]string{"Hello", "World", "Test"})
	assert.Contains(t, sys, "one per line")
	assert.Contains(t, user, "Hello")
	assert.Contains(t, user, "World")
	assert.Contains(t, user, "Test")
}

func TestQAPromptBuild(t *testing.T) {
	p := &prompt.QAPrompt{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Checks:       []string{"fluency", "accuracy"},
	}

	sys, user := p.Build("Hello World", "Bonjour le monde")
	assert.Contains(t, sys, "quality assurance")
	assert.Contains(t, user, "Hello World")
	assert.Contains(t, user, "Bonjour le monde")
	assert.Contains(t, user, "fluency")
	assert.Contains(t, user, "accuracy")
}

func TestQAPromptWithGlossary(t *testing.T) {
	p := &prompt.QAPrompt{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Checks:       []string{"terminology"},
		Glossary:     map[string]string{"save": "sauvegarder"},
	}

	_, user := p.Build("Save the file", "Enregistrer le fichier")
	assert.Contains(t, user, "save")
	assert.Contains(t, user, "sauvegarder")
}
