package prompt

import (
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/model"
)

// QAPrompt builds prompts for quality assurance checks.
type QAPrompt struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	Checks       []string // e.g., "terminology", "fluency", "accuracy", "consistency"
	Glossary     map[string]string
}

// Build creates a prompt for QA checking a translation.
func (p *QAPrompt) Build(sourceText, targetText string) (system string, user string) {
	var sysBuilder strings.Builder
	sysBuilder.WriteString("You are a translation quality assurance specialist. ")
	sysBuilder.WriteString("Analyze translations and report issues in a structured format. ")
	sysBuilder.WriteString("Be precise and constructive in your feedback. ")

	system = sysBuilder.String()

	var userBuilder strings.Builder
	userBuilder.WriteString(fmt.Sprintf("Check the following %s → %s translation for issues.\n\n",
		p.SourceLocale, p.TargetLocale))

	if len(p.Checks) > 0 {
		userBuilder.WriteString(fmt.Sprintf("Check types: %s\n\n", strings.Join(p.Checks, ", ")))
	}

	if len(p.Glossary) > 0 {
		userBuilder.WriteString("Expected terminology:\n")
		for term, translation := range p.Glossary {
			userBuilder.WriteString(fmt.Sprintf("- %s → %s\n", term, translation))
		}
		userBuilder.WriteString("\n")
	}

	userBuilder.WriteString(fmt.Sprintf("Source: %s\n", sourceText))
	userBuilder.WriteString(fmt.Sprintf("Translation: %s\n\n", targetText))

	userBuilder.WriteString(`Respond in JSON format:
[{"type": "<check-type>", "severity": "<error|warning|info>", "description": "<issue>", "suggestion": "<fix>"}]
If no issues, return: []`)

	user = userBuilder.String()
	return
}
