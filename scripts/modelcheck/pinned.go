package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The catalog (providers/ai/models.json) is the source of truth for the models
// kapi supports, so it is the thing to check against reality. A catalogued model
// that the provider no longer serves is a claim kapi cannot honour — a 404 on a
// user's first call if it is a default, and a lie on the /models page either way.
//
// The catalog holds no dead models (a retired model is removed, not tombstoned),
// so every entry is expected to be live. Local models (Ollama) are skipped: they
// are not served by a cloud list API. And only the providers modelcheck can
// actually enumerate (anthropic, openai, gemini) are checked — an endpoint-specific
// provider like Azure has no public model list, so its models ride under their
// openai-family catalog entry instead.
type checkable struct {
	provider string
	model    string // catalog id, used as a match prefix against live ids
	where    string
}

// catalogModels lists the catalogued models that can and should be checked
// against a live provider list.
func catalogModels() []checkable {
	var out []checkable
	for _, m := range aiprovider.Models() {
		if aiprovider.IsLocalProvider(m.Provider) {
			continue
		}
		switch m.Provider {
		case aiprovider.Anthropic, aiprovider.OpenAI, aiprovider.Gemini:
		default:
			continue // no public list API to check against from here
		}
		out = append(out, checkable{
			provider: string(m.Provider),
			model:    m.ID,
			where:    fmt.Sprintf("catalog: %s (%s) in providers/ai/models.json", m.Label, m.Status),
		})
	}
	return out
}

// priceKeyProblems checks the batch-eval price table against the catalog: every
// metered/subscription price must be for a model the catalog knows, or the
// dashboard is quoting a cost for something kapi does not claim to support. It is
// a static check — no network, no live list — so it runs every time.
//
// Bedrock keys are exempt: their ids are AWS inference-profile strings
// (eu.anthropic.claude-sonnet-4-6), not catalog ids, and Bedrock has its own
// refresh path.
func priceKeyProblems() []string {
	var problems []string
	for _, key := range priceKeys() {
		provider, model, ok := strings.Cut(key, ":")
		if !ok || provider == "bedrock" {
			continue
		}
		if provider == string(aiprovider.ClaudeCode) {
			// claude-code prices are for the subscription aliases (sonnet, opus…),
			// which resolve to a catalogued anthropic model by alias.
			if _, found := aiprovider.ModelByAlias(model); !found {
				problems = append(problems, fmt.Sprintf("price %q: alias resolves to no catalogued model", key))
			}
			continue
		}
		if _, found := aiprovider.ModelForID(model); !found {
			problems = append(problems, fmt.Sprintf("price %q: no catalog entry (add it to models.json or drop the price)", key))
		}
	}
	return problems
}

// priceKeys reads the batch-eval price table: the same file the dashboard is
// priced from, so a model we publish a cost for cannot silently be one that is
// not in the catalog.
func priceKeys() []string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "batcheval", "prices.json"))
	if err != nil {
		return nil
	}
	var t struct {
		Models []struct {
			Key string `json:"key"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return nil
	}
	out := make([]string, 0, len(t.Models))
	for _, m := range t.Models {
		out = append(out, m.Key)
	}
	return out
}
