package main

import (
	"github.com/neokapi/neokapi/core/ai/prompt"
)

// PromptDataset is the generated prompt reference: every prompt kapi sends to a
// language model, rendered from the same builders the binary uses.
//
// It exists so the documentation cannot describe a prompt kapi does not send.
// The prompts page used to claim they "include surrounding blocks, TM matches,
// and format metadata" — none of which was true — because the docs were written
// once and the code moved on. Generating them from prompt.Catalog() and gating
// on drift makes that failure mode structural rather than a matter of
// remembering.
type PromptDataset struct {
	GeneratedAt string        `json:"generatedAt"`
	Version     string        `json:"version"`
	Prompts     []PromptEntry `json:"prompts"`
}

// PromptEntry is one prompt, with its rendered turns.
type PromptEntry struct {
	ID      string `json:"id"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	// Structured is false for a prompt still built inline at its call site. Its
	// text cannot be rendered here, and the page says so rather than paraphrasing
	// it from memory.
	Structured bool         `json:"structured"`
	Turns      []PromptTurn `json:"turns,omitempty"`
}

// PromptTurn is one message of a rendered prompt, with its sections attributed.
type PromptTurn struct {
	Role     string          `json:"role"`
	Sections []PromptSection `json:"sections"`
}

// PromptSection is one addressable piece of a prompt, and who owns it.
type PromptSection struct {
	Kind    string `json:"kind"`
	Origin  string `json:"origin"`
	Heading string `json:"heading,omitempty"`
	Text    string `json:"text"`
}

// collectPromptDataset renders the prompt catalog for the reference.
func collectPromptDataset(now string) PromptDataset {
	catalog := prompt.Catalog()
	entries := make([]PromptEntry, 0, len(catalog))

	for _, c := range catalog {
		e := PromptEntry{
			ID:         c.ID,
			Tool:       c.Tool,
			Summary:    c.Summary,
			Structured: c.Structured,
		}
		for _, t := range c.Turns {
			turn := PromptTurn{Role: t.Role}
			for _, s := range t.Sections {
				turn.Sections = append(turn.Sections, PromptSection{
					Kind:    string(s.Kind),
					Origin:  s.Origin,
					Heading: s.Heading,
					Text:    s.Text,
				})
			}
			e.Turns = append(e.Turns, turn)
		}
		entries = append(entries, e)
	}

	return PromptDataset{GeneratedAt: now, Version: prompt.Version, Prompts: entries}
}
