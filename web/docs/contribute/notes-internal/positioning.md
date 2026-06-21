---
title: Positioning & messaging canon
description: The single source of truth for how we describe neokapi, kapi, and Bowrain in user-facing copy.
---

# Positioning & messaging canon

> Internal contributor note. When you write or edit user-facing prose (docs, landing,
> README, CLI help, UI copy, the kapi Agent Skill, MCP tool descriptions), match this.
> It supersedes older l10n-first phrasing. Pairs with
> [brand-communication](../../../docs/internals/brand-communication.md).

## What neokapi is (the one sentence)

> **neokapi is a faithful, format-aware content engine: parse any format into one
> content model, edit the content inside it, check it, and write it back —
> byte-for-byte. Localization is its deepest application, not its identity.**

`kapi` is the CLI + desktop built on the engine. **Bowrain** is the team
governance platform built on `kapi`.

## The lead wedge (one verb, one buyer)

Lead with **Modify** on **developer / structured formats**:

> **The open engine that lets your AI safely edit the content inside real files —
> JSON, Markdown, HTML, config, i18n catalogs — and write them back, byte-for-byte.**

Why this wedge: headless byte-fidelity gates CI, there is no native AI editor for these
formats, and the engine already serves them. **Do not** lead with editing binary Office
(`.docx`/`.pptx`) — Microsoft Copilot and a wave of Office MCP servers own in-app office
editing. We support those formats and harden them to an SLA; they are not the headline.

## The three pillars (the body)

1. **Parse** — read any format into one faithful content model with roles, structure, and
   stable anchors. Clean input for AI/RAG, *with provenance to write back*.
2. **Edit** — change the content inside a format and save the original, byte-faithfully.
   Programmatic (`ksed`, transforms) and AI-driven, with a safety harness that preserves
   annotations and produces a reviewable diff.
3. **Check** — deterministic + AI checks emit one finding shape with a 0–100 score; gate on
   it (exit 3); loop with an assistant until it passes — *tests for AI output*.

**Applications on top (co-equal, named, not buried):** localization (the flagship),
brand & terminology governance (monolingual), AI ingestion.

## The moat (state it honestly)

The differentiator is **faithful read-modify-write round-trip across the long tail**.
Do **not** claim "everyone else only extracts" — that is false (Aspose, GroupDocs,
python-docx, the Office-MCP wave all write back). The honest, defensible claim is the
**bundle**:

> the only **open-source**, **format-agnostic**, **agent-drivable** engine that round-trips
> the long tail under **one streaming model**, with **overlay-safe** annotations and a
> built-in **QA loop**, **headless**.

Use RAG/AI-ingestion as a *named use case*, never the category. Never publish a
parse-accuracy benchmark or enter a parsing leaderboard (PDF is an off-core plugin).

## Audiences → where they go

| Audience | Lead with | Destination |
|---|---|---|
| Programmatic / AI-editing devs | the **Edit** wedge | OSS kapi adoption |
| AI / RAG engineers | the **Parse** verb (`inspect`, anchored JSONL) | **OSS mindshare only** — no Bowrain funnel |
| AI / agent builders | kapi as an MCP tool / Agent Skill | distribution channel (how the others find kapi) |
| Brand / content ops | monolingual on-brand QA + terminology | **→ Bowrain** (governance SKU) |
| Localization teams | extract / translate / QA / merge, with fidelity | **→ Bowrain** (team TMS/governance) |

**Bowrain = the governance platform for on-brand + translation only.** It does not chase
RAG/agent-builder. Its tagline stays "Govern and steward brand voice, terminology, and
translation — as a team." Frame it as *the governance layer for the content kapi produces
and edits.*

## The Kapi / Bowrain boundary (unchanged)

- **Kapi** = the OSS engine + CLI + desktop. Owns local files + the `.kapi` recipe. Words:
  *engine, toolkit, CLI, library.* Never "platform."
- **Bowrain** = the cloud platform. Governance, collaboration, automation, the
  correction-learning loop. Words: *platform, governance.* "ContentOps / content operations"
  language, if used at all, lands here — never on Kapi.

## Vocabulary (write the left as the right)

| Don't write | Write |
|---|---|
| "primary translatable content unit" | "primary modifiable content unit" |
| "the translatable text of any format" | "the text/content inside any format" |
| "a localization and translation toolkit" | "a format-aware content toolkit — parse, edit, check, and localize any format" |
| "Source / Target" (to non-l10n readers) | "canonical content + variants" (axes: locale, tone, channel) |
| "everyone else only extracts" | "the only OSS, format-agnostic engine that round-trips the long tail …" |

**Code-level (do now, bounded):** the generic tool capability `CapTranslate` →
`CapProduce`, `Translate(TargetView)` → `Produce(VariantView)`, `TargetView` → `VariantView`.
**Keep** the user-facing `translate` tool and `kapi translate` command — that is the l10n
*application*. **Do not** rename `Source`/`Targets`/`VariantKey`/`Block.Translatable`
(good generic names, wire-bound, or — for `Translatable` — a parse-time extraction
classifier, not an editability flag).

## Don'ts

- Don't make the homepage legible to five audiences at once — lead with the one wedge,
  then offer use-case pages.
- Don't put "the content layer your AI assistant orchestrates" (or similar AI-orchestration
  slogans) in the hero. The tagline's "for people and AI agents" carries it; the slogan
  decays fast.
- Don't hardcode counts (formats/tools/providers) — name categories, link to generated
  references (per brand-communication).
- Don't bury or delete the localization story — it is the flagship application and the
  fidelity proof (parity dashboard, XLIFF/TMX, okapi-bridge). Reframe it as proof of a
  generic strength.
