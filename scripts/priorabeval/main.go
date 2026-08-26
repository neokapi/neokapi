// Command priorabeval measures whether showing a model the previously approved
// translation changes what it produces.
//
// It is the one eval on /coordinate that spends model calls, so it is a
// separate command from coordinatereport and its output is committed rather
// than regenerated on every build. coordinatereport must stay deterministic:
// its drift test regenerates it on every pull request, and a test that called a
// model would be a test that fails when a model has a bad day.
//
//	go run ./scripts/priorabeval                    # run it, write the committed data
//	go run ./scripts/priorabeval -repeat 3          # three samples per case
//	go run ./scripts/priorabeval -out /dev/stdout   # inspect without writing
//
// Two measurements, deliberately different in kind:
//
//   - Consistency is DETERMINISTIC. Each case names the wording a translator
//     settled on and the wording a model reaches for instead, and the output is
//     searched for both. No judge is involved, so this half cannot be wrong
//     about itself.
//   - Quality is JUDGED, blind and pairwise, by a model from a different family
//     than the one under test. A judge that shares a family with the model it
//     grades prefers its own output, measurably. The judge never learns which
//     candidate had the reference, and never sees the reference.
//
// The judged half is reported as unvalidated until judge-to-human agreement has
// been measured, following the same discipline as scripts/contexteval. The
// deterministic half needs no such caveat, which is most of why it is here.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/edit"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// DefaultOut is the committed data the /coordinate page reads.
const DefaultOut = "web/src/pages/coordinate/_abeval.json"

func main() {
	out := flag.String("out", DefaultOut, "where to write the report")
	modelProvider := flag.String("model-provider", "claude-code", "provider for the model under test")
	modelName := flag.String("model", "sonnet", "model under test")
	judgeProvider := flag.String("judge-provider", "ollama", "provider for the judge")
	judgeName := flag.String("judge-model", "gemma4:e2b", "judge model, which must be a different family")
	repeat := flag.Int("repeat", 1, "samples per case per arm")
	batchSize := flag.Int("batch-size", 10, "segments per call in the batched arms")
	flag.Parse()

	report, err := Run(context.Background(), RunOpts{
		Model:     target{provider: *modelProvider, model: *modelName},
		Judge:     target{provider: *judgeProvider, model: *judgeName},
		Repeat:    *repeat,
		BatchSize: *batchSize,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "priorabeval:", err)
		os.Exit(1)
	}
	data, err := Marshal(report)
	if err != nil {
		fmt.Fprintln(os.Stderr, "priorabeval:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "priorabeval:", err)
		os.Exit(1)
	}
	fmt.Printf("priorabeval: %d cases, %d samples each → %s\n", len(report.Cases), report.Repeat, *out)
	fmt.Printf("  kept the approved wording: %d/%d with the reference, %d/%d without\n",
		report.KeptWith, report.Samples, report.KeptWithout, report.Samples)
}

// Marshal renders the report as the exact bytes committed.
func Marshal(r *Report) ([]byte, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

// target is one provider/model pair.
type target struct{ provider, model string }

func (t target) String() string { return t.provider + ":" + t.model }

// RunOpts is what one run of the eval is configured with.
type RunOpts struct {
	Model     target
	Judge     target
	Repeat    int
	BatchSize int
}

// Sample is one call to the model under test.
type Sample struct {
	// Without and With are the Norwegian the model produced from each prompt.
	Without string `json:"without"`
	With    string `json:"with"`
	// KeptWithout and KeptWith report whether the approved wording survived.
	KeptWithout bool `json:"keptWithout"`
	KeptWith    bool `json:"keptWith"`
	// DriftedWithout and DriftedWith report the other wording appearing.
	DriftedWithout bool `json:"driftedWithout"`
	DriftedWith    bool `json:"driftedWith"`
	// Judge is the blind pairwise verdict, or nil when the judge was skipped.
	Judge *Verdict `json:"judge,omitempty"`
}

// Verdict is one blind pairwise judgement.
type Verdict struct {
	// Winner is "with", "without" or "tie", resolved back from the shuffled
	// labels the judge actually saw.
	Winner string `json:"winner"`
	Reason string `json:"reason,omitempty"`
	// ShownFirst says which arm the judge saw as candidate 1, so a reader can
	// check the order was actually varied.
	ShownFirst string `json:"shownFirst"`
}

// CaseResult is one case with every sample taken of it.
type CaseResult struct {
	Name        string   `json:"name"`
	PriorSource string   `json:"priorSource"`
	Source      string   `json:"source"`
	PriorTarget string   `json:"priorTarget"`
	Keep        []string `json:"keep"`
	Drift       []string `json:"drift"`
	Why         string   `json:"why"`
	Withheld    bool     `json:"withheld,omitempty"`
	// PromptDiffers is whether the two prompts were actually different. For a
	// withheld case they are not, which is what makes it a control.
	PromptDiffers bool     `json:"promptDiffers"`
	Samples       []Sample `json:"samples"`
	KeptWith      int      `json:"keptWith"`
	KeptWithout   int      `json:"keptWithout"`
}

// Report is the whole eval.
type Report struct {
	Note  string `json:"_note"`
	RanAt string `json:"ranAt"`
	// Model and Judge name what produced these numbers. A reader cannot
	// interpret them without knowing.
	Model       string `json:"model"`
	Judge       string `json:"judge"`
	ModelFamily string `json:"modelFamily"`
	JudgeFamily string `json:"judgeFamily"`
	Repeat      int    `json:"repeat"`
	// Samples is the total across every case and repeat.
	Samples     int `json:"samples"`
	KeptWith    int `json:"keptWith"`
	KeptWithout int `json:"keptWithout"`
	// JudgeWins counts the blind pairwise verdicts.
	JudgeWith    int `json:"judgeWith"`
	JudgeWithout int `json:"judgeWithout"`
	JudgeTie     int `json:"judgeTie"`
	// JudgeValidated is false until judge-to-human agreement has been measured.
	// The page must not present the judged half as a finding while it is false.
	JudgeValidated bool         `json:"judgeValidated"`
	Cases          []CaseResult `json:"cases"`
	// Batched is the three-arm measurement over a realistic document: the path
	// production actually runs, which the per-case arms above do not.
	Batched *BatchedReport `json:"batched,omitempty"`
}

const reportNote = "GENERATED by `go run ./scripts/priorabeval`. This is the one eval on /coordinate " +
	"that spends model calls, so it is committed rather than regenerated per build. The consistency " +
	"numbers are deterministic string checks; the judged numbers are unvalidated until judge-to-human " +
	"agreement is measured."

// Run executes the eval.
func Run(ctx context.Context, opts RunOpts) (*Report, error) {
	if opts.Repeat < 1 {
		opts.Repeat = 1
	}
	mf, jf := modelFamily(opts.Model), modelFamily(opts.Judge)
	if mf != "" && mf == jf {
		return nil, fmt.Errorf(
			"the judge (%s) is the same family as the model under test (%s): self-preference is measurable, so this is refused rather than warned about",
			opts.Judge, opts.Model)
	}

	llm, err := newProvider(opts.Model)
	if err != nil {
		return nil, fmt.Errorf("model under test: %w", err)
	}
	defer llm.Close()

	judge, err := newProvider(opts.Judge)
	if err != nil {
		return nil, fmt.Errorf("judge: %w", err)
	}
	defer judge.Close()

	report := &Report{
		Note:        reportNote,
		Model:       opts.Model.String(),
		Judge:       opts.Judge.String(),
		ModelFamily: mf,
		JudgeFamily: jf,
		Repeat:      opts.Repeat,
	}

	for _, c := range abCases {
		without, with, err := promptsFor(c)
		if err != nil {
			return nil, err
		}
		res := CaseResult{
			Name:          c.name,
			PriorSource:   c.priorSource,
			Source:        c.source,
			PriorTarget:   c.priorTarget,
			Keep:          c.keep,
			Drift:         c.drift,
			Why:           c.why,
			Withheld:      c.withheld,
			PromptDiffers: !c.withheld,
		}

		for i := range opts.Repeat {
			fmt.Fprintf(os.Stderr, "  %s, sample %d/%d\n", c.name, i+1, opts.Repeat)
			a, err := ask(ctx, llm, without)
			if err != nil {
				return nil, fmt.Errorf("%s without the reference: %w", c.name, err)
			}
			b, err := ask(ctx, llm, with)
			if err != nil {
				return nil, fmt.Errorf("%s with the reference: %w", c.name, err)
			}

			s := Sample{
				Without:        a,
				With:           b,
				KeptWithout:    containsAny(a, c.keep),
				KeptWith:       containsAny(b, c.keep),
				DriftedWithout: containsAny(a, c.drift),
				DriftedWith:    containsAny(b, c.drift),
			}
			if s.KeptWith {
				res.KeptWith++
				report.KeptWith++
			}
			if s.KeptWithout {
				res.KeptWithout++
				report.KeptWithout++
			}
			report.Samples++

			// index i varies which arm the judge sees first, deterministically:
			// Math.random is unavailable and a fixed order would let a judge
			// with a position bias decide the result.
			v, err := judgePair(ctx, judge, c, a, b, i%2 == 0)
			if err != nil {
				return nil, fmt.Errorf("%s judging: %w", c.name, err)
			}
			s.Judge = v
			switch v.Winner {
			case "with":
				report.JudgeWith++
			case "without":
				report.JudgeWithout++
			default:
				report.JudgeTie++
			}
			res.Samples = append(res.Samples, s)
		}
		report.Cases = append(report.Cases, res)
	}
	if opts.BatchSize > 0 {
		fmt.Fprintln(stderr, "  batched arms")
		batched, err := runBatchedArms(ctx, llm, opts.BatchSize)
		if err != nil {
			return nil, fmt.Errorf("batched arms: %w", err)
		}
		report.Batched = batched
	}

	report.RanAt = time.Now().UTC().Format(time.RFC3339)
	return report, nil
}

// promptsFor renders the two prompts through the real prompt builder, which is
// the whole point: what is measured is the prompt production would send, not a
// reconstruction of it.
func promptsFor(c abCase) (without, with []aiprovider.Message, err error) {
	t := prompt.Translate{
		SourceLocale: "en",
		TargetLocale: "nb",
		VoiceGuide:   coreprofile.RenderVoiceGuideCompact(abVoice),
		// Scoped as the tool scopes it, so both arms carry the prompt
		// production would send.
		PreferredTerms: coreprofile.ScopedTermRuleMap(abTermRules, c.source),
	}

	bare := prompt.Context{Key: c.name}
	ref := prompt.Context{Key: c.name}
	if !c.withheld {
		ref.Prior = &prompt.PriorVersion{Source: c.priorSource, Target: c.priorTarget}
	}

	return aiprovider.MessagesFromTurns(t.SingleWithContext(c.source, false, bare)),
		aiprovider.MessagesFromTurns(t.SingleWithContext(c.source, false, ref)),
		nil
}

// ask sends one prompt and returns the translation, trimmed of the quoting a
// model sometimes adds around a bare string.
func ask(ctx context.Context, llm aiprovider.LLMProvider, msgs []aiprovider.Message) (string, error) {
	resp, err := llm.Chat(ctx, msgs)
	if err != nil {
		return "", err
	}
	return strings.Trim(strings.TrimSpace(resp.Content), "\"“”"), nil
}

// containsAny reports whether any accepted form of a wording appears in the
// output, as whole words.
//
// The word boundary is the whole point, and the first version of this function
// did not have it: strings.Contains says "handlekurven" contains "kurven", so
// every drift in the corpus scored as the approved wording surviving, and the
// eval reported that the reference changed nothing. See edit.ContainsWords.
func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if edit.ContainsWords(haystack, n) {
			return true
		}
	}
	return false
}

// newProvider is the one place a provider is constructed, so the model under
// test and the judge are reached the same way.
func newProvider(t target) (aiprovider.LLMProvider, error) {
	return aiprovider.NewProvider(aiprovider.ProviderID(t.provider), aiprovider.Config{
		APIKey: os.Getenv(apiKeyEnv(t.provider)),
		Model:  t.model,
	})
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

// modelFamily identifies the family a model belongs to, so a judge sharing one
// with the model under test can be refused. The model name decides it; the
// provider is only a fallback, because one provider serves several families.
func modelFamily(t target) string {
	m := strings.ToLower(t.model)
	switch {
	case strings.Contains(m, "claude") || m == "sonnet" || m == "opus" || m == "haiku" || m == "fable":
		return "anthropic"
	case strings.HasPrefix(m, "gpt") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3"):
		return "openai"
	case strings.Contains(m, "gemini") || strings.Contains(m, "gemma"):
		return "google"
	case strings.Contains(m, "llama"):
		return "meta"
	case strings.Contains(m, "qwen"):
		return "alibaba"
	case strings.Contains(m, "mistral") || strings.Contains(m, "ministral"):
		return "mistral"
	}
	switch aiprovider.ProviderID(strings.ToLower(t.provider)) {
	case aiprovider.Anthropic, aiprovider.ClaudeCode:
		return "anthropic"
	case aiprovider.OpenAI, aiprovider.AzureOpenAI:
		return "openai"
	case aiprovider.Gemini:
		return "google"
	}
	return ""
}
