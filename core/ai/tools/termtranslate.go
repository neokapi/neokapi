package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// MaxTermTranslateExamples caps the sentences one concept carries into the
// prompt. A few show which sense of the term is meant; more add cost and
// nothing a terminologist needs.
const MaxTermTranslateExamples = 3

// TermTranslateRequest is one concept to propose a target-language term for.
type TermTranslateRequest struct {
	// ID names the concept, so an answer maps back to it.
	ID string `json:"id"`
	// Term is the concept's source-language term.
	Term       string `json:"term"`
	Definition string `json:"definition,omitempty"`
	// Examples are source sentences that use the term.
	Examples []string `json:"examples,omitempty"`
	// TurnedDown are target terms a reviewer has rejected for this concept.
	TurnedDown []string `json:"turned_down"`
}

// TermTranslation is the term proposed for one concept.
type TermTranslation struct {
	ID   string `json:"id"`
	Term string `json:"term"`
	Note string `json:"note"`
}

// ProposeTermTranslations asks a provider for the term each concept takes in
// target, in one call, and returns the answers that name a concept asked
// about, carry a term, and repeat no term turned down for that concept. A
// concept the model left empty gets no answer.
func ProposeTermTranslations(ctx context.Context, p aiprovider.LLMProvider, source, target model.LocaleID, reqs []TermTranslateRequest) ([]TermTranslation, aiprovider.TokenUsage, error) {
	var usage aiprovider.TokenUsage
	p = aiprovider.Redacting(p)
	if p == nil {
		return nil, usage, errors.New("term-translate: nil provider")
	}
	if target == "" {
		return nil, usage, errors.New("term-translate: no target language given")
	}
	asked := map[string]TermTranslateRequest{}
	payload := make([]TermTranslateRequest, 0, len(reqs))
	for _, r := range reqs {
		r.Term = strings.TrimSpace(r.Term)
		if r.ID == "" || r.Term == "" {
			continue
		}
		if _, dup := asked[r.ID]; dup {
			continue
		}
		if len(r.Examples) > MaxTermTranslateExamples {
			r.Examples = r.Examples[:MaxTermTranslateExamples]
		}
		if r.TurnedDown == nil {
			r.TurnedDown = []string{}
		}
		asked[r.ID] = r
		payload = append(payload, r)
	}
	if len(payload) == 0 {
		return nil, usage, nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, usage, fmt.Errorf("term-translate: %w", err)
	}

	turns := prompt.TermTranslate{SourceLocale: source, TargetLocale: target}.Turns(string(body))
	ctx = prompt.WithMeta(ctx, prompt.For(prompt.IDTermTranslate, map[string]string{
		"source_locale": string(source), "target_locale": string(target),
	}))
	resp, err := p.ChatStructured(ctx, aiprovider.MessagesFromTurns(turns), termTranslateSchema())
	if err != nil {
		return nil, usage, fmt.Errorf("term-translate: %w", err)
	}
	usage = resp.Usage

	var out struct {
		Terms []TermTranslation `json:"terms"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &out); err != nil {
		return nil, usage, fmt.Errorf("term-translate: decode structured response: %w", err)
	}
	answered := map[string]bool{}
	kept := make([]TermTranslation, 0, len(out.Terms))
	for _, t := range out.Terms {
		req, ok := asked[t.ID]
		t.Term = strings.TrimSpace(t.Term)
		if !ok || answered[t.ID] || t.Term == "" || strings.ContainsAny(t.Term, "\n\t") {
			continue
		}
		if containsFold(req.TurnedDown, t.Term) {
			continue
		}
		answered[t.ID] = true
		t.Note = strings.TrimSpace(t.Note)
		kept = append(kept, t)
	}
	return kept, usage, nil
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), s) {
			return true
		}
	}
	return false
}

func termTranslateSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "term_translations",
		Description: "The term each concept takes in the target language",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"terms": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"term": map[string]any{"type": "string"},
							"note": map[string]any{"type": "string"},
						},
						"required":             []string{"id", "term", "note"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"terms"},
			"additionalProperties": false,
		},
	}
}
