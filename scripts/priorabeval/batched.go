package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/ai/prompt"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The batched arm, which is the one production actually runs.
//
// Eval 5's first version translated one sentence per call. Production batches,
// for cost and to share one preamble across many segments, and the two paths
// build materially different prompts: a batch sends a JSON payload of N segments
// and a schema-constrained reply, where a single call sends one string and gets
// one back. Measuring only the single path answered a question nobody asks.
//
// Three arms, so the comparison is not one number against a memory:
//
//   - batchedBare: batched, no references. What a run did before any of this.
//   - batchedRef: batched, one reference per segment. What it does now.
//   - singleRef: one call per block with its reference. The ceiling — what the
//     mechanism can do when nothing competes for the model's attention.
//
// Cost is reported beside quality, because the whole argument for batching is
// cost and the whole argument against the old pack-alone rule was that it
// destroyed it. An arm that wins on wording and costs twenty times as much has
// not obviously won.

// stderr is where progress goes; the report itself goes to the out file.
var stderr = os.Stderr

// Arm names, used as map keys in the report so the page can label them.
const (
	ArmBatchedBare = "batched, no references"
	ArmBatchedRef  = "batched, one reference per segment"
	ArmSingleRef   = "one call per block, with its reference"
)

// ArmResult is one arm over the whole document.
type ArmResult struct {
	Name string `json:"name"`
	// Calls is how many times the model was asked. The number the cost argument
	// turns on.
	Calls int `json:"calls"`
	// InputTokens is everything the model read, including the parts a provider
	// served from its prompt cache.
	//
	// The cached half is counted deliberately. A provider reports it separately
	// and it is cheaper per token, but it is still prompt the model attends to,
	// and leaving it out makes a batched arm look like it sent nothing: summing
	// only the uncached counter reported six input tokens for two calls
	// carrying a twenty-block payload.
	InputTokens int `json:"inputTokens"`
	// CachedInputTokens is the part of InputTokens the provider served from its
	// cache, reported separately because it is priced differently.
	CachedInputTokens int `json:"cachedInputTokens"`
	OutputTokens      int `json:"outputTokens"`
	// Kept and Drifted count the blocks under test whose approved wording
	// survived, and whose did not. Blocks with no wording under test are
	// translated and not scored: they are there to make the batch a batch.
	Kept    int `json:"kept"`
	Drifted int `json:"drifted"`
	// Translations is every block's output, keyed by block id, so a reader can
	// inspect rather than trust the counts.
	Translations map[string]string `json:"translations"`
}

// BatchedReport is the whole three-arm measurement.
type BatchedReport struct {
	// Blocks is how many the document held, and Scored how many of those carried
	// a wording under test.
	Blocks int `json:"blocks"`
	Scored int `json:"scored"`
	// TermRulesAtCoordinate is how many rules govern the collection, against
	// TermRulesSent, the union any one call actually carried. The gap is what
	// scoping bought.
	TermRulesAtCoordinate int         `json:"termRulesAtCoordinate"`
	TermRulesSentBatched  int         `json:"termRulesSentBatched"`
	Arms                  []ArmResult `json:"arms"`
}

// docBlock is one block of the synthetic document.
type docBlock struct {
	id     string
	source string
	// prior is what this block said before, when it has history. Empty for the
	// filler blocks, which exist so the batch is a realistic size.
	priorSource string
	priorTarget string
	// keep and drift are the wordings scored, when this block is under test.
	keep  []string
	drift []string
}

// buildDocument returns a document shaped like an incremental run: a handful of
// edited blocks with history among blocks that have none.
func buildDocument() []docBlock {
	out := make([]docBlock, 0, len(abCases)+len(fillerSources))
	for _, c := range abCases {
		if c.withheld {
			// The control belongs to the single-call eval, where it proves the
			// harness. Here it would just be a second copy of the basket case.
			continue
		}
		out = append(out, docBlock{
			id:          "t-" + strings.ReplaceAll(c.name, " ", "-"),
			source:      c.source,
			priorSource: c.priorSource,
			priorTarget: c.priorTarget,
			keep:        c.keep,
			drift:       c.drift,
		})
	}
	for i, src := range fillerSources {
		out = append(out, docBlock{id: fmt.Sprintf("f-%d", i+1), source: src})
	}
	return out
}

// fillerSources are ordinary product strings with no history, so a batch is a
// realistic size rather than five sentences that all happen to be under test.
var fillerSources = []string{
	"Save your changes before leaving this page",
	"Your session will end in five minutes",
	"Search for anything in your library",
	"Invite a teammate by email address",
	"Download the report as a spreadsheet",
	"Two-factor authentication is now enabled",
	"We could not reach the server. Try again.",
	"Drag a file here to upload it",
	"Showing the ten most recent entries",
	"This action cannot be undone",
	"Filter results by date added",
	"Your password was changed successfully",
	"Choose which notifications you receive",
	"Export everything in this collection",
	"No results matched your search",
}

// runBatchedArms measures all three arms over one document.
func runBatchedArms(ctx context.Context, llm aiprovider.LLMProvider, batchSize int) (*BatchedReport, error) {
	doc := buildDocument()
	report := &BatchedReport{
		Blocks:                len(doc),
		TermRulesAtCoordinate: len(abTermRules),
	}
	for _, b := range doc {
		if len(b.keep) > 0 {
			report.Scored++
		}
	}

	for _, arm := range []struct {
		name     string
		withRefs bool
		perBlock bool
	}{
		{ArmBatchedBare, false, false},
		{ArmBatchedRef, true, false},
		{ArmSingleRef, true, true},
	} {
		fmt.Fprintf(stderr, "  arm: %s\n", arm.name)
		res, err := runArm(ctx, llm, doc, arm.name, arm.withRefs, arm.perBlock, batchSize)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", arm.name, err)
		}
		report.Arms = append(report.Arms, *res)
	}

	// What one batched call actually carried, against what governs the
	// collection. Computed over the whole document because a batch of this size
	// is most of it.
	texts := make([]string, len(doc))
	for i, b := range doc {
		texts[i] = b.source
	}
	report.TermRulesSentBatched = len(coreprofile.ScopeTermRules(abTermRules, texts...))
	return report, nil
}

// runArm translates the whole document one way and scores it.
func runArm(ctx context.Context, llm aiprovider.LLMProvider, doc []docBlock, name string, withRefs, perBlock bool, batchSize int) (*ArmResult, error) {
	res := &ArmResult{Name: name, Translations: map[string]string{}}

	size := batchSize
	if perBlock {
		size = 1
	}
	for start := 0; start < len(doc); start += size {
		end := min(start+size, len(doc))
		chunk := doc[start:end]

		texts := make([]string, len(chunk))
		for i, b := range chunk {
			texts[i] = b.source
		}
		segments := prompt.BatchSegments(texts)
		for i, b := range chunk {
			segments[i].Key = b.id
			if withRefs && b.priorTarget != "" {
				segments[i].Prior = &prompt.PriorVersion{Source: b.priorSource, Target: b.priorTarget}
			}
		}

		p := prompt.Translate{
			SourceLocale: "en",
			TargetLocale: "nb",
			VoiceGuide:   coreprofile.RenderVoiceGuideCompact(abVoice),
			// Scoped exactly as the tool scopes it, so the eval measures the
			// prompt production sends rather than one assembled for the eval.
			PreferredTerms: coreprofile.ScopedTermRuleMap(abTermRules, texts...),
		}
		msgs := aiprovider.MessagesFromTurns(p.Batch(segments))

		resp, err := llm.ChatStructured(ctx, msgs, batchSchema())
		if err != nil {
			return nil, err
		}
		res.Calls++
		cached := resp.Usage.CacheCreationTokens + resp.Usage.CacheReadTokens
		res.InputTokens += resp.Usage.InputTokens + cached
		res.CachedInputTokens += cached
		res.OutputTokens += resp.Usage.OutputTokens

		got, err := parseBatchReply(resp.Content)
		if err != nil {
			return nil, fmt.Errorf("call %d: %w", res.Calls, err)
		}
		for i, b := range chunk {
			text := got[segments[i].ID]
			res.Translations[b.id] = text
			if len(b.keep) == 0 {
				continue
			}
			if containsAny(text, b.keep) {
				res.Kept++
			} else {
				res.Drifted++
			}
		}
	}
	return res, nil
}

// batchSchema constrains the reply to one translation per segment id, the way
// the tool's batch path does.
func batchSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name: "translations",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"translations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
						},
						"required":             []string{"id", "text"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"translations"},
			"additionalProperties": false,
		},
	}
}

func parseBatchReply(raw string) (map[string]string, error) {
	var reply struct {
		Translations []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &reply); err != nil {
		return nil, fmt.Errorf("unmarshal reply %.160q: %w", raw, err)
	}
	out := make(map[string]string, len(reply.Translations))
	for _, tr := range reply.Translations {
		out[tr.ID] = strings.TrimSpace(tr.Text)
	}
	return out, nil
}
