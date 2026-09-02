package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// authoring-effect: does `kapi voice guide` steer writing toward the profile,
// or does it just make the writing better?
//
// The second question is the one that makes this worth running. Prepending any
// competent writing guidance improves any reasonable score, so "the guided text
// scored higher" is not evidence about kapi. Every document is therefore
// scored twice — against the reference profile the guide came from, and against
// contrastProfile, which wants the opposite on every axis it can.
//
//	effect = (steered_ref - bare_ref) - (steered_contrast - bare_contrast)
//
// A guide that steers toward Harbourlight moves the first term and leaves the
// second alone or negative. A guide that only says "write well" moves both, and
// the effect lands near zero — which would be the honest finding.
//
// Scoring is the offline check, so what is measured is the mechanisms it
// implements: forbidden terms and prohibited patterns. The declared fields
// (person_pov, sentence_length) are in the guide the model reads and are not in
// the score, because nothing offline evaluates them. That asymmetry is stated
// on the card rather than smoothed over.

// steerTasks are the briefs the model writes to. They name a subject the corpus
// covers and say nothing about voice, so the guide is the only thing that
// differs between the two arms.
var steerTasks = []string{
	"Write a short help page, about 120 words, explaining how a port operator files cargo paperwork with customs before a vessel berths.",
	"Write a short help page, about 120 words, explaining how a port operator resolves two vessels booked into the same berth at the same time.",
	"Write a short release note, about 120 words, announcing that the arrivals board now updates when a vessel reports a new position.",
	"Write a short help page, about 120 words, explaining how an administrator gives a new colleague the right level of access.",
	"Write a short onboarding email, about 120 words, to a port operator who has just been given an account.",
	"Write a short status-page entry, about 120 words, explaining that filings were delayed for two hours and have now caught up.",
}

// SteerDoc is one brief written twice and scored twice.
type SteerDoc struct {
	Task string `json:"task"`
	// The prose itself, both arms, so the page can diff them.
	Bare    string `json:"bare"`
	Steered string `json:"steered"`
	// Scores against the profile the guide came from.
	BareRef    int `json:"bareRef"`
	SteeredRef int `json:"steeredRef"`
	// Scores against the profile that wants the opposite.
	BareContrast    int `json:"bareContrast"`
	SteeredContrast int `json:"steeredContrast"`
	// Findings from the reference check, so a reader can see what moved.
	BareFindings    []Finding `json:"bareFindings,omitempty"`
	SteeredFindings []Finding `json:"steeredFindings,omitempty"`
	Error           string    `json:"error,omitempty"`
}

// RefGain is how much the guide moved the reference score for this document.
func (d SteerDoc) RefGain() int { return d.SteeredRef - d.BareRef }

// ContrastGain is how much it moved the score of the profile it was not
// steering toward.
func (d SteerDoc) ContrastGain() int { return d.SteeredContrast - d.BareContrast }

// SteerResult is authoring-effect's dataset.
type SteerResult struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// Guide is the text prepended in the steered arm, verbatim. The whole
	// measurement is about this string, so it is published rather than
	// described.
	Guide string     `json:"guide"`
	Docs  []SteerDoc `json:"docs"`
	// Means over the documents that scored.
	MeanBareRef         float64 `json:"meanBareRef"`
	MeanSteeredRef      float64 `json:"meanSteeredRef"`
	MeanBareContrast    float64 `json:"meanBareContrast"`
	MeanSteeredContrast float64 `json:"meanSteeredContrast"`
	// RefGain and ContrastGain are the two halves of the differential.
	RefGain      float64 `json:"refGain"`
	ContrastGain float64 `json:"contrastGain"`
	// Effect is RefGain - ContrastGain: what the guide did that generic
	// writing advice would not have done.
	Effect  float64 `json:"effect"`
	Scored  int     `json:"scored"`
	Blocked string  `json:"blocked,omitempty"`
}

func runSteer(ctx context.Context, bin, workdir, provider, model string) (*SteerResult, error) {
	res := &SteerResult{Provider: provider, Model: model}

	guide, err := kapiRun(ctx, bin, workdir, "voice", "guide", "--profile-file", workdir+"/voice.yaml")
	if err != nil {
		res.Blocked = "kapi voice guide failed: " + truncate(guide, 300)
		return res, nil
	}
	res.Guide = strings.TrimSpace(guide)
	if res.Guide == "" {
		res.Blocked = "kapi voice guide produced no text, so the steered arm would be identical to the bare one"
		return res, nil
	}

	llm, err := aiprovider.NewProvider(aiprovider.ProviderID(provider), aiprovider.Config{
		APIKey: os.Getenv(apiKeyEnv(provider)),
		Model:  model,
		// Greedy. The effect being measured is small, and sampling noise across
		// twelve generations would swamp it. new(expr) because 0 has to be
		// reachable and distinguishable from unset.
		Temperature: new(float64),
	})
	if err != nil {
		res.Blocked = "provider: " + err.Error()
		return res, nil
	}
	defer llm.Close()

	for _, task := range steerTasks {
		d := SteerDoc{Task: task}
		bare, err := write(ctx, llm, "", task)
		if err != nil {
			d.Error = err.Error()
			res.Docs = append(res.Docs, d)
			continue
		}
		steered, err := write(ctx, llm, res.Guide, task)
		if err != nil {
			d.Error = err.Error()
			res.Docs = append(res.Docs, d)
			continue
		}
		d.Bare, d.Steered = bare, steered

		for _, arm := range []struct {
			text    string
			profile string
			score   *int
			finds   *[]Finding
		}{
			{bare, "voice.yaml", &d.BareRef, &d.BareFindings},
			{steered, "voice.yaml", &d.SteeredRef, &d.SteeredFindings},
			{bare, "contrast.yaml", &d.BareContrast, nil},
			{steered, "contrast.yaml", &d.SteeredContrast, nil},
		} {
			r, err := offlineCheckWithProfile(ctx, bin, workdir, arm.profile, arm.text)
			if err != nil {
				d.Error = err.Error()
				break
			}
			*arm.score = r.Score
			if arm.finds != nil {
				*arm.finds = r.Findings
			}
		}
		res.Docs = append(res.Docs, d)
	}
	return summarize(res), nil
}

// write asks the model for one document, with the guide prepended or not.
//
// The user turn is identical in both arms. Only the system turn differs, and in
// the bare arm there is none — not an empty one, and not a "write well" one,
// because a placebo instruction would be a third condition rather than a
// control.
func write(ctx context.Context, llm aiprovider.LLMProvider, guide, task string) (string, error) {
	var msgs []aiprovider.Message
	if guide != "" {
		msgs = append(msgs, aiprovider.TextMessage(aiprovider.RoleSystem, guide))
	}
	msgs = append(msgs, aiprovider.TextMessage(aiprovider.RoleUser,
		task+"\n\nReturn the document only. No preamble, no commentary."))
	resp, err := llm.Chat(ctx, msgs)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func summarize(res *SteerResult) *SteerResult {
	var br, sr, bc, sc float64
	for _, d := range res.Docs {
		if d.Error != "" {
			continue
		}
		res.Scored++
		br += float64(d.BareRef)
		sr += float64(d.SteeredRef)
		bc += float64(d.BareContrast)
		sc += float64(d.SteeredContrast)
	}
	if res.Scored == 0 {
		if res.Blocked == "" {
			res.Blocked = "no document scored; see the per-document errors"
		}
		return res
	}
	n := float64(res.Scored)
	res.MeanBareRef, res.MeanSteeredRef = br/n, sr/n
	res.MeanBareContrast, res.MeanSteeredContrast = bc/n, sc/n
	res.RefGain = res.MeanSteeredRef - res.MeanBareRef
	res.ContrastGain = res.MeanSteeredContrast - res.MeanBareContrast
	res.Effect = res.RefGain - res.ContrastGain
	return res
}

// apiKeyEnv names the environment variable a provider reads its key from.
func apiKeyEnv(provider string) string {
	switch provider {
	case "openai":
		return "OPENAI_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	case "ollama", "claude-code", "demo":
		return ""
	default:
		return "ANTHROPIC_API_KEY"
	}
}

func reportSteer(r *SteerResult) {
	fmt.Printf("\nauthoring-effect (%s:%s)\n", r.Provider, r.Model)
	if r.Blocked != "" {
		fmt.Printf("  NOT MEASURED: %s\n", r.Blocked)
		return
	}
	fmt.Printf("  %d documents written twice\n", r.Scored)
	fmt.Printf("    against the profile the guide came from:  %.1f → %.1f  (%+.1f)\n",
		r.MeanBareRef, r.MeanSteeredRef, r.RefGain)
	fmt.Printf("    against the profile it did not:           %.1f → %.1f  (%+.1f)\n",
		r.MeanBareContrast, r.MeanSteeredContrast, r.ContrastGain)
	fmt.Printf("    effect (the difference of the two):       %+.1f\n", r.Effect)
}
