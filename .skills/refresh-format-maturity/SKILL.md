---
name: refresh-format-maturity
description: >-
  Audit a neokapi format against the maturity rubric, assign it an L0-L4 level,
  list the ranked gaps to the next level, and sweep the upstream Okapi GitLab
  tracker + tests for new fixes to backport. Use when the user asks to "audit
  <format>", "check <format>'s maturity", "is <format> rock-solid?", "harden
  <format>", "is our <format> support solid?", or "check Okapi for new fixes to
  port". Run it periodically as the system evolves to keep formats up to scratch.
---

# Refresh Format Maturity

Score one format against the rubric in
[`docs/internals/format-maturity.md`](../../docs/internals/format-maturity.md),
find what is missing, and check upstream Okapi for fixes worth backporting. The
engine background it assumes is
[`docs/internals/format-engineering.md`](../../docs/internals/format-engineering.md).

## When to use

A periodic, per-format health check — not a build task. It produces: a level, a
ranked gap list, divergence/xfail hygiene findings, and backport candidates. To
*build* a new format use the `implement-format` skill instead.

**For one format, use this skill. For all formats at once**, trigger the
`format-triage` workflow (`.claude/workflows/format-triage.js`) — it scores every
format, ranks the work toward a target level, optionally remediates, and
refreshes the `/format-maturity` dashboard
(`web/static/data/format-maturity.json`). This skill is the interactive,
single-format counterpart that also sweeps the Okapi tracker.

## Step 1 — First pass (deterministic)

Run the bundled helper for a fast file-signal score and the Okapi tracker query:

```bash
python3 .skills/refresh-format-maturity/scripts/audit-format.py <id>
```

It reports file presence (reader/writer/config/schema/spec/parity/testdata + test
kinds), whether `ApplyMap` rejects unknown keys, the Okapi counterpart (if any),
a coarse L-level estimate, and the ready-to-run GitLab tracker query. Exclude
`exec`/`jsx`/`memorytest` — they are not real formats.

## Step 2 — Real scoring (read the assertions)

The helper only sees file presence. Now **read the test bodies** and score the 9
rubric dimensions `none`/`partial`/`complete`/`na`:

- Do `roundtrip_test`/`skeleton_test` assert byte/semantic **equality**, or just
  "no error"? Equality is required for credit.
- Is there a `malformed_test` asserting a clean `Error` + `NotPanics`? (Run with
  `-race`.)
- For harvest formats, are `invariants_test` + `corpus_test` real, and is there a
  prose `okapi_skip_test`?

Run the targeted tests: `go test ./core/formats/<id>/...` and, if the format has
a parity counterpart, `make parity-sandbox` then
`cd cli && go test -tags parity -count=1 -run TestParity<Id>Spec ./parity/formats/`.

Assign exactly one level (the strictest unmet criterion caps it) and list the
ranked blocking gaps to the next tier.

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

Produce: the assigned **level**; the **gap list ranked by rubric weight**;
**backport candidates** from the Okapi sweep; and **stale annotations/xfails** to
clean up. If asked, apply the safe fixes (port a missing test, converge a
default-diff, remove a now-passing xfail) and re-run the relevant tests.

## Footguns

- Never gloss a FAIL as "pre-existing" — investigate it (pdf #617, xliff2 #560,
  archive #504, openxml RunFonts are the known tracked-open ones).
- Dashboard JSON are caches, not live results — regenerate.
- golangci-lint silently under-reports without `icu4c` on `PKG_CONFIG_PATH`;
  parity needs the `fts5` tag + the sandbox.
- The Okapi tracker is GitLab; the local clone is one pinned version — "fixed
  upstream" means relative to 1.48.0.

## References

- Rubric + levels + audit procedure:
  [`docs/internals/format-maturity.md`](../../docs/internals/format-maturity.md)
- Engine + Okapi mapping + tracker recipe:
  [`docs/internals/format-engineering.md`](../../docs/internals/format-engineering.md)
- `scripts/contract-audit/main.go`, `cli/parity/`,
  `core/formats/<id>/parity-annotations.yaml`
