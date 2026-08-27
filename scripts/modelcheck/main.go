// Command modelcheck asks the providers what models they actually serve today, and
// checks that against the neokapi model catalog (providers/ai/models.json).
//
// The catalog is curated, because the lifecycle facts it carries — when a model
// entered neokapi, what replaced it, when it retires — are not in any provider API.
// modelcheck is the alarm that keeps that curation honest, catching the two things
// that rot silently:
//
//   - **A catalogued model retires.** gemini-3-pro-preview answered 404 "no longer
//     available" in the middle of a benchmark sweep. A model kapi still lists as
//     active — worse, defaults to — is a hard failure on a user's first call. Such a
//     model should be removed from the catalog (it holds no dead entries), and this
//     reports it.
//   - **A price outlives its model.** The /batch-eval price table must only quote
//     models the catalog knows; a stale key is a cost published for something kapi
//     no longer supports. This is checked statically, every run.
//
// It also reports, on request, live models the catalog does not list — candidates
// worth considering. Prices themselves are refreshed separately by
// `make update-model-prices`; no vendor exposes prices over an API.
//
//	go run ./scripts/modelcheck              # report
//	go run ./scripts/modelcheck -check       # non-zero exit if a catalogued model is gone
//	go run ./scripts/modelcheck -candidates  # also list live models not in the catalog
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"
	"time"
)

// checkedNames lists the providers a run actually reached, in a stable order.
func checkedNames(live map[string][]string) []string {
	names := make([]string, 0, len(live))
	for k := range live {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "modelcheck: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	check := flag.Bool("check", false, "exit non-zero if a catalogued model is no longer served, or a price outlives its model")
	candidates := flag.Bool("candidates", false, "also list live models the catalog does not include")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	live := map[string][]string{}
	for _, p := range []struct {
		name string
		fn   func(context.Context) ([]string, error)
	}{
		{"gemini", listGemini},
		{"anthropic", listAnthropic},
		{"openai", listOpenAI},
	} {
		models, err := p.fn(ctx)
		if err != nil {
			// No key is not a failure: it means this provider cannot be checked from
			// this machine, which must not be confused with "its models are gone".
			fmt.Fprintf(os.Stderr, "  %s: skipped (%v)\n", p.name, err)
			continue
		}
		sort.Strings(models)
		live[p.name] = models
		fmt.Printf("%s: %d models live\n", p.name, len(models))
	}

	// Static: the price table must only quote catalogued models. Needs no live
	// list, so it runs even with no API keys.
	priced := priceKeyProblems()
	for _, p := range priced {
		fmt.Printf("\n  PRICE: %s\n", p)
	}

	missing := checkCatalogued(live)
	for _, m := range missing {
		fmt.Printf("\n  GONE: %s:%s\n    %s\n    → the provider no longer serves it; remove it from models.json\n", m.provider, m.model, m.where)
	}

	if *candidates {
		reportCandidates(live)
	}

	if len(missing) == 0 && len(priced) == 0 {
		// Say what was actually checked. Skipping every provider for want of a
		// key and then reporting that the catalog matches what they serve is a
		// pass that cannot fail, which is indistinguishable from a real one at
		// the moment a reader most needs to tell them apart.
		switch {
		case len(live) == 0:
			fmt.Println("\nno provider could be reached, so nothing was checked against a live list. " +
				"Every price is for a catalogued model, which is all this run verified.")
		case len(live) < 3:
			fmt.Printf("\n%s: catalogued models are all still served, and every price is for a "+
				"catalogued model. The other provider(s) were skipped and are unchecked.\n",
				strings.Join(checkedNames(live), ", "))
		default:
			fmt.Println("\nthe catalog matches what the providers serve, and every price is for a catalogued model")
		}
		return nil
	}
	if *check {
		return fmt.Errorf("%d catalogued model(s) gone, %d price(s) off-catalog", len(missing), len(priced))
	}
	return nil
}

// checkCatalogued reports catalogued models the provider no longer serves. Only
// providers we could actually reach are judged — an unreachable provider tells us
// nothing about its models, and treating that as "retired" would be the same
// false-cliff mistake the batch eval is built to avoid.
func checkCatalogued(live map[string][]string) []checkable {
	var out []checkable
	for _, p := range catalogModels() {
		models, reachable := live[p.provider]
		if !reachable {
			continue
		}
		if !slices.ContainsFunc(models, func(m string) bool {
			return m == p.model || strings.HasPrefix(m, p.model)
		}) {
			out = append(out, p)
		}
	}
	return out
}

// reportCandidates lists live models no catalog entry matches — the models a
// provider serves that kapi has not adopted. Informational only: providers serve
// many models (embeddings, TTS, legacy snapshots) kapi will never translate with,
// so this is a prompt to look, not a failure.
func reportCandidates(live map[string][]string) {
	catalogued := catalogModels()
	for provider, models := range live {
		var news []string
		for _, m := range models {
			matched := slices.ContainsFunc(catalogued, func(c checkable) bool {
				return c.provider == provider && (m == c.model || strings.HasPrefix(m, c.model))
			})
			if !matched {
				news = append(news, m)
			}
		}
		if len(news) > 0 {
			fmt.Printf("\n  %s: %d live models not in the catalog:\n", provider, len(news))
			for _, m := range news {
				fmt.Printf("    - %s\n", m)
			}
		}
	}
}

func listGemini(ctx context.Context) ([]string, error) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return nil, errors.New("GEMINI_API_KEY not set")
	}
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := getJSON(ctx, "https://generativelanguage.googleapis.com/v1beta/models",
		map[string]string{"x-goog-api-key": key}, &body); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		out = append(out, strings.TrimPrefix(m.Name, "models/"))
	}
	return out, nil
}

func listAnthropic(ctx context.Context) ([]string, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, errors.New("ANTHROPIC_API_KEY not set")
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, "https://api.anthropic.com/v1/models?limit=100",
		map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"}, &body); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		out = append(out, m.ID)
	}
	return out, nil
}

func listOpenAI(ctx context.Context) ([]string, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return nil, errors.New("OPENAI_API_KEY not set")
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, "https://api.openai.com/v1/models",
		map[string]string{"Authorization": "Bearer " + key}, &body); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		out = append(out, m.ID)
	}
	return out, nil
}

func getJSON(ctx context.Context, url string, headers map[string]string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, val := range headers {
		req.Header.Set(k, val)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
