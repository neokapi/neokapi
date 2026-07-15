package main

import (
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// ModelDataset is the generated model reference: the curated catalog of models
// kapi supports, with their neokapi lifecycle, rendered for the /models page.
//
// The catalog itself (providers/ai/models.json) is the source of truth; this
// dataset is it, resolved through the provider registry so each row carries its
// provider's display label without the page having to know provider ids. Gating
// on drift keeps the published page from describing a model kapi no longer ships,
// or omitting one it just added.
type ModelDataset struct {
	GeneratedAt string       `json:"generatedAt"`
	AsOf        string       `json:"asOf"`
	Note        string       `json:"note"`
	Models      []ModelEntry `json:"models"`
}

// ModelEntry is one catalogued model, flattened for the page.
type ModelEntry struct {
	ID              string   `json:"id"`
	Provider        string   `json:"provider"`
	ProviderLabel   string   `json:"providerLabel"`
	Label           string   `json:"label"`
	Aliases         []string `json:"aliases,omitempty"`
	DefaultFor      []string `json:"defaultFor,omitempty"`
	Recommended     bool     `json:"recommended"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	ContextWindow   int      `json:"contextWindow,omitempty"`
	Status          string   `json:"status"`
	Introduced      string   `json:"introduced,omitempty"`
	SupersededBy    string   `json:"supersededBy,omitempty"`
	RetirementDate  string   `json:"retirementDate,omitempty"`
	Note            string   `json:"note,omitempty"`
}

// collectModelDataset renders the catalog for the reference, resolving provider
// labels from the registry.
func collectModelDataset(now string) ModelDataset {
	models := aiprovider.Models()
	entries := make([]ModelEntry, 0, len(models))
	for _, m := range models {
		label := string(m.Provider)
		if info, ok := aiprovider.ProviderInfoFor(m.Provider); ok {
			label = info.Label
		}
		defaults := make([]string, 0, len(m.DefaultFor))
		for _, p := range m.DefaultFor {
			defaults = append(defaults, string(p))
		}
		entries = append(entries, ModelEntry{
			ID:              m.ID,
			Provider:        string(m.Provider),
			ProviderLabel:   label,
			Label:           m.Label,
			Aliases:         m.Aliases,
			DefaultFor:      defaults,
			Recommended:     m.IsRecommended(),
			MaxOutputTokens: m.MaxOutputTokens,
			ContextWindow:   m.ContextWindow,
			Status:          string(m.Status),
			Introduced:      m.Introduced,
			SupersededBy:    m.SupersededBy,
			RetirementDate:  m.RetirementDate,
			Note:            m.Note,
		})
	}
	return ModelDataset{
		GeneratedAt: now,
		AsOf:        aiprovider.CatalogAsOf(),
		Note:        "Generated from providers/ai/models.json via scripts/gen-refs. Do not edit here; edit the catalog and run `make generate-reference-docs`.",
		Models:      entries,
	}
}
