package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Cost is the question a user actually asks — not "how many tokens", which is an
// implementation detail of a vendor's tokenizer, but "what does it cost to put my
// content through a model, and what do I give up to make it cheaper".
//
// The unit is **USD per 1,000 source words**. Words, because that is what content
// budgets are denominated in and what every localization vendor quotes against;
// source, because target-side word counts vary by language and would make the same
// job look like different amounts of work in German and in Japanese. Tokens are
// recorded too, but they are the input to the number, not the number.
//
// Nothing here is translation-specific. Any AI pass over content — translate,
// review, a check, terminology, entity extraction — has the same shape: content
// goes in, tokens are billed, an answer comes back. These are the economics of
// running a model over your words, whatever it is being asked to do with them.
// Translation is simply the pass this harness drives, because it is the one that
// batches.
//
// Rates are DATA, not code: scripts/batcheval/prices.json, refreshed by
// `make update-model-prices`. A rate hardcoded in a source file goes stale
// silently, and a stale rate published as a cost is worse than no cost at all.

//go:embed prices.json
var pricesJSON []byte

// Routes a price can be quoted against. The same weights cost different amounts
// depending on how they are reached.
const (
	RouteAPI          = "api"          // first-party metered API
	RouteSubscription = "subscription" // e.g. claude-code, on the local Claude subscription
	// RouteBedrock is the route the Bowrain platform actually runs on
	// (bowrain/ai/bedrock, Converse API, ambient AWS credentials). Its rates are
	// NOT the first-party API's and must never be assumed equal to them: AWS prices
	// Bedrock independently, and cross-region inference profiles — which the
	// platform uses (`eu.anthropic.…`) — are priced separately again.
	//
	// No Bedrock rates are recorded yet, because none could be established: the AWS
	// Pricing API does not list the Claude 4.6 models for eu-north-1 (its `model`
	// attribute still tops out at "Claude 3 Sonnet"). Bedrock therefore measures
	// clean on quality and throughput and shows a blank for cost, which is the
	// honest answer until a rate is confirmed.
	RouteBedrock = "bedrock"
)

// Price is a model's rate, as published by the vendor on a given date.
//
// Rates are pinned into each recorded run rather than looked up at render time:
// re-pricing a historical run at today's rates would quietly rewrite what it cost
// to run, which is the one thing the record exists to remember.
type Price struct {
	Key           string  `json:"key"` // provider:model prefix
	ResolvesTo    string  `json:"resolves_to,omitempty"`
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
	Route         string  `json:"route"`
	// Metered is false for a subscription route — the rates are still the right
	// ones to reason with (they are what the same model costs on the API) but the
	// run was not billed per token, and the dashboard must say so rather than
	// present a fabricated invoice.
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
		panic(fmt.Sprintf("batcheval: prices.json: %v", err))
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
// first, so `gemini-3.1-pro-preview` picks up the `gemini-3.1-pro` entry.
//
// An unpriced model is not guessed at. It reports no price, and the dashboard
// shows a blank — a number someone might budget against must not be invented.
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

// cost prices a call's token usage.
func (p Price) cost(inputTokens, outputTokens int) float64 {
	return float64(inputTokens)*p.InputPerMTok/1e6 + float64(outputTokens)*p.OutputPerMTok/1e6
}
