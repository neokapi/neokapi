// Command contexteval measures how well a model uses the context kapi injects —
// the term rules, the voice profile, and the instruction — rather than how
// well it translates. The two are different questions: a capable model may pick
// the right register unprompted, and that earns our brand guide no credit.
//
// So the core metric is a differential. Every fixture is translated twice
// through the production pipeline (core/ai/tools.AITranslateTool):
//
//   - bare: no term rules, no voice profile, no instruction;
//   - steered: the full context, injected exactly as production injects it
//     (term rules → prompt terminology section, profile.VoiceProfile →
//     RenderVoiceGuideCompact → voice section, instruction → instruction
//     section).
//
// Both passes are scored with the framework's own check tools (term-check,
// dnt-check, voice-vocab-check, pattern-check) against expectations the corpus
// declares per fixture. Two numbers fall out per dimension — absolute adherence
// (did the steered output satisfy the requirement) and lift (steered minus
// bare: how much the context actually moved the model). Lift is the number that
// says whether a model is steerable, which is what kapi's context injection is
// buying; and lift per 1,000 context tokens says whether the guide earns what
// it costs to send on this model.
//
// Results are reported per dimension (terminology / voice / instruction), never
// as one collapsed score, and appended to a committed history behind the
// /context-eval dashboard — steerability is model- and time-specific, and an
// alias like `sonnet` points at different weights over time.
//
// The deterministic checks are the backbone. The genuinely subjective remainder
// of voice (register, naturalness, restraint) is scored by an optional
// cross-family LLM judge (-judge) under a fixed yes/no rubric, blind to which
// model and which variant produced a text — and the dashboard publishes that
// dimension only once judge–human agreement has been measured on a labeled
// seed set (-judge-validate) and meets the bar. Reporting an unvalidated
// judge's opinion as a model's behavior is how adherence evals rot.
//
//	go run ./scripts/contexteval                                    # demo stub: proves the harness
//	go run ./scripts/contexteval -models claude-code:sonnet         # the real measurement
//	go run ./scripts/contexteval -models gemini:gemini-3.5-flash -targets de,fr -append <path>
//
// Real numbers require a real model. A demo run is marked `simulated` and is
// excluded from every chart on the dashboard.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"

	// Bedrock is the provider the Bowrain platform actually runs on. It lives in
	// the bowrain module so the AWS SDK stays out of the kapi CLI, which is why
	// this eval has a module of its own (see go.mod).
	_ "github.com/neokapi/neokapi/bowrain/ai/bedrock"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "contexteval: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		models      = flag.String("models", "demo:", "provider:model pairs to sweep, comma-separated; `catalog` expands to the model catalog's recommended active models")
		targets     = flag.String("targets", strings.Join(Targets(), ","), "target locales to sweep — adherence varies by language, so one locale is not representative")
		repeat      = flag.Int("repeat", 1, "passes per variant (a single run of a stochastic model is an anecdote)")
		concurrency = flag.Int("concurrency", 4, "concurrent batch calls inside a pass")
		date        = flag.String("date", time.Now().Format(dateLayout), "run date recorded in the history")
		appendTo    = flag.String("append", "", "merge the runs into this history file (the /context-eval dashboard's data)")
		dump        = flag.Bool("dump", false, "print every failed expectation with the offending output, so a rate can be inspected rather than trusted")
		recost      = flag.String("recost", "", "re-price an existing history from prices.json and exit (no model calls)")
		saveOutputs = flag.String("save-outputs", "", "also write every (fixture, variant, translation) to this JSON file — inspection, and the raw material for judge-validation labels")
		judgeSpec   = flag.String("judge", "", "provider:model to judge the subjective voice criteria (must be a different model family than the model under test)")
		validate    = flag.String("judge-validate", "", "labels JSON of human verdicts; measures judge–human agreement with -judge and records it via -append, then exits")
		label       = flag.String("label", "", "label saved outputs interactively for judge validation; takes the -save-outputs file and writes verdicts to -labels")
		labelsOut   = flag.String("labels", "scripts/contexteval/labels.json", "where -label reads and writes the human verdicts")
		labelTarget = flag.Int("label-target", TargetJudgeItems, "how many labelled items -label aims for")
	)
	flag.Parse()

	if *label != "" {
		// No provider is constructed: labelling is a person and a file, and
		// asking for credentials to do it would be a reason not to.
		return runLabelling(*label, *labelsOut, *labelTarget)
	}
	dumpFailures = *dump

	if *recost != "" {
		return recostHistory(*recost)
	}

	var judge *judgeTarget
	if *judgeSpec != "" {
		j, err := newJudge(*judgeSpec)
		if err != nil {
			return err
		}
		defer j.Close()
		judge = j
	}

	if *validate != "" {
		if judge == nil {
			return errors.New("-judge-validate needs -judge")
		}
		if *appendTo == "" {
			return errors.New("-judge-validate needs -append to record the measured agreement")
		}
		return runJudgeValidation(context.Background(), judge, *validate, *appendTo, *date)
	}

	locales, err := parseTargets(*targets)
	if err != nil {
		return err
	}
	modelTargets, err := parseModels(*models)
	if err != nil {
		return err
	}

	// Outputs are buffered only when they will be written; the nil receiver is
	// the gate outputLog.log checks.
	var outputs *outputLog
	if *saveOutputs != "" {
		outputs = &outputLog{}
	}
	var runs []Run
	for _, mt := range modelTargets {
		provider, err := mt.newLLM()
		if err != nil {
			// One model failing (no key, a retired alias) must not throw away the
			// models that did run.
			fmt.Fprintf(os.Stderr, "contexteval: %s: %v\n", mt, err)
			continue
		}
		for _, locale := range locales {
			corpus := CorpusFor(locale)
			if len(corpus.Fixtures) == 0 {
				fmt.Fprintf(os.Stderr, "contexteval: no fixtures carry expectations for %q (authored targets: %s)\n",
					locale, strings.Join(Targets(), ", "))
				continue
			}
			r := measure(context.Background(), provider, mt, corpus, sweepOpts{
				repeat:      max(*repeat, 1),
				concurrency: max(*concurrency, 1),
				date:        *date,
				judge:       judge,
				outputs:     outputs,
			})
			printTable(r)
			runs = append(runs, r)
		}
		provider.Close()
	}
	if len(runs) == 0 {
		return errors.New("no model produced a run")
	}

	if *saveOutputs != "" {
		if err := outputs.save(*saveOutputs); err != nil {
			return err
		}
		fmt.Printf("\nwrote %s (%d outputs)\n", *saveOutputs, len(outputs.Items))
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

// dumpFailures makes every scored failure printable (-dump).
var dumpFailures bool

// modelTarget is one provider/model pair to sweep.
type modelTarget struct{ provider, model string }

func (m modelTarget) String() string {
	if m.model == "" {
		return m.provider
	}
	return m.provider + ":" + m.model
}

// newLLM is the one place a provider is constructed — the sweep and the judge
// must reach models the same way, or a change to key wiring reaches one and
// not the other.
func (m modelTarget) newLLM() (aiprovider.LLMProvider, error) {
	return aiprovider.NewProvider(aiprovider.ProviderID(m.provider), aiprovider.Config{
		APIKey:      os.Getenv(apiKeyEnv(m.provider)),
		Model:       m.model,
		Temperature: evalTemperature,
	})
}

type sweepOpts struct {
	repeat      int
	concurrency int
	date        string
	judge       *judgeTarget
	outputs     *outputLog
}

// measure runs the with/without-context differential for one model over one
// target's corpus, aggregated over repeats.
func measure(ctx context.Context, provider aiprovider.LLMProvider, mt modelTarget, corpus TestCorpus, opts sweepOpts) Run {
	out := Run{
		Date:           opts.date,
		Provider:       mt.provider,
		Model:          mt.model,
		Target:         corpus.Target,
		Repeat:         opts.repeat,
		Concurrency:    opts.concurrency,
		Corpus:         corpus.Describe(),
		CorpusFixtures: len(corpus.Fixtures),
		CorpusChecks:   corpus.Checks(),
		CorpusWords:    corpus.Words(),
		CorpusDigest:   corpus.Digest(),
		Simulated:      mt.provider == string(aiprovider.Demo),
	}
	if p, ok := priceFor(mt.provider, mt.model); ok {
		out.Price = &p
	}

	bareScore, steeredScore := newPassScore(), newPassScore()
	bareRec, steeredRec := &PassRecord{}, &PassRecord{}
	var judgeRec *JudgeRecord
	if opts.judge != nil && !out.Simulated {
		judgeRec = opts.judge.record(mt)
		judgeRec.SkippedSameLanguage = !judgeableTarget(corpus.Target)
	}

	for r := range opts.repeat {
		for _, variant := range []string{"bare", "steered"} {
			fmt.Fprintf(os.Stderr, "  %s %s %s run %d/%d …\n", mt, corpus.Target, variant, r+1, opts.repeat)
			blocks, rec, err := translateWithRetry(ctx, provider, corpus, variant == "steered", opts.concurrency)
			if err != nil {
				// A throttled pass is a hole in the record, not a 0% adherence; a
				// genuine failure is recorded with its reason. Either way the run
				// carries no dimension results — half a differential is not a
				// differential.
				out.Unmeasured = throttled(err)
				out.Error = err.Error()
				if out.Unmeasured {
					fmt.Fprintf(os.Stderr, "  %s %s: THROTTLED — not measured (lower -concurrency and re-run)\n", mt, corpus.Target)
				} else {
					fmt.Fprintf(os.Stderr, "  %s %s: FAILED — %v\n", mt, corpus.Target, err)
				}
				return out
			}
			score, err := scorePass(ctx, corpus, blocks, variant)
			if err != nil {
				out.Error = err.Error()
				return out
			}
			opts.outputs.log(mt, corpus, blocks, variant)
			if variant == "bare" {
				mergeScore(&bareScore, score, bareRec, rec)
			} else {
				mergeScore(&steeredScore, score, steeredRec, rec)
			}
			if judgeRec != nil && !judgeRec.SkippedSameFamily && !judgeRec.SkippedSameLanguage {
				counts := &judgeRec.Bare
				if variant == "steered" {
					counts = &judgeRec.Steered
				}
				if err := opts.judge.judgePass(ctx, corpus, blocks, counts); err != nil {
					judgeRec.Error = err.Error()
				}
			}
		}
	}

	if p := out.Price; p != nil && p.Metered {
		bareRec.CostUSD = p.cost(bareRec.InputTokens, bareRec.OutputTokens)
		steeredRec.CostUSD = p.cost(steeredRec.InputTokens, steeredRec.OutputTokens)
	}
	out.Bare, out.Steered = bareRec, steeredRec
	// The measured price of the context: extra input tokens per steered pass.
	// Averaged per pass so the number reads as "sending this guide costs N
	// tokens", independent of -repeat.
	if d := steeredRec.InputTokens - bareRec.InputTokens; d > 0 {
		out.ContextTokens = d / opts.repeat
	}
	out.Dimensions = dimensionRecords(bareScore, steeredScore)
	out.Kinds = kindRecords(bareScore, steeredScore)
	out.Judge = judgeRec
	return out
}

func mergeScore(agg *PassScore, s PassScore, aggRec, rec *PassRecord) {
	for dim, c := range s.ByDim {
		if agg.ByDim[dim] == nil {
			agg.ByDim[dim] = &Counts{}
		}
		agg.ByDim[dim].Scored += c.Scored
		agg.ByDim[dim].Passed += c.Passed
	}
	for kind, c := range s.ByKind {
		if agg.ByKind[kind] == nil {
			agg.ByKind[kind] = &Counts{}
		}
		agg.ByKind[kind].Scored += c.Scored
		agg.ByKind[kind].Passed += c.Passed
	}
	aggRec.InputTokens += rec.InputTokens
	aggRec.OutputTokens += rec.OutputTokens
	aggRec.CacheCreationTokens += rec.CacheCreationTokens
	aggRec.CacheReadTokens += rec.CacheReadTokens
	aggRec.Elapsed += rec.Elapsed
	aggRec.Missing += s.Missing
	aggRec.Untranslated += s.Untranslated
}

func dimensionRecords(bare, steered PassScore) []DimensionRecord {
	var out []DimensionRecord
	for _, dim := range []string{DimTerminology, DimVoice, DimInstruction} {
		b, s := bare.ByDim[dim], steered.ByDim[dim]
		if b == nil && s == nil {
			continue
		}
		rec := DimensionRecord{Dimension: dim}
		if b != nil {
			rec.Bare = *b
		}
		if s != nil {
			rec.Steered = *s
		}
		out = append(out, rec)
	}
	return out
}

func kindRecords(bare, steered PassScore) []KindRecord {
	keys := map[string]bool{}
	for k := range bare.ByKind {
		keys[k] = true
	}
	for k := range steered.ByKind {
		keys[k] = true
	}
	var out []KindRecord
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		dim, kind, _ := strings.Cut(key, "/")
		rec := KindRecord{Dimension: dim, Kind: kind}
		if c := bare.ByKind[key]; c != nil {
			rec.Bare = *c
		}
		if c := steered.ByKind[key]; c != nil {
			rec.Steered = *c
		}
		out = append(out, rec)
	}
	return out
}

// translateWithRetry runs one variant pass with the retry discipline batcheval
// established: transient blips are re-attempted so an overloaded 529 or a
// dropped socket never publishes a finding no model produced, and throttling
// gets more patience because a 429 is not a failure at all.
func translateWithRetry(ctx context.Context, provider aiprovider.LLMProvider, corpus TestCorpus, steered bool, concurrency int) ([]*model.Block, *PassRecord, error) {
	var err error
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			limit := transientRetries
			wait := time.Duration(attempt) * 2 * time.Second
			if throttled(err) {
				limit = throttleRetries
				wait = time.Duration(1<<attempt) * 2 * time.Second
			}
			if attempt > limit {
				return nil, nil, err
			}
			fmt.Fprintf(os.Stderr, "    retry %d/%d in %s after: %v\n", attempt, limit, wait, truncErr(err))
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}
		var blocks []*model.Block
		var rec *PassRecord
		if blocks, rec, err = translatePass(ctx, provider, corpus, steered, concurrency); err == nil {
			return blocks, rec, nil
		}
	}
}

const (
	transientRetries = 2
	throttleRetries  = 5
)

// translatePass translates the corpus once through the production translate
// tool. The steered pass binds the corpus context the way production binds it;
// the bare pass differs in nothing else — same tool, same batching, same
// prompts apart from the steering sections.
func translatePass(ctx context.Context, provider aiprovider.LLMProvider, corpus TestCorpus, steered bool, concurrency int) ([]*model.Block, *PassRecord, error) {
	blocks := corpus.Blocks()

	cfg := aitools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleID(corpus.Target),
		// Production defaults: auto batching, key context. Held identical across
		// variants so the only difference between the two passes is the context.
		Context:          aitools.ContextKey,
		BatchConcurrency: concurrency,
	}
	if steered {
		cfg.TermRules = corpus.Ctx.TermRules
		cfg.Instruction = corpus.Ctx.Instruction
		cfg.Profile = corpus.Ctx.Profile
	}
	tool := aitools.NewAITranslateTool(provider, cfg)

	start := time.Now()
	parts := make([]*model.Part, len(blocks))
	for i, b := range blocks {
		parts[i] = &model.Part{Type: model.PartBlock, Resource: b}
	}
	if err := runTool(ctx, tool, parts); err != nil {
		return nil, nil, err
	}

	usage := tool.TotalUsage()
	rec := &PassRecord{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		// Cache tokens are recorded because on some routes they are where the
		// prompt actually goes (claude-code bills its agent prompt as cache
		// creation).
		CacheCreationTokens: usage.CacheCreationTokens,
		CacheReadTokens:     usage.CacheReadTokens,
		Elapsed:             time.Since(start).Seconds(),
	}
	return blocks, rec, nil
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
	go func() {
		for range out { //nolint:revive // draining
		}
	}()
	err := <-errc
	close(out)
	return err
}

// throttled reports whether the provider asked us to slow down rather than
// failing to answer. Matched on the wire vocabulary because it has to hold
// across unrelated SDKs.
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

// parseModels parses "provider:model,provider:model". A bare "provider" takes
// the provider's default model; the special value "catalog" expands to the
// model catalog's recommended active models, so a newly catalogued model gets
// swept without editing a flag.
func parseModels(s string) ([]modelTarget, error) {
	var out []modelTarget
	for f := range strings.SplitSeq(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if f == "catalog" {
			out = append(out, catalogTargets()...)
			continue
		}
		provider, mdl, _ := strings.Cut(f, ":")
		if provider == "" {
			return nil, fmt.Errorf("contexteval: bad model target %q", f)
		}
		out = append(out, modelTarget{provider: provider, model: strings.TrimSpace(mdl)})
	}
	if len(out) == 0 {
		return nil, errors.New("contexteval: no model targets")
	}
	return out, nil
}

// catalogTargets derives a sweep list from providers/ai/models.json: active,
// recommended models on the direct-API providers this harness can reach with an
// API key from the environment.
func catalogTargets() []modelTarget {
	swept := map[aiprovider.ProviderID]bool{aiprovider.Anthropic: true, aiprovider.OpenAI: true, aiprovider.Gemini: true}
	var out []modelTarget
	for _, m := range aiprovider.Models() {
		if m.Status != aiprovider.StatusActive || !m.IsRecommended() || !swept[m.Provider] {
			continue
		}
		out = append(out, modelTarget{provider: string(m.Provider), model: m.ID})
	}
	return out
}

func parseTargets(s string) ([]string, error) {
	var out []string
	for f := range strings.SplitSeq(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("contexteval: no target locales")
	}
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

// outputLog collects every scored translation for -save-outputs: the raw
// material for inspection and for authoring judge-validation labels.
type outputLog struct {
	Items []outputItem `json:"items"`
}

type outputItem struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Target      string `json:"target"`
	Variant     string `json:"variant"`
	Fixture     string `json:"fixture"`
	Source      string `json:"source"`
	Translation string `json:"translation"`
}

func (l *outputLog) log(mt modelTarget, corpus TestCorpus, blocks []*model.Block, variant string) {
	if l == nil {
		return
	}
	loc := model.LocaleID(corpus.Target)
	for i, f := range corpus.Fixtures {
		l.Items = append(l.Items, outputItem{
			Provider: mt.provider, Model: mt.model, Target: corpus.Target, Variant: variant,
			Fixture: f.Key, Source: f.Source, Translation: blocks[i].TargetText(loc),
		})
	}
}

func (l *outputLog) save(path string) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func printTable(r Run) {
	fmt.Println()
	if r.Simulated {
		fmt.Println("⚠  demo provider: this run exercises the harness and measures NOTHING about context adherence.")
		fmt.Println("   Real numbers need a real model:  go run ./scripts/contexteval -models claude-code:sonnet")
		fmt.Println()
	}
	fmt.Printf("provider=%s model=%s target=%s repeat=%d corpus=%s (%s)\n\n",
		r.Provider, orDefault(r.Model, "(default)"), r.Target, r.Repeat, r.Corpus, r.CorpusDigest)
	if r.Unmeasured {
		fmt.Printf("  THROTTLED — nothing measured (lower -concurrency and re-run): %s\n", truncErr(errors.New(r.Error)))
		return
	}
	if r.Error != "" {
		fmt.Printf("  FAILED — %s\n", r.Error)
		return
	}
	fmt.Printf("%-14s  %8s  %8s  %8s\n", "dimension", "bare", "steered", "lift")
	for _, d := range r.Dimensions {
		lift := "—"
		if l, ok := d.Lift(); ok {
			lift = fmt.Sprintf("%+.1fpp", l)
		}
		fmt.Printf("%-14s  %8s  %8s  %8s\n", d.Dimension, fmtRate(d.Bare), fmtRate(d.Steered), lift)
	}
	overallBare, overallSteered := poolDimensions(r.Dimensions)
	if overallBare.Scored > 0 && overallSteered.Scored > 0 {
		lift := overallSteered.Rate() - overallBare.Rate()
		fmt.Printf("%-14s  %8s  %8s  %+7.1fpp", "overall", fmtRate(overallBare), fmtRate(overallSteered), lift)
		if r.ContextTokens > 0 {
			fmt.Printf("   (%d context tokens/pass → %+.1fpp per 1k)", r.ContextTokens, lift/(float64(r.ContextTokens)/1000))
		}
		fmt.Println()
	}
	for _, pass := range []struct {
		name string
		rec  *PassRecord
	}{{"bare", r.Bare}, {"steered", r.Steered}} {
		if pass.rec == nil {
			continue
		}
		if pass.rec.Missing > 0 || pass.rec.Untranslated > 0 {
			fmt.Printf("  %s: %d missing, %d untranslated — excluded from adherence, an echo trivially passes DNT\n",
				pass.name, pass.rec.Missing, pass.rec.Untranslated)
		}
	}
	if j := r.Judge; j != nil {
		switch {
		case j.SkippedSameFamily:
			fmt.Printf("  judge: skipped — %s:%s is the same model family as the model under test\n", j.Provider, j.Model)
		case j.SkippedSameLanguage:
			fmt.Printf("  judge: skipped — a same-language target's register grades the source, not the adaptation\n")
		case j.Error != "":
			fmt.Printf("  judge: error — %s\n", j.Error)
		case j.Bare.Scored == 0 && j.Steered.Scored == 0:
			fmt.Println("  judge: nothing judged (simulated runs are never judged)")
		default:
			fmt.Printf("  judged voice (unpublished until agreement is validated): bare %.1f%%  steered %.1f%%\n",
				j.Bare.Rate(), j.Steered.Rate())
		}
	}
}

// fmtRate renders an adherence rate, keeping the "nothing was scored" sentinel
// out of print: a dash, never a number that reads as measured.
func fmtRate(c Counts) string {
	if c.Scored == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", c.Rate())
}

func poolDimensions(dims []DimensionRecord) (bare, steered Counts) {
	for _, d := range dims {
		bare.Scored += d.Bare.Scored
		bare.Passed += d.Bare.Passed
		steered.Scored += d.Steered.Scored
		steered.Passed += d.Steered.Passed
	}
	return bare, steered
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
