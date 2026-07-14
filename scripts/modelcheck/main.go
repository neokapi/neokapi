// Command modelcheck asks the providers what models they actually serve today, and
// checks that against what kapi claims to know about.
//
// Two things rot silently, and both are load-bearing:
//
//   - **Models retire.** gemini-3-pro-preview answered 404 "no longer available"
//     in the middle of a benchmark sweep. A default model that has been retired is
//     not a degraded experience, it is a hard failure on the first call.
//   - **Prices move.** A cost published on a dashboard is a number people budget
//     against. A stale one is worse than none.
//
// This command handles the first: it lists live models from each provider's own
// API (the authoritative source — the docs lag) and reports anything kapi pins,
// prices, or defaults to that no longer exists. Pricing is refreshed separately by
// `make update-model-prices`, which reads the vendors' pricing pages, because no
// vendor exposes prices over an API.
//
//	go run ./scripts/modelcheck              # report
//	go run ./scripts/modelcheck -check       # non-zero exit if anything kapi uses is gone
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

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "modelcheck: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	check := flag.Bool("check", false, "exit non-zero if a model kapi pins or prices no longer exists")
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

	missing := checkPinned(live)
	for _, m := range missing {
		fmt.Printf("\n  GONE: %s\n    %s\n", m.model, m.where)
	}
	if len(missing) == 0 {
		fmt.Println("\nevery model kapi pins or prices is still served")
		return nil
	}
	if *check {
		return fmt.Errorf("%d model(s) kapi uses no longer exist", len(missing))
	}
	return nil
}

type gone struct{ model, where string }

// checkPinned reports models kapi depends on that the provider no longer serves.
// Only providers we could actually reach are judged — an unreachable provider
// tells us nothing about its models, and treating that as "retired" would be the
// same false-cliff mistake the batch eval is built to avoid.
func checkPinned(live map[string][]string) []gone {
	var out []gone
	for _, p := range pinnedModels() {
		models, reachable := live[p.provider]
		if !reachable {
			continue
		}
		if !slices.ContainsFunc(models, func(m string) bool {
			return m == p.model || strings.HasPrefix(m, p.model)
		}) {
			out = append(out, gone{model: p.provider + ":" + p.model, where: p.where})
		}
	}
	return out
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
