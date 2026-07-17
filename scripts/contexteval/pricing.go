package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Cost carries the same framing as batcheval: USD per pass at the vendor's
// published per-token rates, recorded so the lift can be read against what the
// context costs to send. The canonical rate table is
// scripts/batcheval/prices.json, refreshed by `make update-model-prices`; the
// copy embedded here is kept byte-identical by that target and by
// TestPricesMatchBatcheval — two silently diverging tables would price the same
// run differently depending on which eval measured it.

//go:embed prices.json
var pricesJSON []byte

// Price is a model's rate, as published by the vendor on a given date. The
// struct (and its JSON shape) is identical to batcheval's, so a reader of
// either history file sees one vocabulary.
type Price struct {
	Key           string  `json:"key"` // provider:model prefix
	ResolvesTo    string  `json:"resolves_to,omitempty"`
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
	Route         string  `json:"route"`
	// Metered is false for a subscription route — the rates are the right ones
	// to reason with, but the run was not billed per token, and the dashboard
	// must say so rather than present a fabricated invoice.
	Metered bool   `json:"metered"`
	Source  string `json:"source"`
	Note    string `json:"note,omitempty"`
	// AsOf dates the rate. Prices move; a cost without a date is a rumour.
	AsOf string `json:"as_of,omitempty"`
}

type priceTable struct {
	AsOf    string            `json:"as_of"`
	Note    string            `json:"note"`
	Sources map[string]string `json:"sources"`
	Models  []Price           `json:"models"`
}

var prices = loadPrices()

func loadPrices() priceTable {
	var t priceTable
	if err := json.Unmarshal(pricesJSON, &t); err != nil {
		panic(fmt.Sprintf("contexteval: prices.json: %v", err))
	}
	for i := range t.Models {
		t.Models[i].AsOf = t.AsOf
		if url, ok := t.Sources[t.Models[i].Source]; ok {
			t.Models[i].Source = url
		}
	}
	return t
}

// priceFor resolves a provider/model pair to its rates, longest matching prefix
// first. An unpriced model reports no price and the dashboard shows a blank —
// a number someone might budget against must not be invented.
func priceFor(provider, model string) (Price, bool) {
	key := provider + ":" + model
	var best Price
	found := false
	for _, p := range prices.Models {
		if strings.HasPrefix(key, p.Key) && len(p.Key) > len(best.Key) {
			best, found = p, true
		}
	}
	return best, found
}

// cost prices a pass's token usage.
func (p Price) cost(inputTokens, outputTokens int) float64 {
	return float64(inputTokens)*p.InputPerMTok/1e6 + float64(outputTokens)*p.OutputPerMTok/1e6
}
