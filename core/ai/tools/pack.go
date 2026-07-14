package tools

import (
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// How many blocks go in one LLM call — and why it is not a number a user should
// have to pick.
//
// The old answer was a fixed 100, exposed as a form field. Both halves were
// wrong.
//
// A fixed *count* is the wrong unit. 100 blocks is 300 tokens of UI strings or
// 200k tokens of prose; the same number means wildly different requests. And
// because translation output tracks input size, a batch too large to *emit*
// comes back truncated — which, under a JSON schema, means invalid JSON and a
// dead run. The unit that actually bounds a batch is tokens of output.
//
// And it is not a decidable question for a user. The right value depends on the
// model's output ceiling, the length of their particular segments, and a
// quality-vs-batch-size curve that nobody has published. A field defaulting to
// 100 does not give someone control; it gives them a way to break their run.
//
// So kapi packs the batch itself, against two bounds:
//
//   - a token budget derived from what the model can actually emit, and
//   - a ceiling on the number of segments, because quality degrades with batch
//     size independently of length.
//
// On the ceiling: batch-prompting studies find accuracy dropping measurably by
// N≈16 and sharply past it, and the failure mode is not worse wording — it is
// dropped, merged and renumbered items. That is a correctness failure in a
// localization pipeline, not a quality one. MaxBlocksPerCall is deliberately set
// at the conservative end of that evidence, pending our own measurement on
// translation specifically (no published quality-vs-N curve for segment
// translation exists — see the eval issue).
const (
	// MaxBlocksPerCall bounds how many blocks may share one LLM call.
	MaxBlocksPerCall = 16

	// OutputBudgetFraction is the share of the model's usable output budget a
	// batch's translations may be estimated to need. The headroom absorbs the
	// things the estimate cannot see: a target language that runs longer than
	// the source (German, Finnish), the JSON scaffolding around each segment,
	// and tokenizer drift — Anthropic's newer models emit roughly 30% more
	// tokens for the same text than their predecessors.
	OutputBudgetFraction = 0.5

	// MinOutputBudgetTokens keeps a tiny or unknown ceiling from collapsing the
	// budget to zero, which would pack one block per call forever.
	MinOutputBudgetTokens = 512
)

// packBlocks groups entries into batches that a model can actually answer.
//
// Two bounds, whichever binds first: the estimated output tokens of the batch
// must fit the budget, and the batch must not exceed maxBlocks. A single block
// that alone exceeds the budget still gets its own call — refusing to translate
// it would be worse than asking for more than the ceiling and letting the
// truncation retry handle it.
func packBlocks(entries []blockEntry, budgetTokens, maxBlocks int) [][]blockEntry {
	if maxBlocks < 1 {
		maxBlocks = 1
	}
	if budgetTokens < MinOutputBudgetTokens {
		budgetTokens = MinOutputBudgetTokens
	}

	var (
		batches [][]blockEntry
		cur     []blockEntry
		curCost int
	)
	flush := func() {
		if len(cur) > 0 {
			batches = append(batches, cur)
			cur, curCost = nil, 0
		}
	}

	for _, e := range entries {
		cost := estimateTokens(e.sourceText)
		// Close the current batch before it would overflow either bound. A block
		// that busts the budget on its own lands in a batch of one.
		if len(cur) > 0 && (len(cur) >= maxBlocks || curCost+cost > budgetTokens) {
			flush()
		}
		cur = append(cur, e)
		curCost += cost
	}
	flush()

	return batches
}

// outputBudget reports the token budget for one call to p: a fraction of what
// the model can emit on a blocking request.
func outputBudget(p aiprovider.LLMProvider) int {
	ceiling := aiprovider.LimitsOf(p).EffectiveMaxOutputTokens()
	budget := int(float64(ceiling) * OutputBudgetFraction)
	if budget < MinOutputBudgetTokens {
		return MinOutputBudgetTokens
	}
	return budget
}

// estimateTokens approximates a text's token count.
//
// Deliberately crude (chars/4) and deliberately not load-bearing: it decides how
// optimistically we pack, never whether output is correct. A bad estimate costs
// an extra round-trip via the truncation retry; it cannot corrupt a translation.
// Tokenizing exactly would mean shipping a tokenizer per provider and would
// still be wrong the day a model changes its own.
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(text)/4 + 1
}
