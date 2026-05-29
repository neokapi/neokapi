---
id: 019-correction-learning-loop
sidebar_position: 19
title: "AD-019: The correction-learning loop"
---

# AD-019: The correction-learning loop

## Summary

Bowrain turns a team's corrections into versioned, enforced checks. When a
person or an AI assistant corrects content that a brand profile should have
caught, that correction is captured with provenance. Repeated corrections
aggregate into **candidate rules**; a reviewer (or, above a configured
threshold, the platform itself) **promotes** a candidate into the brand
profile, where it becomes a deterministic forbidden-term check enforced on
every future generation. Before a candidate lands, its **blast radius** over
existing content is computed and shown. After it lands, aggregate compliance is
monitored and a **drift** alert fires when it falls. This is the closed loop:
correct once, and the rule that prevents the mistake from recurring is authored,
versioned, and shared — the way adding a regression test stops a fixed bug from
returning.

The detection and scoring primitives live in the framework (`core/brand`,
`core/check`); bowrain supplies the durable governance layer (decisions,
versioning, provenance, autonomy, drift events) and the surfaces that drive it
(REST, MCP, web UI).

## Context

A brand or style profile is only as good as its rules, and the rules a team
most needs are the ones it discovers by correcting real output — the phrasings
it keeps changing, the competitor names that slip in, the terms it has decided
against. Those corrections are normally lost to chat history or a reviewer's
memory. Competing tools feed corrections back as prose appended to a prompt,
which is neither deterministic, nor versioned, nor shared across tools. The
durable, valuable form of a correction is a **check**: a rule that fails
predictably and can be enforced in CI, in the editor, and through any AI tool a
team uses.

Two properties make this safe to automate. First, a candidate must be
**reviewable before it is enforced** — a new rule can flag a great deal of
existing content, so the impact is shown first. Second, the loop must be
**progressive** — a workspace starts with every promotion under human review and
widens autonomy only as it learns to trust the candidates.

## Decision

### The model

- A **correction** records the original text, the corrected text, the brand
  dimension, and provenance (who/when, the finding it answered). Corrections are
  the raw signal; they are never enforced directly.
- A **candidate rule** is derived live from the correction stream by aggregating
  repeated `(original, corrected, dimension)` corrections above a minimum count
  (`GetSuggestedRules`). Candidates are not stored — they are recomputed from
  corrections, so they always reflect current evidence.
- A **decision** is the durable record of what a team chose about a candidate
  for a profile: `pending` (the implicit state — no decision yet), `approved`,
  `rejected`, or `promoted`. A rejection suppresses the term from future
  candidate lists; a promotion records the exact profile version the rule landed
  in, and whether a human or the autonomy threshold promoted it. Decisions are
  keyed by `(profile_id, term)` and matched case-insensitively, mirroring how the
  vocabulary matcher compares terms.
- **Promotion** appends the candidate to the profile's vocabulary as a forbidden
  term whose replacement is what the team corrected it to, bumps the profile
  version (archiving the prior version for audit and rollback), and records the
  decision. From that point the rule is a deterministic check
  (`core/brand.MatchVocabulary`) on every generation.

### Blast radius

Before a candidate is promoted, `core/brand.EvaluateBlastRadius` runs the
profile's vocabulary checks over the project's stored content with and without
the candidate and reports the impact: how many blocks the rule would newly flag
(`new violations`), how many it resolves, how many blocks improve or degrade,
the count of new critical violations, and a per-item breakdown. The reviewer
sees what a change will do before it lands; nothing is persisted by the preview.

### Progressive autonomy

Each profile carries an autonomy setting (`AutoPromoteAtCount`). It starts at
zero — every promotion is manual. When a workspace raises it, a correction that
pushes its term's count to the threshold promotes the rule automatically,
recording the decision as `auto` and announcing it. Autonomy is opt-in and
per-profile, so a team widens it dimension by dimension as confidence grows.

### Drift

Promotion enforces a rule going forward, but aggregate compliance can still
erode as content and generators change. `core/brand.AnalyzeDrift` splits a
project's daily score trend into a recent window and a preceding baseline, takes
the count-weighted average of each (so a low-volume day cannot dominate), and
flags drift when the recent average falls below an absolute floor or drops
materially from baseline. A scheduled drift check publishes `brand.voice.drift`
so automations and notifications react.

### Events

The loop is observable on the event bus: `brand.voice.rule_promoted`,
`brand.voice.rule_rejected`, `brand.voice.rule_auto_promoted`, and
`brand.voice.drift`. Automations subscribe to these to notify reviewers, open
tasks, or trigger a re-check.

### Surfaces

The loop is drivable three ways, all over the same `core/brand` primitives:

- **REST** — under `/:ws/brand-profiles/:id`: `GET /candidates` (candidates
  joined with decisions), `POST /promote-rule`, `POST /reject-rule`,
  `POST /evaluate-rule` (blast radius); and under `/:ws/:id/brand-voice/:ref`:
  `GET /drift` and `POST /drift-check`.
- **MCP** — `get_suggested_rules`, `promote_rule`, and `evaluate_rule`, so an AI
  assistant participates in the loop: it can read what its own corrections have
  surfaced, preview the impact, and promote.
- **Web** — a review surface lists pending candidates with the corrections
  behind them, shows a candidate's blast radius on demand, and offers promote or
  reject; promoted and rejected candidates move to history, and drift surfaces as
  an alert on the brand dashboard.

### Storage

The loop's tables are part of the brand schema baseline (SQLite for local and
single-tenant; PostgreSQL for the platform): `brand_voice_corrections`,
`brand_rule_decisions`, `brand_profile_versions` (immutable snapshots), and the
`autonomy` setting on `brand_profiles`. Both backends implement the full store
interface; the platform is pre-launch, so the schema is expressed as one clean
baseline rather than an incremental history.

## Consequences

- A team's most valuable rules — the ones it discovers by correcting — become
  durable, versioned, shared checks rather than ephemeral prompt prose.
- Single-language governance becomes real: the rules are grounded in a team's own
  corrections, not arbitrary weights, so the loop authors the rules a non-expert
  team could not write itself. Multilingual is where the value compounds, because
  one promoted rule is enforced across every locale the profile covers.
- Enforcement stays deterministic and reviewable: a promotion is a versioned,
  reversible change with its impact shown first, and autonomy is opt-in.
- The framework stays platform-agnostic — the detection and scoring live in
  `core/brand`/`core/check`; bowrain owns only the governance and surfaces.
