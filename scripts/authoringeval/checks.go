package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// authoring-checks: does a voice check find the violations a profile declares,
// and does it stay quiet on prose that obeys it?
//
// Two checks answer to the same profile and are measured the same way, so the
// numbers sit side by side:
//
//	offline  `kapi voice check` — string and regex matching, no model, no cost.
//	llm      `kapi exec voice-check` — the profile handed to a model.
//
// The two halves are measured on different documents on purpose:
//
//	recall            over the off-profile documents. Every violation there was
//	                  written in deliberately, so a plant the check did not
//	                  reach is a miss.
//	false positives   over the on-profile documents, where the right answer is
//	                  silence and any finding is wrong.
//
// A single pooled precision was the first attempt and it measured the wrong
// thing. On an off-profile document the check finds violations beyond the
// planted ones — help-passive.md was written with three passive constructions
// marked and contains eight — so unplanted findings there are true positives the
// labelling missed, and counting them against the check turned "how complete are
// my labels" into a number about kapi. Recall needs planted violations;
// precision needs documents where nothing is planted; neither needs the other's
// documents.
//
// Recall is per plant, not per finding. Two findings on one plant (the passive
// regex matches inside the hedge phrase) count once.
// Finding is one thing a check reported, in the shape both checks return.
type Finding struct {
	Category     string `json:"category"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	OriginalText string `json:"original_text"`
}

// checkResult is one check's output for one document.
type checkResult struct {
	Score    int       `json:"score"`
	Findings []Finding `json:"findings"`
}

// DocOutcome is what one check did with one document, kept whole so the
// dashboard can show the prose, the plants and the findings together.
type DocOutcome struct {
	Doc      string    `json:"doc"`
	Kind     string    `json:"kind"`
	Score    int       `json:"score"`
	Plants   []Plant   `json:"plants,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
	// Covered is the plant text each finding landed on, parallel to Findings.
	// Empty means the finding matched no plant, which is what makes it a false
	// positive.
	Covered []string `json:"covered,omitempty"`
	// Missed is the plants no finding reached.
	Missed []Plant `json:"missed,omitempty"`
	Error  string  `json:"error,omitempty"`
}

// CheckAccuracy is one check's score over the whole corpus.
type CheckAccuracy struct {
	Check string `json:"check"`
	// Surface is the exact command, so a reader can run it.
	Surface string `json:"surface"`
	// Plants and Covered are recall, over the off-profile documents.
	Plants   int     `json:"plants"`
	Covered  int     `json:"covered"`
	Recall   float64 `json:"recall"`
	Findings int     `json:"findings"`
	// ByMechanism is recall split by how the profile expresses the rule. This
	// is the number the whole eval exists for: a check can score well overall
	// and have zero recall on a mechanism it does not implement.
	ByMechanism map[string]MechanismScore `json:"byMechanism"`
	// CleanDocs is the on-profile half, and CleanFalsePositives is what the
	// check said about it. Both are reported, because 3 findings across 6
	// documents and 3 across 60 are different results.
	CleanDocs           int `json:"cleanDocs"`
	CleanFalsePositives int `json:"cleanFalsePositives"`
	// CleanDocsWithFindings is how many of those documents drew at least one
	// finding — the rate a reader would notice.
	CleanDocsWithFindings int          `json:"cleanDocsWithFindings"`
	Outcomes              []DocOutcome `json:"outcomes"`
	// Blocked says why this check has no numbers, when it has none. A check
	// that returns nothing scores perfect precision on zero recall, and
	// publishing that is worse than publishing nothing.
	Blocked string `json:"blocked,omitempty"`
}

// MechanismScore is recall for one of the three rule mechanisms.
type MechanismScore struct {
	Plants  int     `json:"plants"`
	Covered int     `json:"covered"`
	Recall  float64 `json:"recall"`
}

// runOfflineCheck scores every document with the deterministic checker.
func runOfflineCheck(ctx context.Context, bin, workdir string) (CheckAccuracy, error) {
	acc := CheckAccuracy{
		Check:   "voice check (offline)",
		Surface: "kapi voice check --profile-file voice.yaml --json < <doc>",
	}
	for _, d := range corpus {
		res, err := offlineCheckOne(ctx, bin, workdir, d)
		out := DocOutcome{Doc: d.Name, Kind: d.Kind, Plants: d.Plants}
		if err != nil {
			out.Error = err.Error()
			acc.Outcomes = append(acc.Outcomes, out)
			continue
		}
		out.Score, out.Findings = res.Score, res.Findings
		acc.Outcomes = append(acc.Outcomes, scoreDoc(out))
	}
	return tally(acc), nil
}

func offlineCheckOne(ctx context.Context, bin, workdir string, d Doc) (checkResult, error) {
	res, err := offlineCheckWithProfile(ctx, bin, workdir, "voice.yaml", d.Body)
	if err != nil {
		return checkResult{}, fmt.Errorf("%s: %w", d.Name, err)
	}
	return res, nil
}

// offlineCheckWithProfile scores text against one of the workspace's profiles.
// authoring-effect needs both, since the point of the contrast profile is to
// score the same prose a second way.
func offlineCheckWithProfile(ctx context.Context, bin, workdir, profile, text string) (checkResult, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "voice", "check",
		"--profile-file", filepath.Join(workdir, profile), "--json")
	cmd.Stdin = strings.NewReader(text)
	cmd.Env = append(os.Environ(), isolationEnv(workdir)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return checkResult{}, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var res checkResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return checkResult{}, err
	}
	return res, nil
}

// runLLMCheck scores every document with the model-backed checker.
func runLLMCheck(ctx context.Context, bin, workdir, provider, model string) (CheckAccuracy, error) {
	acc := CheckAccuracy{
		Check:   "exec voice-check (" + provider + ":" + model + ")",
		Surface: "kapi exec voice-check <doc> --profile-id " + referenceProfileID + " --provider " + provider + " --json",
	}
	for _, d := range corpus {
		out := DocOutcome{Doc: d.Name, Kind: d.Kind, Plants: d.Plants}
		res, err := llmCheckOne(ctx, bin, workdir, provider, model, d)
		if err != nil {
			out.Error = err.Error()
			acc.Outcomes = append(acc.Outcomes, out)
			continue
		}
		out.Score, out.Findings = res.Score, res.Findings
		acc.Outcomes = append(acc.Outcomes, scoreDoc(out))
	}
	acc = tally(acc)
	// `kapi exec voice-check` writes zero bytes to stdout and exits 0 under
	// every provider, profile and input tried, while sibling tools on the same
	// file print results (issue #2225). An empty finding list is not a score of
	// zero recall, it is the absence of a measurement, and the difference has
	// to survive onto the page.
	if acc.Findings == 0 {
		acc.Blocked = "the tool returned no findings on any document, including the six written to violate the profile. " +
			"It writes nothing to stdout and exits 0 under every provider and profile tried. See issue #2225. " +
			"Until it emits results there is nothing here to score."
		acc.Recall = 0
	}
	return acc, nil
}

func llmCheckOne(ctx context.Context, bin, workdir, provider, model string, d Doc) (checkResult, error) {
	// --profile-id, not --profile-file: this tool resolves from the store, so
	// setupWorkspace imports the reference profile and it is addressed by the
	// id `kapi voice import` assigned it.
	args := []string{"exec", "voice-check", filepath.Join(workdir, "docs", d.Name),
		"--profile-id", referenceProfileID,
		"--provider", provider, "--json"}
	if model != "" {
		args = append(args, "--model", model)
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), isolationEnv(workdir)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return checkResult{}, fmt.Errorf("%s: timed out after %s", d.Name, toolTimeout)
		}
		return checkResult{}, fmt.Errorf("%s: %w: %s", d.Name, err, truncate(strings.TrimSpace(stderr.String()), 300))
	}
	if len(bytes.TrimSpace(stdout.Bytes())) == 0 {
		return checkResult{}, fmt.Errorf("%s: the tool exited 0 and wrote nothing to stdout (issue #2225)", d.Name)
	}
	return parseToolCheck(stdout.Bytes(), d.Name)
}

// parseToolCheck reads `kapi exec voice-check --json`, whose envelope nests the
// score under a summary rather than putting it at the top level the way
// `kapi voice check` does.
func parseToolCheck(raw []byte, doc string) (checkResult, error) {
	var env struct {
		Summary struct {
			Score int `json:"score"`
		} `json:"summary"`
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return checkResult{}, fmt.Errorf("%s: %w", doc, err)
	}
	return checkResult{Score: env.Summary.Score, Findings: env.Findings}, nil
}

// scoreDoc matches a document's findings against its plants.
//
// A finding lands on a plant when either text contains the other, compared
// without case. Containment in both directions is deliberate: the term check
// reports the word it matched ("utilize"), while the pattern check reports the
// span it matched ("been confirmed"), which is shorter than the plant and
// longer than a word.
func scoreDoc(out DocOutcome) DocOutcome {
	covered := make(map[int]bool, len(out.Plants))
	out.Covered = make([]string, len(out.Findings))
	for i, f := range out.Findings {
		// Flattened, for the reason the corpus test gives: the documents wrap at
		// 78 columns, and a pattern match that spans a line break comes back as
		// "be\nrecorded". Comparing that to a plant written on one line fails on
		// the newline alone.
		text := flattenLower(f.OriginalText)
		if text == "" {
			// A required-pattern finding has no snippet: the violation is an
			// absence. Fall back to the message, which names the rule.
			text = flattenLower(f.Message)
		}
		for j, p := range out.Plants {
			plant := flattenLower(p.Text)
			if text == "" || plant == "" {
				continue
			}
			if strings.Contains(plant, text) || strings.Contains(text, plant) {
				covered[j] = true
				out.Covered[i] = p.Text
				break
			}
		}
	}
	for j, p := range out.Plants {
		if !covered[j] {
			out.Missed = append(out.Missed, p)
		}
	}
	return out
}

// tally rolls the per-document outcomes into the corpus-level score.
func tally(acc CheckAccuracy) CheckAccuracy {
	acc.ByMechanism = map[string]MechanismScore{}
	for _, out := range acc.Outcomes {
		missed := map[string]bool{}
		for _, m := range out.Missed {
			missed[m.Text] = true
		}
		for _, p := range out.Plants {
			acc.Plants++
			ms := acc.ByMechanism[p.Mechanism]
			ms.Plants++
			if !missed[p.Text] {
				acc.Covered++
				ms.Covered++
			}
			acc.ByMechanism[p.Mechanism] = ms
		}
		acc.Findings += len(out.Findings)
		if out.Kind == onProfile {
			acc.CleanDocs++
			acc.CleanFalsePositives += len(out.Findings)
			if len(out.Findings) > 0 {
				acc.CleanDocsWithFindings++
			}
		}
	}
	acc.Recall = ratio(acc.Covered, acc.Plants)
	for k, ms := range acc.ByMechanism {
		ms.Recall = ratio(ms.Covered, ms.Plants)
		acc.ByMechanism[k] = ms
	}
	return acc
}

// flattenLower collapses whitespace and lowercases, so a match is about words
// rather than about how the file happens to be wrapped.
func flattenLower(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// mechanisms lists the mechanisms the corpus has evidence for, in a stable
// order, so a report reads the same twice.
func mechanisms() []string {
	seen := plantsByMechanism()
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
