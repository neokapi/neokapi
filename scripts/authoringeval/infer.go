package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// voice-infer-quality: does an inferred draft recover the profile the corpus
// was written from?
//
// The corpus makes this answerable. Its on-profile half was written from
// referenceProfile, so the reference is not an opinion about what a good draft
// would look like — it is the thing the material expresses, and recovery is
// checkable field by field rather than by asking a judge.
//
// Recovery is scored per field, because a draft that gets `formality: neutral`
// right and every forbidden term wrong is a different failure from the reverse,
// and one number hides which happened.

// FieldRecovery is one profile field, what the reference says, and what the
// draft said.
type FieldRecovery struct {
	Field     string `json:"field"`
	Reference string `json:"reference"`
	Draft     string `json:"draft"`
	Recovered bool   `json:"recovered"`
	// Partial marks a set-valued field where the draft found some of the
	// reference's members. Recall for that field is in Ratio.
	Partial bool    `json:"partial,omitempty"`
	Ratio   float64 `json:"ratio,omitempty"`
}

// InferResult is voice-infer-quality's dataset.
type InferResult struct {
	Surface string `json:"surface"`
	Sources int    `json:"sources"`
	// Draft is the profile the tool produced, verbatim, so a reader can judge
	// it themselves rather than taking the field table's word for it.
	Draft  string          `json:"draft,omitempty"`
	Fields []FieldRecovery `json:"fields,omitempty"`
	// Recovered is fields fully recovered over fields compared.
	Recovered int     `json:"recovered"`
	Compared  int     `json:"compared"`
	Rate      float64 `json:"rate"`
	// Invented counts forbidden terms the draft added that the reference does
	// not carry. A draft is not better for being longer.
	Invented []string `json:"invented,omitempty"`
	Blocked  string   `json:"blocked,omitempty"`
}

// profileShape is the subset of the schema this eval compares. Fields the
// corpus cannot express are left out rather than scored as misses.
type profileShape struct {
	Name string `yaml:"name"`
	Tone struct {
		Personality []string `yaml:"personality"`
		Formality   string   `yaml:"formality"`
		Emotion     string   `yaml:"emotion"`
		Humor       string   `yaml:"humor"`
	} `yaml:"tone"`
	Style struct {
		ActiveVoice    bool   `yaml:"active_voice"`
		SentenceLength string `yaml:"sentence_length"`
		PersonPOV      string `yaml:"person_pov"`
		Contractions   string `yaml:"contractions"`
	} `yaml:"style"`
	Vocabulary struct {
		ForbiddenTerms []struct {
			Term string `yaml:"term"`
		} `yaml:"forbidden_terms"`
	} `yaml:"vocabulary"`
}

// inferTimeout bounds the one inference this eval runs.
const inferTimeout = 6 * time.Minute

func runInfer(ctx context.Context, bin, workdir, provider, model string) (*InferResult, error) {
	sources := docsOfKind(onProfile)
	res := &InferResult{
		Surface: "kapi exec voice-infer <on-profile docs> --provider " + provider + " --json",
		Sources: len(sources),
	}

	args := []string{"exec", "voice-infer"}
	for _, d := range sources {
		args = append(args, filepath.Join(workdir, "docs", d.Name))
	}
	// No --json: the command emits the drafted profile as YAML, which is a
	// profile and parses straight into profileShape. The JSON form nests it
	// under a "profile" key, and reading that envelope as a profile finds no
	// tone and no style and reports every field missing, which is what the
	// first run of this did.
	args = append(args, "--provider", provider, "--profile-name", "Harbourlight Draft")
	if model != "" {
		args = append(args, "--model", model)
	}
	// One call over the whole corpus, not one per document, so it gets its own
	// budget: the shared per-invocation timeout is sized for a check on one
	// file and a local model needed longer than that to read six. The first
	// run reported "wrote nothing" for what was actually a deadline.
	ctx, cancel := context.WithTimeout(ctx, inferTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), isolationEnv(workdir)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	draft := strings.TrimSpace(stdout.String())
	if ctx.Err() != nil {
		res.Blocked = fmt.Sprintf("voice-infer did not return within %s", inferTimeout)
		return res, nil
	}
	// The error is read before the empty output, not after. The first version
	// had these the other way round and reported "wrote nothing" for a run that
	// had failed with a message, which sent the reader looking for a missing
	// output path instead of at the error the tool had already printed.
	if err != nil {
		res.Blocked = fmt.Sprintf("voice-infer failed: %v: %s", err, truncate(stderr.String(), 400))
		return res, nil
	}
	if draft == "" {
		res.Blocked = "voice-infer exited 0 and wrote nothing to stdout. There is no draft to compare."
		return res, nil
	}
	res.Draft = draft
	return compareToReference(res, draft)
}

// compareToReference scores a draft against the profile the corpus was written
// from, field by field.
func compareToReference(res *InferResult, draft string) (*InferResult, error) {
	var want, got profileShape
	if err := yaml.Unmarshal([]byte(referenceProfile), &want); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal([]byte(draft), &got); err != nil {
		// A draft kapi cannot parse is a finding, not an error: the tool
		// produced something and it is not a profile.
		res.Blocked = "the draft is not parseable as a voice profile: " + err.Error()
		return res, nil
	}

	scalar := func(field, w, g string) FieldRecovery {
		return FieldRecovery{
			Field: field, Reference: w, Draft: g,
			Recovered: strings.EqualFold(strings.TrimSpace(w), strings.TrimSpace(g)),
		}
	}
	res.Fields = []FieldRecovery{
		scalar("tone.formality", want.Tone.Formality, got.Tone.Formality),
		scalar("tone.emotion", want.Tone.Emotion, got.Tone.Emotion),
		scalar("tone.humor", want.Tone.Humor, got.Tone.Humor),
		scalar("style.sentence_length", want.Style.SentenceLength, got.Style.SentenceLength),
		scalar("style.person_pov", want.Style.PersonPOV, got.Style.PersonPOV),
		scalar("style.contractions", want.Style.Contractions, got.Style.Contractions),
		scalar("style.active_voice", strconv.FormatBool(want.Style.ActiveVoice), strconv.FormatBool(got.Style.ActiveVoice)),
		setRecovery("tone.personality", want.Tone.Personality, got.Tone.Personality),
		setRecovery("vocabulary.forbidden_terms", termList(want), termList(got)),
	}

	for _, f := range res.Fields {
		res.Compared++
		if f.Recovered {
			res.Recovered++
		}
	}
	res.Rate = ratio(res.Recovered, res.Compared)
	res.Invented = extra(termList(want), termList(got))
	return res, nil
}

// setRecovery scores a list-valued field by how much of the reference the draft
// found. Exact set equality is the wrong bar: a draft that names four of five
// forbidden terms has done most of the work.
func setRecovery(field string, want, got []string) FieldRecovery {
	found := 0
	for _, w := range want {
		for _, g := range got {
			if strings.EqualFold(strings.TrimSpace(w), strings.TrimSpace(g)) {
				found++
				break
			}
		}
	}
	r := ratio(found, len(want))
	return FieldRecovery{
		Field:     field,
		Reference: strings.Join(want, ", "),
		Draft:     strings.Join(got, ", "),
		Recovered: r == 1,
		Partial:   r > 0 && r < 1,
		Ratio:     r,
	}
}

// extra is what the draft added that the reference does not carry.
func extra(want, got []string) []string {
	have := map[string]bool{}
	for _, w := range want {
		have[strings.ToLower(strings.TrimSpace(w))] = true
	}
	var out []string
	for _, g := range got {
		if !have[strings.ToLower(strings.TrimSpace(g))] {
			out = append(out, g)
		}
	}
	sort.Strings(out)
	return out
}

func termList(p profileShape) []string {
	out := make([]string, 0, len(p.Vocabulary.ForbiddenTerms))
	for _, t := range p.Vocabulary.ForbiddenTerms {
		out = append(out, t.Term)
	}
	return out
}

func reportInfer(r *InferResult) {
	fmt.Printf("\nvoice-infer-quality\n")
	if r.Blocked != "" {
		fmt.Printf("  NOT MEASURED: %s\n", r.Blocked)
		return
	}
	fmt.Printf("  recovered %d/%d fields (%.0f%%) from %d documents\n",
		r.Recovered, r.Compared, 100*r.Rate, r.Sources)
	for _, f := range r.Fields {
		mark := "miss"
		switch {
		case f.Recovered:
			mark = "ok"
		case f.Partial:
			mark = fmt.Sprintf("%.0f%%", 100*f.Ratio)
		}
		fmt.Printf("    %-28s %-5s want %q got %q\n", f.Field, mark, f.Reference, f.Draft)
	}
	if len(r.Invented) > 0 {
		fmt.Printf("    invented terms: %s\n", strings.Join(r.Invented, ", "))
	}
}
