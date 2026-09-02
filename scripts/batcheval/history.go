package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

// The point of a history file is that a batch ceiling chosen today does not
// quietly become folklore. Models and APIs move; a curve measured against
// `sonnet` in July 2026 says nothing about the model that answers to the same
// alias six months later. So every run is stamped with the date, the provider,
// the resolved model, and — crucially — a digest of the corpus it was measured
// on. Two runs are only comparable if that digest matches; a dashboard that
// silently plots them together would be inventing a trend.

// History is the committed record: one entry per (date, provider, model, target,
// corpus).
type History struct {
	Runs []Run `json:"runs"`
}

// Run is one model's sweep, on one day, over one corpus.
type Run struct {
	Date     string `json:"date"` // YYYY-MM-DD
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Target   string `json:"target"`
	Repeat   int    `json:"repeat"`
	// Concurrency is recorded because throughput without it is meaningless: words
	// per second at four concurrent calls is not the same claim as words per second
	// at one, and two models swept at different concurrencies cannot be raced
	// against each other on speed.
	Concurrency int `json:"concurrency,omitempty"`
	// Temperature is what the sweep sampled at. Recorded for the same reason as
	// concurrency: a command alone does not reproduce a sampled number, and
	// until this was pinned the honest answer was "whatever the API defaults
	// to", which nobody wrote down. A pointer so an older entry, made before
	// the field existed, reads as unknown rather than as zero.
	Temperature *float64 `json:"temperature,omitempty"`

	// Price is what the tokens were charged at, pinned at measurement time. Looking
	// the rate up at render time would re-price history whenever a vendor changed
	// its list, silently rewriting what a past run cost.
	Price *Price `json:"price,omitempty"`

	Corpus       string `json:"corpus"`
	CorpusWords  int    `json:"corpus_words,omitempty"`
	CorpusBlocks int    `json:"corpus_blocks,omitempty"`
	// CorpusDigest is what makes the history honest. Change the corpus and the
	// digest moves, and runs measured on the old one stop being comparable —
	// the dashboard says so rather than drawing a line through both.
	CorpusDigest string `json:"corpus_digest"`

	// Simulated marks a run against the offline stub. It measures the harness and
	// nothing else, and must never be plotted as a model's curve.
	Simulated bool `json:"simulated,omitempty"`

	Results []Result `json:"results"`
}

// key identifies a run. The corpus digest is part of it, and has to be: sweeping
// the same model on the same day over a 30-block corpus and over a 600-block one
// is two experiments, not one measurement taken twice. Keying without the digest
// made the second sweep silently overwrite the first — which is precisely the
// "different corpora are not comparable" rule this file exists to enforce, broken
// by the file itself.
func (r Run) key() string {
	return strings.Join([]string{r.Date, r.Provider, r.Model, r.Target, r.CorpusDigest}, "|")
}

// LoadHistory reads the committed record. A missing file is an empty history,
// not an error — the first run has to be able to create it.
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
// appends otherwise. Re-running a sweep on the same day corrects it; it does not
// accumulate duplicate points that would read as a trend.
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
		if c := cmp.Compare(b.CorpusBlocks, a.CorpusBlocks); c != 0 {
			return c // bigger corpus first: it is the more informative experiment
		}
		if c := strings.Compare(a.Provider, b.Provider); c != 0 {
			return c
		}
		return strings.Compare(a.Model, b.Model)
	})
}

// recostHistory fills in words and cost for every run from the committed price
// table, without calling a model: the tokens were measured when the run happened,
// and cost is just tokens times a published rate.
//
// It re-prices *every* run at the table's current rates, which is right when the
// table is being corrected (a rate we got wrong, or a cost column added after the
// fact) and wrong if a vendor has since changed its list — that would restate what
// a past run cost. The date stamped on each run's price says which of the two you
// are looking at.
func recostHistory(path string) error {
	h, err := LoadHistory(path)
	if err != nil {
		return err
	}
	corpus := Corpus()

	priced, unpriced := 0, []string{}
	for i := range h.Runs {
		r := &h.Runs[i]
		if r.CorpusWords == 0 && r.CorpusDigest == corpus.Digest() {
			r.CorpusWords = corpus.Words()
		}
		if r.CorpusBlocks == 0 && r.CorpusDigest == corpus.Digest() {
			r.CorpusBlocks = len(corpus.Cases)
		}
		p, ok := priceFor(r.Provider, r.Model)
		if !ok {
			unpriced = append(unpriced, r.Provider+":"+r.Model)
			continue
		}
		r.Price = &p
		for j := range r.Results {
			r.Results[j] = r.price(r.Results[j])
		}
		priced++
	}

	if err := h.Save(path); err != nil {
		return err
	}
	fmt.Printf("re-priced %d/%d runs at rates as of %s → %s\n", priced, len(h.Runs), prices.AsOf, path)
	for _, u := range unpriced {
		// Named, not silently skipped: a model missing from the price table shows a
		// blank on the dashboard, and a blank nobody was told about looks like zero.
		fmt.Printf("  no price for %s. Add it to scripts/batcheval/prices.json\n", u)
	}
	return nil
}

func (h History) Save(path string) error {
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
