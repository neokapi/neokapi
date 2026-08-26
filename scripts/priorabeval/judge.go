package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The blind pairwise judge.
//
// It sees the English, the guidance the translator was given, and two Norwegian
// candidates labelled 1 and 2. It does not see the previously approved
// translation, and it is not told that one candidate had a reference and the
// other did not. Without both of those it would be scoring which candidate
// matches a text it was handed, which answers nothing.
//
// Which candidate is shown first alternates per sample, and the verdict is
// resolved back to the arm afterwards, so a judge with a position preference
// cannot decide the outcome.

// judgePair asks the judge which candidate is better. withFirst puts the
// reference arm in position 1.
func judgePair(ctx context.Context, judge aiprovider.LLMProvider, c abCase, without, with string, withFirst bool) (*Verdict, error) {
	first, second := without, with
	shownFirst := "without"
	if withFirst {
		first, second = with, without
		shownFirst = "with"
	}

	var sb strings.Builder
	sb.WriteString("English source:\n")
	sb.WriteString(c.source)
	sb.WriteString("\n\nGuidance the translator was given: a precise, calm, neutral register.\n\n")
	sb.WriteString("Candidate 1:\n")
	sb.WriteString(first)
	sb.WriteString("\n\nCandidate 2:\n")
	sb.WriteString(second)
	sb.WriteString("\n\nWhich candidate is the better Norwegian Bokmål translation of the English, " +
		"given the guidance? If they are equally good, answer tie.")

	msgs := []aiprovider.Message{
		aiprovider.TextMessage(aiprovider.RoleSystem,
			"You are grading Norwegian Bokmål translations. Answer only with the JSON object the "+
				"schema describes. Judge accuracy to the English first, then naturalness, then "+
				"register. Answer tie when neither is better; do not invent a preference."),
		aiprovider.TextMessage(aiprovider.RoleUser, sb.String()),
	}

	raw, err := chatJSON(ctx, judge, msgs)
	if err != nil {
		return nil, err
	}

	var answer struct {
		Winner string `json:"winner"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return nil, fmt.Errorf("judge returned %q, which is not the JSON the schema asked for: %w", raw, err)
	}

	v := &Verdict{Reason: strings.TrimSpace(answer.Reason), ShownFirst: shownFirst}
	switch strings.ToLower(strings.TrimSpace(answer.Winner)) {
	case "1", "candidate 1", "one":
		v.Winner = shownFirst
	case "2", "candidate 2", "two":
		v.Winner = otherArm(shownFirst)
	case "tie", "neither", "equal":
		v.Winner = "tie"
	default:
		// An unparseable preference is a tie rather than an error: a judge that
		// cannot answer has not expressed one, and counting it either way would
		// invent a result.
		v.Winner = "tie"
		v.Reason = strings.TrimSpace("unparseable verdict " + answer.Winner + ". " + v.Reason)
	}
	return v, nil
}

func otherArm(a string) string {
	if a == "with" {
		return "without"
	}
	return "with"
}

// judgeSchema pins the answer shape. A judge free to write prose writes prose,
// and a parser for prose is a second judge nobody validated.
func judgeSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name: "verdict",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"winner": map[string]any{
					"type":        "string",
					"enum":        []string{"1", "2", "tie"},
					"description": "which candidate is better, or tie",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "one short sentence",
				},
			},
			"required":             []string{"winner", "reason"},
			"additionalProperties": false,
		},
	}
}

// chatJSON asks for a structured answer. Every provider implements it, but a
// small local model still wraps the object in a fence or a sentence often
// enough to be worth unwrapping.
func chatJSON(ctx context.Context, p aiprovider.LLMProvider, msgs []aiprovider.Message) (string, error) {
	resp, err := p.ChatStructured(ctx, msgs, judgeSchema())
	if err != nil {
		return "", err
	}
	return extractJSON(resp.Content), nil
}

// extractJSON pulls the object out of a reply that wrapped it in a fence or a
// sentence, which a smaller model does even when asked not to.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return s
	}
	return s[start : end+1]
}
