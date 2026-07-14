package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// A "pinned" model is one kapi will actually try to call, or will quote a price
// for. There are two such places, and both are hazards when a model retires:
//
//   - the provider defaults in the registry — the model a user gets when they name
//     none, and the one `kapi models` advertises. A retirement here is a 404 on a
//     first-time user's very first call, for something they did nothing to cause.
//   - the batch-eval price table — a model whose cost we publish on a dashboard.
//
// Both are read from their own source of truth rather than re-listed here, so this
// check cannot drift away from the thing it is checking.
type pinned struct {
	provider string
	model    string
	where    string
}

func pinnedModels() []pinned {
	var out []pinned

	for _, info := range aiprovider.Providers() {
		if info.DefaultModel == "" || info.Local {
			continue // nothing to check, or served off a local machine
		}
		if info.Name == aiprovider.ClaudeCode {
			// claude-code resolves its aliases (sonnet, opus, haiku) against the
			// signed-in account, not a public model list. Nothing to check them against.
			continue
		}
		out = append(out, pinned{
			provider: string(info.Name),
			model:    info.DefaultModel,
			where:    "provider default — a retirement here 404s a user's first call",
		})
	}

	for _, key := range priceKeys() {
		provider, model, ok := strings.Cut(key, ":")
		if !ok || provider == string(aiprovider.ClaudeCode) {
			continue
		}
		out = append(out, pinned{
			provider: provider,
			model:    model,
			where:    "priced on /batch-eval — scripts/batcheval/prices.json",
		})
	}
	return out
}

// priceKeys reads the batch-eval price table: the same file the dashboard is
// priced from, so a model we publish a cost for cannot silently be one that no
// longer runs.
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
