---
name: refresh-format-maturity
description: >-
  Audit a neokapi format against the multi-axis maturity rubric, assign its
  five-axis vector (Engine L0-L4, Vocabulary V0-V3, Editor E0-E4, Knowledge
  K0-K3, Corpus C0-C3), list the ranked gaps per axis, and sweep the upstream
  Okapi GitLab tracker + tests for new fixes to backport. Use when the user
  asks to "audit <format>", "check <format>'s maturity", "is <format>
  rock-solid?", "harden <format>", "is our <format> support solid?", or "check
  Okapi for new fixes to port". Run it periodically as the system evolves to
  keep formats up to scratch.
---

# Refresh Format Maturity

Score one format against the rubric in
[`docs/internals/format-maturity.md`](../../docs/internals/format-maturity.md) —
one grade on each of the **five axes** (Engine, Vocabulary, Editor, Knowledge,
Corpus; the audit procedure is §4 there) — find what is missing per axis, and
check upstream Okapi for fixes worth backporting. The engine background it
assumes is
[`docs/internals/format-engineering.md`](../../docs/internals/format-engineering.md).

## When to use

A periodic, per-format health check — not a build task. It produces: a
per-axis level vector, a ranked gap list, divergence/xfail hygiene findings,
and backport candidates. To *build* a new format use the `implement-format`
skill instead.

**For one format, use this skill. For all formats at once**, trigger the
`format-triage` workflow (`.claude/workflows/format-triage.js`) — it scores
every format across the five axes, ranks the work toward the targets,
optionally remediates, and refreshes the `/format-maturity` dashboard
(`web/static/data/format-maturity.json`). This skill is the interactive,
single-format counterpart that also sweeps the Okapi tracker.

## Step 1 — First pass (deterministic)

Run the bundled helper for a fast file-signal score and the Okapi tracker query:

```bash
python3 .skills/refresh-format-maturity/scripts/audit-format.py <id> \
  --ledger docs/internals/format-ops-ledger.json
```

It reports file presence (reader/writer/config/schema/spec/parity/testdata + test
kinds), whether `ApplyMap` rejects unknown keys, the Okapi counterpart (if any),
a coarse Engine estimate, **a per-axis `base..ceiling` band for all five axes**
(JSON mode adds the full `axes:{…,signals}` block), and the ready-to-run GitLab
tracker query. The `--ledger` flag unlocks the ledger-dependent signals
(citations/context-pack on Knowledge, acceptance/sweep on Corpus, mutation-check
status for remediation-added tests); without it those report unknown. A missing
`vocabulary.yaml`/`dossier.yaml`/`corpus.yaml` is the **zero floor** for that
axis (V0/K0/C0), not an error. Exclude `exec`/`jsx`/`memorytest` — they are not
real formats.

## Step 2 — Real scoring (five axes; read the assertions)

The helper only sees file/artifact floors. Now score **all five axes** per the
procedure in
[format-maturity.md §4](../../docs/internals/format-maturity.md#4-how-to-score-a-format-audit-procedure),
each dimension `none`/`partial`/`complete`/`na`. Quality demotions need a
`file:line`/`TestName` citation or they are dropped.

**Engine (L0–L4)** — the nine v2 dimensions, unchanged (`docs`/`detection`
remain floor constants; the real documentation signals are measured on the
Knowledge axis). **Read the test bodies**:

- Do `roundtrip_test`/`skeleton_test` assert byte/semantic **equality**, or just
  "no error"? Equality is required for credit.
- Is there a `malformed_test` asserting a clean `Error` + `NotPanics`? (Run with
  `-race`.)
- For harvest formats, are `invariants_test` + `corpus_test` real, and is there a
  prose `okapi_skip_test`?

Run the targeted tests: `go test ./core/formats/<id>/...` and, if the format has
a parity counterpart, `make parity-sandbox` then
`cd cli && go test -tags parity -count=1 -run TestParity<Id>Spec ./parity/formats/`.

**Vocabulary (V0–V3)** — open `vocabulary.yaml` and spot-check the evidence the
audit resolved: do the cited tests prove the reader emits the claimed canonical
types (`fmt:*`/`link:*`/`media:*`/`code:*`), and (for V2) do the `write`-cell
tests *author from canonical* rather than echo what was read? (`writecells` is
the axis's one quality dimension — demote-only, citation required.)

**Editor (E0–E4)** — `core/formats/integrations.yaml` *declares* depth; the
audit's probes *corroborate* it; the floor is min(declared, probed) on HEAD.
Verify each declared entry's evidence resolves (PreviewBuilder /
`path:TestName` anchor round-trip test / committed add-in manifest / webhook
handler symbol). No entry = E0; there are no editor quality dims to judge.

**Knowledge (K0–K3)** — open `dossier.yaml` (spec sources catalog-registered in
`specs/catalog.yaml`?); run
`node scripts/format-ops/check-citations.mjs <id>`; check `divergence_kind`
coverage over every `expected_fail` and that reference docs regenerate clean;
for K3 try `node scripts/format-ops/context-pack.mjs <id>`.

**Corpus (C0–C3)** — verify the manifest:
`node scripts/format-ops/gen-corpus-manifest.mjs <id> --check`, and spot
re-fetch one `origin: url` entry; confirm Tier B fetch wiring skips-not-fails;
check the latest acceptance CI conclusion and corpus-sweep record. The `corpus`
quality dimension (real-vs-synthetic) is **shared**: one cited judgment feeds
both the Engine gate and the Corpus gate.

Assign exactly one level **per axis** (the strictest unmet criterion caps each
axis independently) and list the ranked blocking gaps to each axis's next
level.

## Step 3 — Divergence / xfail hygiene

Open `spec.yaml` and `core/formats/<id>/parity-annotations.yaml`. For **every**
`expected_fail`:

1. `divergence_kind` is set;
2. it is **not** a `native-bug` (if it is → fix the bug, don't document it);
3. it is **not** a pure `default-diff` (→ converge with explicit config in
   `bridge_config`, don't xfail);
4. it cites the format spec **and** the Okapi class/method.

Flag any `expected_fail` whose runner now logs that assertions pass (a stale
xfail to remove) — that log is the only safety net. Treat
`parity-annotations.yaml` reason text as suspect until re-verified against a live
`PARITY_DUMP` (it has been stale/inverted before).

## Step 4 — Sweep upstream Okapi for fixes

The tracker is **GitLab, not GitHub** (`gh` does not apply). Project
`okapiframework/Okapi` == id `62298414`; the local clone is pinned to **1.48.0**.
(Fleet-wide cadence for this — Okapi issues/tags plus every dossier `watch`
feed — is owned by the `upstream-watch` ritual in
[format-ops.md §3](../../docs/internals/format-ops.md); this step is the
interactive per-format slice, so don't update the ritual's ledger watermarks
from here.)

```bash
# Issues mentioning this format (public, no auth):
curl -s "https://gitlab.com/api/v4/projects/62298414/issues?search=<id>&state=all&per_page=50"
# Open bugs, most-recently-updated first:
curl -s "https://gitlab.com/api/v4/projects/62298414/issues?labels=bug&state=opened&order_by=updated_at&per_page=50"
# Browse one:  https://gitlab.com/okapiframework/Okapi/-/issues/<iid>
# or:          glab issue list -R okapiframework/Okapi --search "<id>"
```

Cross-reference: issue numbers are embedded in the Okapi checkout's comments and
fixture names (`grep -rn "issue_" /Users/asgeirf/src/okapi/Okapi/okapi/filters/<format>`),
and in this repo's annotations. Report: any `okapi-bug` xfail now fixed upstream
(candidate to un-xfail), and new `@Test` cases worth porting (harvest them with
`scripts/okapi-test-scan`). If offline, say so and report from the local clone
only.

## Step 5 — Drift check

`make contract-audit` for the filter: confirm `okapi_refs` resolve in the pinned
Surefire output and that config keys match the bridge schema. **Dashboards are
stale caches** — regenerate before trusting them; a bridge that stayed
byte-stable while the tier moved means native regressed. Note that native
round-trip defaults to `TierDivergent` unless a per-format `MinTier` is set, so
the absence of red is **not** proof of parity.

## Step 6 — Report

Produce: the assigned **level per axis** (the five-grade vector, alongside the
format's declared tier from `core/formats/support.yaml`); the **gap list
ranked by tier impact** — gaps on the tier-**gating** axes first (Supported
requires Engine ≥ L3 ∧ Corpus ≥ C2 ∧ Knowledge ≥ K2; Maintained requires
Engine ≥ L2), then non-gating axis gaps by rubric weight; **backport
candidates** from the Okapi sweep; and **stale annotations/xfails** to clean
up. If asked, apply the safe fixes (port a missing test, converge a
default-diff, remove a now-passing xfail, fill an `unknown` vocabulary cell
with resolved evidence) and re-run the relevant tests. Tier changes themselves
are **never** applied here — they are `tier-review` proposals into the ops
ledger's `pending[]` queue.

## Footguns

- Never gloss a FAIL as "pre-existing" — investigate it (pdf #617, xliff2 #560,
  archive #504, openxml RunFonts are the known tracked-open ones).
- **Change control** (rubric §3): a remediation that improves any score may not
  edit the scorer (`format-triage.js`), `audit-format.py`, `repro-check.mjs`,
  the rubric, `constructs.yaml`, or `integrations.yaml` in the same change —
  fix the format, not the gate.
- Evidence resolves or it doesn't count: an unresolvable `vocabulary.yaml`
  citation degrades the cell to `unknown`; a remediation-added test scores only
  `partial` until its mutation check (broke-the-reader + red output) is in the
  ledger's `runs[].evidence`.
- Dashboard JSON are caches, not live results — regenerate.
- golangci-lint silently under-reports without `icu4c` on `PKG_CONFIG_PATH`;
  parity needs the `fts5` tag + the sandbox.
- The Okapi tracker is GitLab; the local clone is one pinned version — "fixed
  upstream" means relative to 1.48.0.

## References

- Rubric: tiers, five axes, scorer rules, audit procedure (§4):
  [`docs/internals/format-maturity.md`](../../docs/internals/format-maturity.md)
- Operating process — rituals, cadences, the ops ledger:
  [`docs/internals/format-ops.md`](../../docs/internals/format-ops.md)
- Engine + Okapi mapping + tracker recipe:
  [`docs/internals/format-engineering.md`](../../docs/internals/format-engineering.md)
- Axis artifacts: `core/formats/<id>/{vocabulary,dossier,corpus}.yaml`,
  registries `core/formats/{constructs,support,integrations}.yaml`,
  `specs/catalog.yaml`
- Axis checkers: `scripts/format-ops/{check-citations,context-pack,gen-corpus-manifest,check-support-gates}.mjs`
- `scripts/contract-audit/main.go`, `cli/parity/`,
  `core/formats/<id>/parity-annotations.yaml`
