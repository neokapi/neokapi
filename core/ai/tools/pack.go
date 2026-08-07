package tools

import (
	"github.com/neokapi/neokapi/core/ai/prompt"
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
// localization pipeline, not a quality one. So MaxBlocksPerCall started at the
// conservative end of that evidence, pending our own measurement on translation
// specifically.
//
// We have now measured it (scripts/batcheval, published at /batch-eval), and the
// borrowed N≈16 does not survive contact with the task. Sweeping a 600-block
// corpus of real localization stressors — ambiguous UI strings, placeholders,
// inline tags, prose — at N = 8…600 against gemini-3.5-flash, gemini-3.1-flash-lite
// and Claude Sonnet 4.6 on Bedrock, structural integrity was 100% at every batch
// size from 32 up, on every model. The only structural damage measured anywhere in
// the sweep was at the *small* end: gemini-3.5-flash broke 14 placeholders at N=8
// and 10 at N=16, and none at all from N=32. More calls means more chances to
// fumble, and less context to fumble in.
//
// The generalization batch-prompting papers make — degradation with N, on
// reasoning and classification tasks — does not transfer to translation, and it is
// not hard to see why: translating segment 300 does not require having reasoned
// correctly about segment 299. The items are independent, and the model is doing
// the one thing it is best at.
//
// What does bite at large N is the output ceiling, and it bites as cost rather
// than as corruption: a batch whose reply exceeds what a blocking call may emit
// (NonStreamingMaxOutputTokens) comes back truncated, splitAndRetry halves it, and
// the work is redone. The eval measured a 600-block batch costing 2.4x the tokens
// and 6x the wall-clock of the same corpus at N=256 — at an unchanged 100%
// integrity, because the retry quietly absorbed it.
//
// Hence the two bounds below, and their relative importance: the token budget is
// the one that matters, and MaxBlocksPerCall is now a backstop rather than the
// binding constraint. It is raised to 64 — comfortably inside the measured-clean
// range, four times fewer calls on catalogs of short UI strings, and still short of
// the point where a reply could not be re-attempted cheaply.
const (
	// MaxBlocksPerCall bounds how many blocks may share one LLM call. A backstop:
	// the output-token budget below almost always binds first.
	MaxBlocksPerCall = 64

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

	// ReplyItemOverheadTokens is what one reply item costs *before* its
	// translation: the JSON scaffolding the schema forces around it —
	// `{"id":"s123","text":"…"},` — braces, field names, quotes, comma, and the
	// echoed id.
	//
	// It is a per-*item* cost, and that is the whole point. Budgeting a batch from
	// its source text alone charges nothing for the number of segments, so a batch
	// of 600 short UI strings — which is mostly scaffolding — asks for a cap smaller
	// than the reply it just requested. The model stops at the cap, the reply is a
	// fragment, and splitAndRetry re-translates the batch in halves. Nothing is
	// corrupted; you simply pay twice. The eval measured that: a 600-block batch cost
	// 2.4x the tokens and 6x the wall-clock of the same corpus at N=256, at an
	// unchanged 100% structural integrity, which is exactly what a self-healing
	// retry looks like from the outside.
	ReplyItemOverheadTokens = 10
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

// batchOutputBudget is the cap to request for a batch's reply: room for a target
// that runs longer than its source, plus the scaffolding of each reply item, plus
// a floor so a batch of very short strings still has somewhere to land.
func batchOutputBudget(segments []prompt.BatchSegment) int {
	need := MinOutputBudgetTokens
	for _, s := range segments {
		need += estimateTokens(s.Text)*2 + estimateTokens(s.ID) + ReplyItemOverheadTokens
	}
	return need
}

// blockOutputBudget is batchOutputBudget for a single block translated on its
// own: the same doubling for a longer target and the same floor, minus the JSON
// scaffolding a bare-text reply does not carry.
//
// The single-block path needs a budget of its own precisely because of how a
// block gets there. packBlocks isolates a block into a batch of one when its
// estimated output exceeds the whole call budget — so the blocks that reach this
// path are the ones already known to be large, and letting them fall back to a
// provider's own default truncates exactly the work that needed more room.
//
// It only ever raises the cap. ConservativeMaxOutputTokens is the floor because
// it is what every built-in provider configures as its default max_tokens, and a
// budget that tightened around a short block would be a new way to truncate one:
// a reasoning model draws its thoughts from the same allowance as its answer, so
// the tokens a short translation needs are not the tokens it must be allowed.
func blockOutputBudget(text string) int {
	need := MinOutputBudgetTokens + estimateTokens(text)*2
	return max(need, aiprovider.ConservativeMaxOutputTokens)
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
