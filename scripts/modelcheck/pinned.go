package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The catalog (providers/ai/models.json) is the source of truth for the models
// kapi supports, so it is the thing to check against reality. A catalogued model
// that the provider no longer serves is a claim kapi cannot honour: a 404 on a
// user's first call if it is a default, and a lie on the /models page either way.
//
// The catalog holds no dead models (a retired model is removed, not tombstoned),
// so every entry is expected to be live. Local models (Ollama) are skipped: they
// are not served by a cloud list API. And only the providers modelcheck can
// actually enumerate (anthropic, openai, gemini) are checked. An endpoint-specific
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
// a static check with no network and no live list, so it runs every time.
//
// Bedrock keys are exempt: their ids are AWS inference-profile strings
// (eu.anthropic.claude-sonnet-4-6), not catalog ids, and Bedrock has its own
// refresh path.
func priceKeyProblems() []string {
	keys, err := priceKeys()
	if err != nil {
		return []string{fmt.Sprintf("price table: %v", err)}
	}
	var problems []string
	for _, key := range keys {
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
// not in the catalog. The table is found from the repository root, the nearest
// directory at or above the working directory that holds go.work. A table that
// cannot be found or read is an error, so the check never passes by not running.
func priceKeys() ([]string, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, "scripts", "batcheval", "prices.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the price table: %w", err)
	}
	var t struct {
		Models []struct {
			Key string `json:"key"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(t.Models) == 0 {
		return nil, fmt.Errorf("%s lists no models", path)
	}
	out := make([]string, 0, len(t.Models))
	for _, m := range t.Models {
		out = append(out, m.Key)
	}
	return out, nil
}

// repoRoot returns the nearest directory at or above the working directory
// that holds go.work.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.work at or above the working directory; run modelcheck inside the repository")
		}
		dir = parent
	}
}
