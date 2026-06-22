---
title: Positioning & messaging canon
description: The single source of truth for how we describe neokapi and kapi in user-facing copy on the open-source project site.
---

# Positioning & messaging canon

> Internal contributor note. When you write or edit user-facing prose (docs, landing,
> README, CLI help, UI copy, the kapi Agent Skill, MCP tool descriptions), match this.
> It supersedes older l10n-first phrasing **and** the earlier "dual-heart" framing. Pairs
> with the repo-root `docs/internals/brand-communication.md` guide.

## What neokapi is (the one sentence)

> **neokapi is an open-source, format-aware content engine: parse any format into one
> unified content model, let a person or an AI agent edit the content inside it, check
> it, and write it back — byte-for-byte. The same engine makes that content work in every
> language, so going multilingual is part of the journey, not a separate tool.**

`kapi` is the CLI + desktop built on the engine. neokapi is one continuum — *get your
content right, then get it everywhere* — where "everywhere" means every format and every
language. There is no second story.

## Who it's for

The **forward-looking builder** — developers, AI builders, and content people who ship
across formats and languages and want it to **just work**, without becoming localization
engineers.

We are **not** optimizing for the traditional localization engineer / CAT-tool / agency
buyer, and we do not court them with their own jargon. We **use localization technology as
the machinery that makes content just work** — it is the engine room, never the product
identity or the headline. A builder should reach "and now it's in French, with the tags
and placeholders intact" without ever learning what a TMX file is.

## The journey (one continuum, optional multilingualism)

Point kapi at your content → **get it right** (parse · edit · check · brand) in your real
files → **and it works in every language**, automatically → ship it back unchanged.

- Multilingualism is **optional and woven in** — a natural turn of the *same* project, not
  a fork, not a ceremony, not a "Languages" nav header. A monolingual builder has a
  complete tool after "get it right"; languages are simply the next thing the same engine
  does.
- The hard l10n tech — byte-for-byte round-trip, translation memory, segmentation,
  bilingual interchange — is the **under-the-hood reason it just works**. Name it once in
  depth/reference and link to it; never lead with it, never make the visitor learn it.

## The lead wedge (one verb, one buyer)

Lead with **Edit** on **developer / structured formats**:

> **The open engine that lets you — or your AI — safely edit the content inside real files
> (JSON, Markdown, HTML, config, i18n catalogs) and write them back, byte-for-byte.**

Why this wedge: headless byte-fidelity gates CI, there is no native AI editor for these
formats, and the engine already serves them. **Do not** lead with editing binary Office
(`.docx`/`.pptx`) — Microsoft Copilot and a wave of Office MCP servers own in-app office
editing. We support those formats and harden them to an SLA; they are not the headline.

## The three pillars (the body)

1. **Parse** — read any format into one unified content model with roles, structure, and
   stable anchors. Clean input for AI/RAG, *with provenance to write back*.
2. **Edit** — change the content inside a format and save the original, byte-for-byte.
   Programmatic (`ksed`, transforms) and AI-driven, with a safety harness that preserves
   annotations and produces a reviewable diff.
3. **Check** — deterministic + AI checks emit one finding shape with a 0–100 score; gate on
   it (exit 3); loop with an assistant until it passes — *tests for AI output*.

These three run identically whether you **rewrite the source** or **produce a target**:
the same byte-for-byte write-back holds at the monolingual and the multilingual end. (They
are distinct capabilities in code — rewrite is a `Transform` over `SourceView`, translate a
`Produce` over `VariantView`, AD-006 — so never write "translate is just rewrite with a
target"; write "the same write-back holds whether you rewrite the source or produce a
target".) Brand voice is **one check in this checkset, not a separate system**.

## The moat (state it honestly)

The differentiator is **byte-for-byte read-modify-write round-trip across the long tail**.
Do **not** claim "everyone else only extracts" — that is false (Aspose, GroupDocs,
python-docx, the Office-MCP wave all write back). The honest, defensible claim is the
**bundle**:

> the only **open-source**, **format-agnostic**, **agent-drivable** engine that round-trips
> the long tail under **one streaming model**, with **overlay-safe** annotations and a
> built-in **QA loop**, **headless**.

State the moat **once** canonically (the hero subhead) and cross-link it — do not multiply
it across sections. It is true identically at both ends and proven at both ends: the
**format-maturity** dashboard is the monolingual proof; **parity** and **test-comparison**
are the multilingual proof. Never assert "survives anything" or "every … preserved" as an
unbacked claim — point to the dashboards ("under load, per format").

Use RAG/AI-ingestion as a *named use case*, never the category. Never publish a
parse-accuracy benchmark or enter a parsing leaderboard (PDF is an off-core plugin).

## Who lands where (all open-source — no commercial funnel)

| Audience | Lead with | Lands on |
|---|---|---|
| Builders editing structured files | the **Edit** wedge | `kapi rewrite`, the format-aware toolbox |
| AI / agent builders | kapi as an MCP tool / Agent Skill | use-with-Claude, the agent loop |
| AI / RAG engineers | the **Parse** verb (`inspect`, anchored JSONL) | OSS mindshare |
| Builders shipping in many languages | "and in every language" — it just works | the translate/TM/segmentation recipes, kapi-react i18n |

Every destination is an open-source page on the project site. There is **no** sales funnel.

## Bowrain is out of scope for the project site

Bowrain is a separate, commercial team-governance platform with **its own site and its own
positioning**. **Do not sell or mention Bowrain on the neokapi project site** — not the
homepage, not the docs, not the README, not CLI help. neokapi is an open-source project;
keep the project site about the engine, the CLI, and what builders can do with it. (The
kapi/Bowrain architectural boundary still holds in the code; it is simply not project-site
messaging.)

## Vocabulary (write the left as the right)

| Don't write | Write |
|---|---|
| "primary translatable content unit" | "primary modifiable content unit" |
| "the translatable text of any format" | "the text/content inside any format" |
| "a localization and translation toolkit" | "a format-aware content engine — parse, edit, check any format, in any language" |
| "Source / Target" (to non-l10n readers) | "canonical content + variants" (axes: locale, tone, channel) |
| "everyone else only extracts" | "the only OSS, format-agnostic engine that round-trips the long tail …" |
| "faithful" (as a brand adjective) | drop it — use the concrete "**byte-for-byte**" / "write it back unchanged" |
| "faithful content model" | "**unified content model**" |
| "at heart, a localization engine and the tool that keeps your source content on brand" (dual-heart) | one continuum: "**get your content right, then get it everywhere**" — multilingualism woven in, not a second heart |
| "Add a language" / "Localize" / "Languages" as a nav header or ceremonious stage | weave it as a natural outcome: "**and in every language**" — part of the journey, not a separate destination |
| l10n-engineer jargon in the lead (XLIFF, TMX, "Okapi alternative", "translation memory") | builder outcomes ("it works in every language"); keep the jargon in depth/reference only |
| a lone narrow format in a generic example (XLIFF, `.docx`) | a broad/recognizable set ("JSON, HTML, Markdown, config, office formats"); keep XLIFF/PO only in explicit multilingual copy |

**Code-level (DONE — bounded rename):** the generic tool capability was de-l10n-coded —
`CapTranslate` → `CapProduce`, the `Translate(TargetView)` handler → `Produce(VariantView)`,
`TargetView` → `VariantView` (~24 files; build + tests green; the ITS `Translate` field untouched).
**Keep** the user-facing `translate` tool and `kapi translate` command — that is the l10n
*application*. **Do not** rename `Source`/`Targets`/`VariantKey`/`Block.Translatable`
(good generic names, wire-bound, or — for `Translatable` — a parse-time extraction
classifier, not an editability flag).

## Don'ts

- Don't make the homepage legible to five audiences at once — lead with the one wedge,
  then offer use-case pages.
- Don't sell or mention Bowrain on the project site (see above).
- Don't make "Languages"/"Localize" a nav header or a ceremonious "add a language" step;
  weave multilingualism into the journey as a natural outcome.
- Don't court the traditional localization engineer with their own jargon — serve the
  builder who wants it to "just work," and keep l10n tech under the hood.
- Don't put "the content layer your AI assistant orchestrates" (or similar AI-orchestration
  slogans) in the hero. The tagline's "for people and AI agents" carries it; the slogan
  decays fast.
- Don't hardcode counts (formats/tools/providers) — name categories, link to generated
  references (per brand-communication).
- Don't bury or delete the multilingual story — it is the proof that byte-for-byte
  write-back holds under the hardest workload (parity dashboard, XLIFF/TMX, okapi-bridge).
  Present it as the engine quietly doing the hard thing, not as a separate product.
