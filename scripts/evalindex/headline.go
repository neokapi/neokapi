package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The one number each eval is for, read out of its own dataset.
//
// The index listed a status and a paragraph, so a reader wanting to know how
// parity was doing had to open a card, read two sentences, follow a link and
// find the number on another page. Status is not a result: "partial" is true of
// an eval at 96% and of one at 40%.
//
// Typing the number into the registry beside the prose was the obvious fix and
// the wrong one — it is the same mistake as a hand-written date, and it goes
// wrong the same way, silently and in the direction that flatters. So each
// headline is a function of the dataset, extracted here, and an eval whose
// dataset changes shape loses its headline loudly rather than keeping an old
// one.

// Headline is the number a row shows before anyone clicks.
type Headline struct {
	// Value is the number, formatted, e.g. "96.4%" or "823/844".
	Value string `json:"value"`
	// Of says what it is a number of, in three or four words.
	Of string `json:"of"`
	// Tone is how it reads: ok, warn, or gap. Judged against the bar the eval
	// itself sets, not against a shared threshold — 90% is good for a check's
	// recall and poor for round-trip fidelity.
	Tone string `json:"tone"`
}

// extractors maps an eval id to the reader for its dataset.
//
// A map rather than a method on the card, because the datasets were written by
// different harnesses at different times and share no shape. What they share is
// that each one holds its own answer.
var extractors = map[string]func(doc any) *Headline{
	"authoring-checks":      headlineAuthoringChecks,
	"authoring-effect":      headlineAuthoringEffect,
	"conversion-comparison": headlineConversion,
	"skill-triggering":      headlineSkillMode("skill:trigger", "scenarios fired correctly"),
	"skill-completion":      headlineSkillMode("skill:completion", "scenarios reached a green gate"),
	"mcp-surface":           headlineSkillMode("mcp:trigger", "MCP scenarios picked the right tool"),
	"context-value":         headlineContext,
	"check-accuracy":        headlineCheckAccuracy,
	"format-maturity":       headlineMaturity,
	"engine-speed":          headlineEngineSpeed,
	"reuse-effect":          headlineReuseEffect,
	"batching-cost":         headlineBatching,
	"reuse-rules":           headlineReuseRules,
	"prompt-contents":       headlinePromptContents,
}

// readHeadline pulls the number out of a dataset, or returns nil.
func readHeadline(root, rel, id string) *Headline {
	fn, ok := extractors[id]
	if !ok || rel == "" {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil
	}
	var doc any
	if json.Unmarshal(body, &doc) != nil {
		return nil
	}
	return fn(doc)
}

// --- per-eval readers -------------------------------------------------------

func headlineAuthoringChecks(doc any) *Headline {
	checks, ok := dig(doc, "checks").([]any)
	if !ok || len(checks) == 0 {
		return nil
	}
	// The offline check is the one with numbers; the LLM arm is blocked.
	for _, c := range checks {
		if str(dig(c, "blocked")) != "" {
			continue
		}
		r, ok := num(dig(c, "recall"))
		if !ok {
			continue
		}
		return &Headline{
			Value: fmt.Sprintf("%.0f%%", 100*r),
			Of:    "of marked violations found",
			Tone:  band(r, 0.95, 0.8),
		}
	}
	return nil
}

func headlineAuthoringEffect(doc any) *Headline {
	steer := dig(doc, "steer")
	if steer == nil || str(dig(steer, "blocked")) != "" {
		return nil
	}
	e, ok := num(dig(steer, "effect"))
	if !ok {
		return nil
	}
	// Positive is the whole claim: the guide moved writing toward its own
	// profile and not toward the other one. Zero would mean generic polish.
	return &Headline{
		Value: fmt.Sprintf("%+.1f", e),
		Of:    "points of profile-specific steering",
		Tone:  band(e, 5, 0),
	}
}

// headlineConversion reports kapi's weakest format, not its corpus-wide score.
//
// The corpus-wide number is 99.9%, and it is dominated by spreadsheets: .xlsx
// contributes 3.27M of the 3.56M ground-truth words, so a headline computed
// over everything is very nearly a headline about .xlsx alone. The lowest
// per-format figure cannot oversell, and the per-format table is one click
// away.
func headlineConversion(doc any) *Headline {
	convs, ok := dig(doc, "converters").([]any)
	if !ok {
		return nil
	}
	for _, c := range convs {
		if str(dig(c, "id")) != "kapi" {
			continue
		}
		byExt, ok := dig(c, "byExt").(map[string]any)
		if !ok || len(byExt) == 0 {
			return nil
		}
		worst, worstExt := 2.0, ""
		for ext, e := range byExt {
			r, ok := num(dig(e, "recall"))
			if ok && r < worst {
				worst, worstExt = r, ext
			}
		}
		if worstExt == "" {
			return nil
		}
		return &Headline{
			Value: fmt.Sprintf("%.1f%%", 100*worst),
			Of:    "of text kept, in its weakest format (" + worstExt + ")",
			Tone:  band(worst, 0.99, 0.95),
		}
	}
	return nil
}

// headlineSkillMode reads one mode out of the skill dataset, which holds all
// three under keys like "skill:trigger". Three cards read the same file and
// each wants a different one of them.
func headlineSkillMode(key, of string) func(any) *Headline {
	return func(doc any) *Headline {
		sum := dig(doc, key, "summary")
		pass, okp := num(dig(sum, "pass"))
		total, okt := num(dig(sum, "scenarios"))
		if !okp || !okt || total == 0 {
			return nil
		}
		return &Headline{
			Value: fmt.Sprintf("%d/%d", int(pass), int(total)),
			Of:    of,
			Tone:  band(pass/total, 1, 0.85),
		}
	}
}

// headlineCheckAccuracy reads the pooled F1 over every labelled case.
func headlineCheckAccuracy(doc any) *Headline {
	f1, ok := num(dig(doc, "total", "f1"))
	if !ok {
		return nil
	}
	cases, _ := num(dig(doc, "total", "cases"))
	return &Headline{
		Value: fmt.Sprintf("%.2f", f1),
		Of:    fmt.Sprintf("F1 over %d labelled cases", int(cases)),
		Tone:  band(f1, 0.9, 0.75),
	}
}

// headlineMaturity reports how many formats have reached the target level.
//
// Levels are strings — "L1", "L4" — not numbers. Reading them as numbers gave
// every format level 0 against target 0 and produced "0/36 formats at L0 or
// above", which is both wrong and impossible. Hence rung().
func headlineMaturity(doc any) *Headline {
	target, ok := rung(str(dig(doc, "target_level")))
	if !ok {
		return nil
	}
	formats, ok2 := dig(doc, "formats").([]any)
	if !ok2 || len(formats) == 0 {
		return nil
	}
	at := 0
	for _, f := range formats {
		if lvl, ok := rung(str(dig(f, "level"))); ok && lvl >= target {
			at++
		}
	}
	return &Headline{
		Value: fmt.Sprintf("%d/%d", at, len(formats)),
		Of:    fmt.Sprintf("formats at L%d", target),
		Tone:  band(float64(at)/float64(len(formats)), 0.9, 0.5),
	}
}

// rung parses a maturity level such as "L3".
func rung(s string) (int, bool) {
	var n int
	if _, err := fmt.Sscanf(s, "L%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

// headlineEngineSpeed compares kapi against the reference engine, on the files
// both engines read.
//
// Not the two total wall times, which is what this did first. The engines do
// not succeed on the same files — 725 against 802 in the committed run — so
// dividing the totals compares one engine's corpus against another's, and the
// difference in coverage lands in the speed number where nobody would look for
// it. Summing only the shared successes asks one question at a time.
//
// The two numbers happen to be close here (22.7× against 23.8×), which is
// exactly why the wrong one is worth avoiding: it was not wrong enough to
// notice, and there is no reason the next run would be as kind.
//
// Speed is not the whole story and the card says so: on this corpus kapi is
// faster and extracts less text than the reference.
func headlineEngineSpeed(doc any) *Headline {
	exps, ok := dig(doc, "experiments").([]any)
	if !ok {
		return nil
	}
	native, okapi := succeededTimings(exps, "kapi-native"), succeededTimings(exps, "okapi")
	if len(native) == 0 || len(okapi) == 0 {
		return nil
	}
	var tn, to float64
	shared := 0
	for name, ms := range native {
		om, ok := okapi[name]
		if !ok {
			continue
		}
		shared++
		tn += ms
		to += om
	}
	if shared == 0 || tn == 0 {
		return nil
	}
	return &Headline{
		Value: fmt.Sprintf("%.1f×", to/tn),
		Of:    fmt.Sprintf("faster on the %d files both engines read", shared),
		Tone:  band(to/tn, 2, 1),
	}
}

// succeededTimings is one engine's per-file wall time, for the files it read.
func succeededTimings(exps []any, engine string) map[string]float64 {
	for _, e := range exps {
		if str(dig(e, "engine")) != engine {
			continue
		}
		timings, ok := dig(e, "fileTimings").([]any)
		if !ok {
			return nil
		}
		out := make(map[string]float64, len(timings))
		for _, t := range timings {
			if ok, _ := dig(t, "success").(bool); !ok {
				continue
			}
			ms, ok := num(dig(t, "wallMs"))
			if !ok {
				continue
			}
			out[str(dig(t, "name"))] = ms
		}
		return out
	}
	return nil
}

// headlineReuseEffect reports how often a judge preferred the governed output.
func headlineReuseEffect(doc any) *Headline {
	with, okw := num(dig(doc, "judgeWith"))
	without, oko := num(dig(doc, "judgeWithout"))
	tie, _ := num(dig(doc, "judgeTie"))
	if !okw || !oko {
		return nil
	}
	total := with + without + tie
	if total == 0 {
		return nil
	}
	// The caveat travels with the number. Six samples scored by a judge whose
	// agreement with a person has never been measured is not a finding, and
	// "0/6" on its own reads like one.
	of := fmt.Sprintf("judged better with context, n=%d", int(total))
	if validated, _ := dig(doc, "judgeValidated").(bool); !validated {
		of += ", judge unvalidated"
	}
	return &Headline{
		Value: fmt.Sprintf("%d/%d", int(with), int(total)),
		Of:    of,
		Tone:  band(with/total, 0.7, 0.5),
	}
}

// headlineContext sums the per-dimension pass counts of the newest run.
//
// `bare` and `steered` at the top of a run hold token counts and elapsed time,
// not scores — the scores are under `dimensions`, one entry per rubric axis
// with scored/passed on each side. Reading the wrong pair produced no headline
// at all, which is the failure mode this shape is meant to have.
func headlineContext(doc any) *Headline {
	runs, ok := dig(doc, "runs").([]any)
	if !ok || len(runs) == 0 {
		return nil
	}
	dims, ok := dig(runs[len(runs)-1], "dimensions").([]any)
	if !ok || len(dims) == 0 {
		return nil
	}
	var bareP, bareN, steerP, steerN float64
	for _, d := range dims {
		bp, _ := num(dig(d, "bare", "passed"))
		bn, _ := num(dig(d, "bare", "scored"))
		sp, _ := num(dig(d, "steered", "passed"))
		sn, _ := num(dig(d, "steered", "scored"))
		bareP, bareN, steerP, steerN = bareP+bp, bareN+bn, steerP+sp, steerN+sn
	}
	if bareN == 0 || steerN == 0 {
		return nil
	}
	lift := 100*(steerP/steerN) - 100*(bareP/bareN)
	return &Headline{
		Value: fmt.Sprintf("%+.0fpp", lift),
		Of:    "adherence, context against none",
		Tone:  band(lift, 20, 5),
	}
}

// headlineBatching reports the largest batch size that came back intact.
//
// The question batching poses is not how fast it is but where it breaks: a
// block that never returns, a placeholder that does not survive, a segment that
// comes back untranslated. So the headline is the biggest N with none of those.
func headlineBatching(doc any) *Headline {
	runs, ok := dig(doc, "runs").([]any)
	if !ok || len(runs) == 0 {
		return nil
	}
	results, ok := dig(runs[len(runs)-1], "results").([]any)
	if !ok || len(results) == 0 {
		return nil
	}
	best, tried := 0.0, 0.0
	for _, r := range results {
		n, ok := num(dig(r, "n"))
		if !ok {
			continue
		}
		tried = maxf(tried, n)
		broken := false
		for _, k := range []string{"missing", "placeholder_breaks", "tag_breaks", "untranslated"} {
			if v, ok := num(dig(r, k)); ok && v > 0 {
				broken = true
			}
		}
		if !broken {
			best = maxf(best, n)
		}
	}
	if tried == 0 {
		return nil
	}
	return &Headline{
		Value: fmt.Sprintf("%d", int(best)),
		Of:    fmt.Sprintf("blocks per call still intact, of %d tried", int(tried)),
		Tone:  band(best/tried, 1, 0.5),
	}
}

// headlineReuseRules reports how many governed points the recipe resolves.
func headlineReuseRules(doc any) *Headline {
	points, ok := dig(doc, "points").([]any)
	if !ok || len(points) == 0 {
		return nil
	}
	chains, _ := dig(doc, "chains").([]any)
	return &Headline{
		Value: fmt.Sprintf("%d", len(points)),
		Of:    fmt.Sprintf("governed points resolved, %d reuse chains traced", len(chains)),
		Tone:  "ok",
	}
}

// headlinePromptContents reports how many prompts were captured and checked.
//
// This eval reads what kapi sent rather than what came back, so its number is a
// count of prompts inspected. Nothing here is a quality score, and the card says
// so.
func headlinePromptContents(doc any) *Headline {
	// `cases` is the list of them, not a count. Reading it as a number returned
	// nothing, which TestARegisteredHeadlineResolves caught — the point of that
	// test being that a nil headline is invisible on the page.
	cases, ok := dig(doc, "cases").([]any)
	if !ok || len(cases) == 0 {
		return nil
	}
	return &Headline{
		Value: fmt.Sprintf("%d", len(cases)),
		Of:    "prompts captured and inspected",
		Tone:  "ok",
	}
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// --- helpers ----------------------------------------------------------------

// dig walks a decoded JSON document by key.
func dig(doc any, keys ...string) any {
	cur := doc
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

func num(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// band picks a tone from two thresholds, so each eval judges its own number
// against its own bar.
func band(v, good, fair float64) string {
	switch {
	case v >= good:
		return "ok"
	case v >= fair:
		return "warn"
	default:
		return "gap"
	}
}
