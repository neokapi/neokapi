---
id: 022-brand-voice
sidebar_position: 22
title: "AD-022: Brand Voice"
description: "Architecture decision: a brand voice subsystem with portable VoiceProfile YAML, built-in starter packs, a deterministic vocabulary check and an LLM-based voice check, a kapi brand command tree, and an offline MCP surface — keeping AI-generated content on-brand."
keywords: [brand voice, voice profile, brand check, brand rewrite, vocabulary, tone, MCP, starter packs, architecture decision, neokapi]
---

# AD-022: Brand Voice

## Summary

The brand voice subsystem keeps AI-generated and translated content on-brand.
Its core type, `brand.VoiceProfile`, is a portable YAML document describing
tone, style, vocabulary rules, examples, and locale/channel overrides. Two
registered tools evaluate text against a profile: a deterministic, offline
`brand-vocab-check` (rule-based vocabulary) and an LLM-based `brand-voice-check`
(tone/style/clarity). Findings carry a severity and a run-anchored position and
roll up into an MQM-inspired 0–100 compliance score. The `kapi brand` command
tree (`new`, `guide`, `check`, `rewrite`, `profiles`, `show`, `import`, `pack`)
exposes this as a text-first, JSON-first surface that works fully offline
against a starter pack, a standalone YAML file, the local SQLite brand store, or
a profile bound to a `.kapi` project. A small MCP surface (`brand_guide`,
`brand_check`, `brand_rewrite`) mirrors the deterministic path for AI agents.

## Context

neokapi's positioning is to plug into an AI assistant and keep its output
on-brand and consistent before publishing it in other languages and formats. A
brand voice is the natural unit of that guardrail: a reusable description of how
a brand wants to sound, against which a draft can be scored and rewritten. The
subsystem must satisfy several constraints:

- **Portable and git-shareable.** A profile is a YAML document a team can commit
  and review, with no backing store required — the same way a `.kapi` recipe is
  portable ([AD-008](008-project-model.md)).
- **Offline by default, AI-optional.** A vocabulary check (forbidden,
  competitor, and preferred terms; regex patterns) is deterministic and needs no
  network. An LLM check for the subjective dimensions (tone, style, clarity) is
  opt-in and credential-gated.
- **Composable with the rest of the engine.** Brand evaluation runs as
  registered tools ([AD-006](006-tool-system.md)) so it composes into flows,
  reuses the schema/config machinery, and writes findings as block annotations
  that other tools and the UI can read.
- **Multiple surfaces.** The same capability must be reachable from the CLI, the
  MCP server for agents, and the bundled Agent Skill ([AD-024](024-agent-skills.md)).

[AD-010](010-terminology.md) handles terminology consistency at the concept
level; brand voice is the broader, prose-level guardrail. The two intersect at
vocabulary rules, which the vocab check can optionally cross-reference against a
termbase.

## Decision

### Data model — `core/brand`

`VoiceProfile` is the canonical type. It is loaded from YAML by
`brand.LoadProfileYAML`, the single loader used by standalone files, the
embedded starter packs, and the SQLite store. Its shape:

- **`ToneProfile`** — personality adjectives, formality, emotion, humor, and
  free-text guidelines.
- **`StyleRules`** — active voice, sentence length, point-of-view, contractions,
  and `prohibited`/`required` regex `Pattern`s, each with a severity.
- **`VocabularyRules`** — `preferred`, `forbidden`, and `competitor`
  `TermRule`s (each with an optional replacement, note, and severity), plus
  abbreviations.
- **`VoiceExample`s** — before/after rewrites with explanations.
- **`LocaleOverride` / `ChannelOverride`** maps — locale- and channel-specific
  adjustments resolved on top of the base profile.

The profile also carries versioning fields (a `ProfileVersion` snapshot per
update, named `ProfileTag` references) for stores that track history.

### Findings and scoring

A finding is a `brand.BrandVoiceFinding`, which is a type alias to
`check.Finding` from the framework's content-verification core (`core/check`).
The struct carries a free-form `Category string` — a brand finding sets it to a
brand dimension (tone, style, vocabulary, clarity, brand_compliance), modeled by
the brand-local `Dimension` type — a `Severity` (neutral, minor, major,
critical), a human message, an optional suggestion, the original text, optional
metadata, and a **`Position model.RunRange`** — so a finding is anchored to the
runs it concerns, the same run-range model used for overlays and redaction
([AD-002](002-content-model.md)). `Severity` and `SeverityWeight` are
re-exported from `core/check`. Tools attach findings to a block as a
`BrandVoiceAnnotation` (annotation type `brand-voice`), which also carries the
profile id, the overall score, and its own `Position`.

This finding/severity/scoring path is shared across all checkers (terminology,
do-not-translate, placeholder, register, brand), not bespoke to brand voice —
brand voice is one checkset over the generic core, with `BrandComplianceScore`
providing a dimension-shaped presentation of the generic `check.Score`.

`brand.CalculateScore` rolls findings up using the MQM-inspired penalty weights
defined in `core/check.SeverityWeight` — neutral 0, minor 1, major 5,
critical 25 — per dimension. Each dimension starts at 100 and is reduced by its
penalty (clamped to 0); the overall score is 100 minus the total penalty. The
dimensions are fixed (tone, style, vocabulary, clarity, brand_compliance), so a
`BrandComplianceScore` always has a consistent shape.

### The two tools

Brand evaluation is implemented as two registered tools so it composes into
flows and shares the tool schema/config machinery
([AD-006](006-tool-system.md)):

- **`brand-vocab-check`** (`core/tools`) — deterministic and offline. It scans
  source text for forbidden, competitor, and preferred-term violations and
  regex pattern hits, emitting findings with positions. It optionally takes a
  termbase to filter by brand vocabulary. It is an `Annotate` tool (read-only;
  writes the annotation, not the content). This is the fast first pass.
- **`brand-voice-check`** (`core/ai/tools`) — LLM-backed. It asks an AI provider
  ([AD-011](011-ai-providers.md)) to score the subjective dimensions (tone,
  style, clarity) against the rendered voice guide, returning findings. It
  declares `RequiresCredentials` and an API-call side effect, produces the
  `quality.brand-voice` annotation, and runs with bounded per-block parallelism.

Both resolve their profile eagerly (supplied programmatically) or lazily through
a `ProfileResolver` against an organizational context hierarchy, so a host can
defer profile selection to runtime.

### Profile sources and the `kapi brand` command tree

`NewBrandCmd` (`cli/brand.go`) builds the `kapi brand` group. A profile is
resolved from one of three mutually exclusive sources:

- `--profile <name>` — a profile in the local SQLite brand store (opened with
  the standard `--name`/`--local`/`--file` resource flags, mirroring termbase
  and TM);
- `--profile-file <path>` — a standalone, git-shareable profile YAML;
- `--pack <name>` — a built-in starter pack.

With no source flag, resolution falls back to the `.kapi` project in scope: the
recipe's `defaults.brand_voice` binding (a `BrandVoiceBinding` selecting a
profile file, store profile, or pack — resolved relative to the project root),
then a convention file at `<root>/brand.yaml` or `<root>/.kapi/brand.yaml`. This
lets `kapi brand check DRAFT.md` work flag-free inside a project. Locale and
channel overrides apply on top via `--locale`/`--channel`.

The subcommands:

| Command | Purpose |
|---|---|
| `new` | Scaffold a commented, schema-valid profile YAML to fill in (optionally seeded from a `--pack`). |
| `guide` / `show` | Render the profile as a markdown voice guide to inject into an assistant's context. |
| `check` | Score text against the profile (vocab always; `--ai` adds the LLM check). `--min-score` turns it into a gate. |
| `rewrite` | Rewrite text to comply — deterministic term substitution by default, full LLM rewrite with `--ai`. |
| `profiles` | List profiles (local store + built-in packs). |
| `import` | Import a profile YAML into the local store. |
| `pack` | Install a built-in starter pack into the local store. |

`check` and `rewrite` read their subject text from `--text`, a positional file,
or stdin. `check --min-score` returns the `ErrQualityGate` sentinel when the
score is below the threshold, which the CLI maps to a distinct exit code
([AD-013](013-kapi-cli.md)) so skills and CI can tell a failed gate from an
operational error.

### Built-in starter packs

The framework embeds a small set of starter packs (`core/brand/packs`, embedded
via `//go:embed *.yaml`): `professional-b2b`, `friendly-dtc`, `technical-docs`,
`marketing-blog`, and `customer-support`. Each is a complete `VoiceProfile`
YAML, loaded through the same `brand.LoadProfileYAML` path as any other profile,
so packs are an on-ramp, not a special case — `kapi brand new --pack <name>`
emits one as an editable base.

### MCP surface

`cli/mcp_brand.go` registers offline brand tools on the shared `kapi mcp` stdio
server ([AD-013](013-kapi-cli.md)) so non-CLI agents get parity:

- `brand_guide` — render a voice guide from a pack or profile YAML;
- `brand_check` — score text using the deterministic vocabulary rules;
- `brand_rewrite` — substitute forbidden/competitor terms (deterministic).

These are hand-authored because each wraps a *resource* — a brand profile, a
termbase, or a TM file — rather than a single processing tool. The same file
also exposes `term_lookup` and `tm_search` so an agent can enforce terminology
and reuse prior translations alongside the brand checks.

The registry's processing tools are exposed over MCP **generically** rather than
curated by hand ([AD-006](006-tool-system.md), `cli/mcp_tools.go`): each
CLI-visible tool becomes an MCP tool whose input schema is projected straight
from the tool's own schema plus a `text` field. The set is **scoped by mode**,
mirroring the desktop's `ListTools` vs `ListProjectTools` split — in a kapi
project only the tools the project declares are advertised (with the project's
target language as the default); ad-hoc, the full set is exposed. So the MCP
surface now mirrors the CLI rather than being a deliberately narrowed subset of
it.

## Consequences

- A brand voice is a portable YAML document that works with or without a store,
  reviewable in git and reusable across the CLI, MCP, flows, and skills.
- The deterministic vocabulary check gives an instant, offline, reproducible
  signal; the LLM check is a clearly bounded, credential-gated opt-in for the
  subjective dimensions.
- Findings are run-anchored and annotation-shaped, so they compose with the
  content model and surface uniformly across tools and UIs rather than being a
  bespoke side channel.
- The MQM-style scoring is a single function over findings, so every surface
  (CLI, MCP, a flow) computes the same 0–100 score the same way.

## Related

- [AD-002: Content Model](002-content-model.md) — `RunRange` positions and
  block annotations
- [AD-006: Tool System](006-tool-system.md) — `brand-vocab-check` and
  `brand-voice-check` as registered tools
- [AD-008: Kapi Project Model](008-project-model.md) — `defaults.brand_voice`
  binding in a `.kapi` recipe
- [AD-010: Terminology](010-terminology.md) — concept-level terminology
  consistency that brand vocabulary intersects
- [AD-011: AI Providers](011-ai-providers.md) — the LLM provider behind the AI
  brand check and rewrite
- [AD-013: Kapi CLI](013-kapi-cli.md) — the `kapi brand` command tree, the MCP
  server, and the gate exit code
- [AD-024: Agent Skills](024-agent-skills.md) — the bundled skill that drives
  the brand commands
