package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

// The history file exists because steerability is model-specific and
// time-specific: lift measured against `sonnet` today says nothing about the
// weights behind that alias in six months. Every run is stamped with the date,
// the provider, the resolved model, the target, and a digest of the exact
// experiment (fixtures + expectations + injected context). Two runs are only
// comparable when the digest matches; the dashboard refuses to plot mismatched
// runs as one trend.

// History is the committed record behind the /context-eval dashboard.
type History struct {
	Runs []Run `json:"runs"`
	// JudgeValidations records measured judge–human agreement per (judge model,
	// rubric). The dashboard publishes a judged dimension only when a validation
	// meets the agreement bar — reporting an unvalidated judge's opinion as a
	// model's behavior is the failure mode adherence evals are known for.
	JudgeValidations []JudgeValidation `json:"judge_validations,omitempty"`
}

// Run is one model's with/without-context measurement, on one day, over one
// target and one experiment digest.
type Run struct {
	Date     string `json:"date"` // YYYY-MM-DD
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Target   string `json:"target"`
	Repeat   int    `json:"repeat"`
	// Concurrency is recorded because throughput depends on it; adherence does
	// not, but the run should be reproducible as swept.
	Concurrency int `json:"concurrency,omitempty"`
	// Temperature is what the sweep sampled at. A pointer so an entry made
	// before the field existed reads as unknown rather than as zero, which
	// would be a claim about a run nobody recorded.
	Temperature *float64 `json:"temperature,omitempty"`

	// Price is what the tokens were charged at, pinned at measurement time —
	// re-pricing history at today's rates would silently rewrite what a past
	// run cost.
	Price *Price `json:"price,omitempty"`

	Corpus         string `json:"corpus"`
	CorpusFixtures int    `json:"corpus_fixtures,omitempty"`
	CorpusChecks   int    `json:"corpus_checks,omitempty"`
	CorpusWords    int    `json:"corpus_words,omitempty"`
	// CorpusDigest covers the fixtures, their expectations, and the injected
	// context. Change any of them and runs stop being comparable — the digest
	// says so.
	CorpusDigest string `json:"corpus_digest"`

	// ContextTokens is the measured price of the injected context: the steered
	// pass's input tokens minus the bare pass's, averaged per corpus pass. It is
	// the denominator of lift-per-1k-context-tokens — "is the brand guide
	// earning the tokens it costs on this model?".
	ContextTokens int `json:"context_tokens,omitempty"`

	// Simulated marks a run against the offline demo stub. It exercises the
	// harness and measures nothing about any model; it must never reach a chart.
	Simulated bool `json:"simulated,omitempty"`

	Bare    *PassRecord `json:"bare,omitempty"`
	Steered *PassRecord `json:"steered,omitempty"`

	Dimensions []DimensionRecord `json:"dimensions,omitempty"`
	Kinds      []KindRecord      `json:"kinds,omitempty"`

	Judge *JudgeRecord `json:"judge,omitempty"`

	// Unmeasured marks a run that never got a fair hearing — the provider
	// throttled it after retries. Nothing about the model was learned; the
	// truthful record is a labelled hole, never a 0% adherence.
	Unmeasured bool   `json:"unmeasured,omitempty"`
	Error      string `json:"error,omitempty"`
}

// PassRecord is one variant pass (bare or steered): what it cost and the
// structural guards. Counts are summed across repeats, batcheval-style —
// averaging integer counts would round "one repeat in three echoed a segment"
// down to zero.
type PassRecord struct {
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int     `json:"cache_read_tokens,omitempty"`
	Elapsed             float64 `json:"elapsed_seconds"`
	// CostUSD prices the pass at the run's pinned rates. Zero when the model is
	// unpriced or unmetered — a blank, never a guess.
	CostUSD float64 `json:"cost_usd,omitempty"`
	// Missing/Untranslated are excluded from adherence and reported separately:
	// an echoed segment trivially "preserves" every DNT term, and counting it
	// would inflate exactly the number being measured.
	Missing      int `json:"missing,omitempty"`
	Untranslated int `json:"untranslated,omitempty"`
}

// DimensionRecord is the primary result: one dimension, both variants.
// Adherence is Passed/Scored per variant; lift is the steered rate minus the
// bare rate. Dimensions are never collapsed into one number in the record — a
// model can be excellent at terminology and poor at voice, and a single score
// would hide the thing a reader would act on.
type DimensionRecord struct {
	Dimension string `json:"dimension"`
	Bare      Counts `json:"bare"`
	Steered   Counts `json:"steered"`
}

// Lift is the decision-relevant number: how much the injected context moved
// the model, in percentage points. A model with high absolute adherence but
// near-zero lift already "knew" it — that is baseline capability, not
// context-following. Returns false when either side is unscored.
func (d DimensionRecord) Lift() (float64, bool) {
	b, s := d.Bare.Rate(), d.Steered.Rate()
	if b < 0 || s < 0 {
		return 0, false
	}
	return s - b, true
}

// KindRecord is the per-trap-kind breakdown inside a dimension, for the
// dashboard drill-down.
type KindRecord struct {
	Dimension string `json:"dimension"`
	Kind      string `json:"kind"`
	Bare      Counts `json:"bare"`
	Steered   Counts `json:"steered"`
}

// JudgeRecord is the judged (subjective) voice slice of one run. It is stored
// regardless of validation state; the dashboard decides what may be published.
type JudgeRecord struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	RubricDigest string `json:"rubric_digest"`
	Criteria     int    `json:"criteria"`
	Bare         Counts `json:"bare"`
	Steered      Counts `json:"steered"`
	// SkippedSameFamily records that no judging happened because the judge and
	// the model under test share a model family — self-preference bias is real,
	// so a same-family verdict is not evidence.
	SkippedSameFamily bool `json:"skipped_same_family,omitempty"`
	// SkippedSameLanguage records that this target is not judgeable: on a
	// same-language target (en → en-GB) a register/tone verdict grades the
	// source, not the adaptation.
	SkippedSameLanguage bool   `json:"skipped_same_language,omitempty"`
	Error               string `json:"error,omitempty"`
}

// JudgeValidation is measured judge–human agreement over a labeled seed set.
type JudgeValidation struct {
	Date         string  `json:"date"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	RubricDigest string  `json:"rubric_digest"`
	Items        int     `json:"items"`
	Agreement    float64 `json:"agreement"` // raw fraction of matching verdicts
	Kappa        float64 `json:"kappa"`     // Cohen's kappa (chance-corrected)
	LabelsDigest string  `json:"labels_digest,omitempty"`
	// Targets records which locales the human labels covered. The labeler can
	// only audit languages they read (here: en, nb), so a validation's scope is
	// part of the finding — a judge validated on en-GB and nb is being *trusted*
	// on de and fr, and the dashboard says so rather than implying otherwise.
	Targets []string `json:"targets,omitempty"`
}

// key identifies a run. The corpus digest is part of it: the same model swept
// on the same day over a changed corpus is a different experiment, not a
// correction of the first.
func (r Run) key() string {
	return strings.Join([]string{r.Date, r.Provider, r.Model, r.Target, r.CorpusDigest}, "|")
}

func (v JudgeValidation) key() string {
	return strings.Join([]string{v.Provider, v.Model, v.RubricDigest}, "|")
}

// LoadHistory reads the committed record. A missing file is an empty history —
// the first run has to be able to create it.
func LoadHistory(path string) (History, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return History{}, nil
	}
	if err != nil {
		return History{}, err
	}
	var h History
	if err := json.Unmarshal(b, &h); err != nil {
		return History{}, fmt.Errorf("%s: %w", path, err)
	}
	return h, nil
}

// Upsert replaces a same-day run for the same model, target and corpus, and
// appends otherwise — a same-day re-run corrects its entry rather than
// accumulating duplicate points that would read as a trend.
func (h *History) Upsert(runs ...Run) {
	for _, run := range runs {
		if i := slices.IndexFunc(h.Runs, func(e Run) bool { return e.key() == run.key() }); i >= 0 {
			h.Runs[i] = run
			continue
		}
		h.Runs = append(h.Runs, run)
	}
	slices.SortStableFunc(h.Runs, func(a, b Run) int {
		if c := strings.Compare(a.Date, b.Date); c != 0 {
			return c
		}
		if c := strings.Compare(a.Target, b.Target); c != 0 {
			return c
		}
		if c := strings.Compare(a.Provider, b.Provider); c != 0 {
			return c
		}
		return strings.Compare(a.Model, b.Model)
	})
}

// UpsertValidation replaces the recorded agreement for the same judge and
// rubric — a re-validation supersedes, it does not accumulate.
func (h *History) UpsertValidation(v JudgeValidation) {
	if i := slices.IndexFunc(h.JudgeValidations, func(e JudgeValidation) bool { return e.key() == v.key() }); i >= 0 {
		h.JudgeValidations[i] = v
		return
	}
	h.JudgeValidations = append(h.JudgeValidations, v)
	slices.SortStableFunc(h.JudgeValidations, func(a, b JudgeValidation) int {
		return cmp.Or(
			strings.Compare(a.Provider, b.Provider),
			strings.Compare(a.Model, b.Model),
			strings.Compare(a.RubricDigest, b.RubricDigest),
		)
	})
}

// recostHistory re-prices every run's passes from the committed price table
// without calling a model — cost is tokens already measured times a published
// rate. Unpriced models are named, not silently skipped: a blank nobody was
// told about looks like zero.
func recostHistory(path string) error {
	h, err := LoadHistory(path)
	if err != nil {
		return err
	}
	priced, unpriced := 0, []string{}
	for i := range h.Runs {
		r := &h.Runs[i]
		p, ok := priceFor(r.Provider, r.Model)
		if !ok {
			unpriced = append(unpriced, r.Provider+":"+r.Model)
			continue
		}
		r.Price = &p
		for _, pass := range []*PassRecord{r.Bare, r.Steered} {
			if pass == nil {
				continue
			}
			pass.CostUSD = 0
			if p.Metered {
				pass.CostUSD = p.cost(pass.InputTokens, pass.OutputTokens)
			}
		}
		priced++
	}
	if err := h.Save(path); err != nil {
		return err
	}
	fmt.Printf("re-priced %d/%d runs at rates as of %s → %s\n", priced, len(h.Runs), prices.AsOf, path)
	for _, u := range unpriced {
		fmt.Printf("  no price for %s — add it to scripts/batcheval/prices.json and run make update-model-prices\n", u)
	}
	return nil
}

func (h History) Save(path string) error {
	// A nil slice marshals as `"runs": null`, which a -judge-validate against a
	// fresh file would write — and null is not an array to the dashboard.
	if h.Runs == nil {
		h.Runs = []Run{}
	}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
