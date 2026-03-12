package prompt

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// TranslatePrompt builds prompts for translation tasks.
type TranslatePrompt struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	Format       string // e.g., "html", "plain", "markdown"
	Glossary     map[string]string
	Context      string // Additional context for the translation
}

// Build creates a system message and user message for translation.
func (p *TranslatePrompt) Build(sourceText string) (system string, user string) {
	var sysBuilder strings.Builder
	sysBuilder.WriteString("You are a software localization specialist performing UI string translation. ")
	sysBuilder.WriteString(fmt.Sprintf("Translate the following user interface text from %s to %s. ", p.SourceLocale, p.TargetLocale))
	sysBuilder.WriteString("Return ONLY the translated text without any explanation, notes, or formatting. ")
	sysBuilder.WriteString("Preserve any markup tags, placeholders ({0}, %s, {{name}}), or special formatting in the text. ")

	if p.Format != "" && p.Format != "plain" {
		sysBuilder.WriteString(fmt.Sprintf("The text is in %s format - preserve all formatting markers. ", p.Format))
	}

	system = sysBuilder.String()

	var userBuilder strings.Builder
	if p.Context != "" {
		userBuilder.WriteString(fmt.Sprintf("Context: %s\n\n", p.Context))
	}

	if len(p.Glossary) > 0 {
		userBuilder.WriteString("Glossary (use these translations for the given terms):\n")
		for term, translation := range p.Glossary {
			userBuilder.WriteString(fmt.Sprintf("- %s → %s\n", term, translation))
		}
		userBuilder.WriteString("\n")
	}

	userBuilder.WriteString(fmt.Sprintf("Translate:\n%s", sourceText))

	user = userBuilder.String()
	return
}

// BuildBatch creates a prompt for batch translation of multiple segments.
func (p *TranslatePrompt) BuildBatch(texts []string) (system string, user string) {
	var sysBuilder strings.Builder
	sysBuilder.WriteString("You are a software localization specialist performing UI string translation. ")
	sysBuilder.WriteString(fmt.Sprintf("Your task is to translate user interface strings from %s to %s. ", p.SourceLocale, p.TargetLocale))
	sysBuilder.WriteString("These are UI labels, error messages, and status texts from a software application. ")
	sysBuilder.WriteString("Return ONLY the translations, one per line, in the same order as the input. ")
	sysBuilder.WriteString("Do not add numbering, bullets, or any other formatting. ")
	sysBuilder.WriteString("Preserve any placeholders like {0}, %s, {{name}}, etc. ")

	system = sysBuilder.String()

	var userBuilder strings.Builder
	if len(p.Glossary) > 0 {
		userBuilder.WriteString("Glossary:\n")
		for term, translation := range p.Glossary {
			userBuilder.WriteString(fmt.Sprintf("- %s → %s\n", term, translation))
		}
		userBuilder.WriteString("\n")
	}

	userBuilder.WriteString("Translate each UI string below:\n")
	for i, text := range texts {
		userBuilder.WriteString(fmt.Sprintf("[%d] %s\n", i+1, text))
	}
	userBuilder.WriteString("\nReturn one translation per line, without the [N] prefix.")

	user = userBuilder.String()
	return
}
