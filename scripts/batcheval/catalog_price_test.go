package main

import (
	"strings"
	"testing"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// A price is a cost published on /batch-eval that people budget against. Quoting
// one for a model the catalog does not list means the dashboard is advertising
// support kapi does not claim — so every priced model must resolve to a catalogued
// one. This is the always-on, keyless half of `make check-models`: the live half
// needs provider credentials, but this needs only the two committed files.
//
// Bedrock keys are exempt: their ids are AWS inference-profile strings
// (eu.anthropic.claude-sonnet-4-6), not catalog ids.
func TestEveryPriceIsForACataloguedModel(t *testing.T) {
	t.Parallel()

	for _, p := range prices.Models {
		provider, model, ok := strings.Cut(p.Key, ":")
		if !ok || provider == "bedrock" {
			continue
		}
		if provider == string(aiprovider.ClaudeCode) {
			if _, found := aiprovider.ModelByAlias(model); !found {
				t.Errorf("price %q: alias resolves to no catalogued model", p.Key)
			}
			continue
		}
		if _, found := aiprovider.ModelForID(model); !found {
			t.Errorf("price %q: no catalog entry (add it to providers/ai/models.json or drop the price)", p.Key)
		}
	}
}
