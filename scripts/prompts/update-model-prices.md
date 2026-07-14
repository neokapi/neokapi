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
  - **AWS Bedrock** (`bowrain/ai/bedrock`) — the route the platform runs on in
    production. Do not read these off a web page: AWS serves them over the Pricing
    API, which is exact and scriptable. The catch is the service code — the rates
    live under `AmazonBedrockFoundationModels`, **not** `AmazonBedrock` (which has
    no model rates at all and sent me on a long detour). The model is identified by
    `servicename`, not by `usagetype`, which is shared across every model:

    ```bash
    aws pricing get-products --region us-east-1 \
      --service-code AmazonBedrockFoundationModels \
      --filters Type=TERM_MATCH,Field=regionCode,Value=eu-north-1 \
               'Type=TERM_MATCH,Field=servicename,Value=Claude Sonnet 4.6 (Amazon Bedrock Edition)'
    ```

    The `EUN1-MP:` prefix on the usage types is AWS Marketplace, which is how the
    Anthropic models are bought. Take `*_InputTokenCount-Units` and
    `*_OutputTokenCount-Units` (plain on-demand; not `_Batch`, not `_Reserved`).

    Mind the profile. A `eu.`-prefixed inference profile is a **regional/geo**
    profile and carries AWS's 10% premium over the otherwise identical `global.`
    one — geo is exactly global × 1.1 ($3.30/$16.50 against $3.00/$15.00 for Sonnet
    4.6). Price the profile the code actually calls, not the cheaper sibling.
  - **claude-code** — a subscription. Not billed per token, and its token counts do
    not describe an API call either (the CLI bills its own agent system prompt as
    cache creation). Keep `metered: false` so no cost is computed.
