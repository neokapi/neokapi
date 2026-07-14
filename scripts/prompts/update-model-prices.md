Refresh `scripts/batcheval/prices.json` from the vendors' own published pricing.

These rates are published on the /batch-eval dashboard as a cost per 1,000 words —
a number people budget against. A stale rate presented as a current cost is worse
than showing no cost at all, so accuracy here matters more than completeness.

## What to do

1. Fetch the pricing pages named in the `sources` block of `scripts/batcheval/prices.json`
   (currently Anthropic and Google). Fetch the page — do not answer from memory.
   Model prices change, introductory rates expire, and your training data is not a
   source.

2. For every entry in `models[]`, update `input_per_mtok` and `output_per_mtok` to
   the vendor's **standard, first-party, non-batch, global-endpoint** list price per
   million tokens. Not the batch-API discount, not a cached-read rate, not a
   regional-endpoint premium — the plain rate a normal call is billed at, because
   that is what the eval harness actually does.

3. Set the top-level `as_of` to today's date (YYYY-MM-DD).

4. Where a price is conditional, record the condition in that entry's `note` rather
   than silently picking one number. Real examples to expect:
   - introductory pricing with an end date (Claude Sonnet 5 is $2/$10 through
     2026-08-31, then $3/$15);
   - long-prompt tiers (Gemini pro charges more above 200k input tokens).

5. Run `go run ./scripts/modelcheck` and reconcile the result. It lists what each
   provider actually serves today, and fails on anything kapi pins or prices that no
   longer exists. If a priced model has been retired, remove it — do not leave a
   price for a model nobody can call. (`gemini-3-pro-preview` retired mid-benchmark
   and started answering 404 "no longer available"; that is the case this guards.)

6. Re-price the recorded history so the dashboard reflects the new rates:

   ```
   go run ./scripts/batcheval -recost web/src/pages/batch-eval/_batcheval.json
   ```

   Note what this does: it restates *past* runs at *today's* rates. That is correct
   when a rate in the table was wrong, and misleading when a vendor has genuinely
   changed its list since the run — in that case the old run really did cost what it
   cost. Each run records the `as_of` date of the rates it was priced with, so the
   distinction is visible; if in doubt, leave historical runs alone and let new runs
   pick up the new rates.

7. Run `go test ./scripts/batcheval/` and commit `prices.json` on its own, with the
   `as_of` date in the message.

## What not to do

- Do not invent a rate for a model whose price you could not find. Leave it out.
  An unpriced model shows a blank on the dashboard, which is honest; a guessed one
  is a number someone may put in a budget.
- Do not price a route against another route's rates. The same weights cost
  different amounts depending on how they are reached, and the routes are not
  interchangeable:
  - **first-party APIs** (Anthropic, OpenAI, Gemini, Azure OpenAI) — the rates on
    the vendors' own pricing pages;
  - **AWS Bedrock** (`bowrain/ai/bedrock`) — the route the Bowrain platform runs on
    in production. Priced independently by AWS, and the cross-region inference
    profiles the platform uses (`eu.anthropic.…`) are priced separately again. Get
    these from <https://aws.amazon.com/bedrock/pricing/>, **not** from Anthropic's
    page and **not** from the AWS Pricing API, which lags badly — as of 2026-07 its
    `model` attribute still tops out at "Claude 3 Sonnet" and has no entry for the
    Claude 4.6 models in eu-north-1. That is why `bedrock:` has no rates yet;
    adding correct ones is the single most valuable thing this refresh can do,
    because it is the only route whose cost describes what Bowrain actually spends.
  - **claude-code** — a subscription. Not billed per token, and its token counts do
    not describe an API call either (the CLI bills its own agent system prompt as
    cache creation). Keep `metered: false` so no cost is computed.
