---
id: c-10-redaction
sidebar_position: 10
title: "C-10: Redaction and clearance"
description: "Architecture decision: redaction replaces sensitive spans with protected placeholders before content leaves the machine, keeping the original in a local vault; the policy is read at extract, at ingest, and by the flow placement gate."
keywords: [redaction, sensitive content, placeholders, unredact, vault, privacy, architecture decision, neokapi]
---

# C-10: Redaction and clearance

## Summary

Redaction replaces sensitive spans (people, unreleased product names, internal
roles, secrets) with protected placeholders before content is sent to a model or
to an external tool, and restores the originals afterwards. The defining property
is **locality**: the original value never leaves the machine. Detection runs
offline by default, the replacement carries only a coarse category, and the
original-to-placeholder mapping lives in a local vault that is never serialized
into the content handed to a tool, a prompt, or an exchange file.

The capability is framework-native, a peer of pseudo-translation and
search-and-replace: it lives under `core/redaction/`, as two pipeline tools
(`redact` / `unredact`), a built-in secure-translate flow, and integration with
extract, merge and ingest.

## Context

Content is increasingly translated with cloud models and external tools, but some
source must not be disclosed to either: pre-announcement product names, named
individuals, confidential roles. The requirement is two-sided. The sensitive
text must be absent from anything that leaves the machine, **and** the finished
translation must read naturally with the originals back in place.

A find-and-replace cannot meet this. It either leaks, because the mapping travels
with the document, or it loses the ability to restore, because there is no record
of what was replaced. The content model already has the right primitive, the
placeholder run, whose semantics name redaction as a use case, and the streaming
pipeline already preserves inline codes across translation.

## Decision

### The placeholder model

A redacted span becomes a `model.PlaceholderRun` with a `Type` of the form
`redaction:<category>`, mirroring the `entity:` prefix convention of the semantic
vocabulary ([F-02](../foundations/f-02-content-model.md)). The run carries a
stable id (the token), a visible stand-in string (default template
`[REDACTED:{category}]`), and constraints marking it non-deletable and
non-cloneable. Categories are free-form strings; a recommended set is surfaced in
the defaults.

**The original text is not present on the run in any field.**

### The locality guarantee

The original lives only in a **vault**, never in the content. Three backings, one
per lifetime:

| Backing | Where | For |
| --- | --- | --- |
| An in-process secret annotation on the block | memory | single-run flows |
| A per-batch JSON sidecar | `.kapi/work/cache/redaction/<batch-id>.json` | the extract → external tool → merge round trip |
| The project vault | `.kapi/work/vault/redaction.json` | continuous ingest, which has no batch id to look under |

The in-process annotation is keyed under a name no format writer serializes, and
`unredact` deletes it after restoring, so it cannot reach an output file. The
sidecars and the vault are written with restrictive permissions.

`vault/` is separate from `cache/`. **The cache is defined by being disposable**:
losing it costs CPU. **The vault is defined by an exclusion**: a named
destination must never read it, and losing it means redacted content can never
be restored. Filing it under `cache/` would make it look regenerable, which it
is not, and put it one deletion away from unrecoverable placeholders. So `rm -rf
.kapi/work/cache` stays free, and `vault/` is the one thing under `work/` that
kapi never deletes on its own initiative.

For model calls the guarantee is *structural*, not advisory: a block with inline
codes is rendered through the placeholder protocol, which presents each
placeholder as an opaque token (the model sees neither the original nor even the
visible label), and the return is matched back to the source run by id, so the
placeholder survives the round trip with full fidelity
([M-05](../multilingual/m-05-prompts-and-batching.md)).

### Detection

Detection produces spans, byte offsets plus a category, consumed by
`redaction.Redact`. Two backends:

- **Rules** (default, fully offline): literal terms and regular expressions from
  a dedicated rules file. Deterministic, and the only backend that preserves the
  locality guarantee without qualification.
- **Entities** (opt-in): `redact` reads the entity annotations already on the
  block, produced upstream by `entity-extract`, and redacts the configured
  categories. The detection model is the caller's choice: a local model keeps
  everything on the machine, a cloud model trades that for coverage during the
  *detection* step only.

  The categories are the **option surface** a user picks (redact people, redact
  dates) through `redact`'s entity-type list, with aliases and the model's
  `entity:` prefix normalized and validated. Naming any category enables entity
  detection, so the user does not also have to list the detector. Dates, times,
  currencies and measurements are excluded from the defaults, because they
  usually need locale formatting rather than hiding, but are available.

  **Conditional requirement, not a new schema language.** Two distinct
  requirements are in play and neither needs a condition DSL. The *resource*
  requirement (named-entity recognition implies a credential) lives statically
  on `entity-extract`; `redact` calls no provider and declares no requirement of
  its own, so enabling a category adds none to redact. You add the extraction
  tool to the flow, which is composition. The *input* requirement (redact needs
  an entity overlay when entity detection is on) is a **config-derived IO
  contract**: a contract resolver flips redact's `entity` consumed port from
  optional to required when its config enables entities, so a flow that redacts
  entities with no upstream producer fails data-flow validation instead of
  silently leaving the content unredacted and leaking it downstream. With
  rule-based detection only, redact reads no upstream port and the contract is
  unchanged.

### Redaction is a structured edit

`redact` is a **transformer** ([E-03](../engine/e-03-tool-system.md)): it
produces an edit plan (the span-to-replacement edits plus the originals to
vault), and the framework applier rewrites the source. Because the plan is a
known span-to-replacement map, the applier **rebases** the surviving
run-anchored source overlays onto the redacted runs in one pass: a term
annotation from an upstream tool follows the rewrite and still reaches downstream
steps, while a span overlapping a redacted span, including the consumed entity
spans, is dropped. The applier vaults the originals as it replaces, so source
rewrite and secret capture are atomic.

### Restoration

`unredact` and `kapi merge` restore through two complementary paths, because
formats differ in whether they preserve inline structure on write:

- **By placeholder id**, for structure-preserving carriers: in-process
  pipelines, and XLIFF, where the placeholder is a real inline element.
- **By visible token text**, for carriers that flatten the placeholder to its
  string on write. The visible token is made unique within a block so the match
  is unambiguous.

Together they cover every supported carrier.

### The policy has three readers

The recipe declares redaction under `defaults.redaction`, and per content item,
pointing at a separate rules file so the sensitive term list stays out of the
committed recipe:

```yaml
defaults:
  redaction:
    enabled: true
    rules: redaction.yaml         # binds any path; keep it out of version control
    detectors: [rules]            # opt in: entities
    placeholder: "[REDACTED:{category}]"
```

A policy with one reader is a policy that fails open everywhere else, so the
policy is read at every point content can leave:

1. **`kapi extract`** resolves and applies it, emitting a redacted bilingual file
   and writing the batch sidecar. Redaction runs **before** memory pre-fill, so
   the content memory is queried with redacted text and pre-fills targets from
   it, and no sensitive value reaches the emitted file by way of a memory match.
   On `kapi merge` the incoming source is always restored, so per-block
   staleness compares original against original; the target is restored unless
   `--no-restore` is set.
2. **Ingest**, which is the push to a venue (`host/venue/source`).
   `host.ProjectRedaction` resolves the project's policy and
   `host.RedactAtIngest` applies it as content is read in, vaulting into the
   project vault. Ingest is continuous rather than batched, which is why its
   vault is project-scoped: a later restore has no batch id to look under.
3. **The flow placement gate.** A project that declares redaction **rejects any
   flow that sends source to a remote destination without a `redact` step**
   (`FlowDefinition.CheckRedactionCoverage`, enforced in flow placement). This is
   a gate rather than an implicit rewrite: silently inserting a step into a
   user's flow would change what their flow means, while refusing to run it says
   so.

### CLI surface

- `kapi run secure-translate -i <file> --target-lang <l>`: the in-process flow
  `reader → redact → translate → unredact → writer`.
- `kapi run redact-pii -i <file>`: the built-in entity flow, `entity-extract`
  then `redact` configured for the common categories. The placement pass keeps
  `redact` ahead of any remote-egress step.
- `kapi extract --redact` (or `--redact-rules <path>`): emits a redacted
  bilingual file and writes the batch sidecar.
- `kapi merge`: restores originals from the batch sidecar after applying the
  returned target.

### Not yet built: clearance as a destination ladder

Redaction is binary at each reader: a span is either withheld or it is not. The
design it points at is **clearance**, governance at a point over a ladder of
destinations, where the local working tree sees originals and each further
destination needs explicit clearance to. Nothing in the tree expresses a
clearance level yet; the exclusion the vault encodes lives in its placement and
in this decision rather than in a policy anyone can write down.

## Rationale

**Why a placeholder run, not text substitution?** Inline codes are already
protected from translation, survive the streaming pipeline, and round-trip
through bilingual formats. Reusing them means a model and an external tool treat
a redaction exactly as they treat any other do-not-touch token.

**Why is the original never on the run?** So the guarantee is auditable: any
serialized artifact can be scanned for the secret and must not contain it. The
run carries only a category and a token.

**Why dual restoration?** Id-based restore is exact but needs the inline
structure to survive. Plain-text carriers drop it, so a vault-backed text match
on a per-block-unique token is the fallback.

**Why rules by default and models opt-in?** Rule-based detection is
deterministic and fully offline; it cannot itself leak. Model-based detection is
more capable but, with a cloud model, discloses source during detection; making
it opt-in keeps the default trustworthy.

**Why a separate rules file?** The term list is itself sensitive. Keeping it out
of the committed recipe lets it stay untracked while the recipe still records
that redaction is enabled.

## See also

- [C-01: The project model](c-01-project-model.md): where the vault sits and
  what deleting `work/` costs.
- [E-01: Processing Engine](../engine/e-01-processing-engine.md): the
  in-process vault riding the block through the stream.
- [E-03: Tool System](../engine/e-03-tool-system.md): transformers, IO
  contracts and the placement pass.
- [F-02: Content Model](../foundations/f-02-content-model.md): placeholder
  runs and the semantic vocabulary.
- [M-01: Bilingual Format Interop](../multilingual/m-01-bilingual-interop.md):
  the extract and merge round trip redaction slots into.
