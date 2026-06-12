# tier-review — tier-change, demotion, and countersign proposals

## Purpose

The only path by which the **promise** (support tiers) changes. Turns the
audit vector's suggestions, certification decay, and `na`-countersign
requests into `pending[]` **proposals**; applies approved ones to
`core/formats/support.yaml` + `docs/internals/format-demotions.md`.

## Due when (watermark-only — `cadence_days: 0`, never due by time alone)

- the published vector suggests a promotion (a format's gating axes exceed
  its declared tier); or
- certification decayed (`last_certified` > 120 days → the dashboard shows
  the decayed tier and this ritual becomes due; > 45 days is only a stale
  badge, cleared by triage-score); or
- countersign requests (`na` claims on tier-gating criteria) are pending; or
- `support.yaml` changed outside this ritual (`watermarks.support_sha`).

## Inputs

- `core/formats/support.yaml` — per format: `tier`, `tier_since`,
  `last_certified`, `gates`, optional `grandfathered`, `notes`.
- Latest dashboard (`web/static/data/format-maturity.json`) — per-row axis
  levels + `tier:{…}` fields.
- Gating requirements (format-maturity.md §1):
  **Supported** = Engine ≥ L3 ∧ Corpus ≥ C2 ∧ Knowledge ≥ K2, gates wired in
  `parity.yml`/`format-acceptance.yml`; **Maintained** = Engine ≥ L2, package
  tests + malformed suite; **Available** = Engine ≥ L0, registration tests.
- Validators: `node scripts/format-ops/check-support-gates.mjs` and
  `TestSupportYAML` in `core/formats/maturity_test.go`.

## Steps

1. **Sweep for triggers**: compare every row's vector vs declared tier
   (promotion suggestions and under-runs), compute decay from
   `generated_at − last_certified`, list pending `na` claims, and diff
   `support.yaml` against the watermark.
2. For each trigger, write a `pending[]` **proposal** (never apply
   directly): `{id, ritual: "tier-review", type:
   "tier-change"|"demotion"|"na-countersign", proposal, evidence, created}`.
   Evidence = the dashboard row + the gate-wiring proof (a Supported
   proposal must name CI workflows that exist and exercise the format — a
   tier not enforced by CI is marketing).
3. **Demotion rules** (when proposing): announce before the release; record
   the reason in `docs/internals/format-demotions.md`; drop at most **one
   tier at a time** (Supported → Maintained → Available → plugin/retired);
   demotion is never deletion — retirement archives the corpus and dossier
   intact (`mo` is the standing candidate). Retirement to plugin tier is a
   product decision, not an audit outcome.
4. **Grandfathered lifting**: after a format's first multi-axis publish,
   propose lifting `grandfathered: true` (or adjusting the tier), format by
   format.
5. **`na` countersigns**: an `na` on a tier-gating criterion is valid only
   as a countersigned state — the claim lives in the relevant artifact with
   `reviewed_by` + date, applied through this ritual, never a bare
   self-declaration.
6. **Apply approved items** (this run or a later one): edit `support.yaml` —
   this ritual writes `tier`/`tier_since`/`gates`/`notes`;
   **`last_certified` belongs to triage-score; no other writer exists** —
   and append to `format-demotions.md` for demotions. Run the validators.

## Verification (→ `runs[].evidence`)

`{check: "check-support-gates", exit, output_sha: sha256(validator output)}`
+ `go test ./core/formats/ -run TestSupportYAML` exit.

## Ledger updates

`last_run`; `watermarks.support_sha` = sha256 of `support.yaml` after apply;
applied pending items removed from `pending[]` with their outcome recorded
in `runs[]`; same commit as the support.yaml/demotions edits.

## Outputs

Pending tier-change/demotion/na-countersign proposals; on approval, updated
`support.yaml` + `format-demotions.md`.

## Failure → blocked

Validator red after an apply → revert the apply, record `blocked` with the
validator output; a tier change that cannot name a real CI gate is rejected
back to proposal stage, never landed on faith.
