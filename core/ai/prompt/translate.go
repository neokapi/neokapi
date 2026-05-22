package prompt

import (
	"fmt"
	"sort"
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
	VoiceGuide   string // brand voice guidance to apply during translation
}

// sortedGlossaryLines renders glossary entries deterministically (sorted by term).
func sortedGlossaryLines(glossary map[string]string) string {
	if len(glossary) == 0 {
		return ""
	}
	keys := make([]string, 0, len(glossary))
	for k := range glossary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "- %s → %s\n", k, glossary[k])
	}
	return b.String()
}

// Build creates a system message and user message for translation.
func (p *TranslatePrompt) Build(sourceText string) (string, string) {
	var sysBuilder strings.Builder
	sysBuilder.WriteString("You are a software localization specialist performing UI string translation. ")
	sysBuilder.WriteString(fmt.Sprintf("Translate the following user interface text from %s to %s. ", p.SourceLocale, p.TargetLocale))
	sysBuilder.WriteString("Return ONLY the translated text without any explanation, notes, or formatting. ")
	sysBuilder.WriteString("Preserve any markup tags, placeholders ({0}, %s, {{name}}), or special formatting in the text. ")

	if p.Format != "" && p.Format != "plain" {
		sysBuilder.WriteString(fmt.Sprintf("The text is in %s format - preserve all formatting markers. ", p.Format))
	}

	system := sysBuilder.String()

	var userBuilder strings.Builder
	if p.Context != "" {
		userBuilder.WriteString(fmt.Sprintf("Context: %s\n\n", p.Context))
	}

	if g := strings.TrimSpace(p.VoiceGuide); g != "" {
		userBuilder.WriteString("Brand voice (apply when translating):\n")
		userBuilder.WriteString(g)
		userBuilder.WriteString("\n\n")
	}

	if lines := sortedGlossaryLines(p.Glossary); lines != "" {
		userBuilder.WriteString("Glossary (use these translations for the given terms):\n")
		userBuilder.WriteString(lines)
		userBuilder.WriteString("\n")
	}

	userBuilder.WriteString("Translate:\n" + sourceText)

	return system, userBuilder.String()
}

// BuildBatch creates a prompt for batch translation of multiple segments.
func (p *TranslatePrompt) BuildBatch(texts []string) (string, string) {
	var sysBuilder strings.Builder
	sysBuilder.WriteString("You are a software localization specialist performing UI string translation. ")
	sysBuilder.WriteString(fmt.Sprintf("Your task is to translate user interface strings from %s to %s. ", p.SourceLocale, p.TargetLocale))
	sysBuilder.WriteString("These are UI labels, error messages, and status texts from a software application. ")
	sysBuilder.WriteString("Return ONLY the translations, one per line, in the same order as the input. ")
	sysBuilder.WriteString("Do not add numbering, bullets, or any other formatting. ")
	sysBuilder.WriteString("Preserve any placeholders like {0}, %s, {{name}}, etc. ")

	system := sysBuilder.String()

	var userBuilder strings.Builder
	if g := strings.TrimSpace(p.VoiceGuide); g != "" {
		userBuilder.WriteString("Brand voice (apply when translating):\n")
		userBuilder.WriteString(g)
		userBuilder.WriteString("\n\n")
	}
	if lines := sortedGlossaryLines(p.Glossary); lines != "" {
		userBuilder.WriteString("Glossary:\n")
		userBuilder.WriteString(lines)
		userBuilder.WriteString("\n")
	}

	userBuilder.WriteString("Translate each UI string below:\n")
	for i, text := range texts {
		userBuilder.WriteString(fmt.Sprintf("[%d] %s\n", i+1, text))
	}
	userBuilder.WriteString("\nReturn one translation per line, without the [N] prefix.")

	return system, userBuilder.String()
}
