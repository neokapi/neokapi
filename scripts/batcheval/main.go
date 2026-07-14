// Command batcheval measures what batching costs, by sweeping the number of
// blocks per LLM call and scoring each N.
//
// It exists because kapi ships a batch ceiling (tools.MaxBlocksPerCall) chosen
// from evidence about *adjacent* tasks — batch prompting on QA and classification,
// BatchGEMBA on MT evaluation — because no quality-versus-N curve for segment
// translation has ever been published. That is a guess. This measures it.
//
// What it scores, and why it is not "quality":
//
// The literature is consistent that the failure mode of batching at size is not
// clumsier wording. It is dropped, merged and renumbered segments — a *structural*
// failure, and in a localization pipeline a correctness catastrophe rather than a
// style complaint. One batched-screening study saw a model return unusable output
// for 72% of items at ten items per prompt, with the batch-size effect landing on
// surface formatting, not answer content. So the primary metrics here are the ones
// that catch that: does every segment come back, under the id it was sent, with its
// placeholders and inline tags intact?
//
// Those need no reference translations, which means they can be measured on any
// corpus, in any language pair, for the price of the calls.
//
//	go run ./scripts/batcheval                        # demo provider: proves the harness
//	go run ./scripts/batcheval -provider anthropic    # the real measurement (costs money)
//	go run ./scripts/batcheval -n 1,4,8,16,32,64,100  # the sweep
//
// Real numbers require a real model. Running this against the demo stub tells you
// the harness works and nothing whatsoever about batching, and it says so.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "batcheval: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		providerID = flag.String("provider", "demo", "AI provider (demo, anthropic, openai, gemini, ollama)")
		modelName  = flag.String("model", "", "model id (provider default when empty)")
		sizes      = flag.String("n", "1,4,8,16,32,64,100", "batch sizes to sweep")
		target     = flag.String("target", "de", "target locale — pick one that runs long (de, fi) to stress the output budget")
		out        = flag.String("out", "", "write the report as JSON to this path")
		repeat     = flag.Int("repeat", 1, "runs per N (a single run of a stochastic model is an anecdote)")
	)
	flag.Parse()

	corpus := Corpus()
	ns, err := parseSizes(*sizes)
	if err != nil {
		return err
	}

	provider, err := aiprovider.NewProvider(aiprovider.ProviderID(*providerID), aiprovider.Config{
		APIKey: os.Getenv(apiKeyEnv(*providerID)),
		Model:  *modelName,
	})
	if err != nil {
		return err
	}
	defer provider.Close()

	report := Report{
		Provider:  *providerID,
		Model:     *modelName,
		Target:    *target,
		Corpus:    corpus.Describe(),
		Simulated: *providerID == string(aiprovider.Demo),
	}

	for _, n := range ns {
		var runs []Result
		for r := range *repeat {
			res, err := runOnce(context.Background(), provider, corpus, n, model.LocaleID(*target))
			if err != nil {
				// One N failing is a data point, not a reason to lose the rest of the
				// sweep — a batch too large for the model is exactly what we're here to
				// find out.
				fmt.Fprintf(os.Stderr, "batcheval: N=%d run %d: %v\n", n, r+1, err)
				continue
			}
			runs = append(runs, res)
		}
		if len(runs) == 0 {
			continue
		}
		report.Results = append(report.Results, mean(n, runs))
	}

	printTable(report)

	if *out != "" {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("\nwrote %s\n", *out)
	}
	return nil
}

// runOnce translates the corpus at one batch size and scores the outcome.
func runOnce(ctx context.Context, provider aiprovider.LLMProvider, corpus TestCorpus, n int, target model.LocaleID) (Result, error) {
	blocks := corpus.Blocks()

	tool := aitools.NewAITranslateTool(provider, aitools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: target,
		// The pin the packer normally makes for us. This is the whole experiment:
		// hold everything else constant and move N.
		BatchSize: n,
		Context:   aitools.ContextKey,
	})

	start := time.Now()
	parts := make([]*model.Part, len(blocks))
	for i, b := range blocks {
		parts[i] = &model.Part{Type: model.PartBlock, Resource: b}
	}
	if err := runTool(ctx, tool, parts); err != nil {
		return Result{}, err
	}
	elapsed := time.Since(start)

	res := Result{N: n, Blocks: len(blocks), Elapsed: elapsed.Seconds()}
	usage := tool.TotalUsage()
	res.InputTokens, res.OutputTokens = usage.InputTokens, usage.OutputTokens

	for i, b := range blocks {
		got := b.TargetText(target)
		switch {
		case got == "":
			// The segment never came back. This is the failure that matters: with
			// positional mapping it used to shift every translation after it, and
			// it is what the batching literature predicts will grow with N.
			res.Missing++
		case !placeholdersIntact(corpus.Cases[i].Source, got):
			// A translation that loses a placeholder cannot be written back to the
			// source file. It is not a worse translation; it is not a translation.
			res.PlaceholderBreaks++
		case !tagsIntact(corpus.Cases[i].Source, got):
			res.TagBreaks++
		case got == corpus.Cases[i].Source && corpus.Cases[i].Source != "":
			// Echoed the source rather than translating it — a documented way for
			// a model to "cope" with a batch that is too large.
			res.Untranslated++
		}
	}

	res.Translated = res.Blocks - res.Missing
	return res, nil
}

func runTool(ctx context.Context, t *aitools.AITranslateTool, parts []*model.Part) error {
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	errc := make(chan error, 1)
	go func() { errc <- t.Process(ctx, in, out) }()

	// Drain, so Process can finish.
	go func() {
		for range out { //nolint:revive // draining
		}
	}()
	err := <-errc
	close(out)
	return err
}

// Report is the sweep, and the honesty about what produced it.
type Report struct {
	Provider string   `json:"provider"`
	Model    string   `json:"model,omitempty"`
	Target   string   `json:"target"`
	Corpus   string   `json:"corpus"`
	Results  []Result `json:"results"`
	// Simulated marks a run against the offline stub. Its numbers describe the
	// harness, not any model, and must never be quoted as a measurement.
	Simulated bool `json:"simulated"`
}

// Result is one batch size, scored.
type Result struct {
	N      int `json:"n"`
	Blocks int `json:"blocks"`

	// Structural integrity — the failure the literature predicts, and the one
	// that breaks a localization pipeline rather than merely disappointing it.
	Missing           int `json:"missing"`            // segments never returned
	PlaceholderBreaks int `json:"placeholder_breaks"` // {0}, %s, {{name}} lost or mangled
	TagBreaks         int `json:"tag_breaks"`         // inline markup lost or mangled
	Untranslated      int `json:"untranslated"`       // source echoed back verbatim
	Translated        int `json:"translated"`

	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Elapsed      float64 `json:"elapsed_seconds"`
}

// Intact is the headline: the share of segments that came back translated, with
// their placeholders and tags. Anything else is unusable, regardless of how well
// the words were chosen.
func (r Result) Intact() float64 {
	if r.Blocks == 0 {
		return 0
	}
	bad := r.Missing + r.PlaceholderBreaks + r.TagBreaks + r.Untranslated
	return float64(r.Blocks-bad) / float64(r.Blocks) * 100
}

func mean(n int, runs []Result) Result {
	out := Result{N: n, Blocks: runs[0].Blocks}
	for _, r := range runs {
		out.Missing += r.Missing
		out.PlaceholderBreaks += r.PlaceholderBreaks
		out.TagBreaks += r.TagBreaks
		out.Untranslated += r.Untranslated
		out.Translated += r.Translated
		out.InputTokens += r.InputTokens
		out.OutputTokens += r.OutputTokens
		out.Elapsed += r.Elapsed
	}
	d := len(runs)
	out.Missing /= d
	out.PlaceholderBreaks /= d
	out.TagBreaks /= d
	out.Untranslated /= d
	out.Translated /= d
	out.InputTokens /= d
	out.OutputTokens /= d
	out.Elapsed /= float64(d)
	return out
}

func printTable(r Report) {
	if r.Simulated {
		fmt.Println("⚠  demo provider: this run exercises the harness and measures NOTHING about batching.")
		fmt.Println("   Real numbers need a real model:  go run ./scripts/batcheval -provider anthropic")
		fmt.Println()
	}
	fmt.Printf("provider=%s model=%s target=%s corpus=%s\n\n", r.Provider, orDefault(r.Model, "(default)"), r.Target, r.Corpus)
	fmt.Printf("%5s  %7s  %8s  %12s  %9s  %13s  %8s\n",
		"N", "intact%", "missing", "placeholder", "tags", "untranslated", "out_tok")
	for _, res := range r.Results {
		fmt.Printf("%5d  %6.1f%%  %8d  %12d  %9d  %13d  %8d\n",
			res.N, res.Intact(), res.Missing, res.PlaceholderBreaks, res.TagBreaks, res.Untranslated, res.OutputTokens)
	}
}

func parseSizes(s string) ([]int, error) {
	var out []int
	for f := range strings.SplitSeq(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("batcheval: bad batch size %q", f)
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

func apiKeyEnv(p string) string {
	switch p {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	default:
		return ""
	}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
