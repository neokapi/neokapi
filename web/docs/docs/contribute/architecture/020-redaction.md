---
id: 020-redaction
sidebar_position: 20
title: "AD-020: Content Redaction"
description: "Architecture decision: redaction replaces sensitive spans with protected placeholders before external translation (using dual ID+text restore and leak-prevention via RunsPlaceholderText), then restores originals via an unredact step."
keywords: [redaction, sensitive content, placeholders, unredact, privacy, architecture decision, neokapi]
---

# AD-020: Content Redaction

## Summary

Redaction replaces sensitive spans — people, unreleased product names,
internal roles, secrets — with protected placeholders before content is sent
to AI translation or to an external translator, and restores the originals
afterwards. The defining property is **locality**: the original value never
leaves the machine. Detection runs offline by default, the replacement carries
only a coarse category, and the original↔placeholder mapping lives in a local
vault that is never serialized into the content handed to a tool, a prompt, or
an exchange file.

The capability is framework-native — a peer of pseudo-translation and
search/replace — and lives under `core/redaction/`, two pipeline tools
(`redact` / `unredact`), a built-in `secure-translate` flow, and `kapi extract`
/ `kapi merge` integration.

## Context

Creators increasingly translate with cloud LLMs and external CAT tools, but
some source content must not be disclosed to either: pre-announcement product
names, named individuals, confidential roles. The requirement is two-sided —
the sensitive text must be absent from anything that leaves the machine, **and**
the finished translation must read naturally with the originals back in place.

A naive find/replace cannot meet this: it either leaks (the mapping travels
with the document) or loses the ability to restore (no record of what was
replaced). The content model already has the right primitive — the
`PlaceholderRun` inline code, whose documentation names redaction as a use case
(AD-002) — and the streaming pipeline already preserves inline codes across
translation (AD-004, AD-011). Redaction builds on both.

## Decision

### Placeholder model

A redacted span becomes a `model.PlaceholderRun` with `Type` of the form
`redaction:<category>` (e.g. `redaction:person`), mirroring the `entity:`
prefix convention of the semantic vocabulary (AD-002). The run carries a stable
ID (the token), a visible stand-in string in `Equiv`/`Data`/`Disp` (default
template `[REDACTED:{category}]`), and constraints that mark it
non-deletable and non-cloneable. Categories are free-form strings; a
recommended set is surfaced in defaults and documentation.

The original text is **not** present on the run in any field.

### The locality guarantee

The original value lives only in a **vault**, never in the content. Two
backings:

- An in-process `SecretAnnotation` on the block for single-run flows. It is
  keyed under a name no format writer serializes, and `unredact` deletes it
  after restoring, so it cannot reach an output file.
- A gitignored JSON **sidecar** (`.kapi/cache/redaction/<batch-id>.json`,
  written `0600`) for the extract → external-translation → merge roundtrip.

For AI translation the guarantee is *structural*, not advisory: a block with
inline codes is sent to the model through `RunsPlaceholderText`, which renders
each placeholder as an opaque `<x id="…"/>` token — the model sees neither the
original nor even the visible label. `ParseRunsPlaceholderText` matches the
token back to the source run by ID on return, so the placeholder survives the
roundtrip with full fidelity.

### Detection

Detection produces `Match` spans (byte offsets + category) consumed by
`redaction.Redact`. Backends:

- **Rules** (default, fully offline): literal terms and regular expressions
  from a dedicated rules file, compiled by `RuleDetector`. Deterministic and
  the only backend that preserves the locality guarantee without qualification.
- **Entities** (opt-in): the `redact` tool reads `model.EntityAnnotation`s
  already on the block — produced upstream by the `ai-entity-extract` tool —
  and redacts the configured entity categories. The detection model is the
  caller's choice; a local model keeps everything on the machine, a cloud model
  trades that for coverage during the *detection* step only. Because an annotator
  can precede the redactor in the flow, `ai-entity-extract` and `redact` sit in
  the same flow (see AD-006).

  The categories are the **option surface** a user picks — "redact people",
  "redact dates", … — via `redact`'s `entityTypes` (person, org, product,
  location, date, time, currency, measurement, role, other; aliases and the
  model `entity:` prefix normalize, validated against `redaction.EntityCategories`).
  Naming any category enables entity detection, so the user doesn't also list the
  `entities` detector. Dates/times/currencies/measurements are excluded from the
  defaults (they usually need locale formatting, not hiding) but are opt-in.

  **Conditional requirement, not a new schema language.** Two distinct
  "requirements" are in play and neither needs a config-condition DSL: the
  *resource* requirement (NER ⇒ an LLM credential) lives statically on
  `ai-entity-extract`; `redact` calls no provider and declares no `Requires`, so
  enabling a category adds no resource requirement to redact — you add the NER
  tool to the flow (composition). The *input* requirement — redact needs an
  entity overlay when entity detection is on — is a **config-derived IO
  contract**: `tools.ResolveRedactContract` (registered via
  `ToolRegistry.SetContractResolver`) flips redact's `entity` consumed port from
  optional to **required** when its config enables entities, so a flow that
  redacts entities with no upstream producer fails `ValidateDataFlow` instead of
  silently leaving the content unredacted (and leaking it downstream). With only
  rule-based detection redact reads no upstream port and the contract is
  unchanged.

`redact` is a **transformer** (AD-006): it produces an edit plan — the
span→replacement edits plus the originals to vault — and the framework applier is
what rewrites the source. Redaction is a structured edit (a known span→replacement
map), so the applier **rebases** the surviving run-anchored source overlays onto
the redacted runs in one pass: a term tag from an upstream annotator follows the
rewrite and still reaches downstream steps, while a span overlapping a redacted
span (including the consumed `entity` spans) is dropped. The applier vaults the
originals as it replaces, so source rewrite and secret capture are atomic.

### Restoration

`unredact` (and `kapi merge`) restore through two complementary paths, because
formats differ in whether they preserve inline structure on write:

- **By placeholder ID** — for structure-preserving carriers: in-process
  pipelines, and XLIFF, where the placeholder is a real `<ph>`/inline element.
- **By visible token text** — for carriers that flatten the placeholder to its
  string on write (JSON, and XLIFF for inline types it does not model). The
  visible token is made unique within a block so the match is unambiguous.

### CLI and recipe surface

- `kapi run secure-translate -i <file> --target-lang <l>` — the in-process flow
  `reader → redact → ai-translate → unredact → writer`.
- `kapi run redact-pii -i <file>` — the built-in NER flow: `ai-entity-extract`
  (detect entities) → `redact` (configured for person/org/location/date). The
  placement pass (AD-006) keeps `redact` ahead of any remote-egress step.
  Equivalent recipe:
  ```yaml
  steps:
    - tool: ai-entity-extract
    - tool: redact
      config:
        detectors: [entities]
        entityTypes: [person, org, location, date]
  ```
- `kapi extract --redact` (or `--redact-rules <path>`) — emits a redacted
  bilingual file and writes the vault sidecar for the batch.
- `kapi merge` — restores originals from the batch sidecar after applying the
  translator's target; `--no-restore` keeps the placeholders.

The recipe declares redaction under `defaults.redaction` (and per content
item), pointing at a separate rules file so the sensitive term list stays out
of the committed recipe:

```yaml
defaults:
  redaction:
    enabled: true
    rules: .kapi/redaction.yaml   # gitignorable
    detectors: [rules]            # opt in: entities
    placeholder: "[REDACTED:{category}]"
```

On `kapi extract`, redaction runs **before** TM pre-fill, so the translation
memory is queried with — and pre-fills targets from — redacted text; no
sensitive value reaches the emitted file by way of a TM match. On `kapi merge`,
the incoming source is always restored (so per-block staleness compares
original-to-original against the re-read source file); the target is restored
unless `--no-restore` is set.

## Relationships to other ADs

- **AD-002 (Content Model)** — redaction is expressed entirely as
  `PlaceholderRun`s; `redaction:<category>` extends the semantic vocabulary.
- **AD-004 (Processing Engine)** — `redact` / `unredact` are ordinary pipeline
  tools; the in-process vault rides the block through the stream.
- **AD-006 (Tool System)** — both tools register with schemas and config
  factories like any other; `secure-translate` is a built-in flow.
- **AD-008 (Project Model)** — `Redaction` is a first-class `Defaults` /
  `ContentItem` field; the sidecar lives under the regenerable cache.
- **AD-011 (AI Providers)** — the `RunsPlaceholderText` placeholder protocol is
  what keeps the original out of the prompt; `ai-entity-extract` feeds the
  optional entities detector.
- **AD-017 (Bilingual Format Interop)** — `--redact` on extract and restore on
  merge slot into the existing bilingual roundtrip without changing its keys.

## Rationale

**Why a `PlaceholderRun`, not text substitution?** Inline codes are already
protected from translation, survive the streaming pipeline, and round-trip
through XLIFF as `<ph>`. Reusing them means the model and CAT tools treat a
redaction exactly as they treat any other do-not-touch token.

**Why is the original never on the run?** So the guarantee is auditable: any
serialized artifact can be scanned for the secret and must not contain it. The
run carries only a category and a token.

**Why dual restoration?** ID-based restore is exact but needs the inline
structure to survive. Plain-text carriers drop it, so a vault-backed text match
on a per-block-unique token is the fallback. Together they cover every
supported carrier.

**Why rules by default, AI opt-in?** Rule-based detection is deterministic and
fully offline — it cannot itself leak. AI detection is more capable but, with a
cloud model, discloses source during detection; making it opt-in keeps the
default trustworthy.

**Why a separate rules file?** The term list is itself sensitive. Keeping it
out of the committed recipe lets it be gitignored while the recipe still
records that redaction is enabled.
