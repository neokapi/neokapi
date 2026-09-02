package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The subjective remainder of voice — register, naturalness, restraint — cannot
// be scored by a regex, and it is exactly where adherence evals rot: naive
// LLM-judging measures the judge, not the model under test. The rules here are
// non-negotiable:
//
//   - the rubric is fixed yes/no criteria, not a free-floating 1–10;
//   - the judge is a *different model family* than the model under test
//     (self-preference bias is real and measurable), enforced, not advised;
//   - the judge is blind — it sees the source, the translation and the rubric,
//     never the model name or whether the text came from the bare or steered
//     pass;
//   - the judged dimension is unpublishable until judge–human agreement has
//     been measured on a labeled seed set and meets the bar below. The gate is
//     data in the history file, not a promise in prose.

// Agreement bar a judge must clear before the dashboard publishes its scores.
// Cohen's kappa, because raw agreement flatters a judge that says "yes" to
// everything; the item floor, because kappa over a handful of labels is noise.
const (
	MinJudgeKappa = 0.6
	// MinJudgeItems is the floor below which kappa is noise rather than a
	// measurement. Thirty was optimistic: the reliability literature puts the
	// useful floor nearer 100, and a kappa from 30 items carries a confidence
	// interval wide enough to span "substantial" and "poor".
	MinJudgeItems = 100
	// TargetJudgeItems is what a labelling session aims for. The interval at
	// 150 is tight enough that the number can be quoted without a caveat
	// longer than the number.
	TargetJudgeItems = 150
)

// Criterion is one yes/no rubric question, derived from the corpus's synthetic
// brand guide. These cover only what the deterministic checks cannot: the
// mechanical rules (forbidden terms, contractions, exclamation marks) are
// scored by the check tools and deliberately excluded here.
type Criterion struct {
	ID   string
	Text string
}

func rubric() []Criterion {
	return []Criterion{
		{ID: "register", Text: "The translation addresses a professional audience in a formal, respectful register, neither chatty nor bureaucratic."},
		{ID: "naturalness", Text: "The translation reads as natural, idiomatic text in the target language, not as a word-for-word rendering of the English."},
		{ID: "restraint", Text: "The tone is calm and precise, with no marketing enthusiasm, exaggeration, or added emphasis that the source did not have."},
	}
}

// judgePromptVersion is folded into the rubric digest: rewording the prompt
// changes what the judge is being asked, which makes past agreement
// measurements void.
const judgePromptVersion = "v2" // v2: house vocabulary included so obedience is not judged unnatural

// rubricDigest identifies the exact rubric + prompt a judgement or a
// validation was made under. Agreement measured under one digest says nothing
// about scores produced under another.
func rubricDigest() string {
	h := sha256.New()
	fmt.Fprintf(h, "prompt:%s\x00", judgePromptVersion)
	for _, c := range rubric() {
		fmt.Fprintf(h, "%s:%s\x00", c.ID, c.Text)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

type judgeTarget struct {
	provider, model string
	llm             aiprovider.LLMProvider
}

func newJudge(spec string) (*judgeTarget, error) {
	provider, mdl, _ := strings.Cut(spec, ":")
	if provider == "" {
		return nil, fmt.Errorf("contexteval: bad -judge %q (want provider:model)", spec)
	}
	mt := modelTarget{provider: provider, model: mdl}
	llm, err := mt.newLLM()
	if err != nil {
		return nil, fmt.Errorf("contexteval: judge: %w", err)
	}
	return &judgeTarget{provider: provider, model: mdl, llm: llm}, nil
}

func (j *judgeTarget) Close() {
	if j != nil && j.llm != nil {
		j.llm.Close()
	}
}

// record initializes the judge slice of a run, enforcing the cross-family
// rule: a same-family pairing is recorded as skipped, never quietly judged.
func (j *judgeTarget) record(mt modelTarget) *JudgeRecord {
	rec := &JudgeRecord{
		Provider:     j.provider,
		Model:        j.model,
		RubricDigest: rubricDigest(),
		Criteria:     len(rubric()),
	}
	jf := modelFamily(j.provider, j.model)
	tf := modelFamily(mt.provider, mt.model)
	if jf != "" && jf == tf {
		rec.SkippedSameFamily = true
	}
	return rec
}

// modelFamily maps a provider/model pair to the family that trained the
// weights — which is what self-preference follows. The model id decides first,
// because Bedrock serves Anthropic models and azureopenai serves OpenAI's; the
// provider is only a fallback.
func modelFamily(provider, mdl string) string {
	m := strings.ToLower(mdl)
	switch {
	case strings.Contains(m, "claude") || m == "sonnet" || m == "opus" || m == "haiku" || m == "fable":
		return "anthropic"
	case strings.HasPrefix(m, "gpt") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4"):
		return "openai"
	case strings.Contains(m, "gemini") || strings.Contains(m, "gemma"):
		return "google"
	case strings.Contains(m, "llama"):
		return "meta"
	case strings.Contains(m, "mistral") || strings.Contains(m, "ministral"):
		return "mistral"
	}
	switch aiprovider.ProviderID(strings.ToLower(provider)) {
	case aiprovider.Anthropic, aiprovider.ClaudeCode:
		return "anthropic"
	case aiprovider.OpenAI, aiprovider.AzureOpenAI:
		return "openai"
	case aiprovider.Gemini:
		return "google"
	}
	return ""
}

// judgeableTarget reports whether the judged voice criteria mean anything for
// this target. On a same-language target (en → en-GB) the register and tone of
// a candidate are dominated by the source's own register passing through —
// production settles source voice in the authoring-side checks before anything
// is translated — so a judged verdict there would grade the source, not the
// adaptation. en-GB stays in the eval where it genuinely measures adaptation
// obedience: the deterministic spelling/contraction/terminology checks.
func judgeableTarget(target string) bool {
	return baseLang(target) != baseLang(string(model.LocaleEnglish))
}

// judgePass scores every answered fixture of one variant pass against the
// rubric, accumulating per-criterion verdicts. The answered-fixture rule is
// corpus.answerStateOf — the same one the deterministic scorer applies — so
// the two can never compute their rates over different fixture sets.
func (j *judgeTarget) judgePass(ctx context.Context, corpus TestCorpus, blocks []*model.Block, counts *Counts) error {
	loc := model.LocaleID(corpus.Target)
	for i, f := range corpus.Fixtures {
		got := blocks[i].TargetText(loc)
		if corpus.answerStateOf(f, got) != answered {
			continue
		}
		verdicts, err := j.judgeOne(ctx, corpus.Target, f.Source, got)
		if err != nil {
			return err
		}
		for _, c := range rubric() {
			pass, ok := verdicts[c.ID]
			if !ok {
				return fmt.Errorf("judge returned no verdict for criterion %q", c.ID)
			}
			counts.add(pass)
		}
	}
	return nil
}

// judgeOne asks the judge for a yes/no per criterion, blind: the prompt names
// the target language, the source, the candidate translation and the rubric —
// never the producing model, and never which variant the text came from.
//
// The house vocabulary IS included. The mandates are deliberately non-naive
// (that is what makes obedience measurable), so a judge who does not know them
// penalizes obedience as unnaturalness; terminology itself is owned by the
// deterministic checks and explicitly excluded from the rubric's scope. The
// list is constant per target, so it identifies no model and no variant.
func (j *judgeTarget) judgeOne(ctx context.Context, target, source, translation string) (map[string]bool, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Target language: %s\n\nSource (English):\n%s\n\nCandidate translation:\n%s\n", target, source, translation)
	if hs := contextFor(target).HouseStyle(); len(hs) > 0 {
		sb.WriteString("\nHouse vocabulary (mandated by the client; treat as given and judge the writing around it, never the term choice itself):\n")
		for _, line := range hs {
			fmt.Fprintf(&sb, "- %s\n", line)
		}
	}
	sb.WriteString("\nCriteria:\n")
	for _, c := range rubric() {
		fmt.Fprintf(&sb, "- %s: %s\n", c.ID, c.Text)
	}
	messages := []aiprovider.Message{
		aiprovider.TextMessage(aiprovider.RoleSystem,
			"You are scoring a translation against a fixed style rubric. "+
				"For each criterion answer pass=true only if the translation clearly satisfies it. "+
				"Some vocabulary is mandated by the client's house style; a mandated rendering is never "+
				"unnatural or off-register by itself. Judge the writing around it. "+
				"Judge only the translation text you are given; you know nothing about how it was produced."),
		aiprovider.TextMessage(aiprovider.RoleUser, sb.String()),
	}
	resp, err := j.llm.ChatStructured(ctx, messages, judgeSchema())
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Verdicts []struct {
			ID   string `json:"id"`
			Pass bool   `json:"pass"`
		} `json:"verdicts"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
		return nil, fmt.Errorf("judge reply: %w", err)
	}
	out := make(map[string]bool, len(parsed.Verdicts))
	for _, v := range parsed.Verdicts {
		out[v.ID] = v.Pass
	}
	return out, nil
}

func judgeSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "rubric_verdicts",
		Description: "One yes/no verdict per rubric criterion",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"verdicts": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"pass": map[string]any{"type": "boolean"},
						},
						"required":             []string{"id", "pass"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"verdicts"},
			"additionalProperties": false,
		},
	}
}

// ---------------------------------------------------------------------------
// Judge–human agreement (-judge-validate)
// ---------------------------------------------------------------------------

// labelsFile is the human-labeled seed set: items with the exact text that was
// judged and a human's yes/no per criterion. Produce candidates for labeling
// with -save-outputs; a labeled item is self-contained so the validation can
// re-run as judges change.
type labelsFile struct {
	Items []labelItem `json:"items"`
}

type labelItem struct {
	ID          string          `json:"id"`
	Target      string          `json:"target"`
	Source      string          `json:"source"`
	Translation string          `json:"translation"`
	Criteria    map[string]bool `json:"criteria"`
}

// runJudgeValidation measures judge–human agreement over the labeled set and
// records it in the history. Until a validation meeting the bar exists for the
// current rubric digest, the dashboard keeps the judged dimension unpublished —
// low agreement means a judged score would report the judge's opinion as the
// model's behavior.
func runJudgeValidation(ctx context.Context, judge *judgeTarget, labelsPath, historyPath, date string) error {
	raw, err := os.ReadFile(labelsPath)
	if err != nil {
		return err
	}
	var labels labelsFile
	if err := json.Unmarshal(raw, &labels); err != nil {
		return fmt.Errorf("%s: %w", labelsPath, err)
	}
	if len(labels.Items) == 0 {
		return errors.New("labels file has no items")
	}

	// perCriterion tracks agreement on each rubric axis, because a single pooled
	// kappa hides which axis the judge and human diverge on — and that is the
	// actionable finding (register may be near-perfect while naturalness is a
	// coin flip).
	type crit struct{ agree, total, judgeYes, humanYes int }
	perCriterion := map[string]*crit{}
	var judgeYes, humanYes, agree, total, skippedSameLang int
	targets := map[string]bool{}
	for _, item := range labels.Items {
		// Same-language items are not judged in sweeps, so agreement measured
		// on them would validate the judge on a distribution it never scores.
		if !judgeableTarget(item.Target) {
			skippedSameLang++
			continue
		}
		verdicts, err := judge.judgeOne(ctx, item.Target, item.Source, item.Translation)
		if err != nil {
			return fmt.Errorf("item %s: %w", item.ID, err)
		}
		for _, c := range rubric() {
			human, ok := item.Criteria[c.ID]
			if !ok {
				continue // the human did not label this criterion — not a disagreement
			}
			jv, ok := verdicts[c.ID]
			if !ok {
				return fmt.Errorf("item %s: judge returned no verdict for %q", item.ID, c.ID)
			}
			targets[item.Target] = true
			pc := perCriterion[c.ID]
			if pc == nil {
				pc = &crit{}
				perCriterion[c.ID] = pc
			}
			pc.total++
			total++
			if jv {
				judgeYes++
				pc.judgeYes++
			}
			if human {
				humanYes++
				pc.humanYes++
			}
			if jv == human {
				agree++
				pc.agree++
			} else if dumpFailures {
				// A disagreement nobody can look at is a number, not a finding —
				// so -dump prints each one, with who said what, exactly as the
				// sweep dumps its scored failures.
				fmt.Fprintf(os.Stderr, "    [disagree:%s] %s\n      human=%v judge=%v\n      %q\n",
					c.ID, item.ID, human, jv, item.Translation)
			}
		}
	}
	if skippedSameLang > 0 {
		fmt.Printf("skipped %d same-language item(s): en → en-GB register grades the source rather than the adaptation, and the judge never scores it\n", skippedSameLang)
	}
	if total == 0 {
		return errors.New("labels file labels none of the rubric's criteria on a judgeable (different-language) target")
	}
	for _, c := range rubric() {
		if pc := perCriterion[c.ID]; pc != nil {
			k := cohenKappa(pc.agree, pc.judgeYes, pc.humanYes, pc.total)
			fmt.Printf("  %-12s agreement %.2f  kappa %.2f  (human failed %d/%d, judge failed %d/%d)\n",
				c.ID, float64(pc.agree)/float64(pc.total), k, pc.total-pc.humanYes, pc.total, pc.total-pc.judgeYes, pc.total)
		}
	}

	agreement := float64(agree) / float64(total)
	kappa := cohenKappa(agree, judgeYes, humanYes, total)
	sum := sha256.Sum256(raw)

	v := JudgeValidation{
		Date:         date,
		Provider:     judge.provider,
		Model:        judge.model,
		RubricDigest: rubricDigest(),
		Items:        total,
		Agreement:    agreement,
		Kappa:        kappa,
		LabelsDigest: hex.EncodeToString(sum[:])[:12],
		Targets:      slices.Sorted(maps.Keys(targets)),
	}
	h, err := LoadHistory(historyPath)
	if err != nil {
		return err
	}
	h.UpsertValidation(v)
	if err := h.Save(historyPath); err != nil {
		return err
	}

	verdict := "BELOW the publication bar: the judged dimension stays unpublished"
	if kappa >= MinJudgeKappa && total >= MinJudgeItems {
		verdict = "meets the publication bar"
	}
	fmt.Printf("judge %s:%s vs human labels: %d verdicts on %s, agreement %.2f, kappa %.2f: %s (need kappa ≥ %.1f over ≥ %d verdicts)\n",
		judge.provider, judge.model, total, strings.Join(v.Targets, "/"), agreement, kappa, verdict, MinJudgeKappa, MinJudgeItems)
	fmt.Printf("recorded in %s\n", historyPath)
	return nil
}

// cohenKappa computes chance-corrected agreement for binary verdicts. Raw
// agreement flatters a judge that answers "yes" to everything; kappa does not.
func cohenKappa(agree, judgeYes, humanYes, total int) float64 {
	if total == 0 {
		return 0
	}
	po := float64(agree) / float64(total)
	pYes := float64(judgeYes) / float64(total) * float64(humanYes) / float64(total)
	pNo := (1 - float64(judgeYes)/float64(total)) * (1 - float64(humanYes)/float64(total))
	pe := pYes + pNo
	if pe >= 1 {
		// Both raters were unanimous. Identical unanimity is perfect agreement;
		// anything else is none.
		if po == 1 {
			return 1
		}
		return 0
	}
	return (po - pe) / (1 - pe)
}
