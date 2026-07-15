Refresh `providers/ai/models.json` — the curated catalog of models kapi supports —
against what the providers actually serve today.

The catalog is the single source of truth: the provider defaults, the output-token
budgeting, and the /models page all read from it. It is curated, not generated,
because the lifecycle fields it carries (introduced, superseded, retirement) are
not in any provider API. Your job is to keep the curation honest.

## What to do

1. Run the drift alarm to see where the catalog and reality disagree:

   ```
   go run ./scripts/modelcheck -candidates
   ```

   It needs `GEMINI_API_KEY`, `ANTHROPIC_API_KEY`, and `OPENAI_API_KEY` to reach
   the live lists (a provider with no key is skipped, not assumed empty). It
   reports two things:
   - **GONE** — a catalogued model the provider no longer serves. This is the
     important one: a model kapi still lists as active (or defaults to) is a 404 on
     a user's first call.
   - **candidates** — live models no catalog entry matches. Most are noise
     (embeddings, TTS, dated snapshots kapi will never translate with); a few are
     genuinely new models worth adopting.

2. For each **GONE** model, **remove its entry** from the catalog. The catalog
   holds only models kapi supports; a dead one is deleted, not kept as a tombstone.
   (If the vendor has announced a *future* retirement for a model that is still
   served, leave the entry and set `retirement_date` to the announced date — a
   warning, not a removal.)

3. For a genuinely new model worth supporting, add an entry: `id` (the family
   prefix), `provider`, `label`, `max_output_tokens` and `context_window` (from the
   vendor's model card), `status: "active"`, and `introduced` set to today. If it
   replaces an existing model, set the old one's `status: "superseded"` and
   `superseded_by` to the new id. If the new model is capable-but-premium (a top-tier
   or reasoning model, dear and overkill for translation) or tuned for other tasks
   (creative writing), add `"recommended": false` so it lands under "Advanced" rather
   than the primary list — it stays fully usable, just not advertised as a default
   choice. Leave the field off for an everyday workhorse. Never mark a provider's
   current default non-recommended; a test rejects it.

4. When you change a provider's current best model, update the corresponding
   `DefaultXModel` constant in `providers/ai/<provider>.go` too, and add
   `"default_for": ["<provider>"]` to the new default's catalog entry (removing it
   from the old one). The test `TestEveryProviderDefaultIsCatalogued` enforces that
   they agree.

5. Set the top-level `as_of` to today.

6. Regenerate the reference dataset and confirm everything is consistent:

   ```
   go run ./scripts/gen-refs
   go test ./providers/ai/ ./scripts/batcheval/
   make check-reference-docs
   ```

7. Commit `models.json`, the regenerated `packages/reference-data/data/models.json`,
   and any default-constant change together, with the `as_of` date in the message.

## What not to do

- Do not add every model a provider serves. The catalog is a *curated support
  matrix* — the models kapi ships support for — not a mirror of the provider's
  full list. A row's lifecycle fields only mean something for a model we chose to
  adopt.
- Do not fabricate an `introduced` or `retirement_date`. The field is a statement
  about neokapi's support; an unknown date is honest as a blank, misleading as a
  guess.
- Do not price a model you have not catalogued: `TestEveryPriceIsForACataloguedModel`
  fails, and rightly — the /batch-eval dashboard must not quote a cost for a model
  the catalog does not list.
