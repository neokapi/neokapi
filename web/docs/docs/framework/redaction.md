---
sidebar_position: 15
title: Redaction
---

# Redaction

Redaction replaces sensitive content — people, unreleased product names,
internal roles, secrets — with protected placeholders before a document is
sent for AI translation or handed to an external translator, then restores the
originals once the translation comes back. The sensitive value never leaves the
local machine.

```
Mr Bean is the new King of England
        │  redact
        ▼
[REDACTED:Person] is the new [REDACTED:Role]      ← what the translator/model sees
        │  translate, then unredact
        ▼
M. Bean est le nouveau Roi d'Angleterre           ← originals restored locally
```

For the architecture and design decisions behind this, see
[AD-020: Content Redaction](/contribute/architecture/020-redaction).

## How it stays local

A redacted span becomes a **placeholder** — the same protected inline-code
primitive kapi uses for variables and tags, with a type like
`redaction:person`. The placeholder carries only a category and a stable token;
the original text is held in a local **vault** and is never written into the
content that leaves the machine.

For AI translation the protection is structural: kapi sends the model an opaque
token (`<x id="…"/>`) in place of each placeholder, so the model receives
neither the original text nor its visible label, then re-attaches the
placeholder by token when the translation returns. For external translation the
placeholder travels in the bilingual file as ordinary protected markup while
the original↔token mapping stays behind in a gitignored sidecar.

## Detection

Redaction finds sensitive spans with one or both detectors:

- **Rules** (default) — literal terms and regular expressions you declare.
  Fully offline and deterministic.
- **Entities** (opt-in) — named entities (people, organizations, products,
  locations) detected by the `ai-entity-extract` tool and redacted by category.
  Run a local model to keep detection on the machine, or a cloud model to trade
  that for broader coverage during the detection step.

Each match is assigned a category. The recommended categories are `person`,
`role`, `product`, `org`, `location`, and `custom`, but categories are
free-form — use whatever the placeholder template should display.

## Rules file

The term list is itself sensitive, so it lives in a dedicated file (keep it
gitignored) rather than in the committed recipe:

```yaml
# .kapi/redaction.yaml
version: v1
placeholder: "[REDACTED:{category}]"   # {category} and {n} are substituted
detectors: [rules]
rules:
  - term: "Mr Bean"
    category: person
  - term: "King of England"
    category: role
  - pattern: "PROJECT-[A-Z]+"          # regular expression
    category: product
    flags: [ignorecase]                # also: wholeword (literal terms)
```

## Workflows

### In-process: secure-translate

The built-in `secure-translate` flow redacts, AI-translates against the
placeholders, and restores the originals — all in a single run, so the secret
is only ever in memory:

```bash
kapi run secure-translate -i src/locales/en.json --target-lang fr
```

The flow is `reader → redact → ai-translate → unredact → writer`. The `redact`
and `unredact` tools are ordinary pipeline steps, so you can also compose them
into your own flows.

### External: extract, translate, merge

When a human translator or CAT tool does the translation, redact on the way out
and restore on the way back. `kapi extract --redact` emits a bilingual file
containing only placeholders and writes the originals to a local vault sidecar;
`kapi merge` restores them after the translator returns the file.

```bash
# Emit redacted XLIFF — originals stay in .kapi/cache/redaction/<batch>.json
kapi extract --redact

# ... translator fills in targets, preserving the placeholders ...

# Restore the originals into the merged target
kapi merge -i out/app.en-US-to-fr-FR.xliff
```

Pass `--redact-rules <path>` to point at a rules file directly, or enable
redaction in the recipe (below) so plain `kapi extract` redacts. Use
`kapi merge --no-restore` to keep the placeholders in the merged output.

## Recipe configuration

Declare redaction in the project recipe to make it the default for `extract`.
It can be set project-wide under `defaults` and overridden per content item:

```yaml
defaults:
  source_language: en
  target_languages: [fr, de]
  redaction:
    enabled: true
    rules: .kapi/redaction.yaml
    detectors: [rules]
    placeholder: "[REDACTED:{category}]"

content:
  - path: "src/locales/*.json"
    format: json
    redaction:
      enabled: false      # skip redaction for this collection
```

## Where the originals live

| Workflow | Vault |
| --- | --- |
| In-process (`secure-translate`, custom flows) | In memory on the block; removed after restore — never written to output |
| External (`extract --redact` → `merge`) | `.kapi/cache/redaction/<batch-id>.json` — under the gitignored cache, written private (`0600`) |

Restoration matches placeholders back to originals by token where the format
preserves inline structure (in-process pipelines and XLIFF), and by the unique
visible token text where the format flattens placeholders to plain text (such
as JSON). On `extract`, redaction runs before translation-memory pre-fill, so
no sensitive value can reach the emitted file through a TM match.
