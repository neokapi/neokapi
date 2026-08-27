// Command batcheval measures what batching costs, by sweeping the number of
// blocks per LLM call and scoring each N.
//
// It exists because kapi shipped a batch ceiling (tools.MaxBlocksPerCall) chosen
// from evidence about *adjacent* tasks — batch prompting on QA and classification,
// BatchGEMBA on MT evaluation — because no quality-versus-N curve for segment
// translation had ever been published. That was a guess. This measured it, and the
// guess did not survive: translation shows no degradation with N up to 600 blocks
// per call, and what damage there is sits at the *small* end. The ceiling moved to
// 64, and the real constraint turned out to be the output-token budget, which bills
// (a truncated reply is re-translated in halves) rather than breaks.
//
// Keep running it. An alias like `sonnet` or `gemini-3.5-flash` points at different
// weights over time, and a ceiling measured once is folklore by the next release.
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
//	go run ./scripts/batcheval                                   # demo stub: proves the harness
//	go run ./scripts/batcheval -models claude-code:sonnet        # the real measurement
//	go run ./scripts/batcheval -models claude-code:opus,gemini:gemini-3.5-flash -append <path>
//
// Real numbers require a real model. Running this against the demo stub tells you
// the harness works and nothing whatsoever about batching, and it says so.
//
// -append maintains the committed history behind the /batch-eval dashboard, so a
// ceiling chosen today does not quietly become folklore as the models underneath
// it change.
package main

import (
	"context"
	"errors"
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

	// Bedrock is the provider the Bowrain platform actually runs on, so it is the
	// one path here whose numbers describe production rather than an alternative.
	// It registers into the aiprovider registry via init(); it lives in the bowrain
	// module so the AWS SDK stays out of the kapi CLI, which is why this eval has a
	// module of its own.
	_ "github.com/neokapi/neokapi/bowrain/ai/bedrock"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "batcheval: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		models      = flag.String("models", "demo:", "provider:model pairs to sweep, comma-separated (e.g. claude-code:sonnet,gemini:gemini-3.5-flash)")
		sizes       = flag.String("n", "1,4,8,16,32,64,100", "batch sizes to sweep")
		target      = flag.String("target", "de", "target locale — pick one that runs long (de, fi) to stress the output budget")
		repeat      = flag.Int("repeat", 1, "runs per N (a single run of a stochastic model is an anecdote)")
		concurrency = flag.Int("concurrency", 4, "concurrent calls (orthogonal to N: it changes wall-clock, not what is measured)")
		date        = flag.String("date", time.Now().Format(dateLayout), "run date recorded in the history")
		appendTo    = flag.String("append", "", "merge the runs into this history file (the /batch-eval dashboard's data)")
		dump        = flag.Bool("dump", false, "print every source→target pair that scored as a break, so a count can be inspected rather than trusted")
		recost      = flag.String("recost", "", "re-price an existing history from prices.json and exit (no model calls)")
		blocks      = flag.Int("blocks", 30, "corpus size. A sweep to N=32 over 30 blocks does not test a batch of 32 — it tests the whole document in one call. To find where a model actually breaks, the corpus must be several times the largest N.")
	)
	flag.Parse()
	dumpBreaks = *dump

	// Cost is a pure function of tokens already measured and rates already
	// published, so it can be filled in without spending a single call. Useful
	// after a price refresh, and after adding the cost columns to runs that predate
	// them.
	if *recost != "" {
		return recostHistory(*recost)
	}

	corpus := CorpusN(*blocks)
	ns, err := parseSizes(*sizes)
	if err != nil {
		return err
	}
	targets, err := parseModels(*models)
	if err != nil {
		return err
	}

	var runs []Run
	for _, mt := range targets {
		r, err := sweep(context.Background(), mt, corpus, ns, sweepOpts{
			target:      model.LocaleID(*target),
			repeat:      max(*repeat, 1),
			concurrency: max(*concurrency, 1),
			date:        *date,
		})
		if err != nil {
			// One model failing (no key, no CLI, a retired alias) must not throw away
			// the models that did run.
			fmt.Fprintf(os.Stderr, "batcheval: %s: %v\n", mt, err)
			continue
		}
		printTable(r)
		runs = append(runs, r)
	}
	if len(runs) == 0 {
		return errors.New("no model produced a sweep")
	}

	if *appendTo != "" {
		h, err := LoadHistory(*appendTo)
		if err != nil {
			return err
		}
		h.Upsert(runs...)
		if err := h.Save(*appendTo); err != nil {
			return err
		}
		fmt.Printf("\nwrote %s (%d runs on record)\n", *appendTo, len(h.Runs))
	}
	return nil
}

const dateLayout = "2006-01-02"

// dumpBreaks makes every scored break printable (-dump). A finding you cannot
// look at is a number, not a finding.
var dumpBreaks bool

// modelTarget is one provider/model pair to sweep.
type modelTarget struct{ provider, model string }

func (m modelTarget) String() string {
	if m.model == "" {
		return m.provider
	}
	return m.provider + ":" + m.model
}

type sweepOpts struct {
	target      model.LocaleID
	repeat      int
	concurrency int
	date        string
}

// sweep runs the corpus through one model at every N.
func sweep(ctx context.Context, mt modelTarget, corpus TestCorpus, ns []int, opts sweepOpts) (Run, error) {
	provider, err := aiprovider.NewProvider(aiprovider.ProviderID(mt.provider), aiprovider.Config{
		APIKey:      os.Getenv(apiKeyEnv(mt.provider)),
		Model:       mt.model,
		Temperature: evalTemperature,
	})
	if err != nil {
		return Run{}, err
	}
	defer provider.Close()

	out := Run{
		Date:         opts.date,
		Provider:     mt.provider,
		Model:        mt.model,
		Target:       string(opts.target),
		Repeat:       opts.repeat,
		Concurrency:  opts.concurrency,
		Temperature:  evalTemperature,
		Corpus:       corpus.Describe(),
		CorpusWords:  corpus.Words(),
		CorpusBlocks: len(corpus.Cases),
		CorpusDigest: corpus.Digest(),
		Simulated:    mt.provider == string(aiprovider.Demo),
	}
	if p, ok := priceFor(mt.provider, mt.model); ok {
		out.Price = &p
	}

	for _, n := range ns {
		var results []Result
		var lastErr error
		for r := range opts.repeat {
			fmt.Fprintf(os.Stderr, "  %s N=%-3d run %d/%d …\n", mt, n, r+1, opts.repeat)
			res, err := runWithRetry(ctx, provider, corpus, n, opts)
			if err != nil {
				// One N failing is a data point, not a reason to lose the rest of the
				// sweep — a batch too large for the model to answer is exactly what we
				// are here to find out.
				fmt.Fprintf(os.Stderr, "batcheval: %s N=%d run %d: %v\n", mt, n, r+1, err)
				lastErr = err
				continue
			}
			results = append(results, res)
		}
		if len(results) == 0 {
			// A throttled N is a hole in the data, not a cliff in the model. Scoring
			// it as failure would invent the very finding this harness exists to test
			// — and would invent it in the most convincing possible shape, since
			// throttling punishes small batches (many calls) and would draw a curve
			// that "proves" batching helps.
			if throttled(lastErr) {
				fmt.Fprintf(os.Stderr, "  %s N=%d: THROTTLED — not measured (lower -concurrency and re-run)\n", mt, n)
				out.Results = append(out.Results, Result{
					N: n, Unmeasured: true, Error: lastErr.Error(),
				})
				continue
			}
			// Record a genuine failure as a point, not a gap: "the model could not
			// answer at this batch size" is the most important thing a curve can say,
			// and an absent point would read as "not measured".
			out.Results = append(out.Results, Result{
				N: n, Blocks: len(corpus.Cases), Missing: len(corpus.Cases),
				Failed: true, Error: lastErr.Error(),
			})
			continue
		}
		out.Results = append(out.Results, out.price(mean(n, results)))
	}
	return out, nil
}

// price fills in the words put through the model and what they cost at this run's
// recorded rates.
func (r Run) price(res Result) Result {
	// Words scale with the blocks actually attempted (corpus x repeats), so the
	// denominator must come from the run's own corpus — not from the authored 30,
	// which is no longer the only corpus there is.
	res.Words = r.CorpusWords * res.Blocks / max(r.CorpusBlocks, 1)
	// Only a metered route gets a cost.
	//
	// The subscription route (claude-code) is not billed per token, and — more to
	// the point — its token counts do not describe an API call. The CLI wraps every
	// request in its own agent system prompt and bills it as cache creation, so it
	// reported 240 input tokens across 60 calls. Any $/word built on that would be
	// fiction, in whichever direction happened to flatter the conclusion. Better a
	// blank the dashboard explains than a number someone budgets against.
	if r.Price != nil && r.Price.Metered {
		res.CostUSD = r.Price.cost(res.InputTokens, res.OutputTokens)
	}
	return res
}

// transientRetries is how many times a failed N is re-attempted before it is
// recorded as a failure of the *model*.
//
// This is not politeness, it is measurement integrity. A hosted API blips: an
// overloaded 529, a dropped socket, a CLI session that dies on startup. Recording
// one of those as "the model could not answer at this batch size" would publish a
// cliff in the curve that no model ever had — a fabricated finding, and the most
// damaging kind, because it is exactly the shape of the result we are looking for
// and would therefore be believed.
const transientRetries = 2

// throttleRetries is separate, and larger, because a 429 is not a failure at all —
// it is the provider asking us to slow down. Bedrock rate-limits per account, so a
// sweep at N=1 (thirty calls per repeat) trips it where N=32 (one call) never will.
// Backing off properly is the difference between measuring the model and measuring
// our own request rate.
const throttleRetries = 5

func runWithRetry(ctx context.Context, provider aiprovider.LLMProvider, corpus TestCorpus, n int, opts sweepOpts) (Result, error) {
	var err error
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			limit := transientRetries
			// Exponential, because a fixed pause into a rate limit just re-trips it.
			wait := time.Duration(attempt) * 2 * time.Second
			if throttled(err) {
				limit = throttleRetries
				wait = time.Duration(1<<attempt) * 2 * time.Second
			}
			if attempt > limit {
				return Result{}, err
			}
			fmt.Fprintf(os.Stderr, "    retry %d/%d in %s after: %v\n", attempt, limit, wait, truncErr(err))
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}
		var res Result
		if res, err = runOnce(ctx, provider, corpus, n, opts.target, opts.concurrency); err == nil {
			return res, nil
		}
	}
}

// throttled reports whether the provider asked us to slow down, rather than
// failing to answer. Matched on the wire vocabulary rather than a typed error
// because it has to hold across four unrelated SDKs.
func throttled(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, sig := range []string{"throttl", "429", "rate limit", "rate_limit", "too many requests", "quota", "resource_exhausted", "overloaded"} {
		if strings.Contains(s, sig) {
			return true
		}
	}
	return false
}

func truncErr(err error) string {
	s := err.Error()
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// parseModels parses "provider:model,provider:model". A bare "provider" (or
// "provider:") takes the provider's default model.
func parseModels(s string) ([]modelTarget, error) {
	var out []modelTarget
	for f := range strings.SplitSeq(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		provider, mdl, _ := strings.Cut(f, ":")
		if provider == "" {
			return nil, fmt.Errorf("batcheval: bad model target %q", f)
		}
		out = append(out, modelTarget{provider: provider, model: strings.TrimSpace(mdl)})
	}
	if len(out) == 0 {
		return nil, errors.New("batcheval: no model targets")
	}
	return out, nil
}

// runOnce translates the corpus at one batch size and scores the outcome.
func runOnce(ctx context.Context, provider aiprovider.LLMProvider, corpus TestCorpus, n int, target model.LocaleID, concurrency int) (Result, error) {
	blocks := corpus.Blocks()

	tool := aitools.NewAITranslateTool(provider, aitools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: target,
		// The pin the packer normally makes for us. This is the whole experiment:
		// hold everything else constant and move N.
		BatchSize: n,
		Context:   aitools.ContextKey,
		// Concurrency changes how long the sweep takes, not what it measures: each
		// call still carries exactly N blocks and sees nothing of the others.
		BatchConcurrency: concurrency,
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
	// Cache tokens are recorded because on some routes they are where the prompt
	// actually goes. claude-code bills almost its entire prompt as cache creation
	// (its agent system prompt), so counting only InputTokens reported 4 tokens per
	// call — and any cost built on that would be fiction.
	res.CacheCreationTokens, res.CacheReadTokens = usage.CacheCreationTokens, usage.CacheReadTokens

	for i, b := range blocks {
		src, got := corpus.Cases[i].Source, b.TargetText(target)
		kind := ""
		switch {
		case got == "":
			// The segment never came back. This is the failure that matters: with
			// positional mapping it used to shift every translation after it, and
			// it is what the batching literature predicts will grow with N.
			res.Missing++
			kind = "missing"
		case !placeholdersIntact(src, got):
			// A translation that loses a placeholder cannot be written back to the
			// source file. It is not a worse translation; it is not a translation.
			res.PlaceholderBreaks++
			kind = "placeholder"
		case !tagsIntact(src, got):
			res.TagBreaks++
			kind = "tag"
		case got == src && src != "":
			// Echoed the source rather than translating it — a documented way for
			// a model to "cope" with a batch that is too large.
			res.Untranslated++
			kind = "untranslated"
		}
		// A count nobody can explain is not evidence. -dump prints the offending
		// pair so a "4 tag breaks" can be confirmed as the model mangling markup
		// rather than the scorer being pedantic about it.
		if kind != "" && dumpBreaks {
			fmt.Fprintf(os.Stderr, "    [%s] N=%d %s\n      src: %q\n      got: %q\n", kind, n, corpus.Cases[i].Key, src, got)
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

	// Failed marks an N the model could not answer at all — a truncated reply, a
	// refused request, an unparseable batch — after transient retries. Recorded as
	// a point rather than a gap: "the model breaks here" is the single most useful
	// thing this curve can say, and an absent point would read as "not measured".
	Failed bool `json:"failed,omitempty"`

	// Unmeasured marks an N that never got a fair hearing: the requests were
	// throttled by the provider, so nothing about the model was learned.
	//
	// This is emphatically NOT the same as Failed, and conflating them fabricates
	// exactly the finding we are hunting. Bedrock rate-limits per account, and a
	// small N issues many more calls than a large one (thirty calls at N=1, one at
	// N=32) — so throttling hits the *small* batches hardest and, scored as
	// failure, would have drawn a curve saying the model breaks below N=16. It says
	// no such thing. The truthful record is a hole, labelled.
	Unmeasured bool `json:"unmeasured,omitempty"`
	// Error is why. A cliff in the curve is worth nothing if the reader cannot see
	// whether the model refused, truncated, or the transport gave out.
	Error string `json:"error,omitempty"`

	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int     `json:"cache_read_tokens,omitempty"`
	Elapsed             float64 `json:"elapsed_seconds"`

	// Words is the source words this point put through the model (corpus words ×
	// repeats). The denominator for cost and throughput.
	Words int `json:"words,omitempty"`
	// CostUSD prices the tokens at the rates recorded on the run. Zero when the
	// model is unpriced — a blank, never a guess.
	CostUSD float64 `json:"cost_usd,omitempty"`
}

// CostPer1kWords is the number to compare models on: what it costs to put a
// thousand words of content through this model at this batch size.
func (r Result) CostPer1kWords() float64 {
	if r.Words == 0 {
		return 0
	}
	return r.CostUSD / float64(r.Words) * 1000
}

// WordsPerSecond is throughput — the speed half of the trade-off.
func (r Result) WordsPerSecond() float64 {
	if r.Elapsed <= 0 {
		return 0
	}
	return float64(r.Words) / r.Elapsed
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

// mean aggregates the repeats of one N by *summing* everything over a corpus that
// is correspondingly larger — not by averaging.
//
// Averaging integer counts would round "one run in three dropped a segment" down
// to zero, which is precisely the failure this harness exists to catch, quietly
// erased by its own arithmetic. Summing keeps every failure, and every rate
// (Intact, cost per 1k words, words per second) divides by the work actually
// attempted, so all of them stay comparable across different -repeat values.
func mean(n int, runs []Result) Result {
	out := Result{N: n}
	for _, r := range runs {
		out.Blocks += r.Blocks
		out.CacheCreationTokens += r.CacheCreationTokens
		out.CacheReadTokens += r.CacheReadTokens
		out.Missing += r.Missing
		out.PlaceholderBreaks += r.PlaceholderBreaks
		out.TagBreaks += r.TagBreaks
		out.Untranslated += r.Untranslated
		out.Translated += r.Translated
		out.InputTokens += r.InputTokens
		out.OutputTokens += r.OutputTokens
		out.Elapsed += r.Elapsed
	}
	return out
}

func printTable(r Run) {
	fmt.Println()
	if r.Simulated {
		fmt.Println("⚠  demo provider: this run exercises the harness and measures NOTHING about batching.")
		fmt.Println("   Real numbers need a real model:  go run ./scripts/batcheval -models claude-code:sonnet")
		fmt.Println()
	}
	fmt.Printf("provider=%s model=%s target=%s repeat=%d corpus=%s (%s)\n\n",
		r.Provider, orDefault(r.Model, "(default)"), r.Target, r.Repeat, r.Corpus, r.CorpusDigest)
	fmt.Printf("%5s  %7s  %8s  %12s  %9s  %13s  %8s\n",
		"N", "intact%", "missing", "placeholder", "tags", "untranslated", "out_tok")
	for _, res := range r.Results {
		if res.Unmeasured {
			fmt.Printf("%5d  %7s  ← throttled by the provider; nothing measured (lower -concurrency)\n", res.N, "—")
			continue
		}
		note := ""
		if res.Failed {
			note = "  ← the model could not answer at this batch size"
		}
		fmt.Printf("%5d  %6.1f%%  %8d  %12d  %9d  %13d  %8d%s\n",
			res.N, res.Intact(), res.Missing, res.PlaceholderBreaks, res.TagBreaks,
			res.Untranslated, res.OutputTokens, note)
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

// evalTemperature is what every eval in this repo samples at.
//
// Zero, because an eval that cannot be re-run to the same numbers is a
// description of a method rather than a measurement. Until recently this was
// not expressible: Config.Temperature was a float64 with `omitempty`, so 0 and
// "unset" were the same value, and four of six providers dropped the field on
// the floor regardless. Every number this harness has published so far was
// sampled at whatever the API defaults to, which for Anthropic is 1.0.
//
// It is a variable rather than a constant so a sweep can deliberately raise it
// to measure variance, which is a different question and should say so.
var evalTemperature = new(0.0)
