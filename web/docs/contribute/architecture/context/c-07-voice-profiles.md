---
id: c-07-voice-profiles
sidebar_position: 7
title: "C-07: Voice profiles"
description: "Architecture decision: a voice-profile subsystem with portable YAML profiles, built-in starter packs, a deterministic vocabulary check and an LLM-based voice check, one resolution chain, and a kapi voice command tree that works fully offline."
keywords: [voice profile, voice check, voice rewrite, vocabulary, term rules, tone, starter packs, MCP, architecture decision, neokapi]
---

# C-07: Voice profiles

## Summary

The voice-profile subsystem keeps generated and translated content in voice. Its
core type, `profile.VoiceProfile`, is a portable YAML document describing tone,
style, vocabulary rules, examples, and locale, channel and persona overrides. Two
registered tools evaluate text against a profile: a deterministic, offline
`voice-vocab-check` and an LLM-based `voice-check`. Findings carry a severity and
a run-anchored position and roll up into an MQM-inspired 0–100 compliance score.

The `kapi voice` command tree exposes this as a text-first, JSON-first surface
that works fully offline against a starter pack, a standalone YAML file, the
project's voice store, or a profile bound by a project recipe. A small MCP
surface mirrors the deterministic path for agents.

## Context

A voice profile is the natural unit of a guardrail on generated text: a reusable
description of how a product wants to sound, against which a draft can be scored
and rewritten. The subsystem has to satisfy several constraints at once:

- **Portable and reviewable.** A profile is a YAML document a team commits and
  reviews, with no backing store required, the same way a recipe is portable
  ([C-01](c-01-project-model.md)).
- **Offline by default, AI-optional.** A vocabulary check (forbidden,
  competitor and preferred terms, plus regular-expression patterns) is
  deterministic and needs no network. A model-backed check for the subjective
  dimensions is opt-in and credential-gated.
- **Composable with the rest of the engine.** Voice evaluation runs as registered
  tools ([E-03](../engine/e-03-tool-system.md)) so it composes into flows, reuses
  the schema and config machinery, and writes findings as block annotations other
  tools and the UI can read.
- **Several surfaces.** The same capability is reachable from the CLI, from an
  MCP client, and from the bundled agent skill
  ([S-03](../surfaces/s-03-agent-surfaces.md)).

Terms ([C-08](c-08-terms.md)) handle consistency at the concept level; a voice
profile is the broader, prose-level guardrail. The two intersect at vocabulary
rules, which the vocabulary check can cross-reference against a terms store.

## Decision

### The data model

`VoiceProfile` (`core/profile`) is the canonical type, loaded from YAML by
`profile.LoadProfileYAML`, the single loader used by standalone files, the
embedded starter packs and the voice store:

- **`ToneProfile`**: personality adjectives, formality, emotion, humor, and
  free-text guidelines. Tone values are described rather than enumerated: a
  register outside the conventional set loads, and validation reports it as an
  advisory note.
- **`StyleRules`**: active voice, sentence length, point of view, contractions,
  and prohibited/required regular-expression patterns. A `Pattern` carries a
  severity, an optional `rate` (`max` matches per `per_words` words, which turns
  a prohibition into a ceiling; under the ceiling nothing is reported, over it
  every match is), and an optional `scope` (`prose`, `code` or `heading`; empty
  means everywhere). The style enums stay closed, because code reads them.
- **`VocabularyRules`**: preferred, forbidden and competitor term rules, plus
  abbreviations. A `TermRule` carries the term, an optional replacement, note
  and severity, `forms` (the other surface shapes the term takes, declared
  rather than derived, which `kapi voice expand` fills in by asking a model once
  in the profile's own language), `case_sensitive` (off by default), `scope`
  (the same values a pattern takes), `do_not_translate` (what gives a bare term
  with no replacement its meaning), and `concept_id`, which ties the rule to a
  concept in the terms store and the graph and stays empty for a standalone
  profile.
- **`VoiceExample`s**: before/after rewrites with explanations.
- **`LocaleOverride`, `ChannelOverride` and persona maps**: adjustments resolved
  on top of the base profile.

The profile also carries versioning fields (a version snapshot per update, and
named tag references) for stores that track history.

`profile.TermRule` is also the one shape every governed step takes its
terminology in. `term-check`, `translate`, `recycle` and `dnt-check` all read
`term_rules:` as `[]profile.TermRule`, whether the rules come from the profile's
own vocabulary, from the terms store, or from a recipe, and
`profile.TermRuleMap` is the single projection of that list into the map a
prompt renders and the context fingerprint hashes, so the staleness gate and the
producers cannot disagree about what governed a target. A rule's severity
decides whether a violation fails or only reports (`minor` and `neutral` warn),
and a rule with an empty replacement is skipped by the tools unless it is marked
do-not-translate, because "say this instead" needs a this. How the store and the
rules are located in a text is [C-08](c-08-terms.md)'s subject.

### Findings and scoring

A finding is `profile.VoiceFinding`, a type alias to `check.Finding` from the
framework's content-verification core (`core/check`). It carries a free-form
`Category` (a voice finding sets it to one of the fixed dimensions: tone, style,
vocabulary, clarity, compliance), a severity, a human message, an optional
suggestion, the original text, optional metadata, and a **`Position
model.Anchor`**, so a finding is anchored to the runs it concerns, the same
run-range model overlays and redaction use
([F-02](../foundations/f-02-content-model.md)).

Tools attach findings to a block as a `VoiceAnnotation` (annotation type
`voice`), which also carries the profile id, the overall score and its own
position.

`profile.CalculateScore` rolls findings up using the MQM-inspired penalty weights
in `core/check.SeverityWeight` (neutral 0, minor 1, major 5, critical 25) per
dimension. Each dimension starts at 100 and is reduced by its penalty, clamped at
0; the overall score is 100 minus the total penalty. The dimensions are fixed, so
a compliance score always has a consistent shape.

This finding, severity and scoring path is shared across every checker
(terminology, do-not-translate, placeholder, register, voice) rather than being
bespoke to voice. Voice is one checkset over the generic core.

### The tools

- **`voice-vocab-check`** (`core/tools`): deterministic and offline. It scans
  source text for forbidden, competitor and preferred-term violations and pattern
  hits, emitting findings with positions. It optionally takes a terms store to
  filter by voice vocabulary. It is an annotate-class tool: it writes the
  annotation, never the content. This is the fast first pass.
- **`voice-check`** (`core/ai/tools`): model-backed. It asks a provider
  ([E-07](../engine/e-07-model-providers.md)) to score the subjective dimensions
  against the rendered voice guide. It declares that it requires credentials and
  has an API-call side effect, produces the `voice` annotation, and runs with
  bounded per-block parallelism.
- **`voice-infer`** (`core/ai/tools`): model-backed inference of a profile from
  existing content, for a team that has a body of writing and no written-down
  voice.

Both checks resolve their profile eagerly, when it is supplied programmatically,
or lazily through a resolver against a context hierarchy, so a host can defer
profile selection to runtime.

Both renderers of a profile (the full guide and the compact form the translation
path sends) carry a pattern's rate and scope, and both carry the profile's
examples. A rate the model never hears is a stricter rule in the prompt than the
check enforces, and a scope it never hears is a wider one.

### Pattern scope

A style pattern's scope follows what it asserts. A prohibited pattern says "this
text must not contain X", which every block answers on its own, so
`profile.Findings` matches it per block beside the vocabulary rules. A required
pattern says "this text must contain X" (the call to action, the trademark line,
the safety notice), and that is a claim about the document: no paragraph of a
page carries it, the page does. `profile.DocumentFindings` therefore evaluates
the required patterns once over a file's content and reports one finding per
unsatisfied rule against the file, with no block, because an absence sits nowhere
in particular. A streaming tool sees one block at a time and so evaluates the
block-scope half only.

`profile.PatternRuleCount` is the number any surface reports as a profile's
pattern-rule total, so what a profile card counts is what the gates apply.

### One resolution chain

`profile.ResolveProfileFromContext` is the only place a profile's precedence is
decided, most specific first:

1. **Explicit**: an id from tool config or a call parameter.
2. **Collection**: a profile the caller already loaded, else the collection's
   own `profile.PropertyProfileID` property.
3. **Stream** → 4. **Project** → 5. **Root**.

The tiers below the collection are property maps read off stored rows, which is
how a project created by a connector or an editor, with no recipe, is governed. A
recipe-governed project fills the *same* collection tier: a collection's
`channel:` selects the profile ([C-02](c-02-coordinates-and-governance.md)), the
host loads it (a recipe binds a profile file or a starter pack, which a store id
cannot name) and hands it over as the already-loaded collection profile. So the
two kinds of project differ in which tiers they populate, never in how the tiers
are ranked, and an explicit per-call profile outranks a recipe exactly as it
outranks a stored row.

Locale, channel and persona overrides are applied once, at the end of that chain,
by `ResolveProfile`. A channel bound to a scope describes where the content is
published; the resolve context's own channel is the caller overriding it for one
call, which is the tier a `--channel` flag occupies.

An id that has nowhere to resolve from is a configuration error rather than a
silent miss. A silent miss would leave the content ungoverned and read as if
nothing were bound.

### The recipe authors; one venue applies

A recipe is an **authoring surface**, not a second runtime source. It is
version-controlled and authoritative over what the governance *is*; at any moment
exactly one venue *applies* it: the recipe when the project runs on its own, a
service's stored rows when it runs connected. Two live sources would mean a voice
that depends on where the loop happened to run, and someone quietly editing over
a committed profile.

What crosses a push is every declared collection, the point it sits at and the
voice governing it, so both venues resolve the same voice for the same content.
What does not cross is a profile's `termstore:`, a path into the local project,
and a path means nothing to a service that governs terminology from a shared
vocabulary. That divergence is real and is reported rather than hidden: a run
over a project that binds a terms store per profile *and* binds a venue prints a
warning to stderr and proceeds. A recipe field that is not readable at the other
venue is not a reason to refuse the run.

### Profile sources and the command tree

`kapi voice` resolves a profile from one of three mutually exclusive sources:

- `--profile <name>`: a profile in the voice store. Inside a project that store
  is the `voice_profiles` table of the project pool, `.kapi/work/store.db`
  ([C-03](c-03-context-store-and-graph.md)), so the same recipe resolves the
  same profile from any directory in the tree; an explicit `--name`, `--local`
  or `--file` selects a standalone store file instead, mirroring the terms
  store and the content memory, and outside a project the standalone `voice.db`
  is the default.
- `--profile-file <path>`: a standalone, reviewable profile YAML.
- `--pack <name>`: a built-in starter pack.

With no source flag, resolution falls back to the project in scope: the voice
governing the content collection that claims the file, else the recipe's
`defaults.voice` (a binding selecting a profile file, a store profile or a pack,
resolved relative to the project root), then the convention file at
`.kapi/voice.yaml`, or `voice.yaml` at the project root for a project that keeps
it there. This lets `kapi voice check DRAFT.md` work flag-free inside a project.
Locale and channel overrides apply on top via `--locale`/`--channel`; an explicit
`--channel` wins over the channel the recipe declares.

| Command | Purpose |
| --- | --- |
| `new` | Scaffold a commented, schema-valid profile YAML, optionally seeded from a pack. |
| `guide` / `show` | Render the profile as a markdown voice guide to inject into an assistant's context. |
| `check` | Score text against the profile: vocabulary always, `--ai` adds the model check. `--min-score` turns it into a gate. |
| `rewrite` | Substitute forbidden and competitor terms for their approved replacements: deterministic, offline, no model. A rule that matches without a replacement is reported under `skipped`. |
| `expand` | Ask a model for the surface forms each vocabulary term takes and write them into the profile as `forms:`, for review in a diff. |
| `validate` | Check a profile document against the schema; blocking problems fail, advisory notes print after the verdict. |
| `profiles` | List profiles: the voice store plus the built-in packs. |
| `import` | Import a profile YAML into the voice store. |
| `pack` | Install a built-in starter pack into the voice store. |
| `pointer` | Write the marker-delimited section into the project's assistant file (`AGENTS.md`, or an existing `CLAUDE.md`) that tells an assistant the voice is held by kapi and that `guide` retrieves it. |

The pointer exists because an assistant standing in a project has no reason to
open `kapi.yaml` when its task is to write a guide, and so never learns the
project has a voice. The section names the voice, says kapi holds it, and gives
the retrieval command; it carries none of the guidance, which stays one command
away and so cannot go stale in the file. `kapi init` writes it whenever the
project it scaffolds or adopts binds a voice (`--no-pointer` opts out), the
desktop writes it when a profile is saved, and `pointer` writes it on demand;
all three go through `host.WriteVoicePointer`. The text is
`coreprofile.RenderVoicePointer`, and `UpsertVoicePointer` replaces the section
between its markers so a re-run is idempotent and hand-written content around
it survives. A project that unbinds its voice has the section removed on the
next run rather than left claiming a voice `guide` cannot resolve.

`check` reads its subject from `--input-text`, a positional file, or stdin.
`check --min-score` returns the quality-gate sentinel when the score is below the
threshold, which the CLI maps to a distinct exit code
([S-01](../surfaces/s-01-kapi-cli.md)) so skills and CI can tell a failed gate
from an operational error. `kapi check --voice` is the project-level style
gate, with `--voice-min` setting the similarity cutoff.

### Fixing off-voice content

kapi does not send content to a model to rewrite it. An in-voice fix is
caller-supplied: the assistant reads what applies at the point
([C-06](c-06-retrieval.md)), rewrites the off-voice text itself, and applies the
result through the one write verb, `kapi apply`. The edits land through the
byte-faithful round-trip with **no provider involved**: structure and inline
codes are preserved, each block is drift-guarded by its content hash, and an edit
that would corrupt markup is rejected.

`kapi voice rewrite` is a separate, deterministic helper: it substitutes
forbidden and competitor terms for their approved replacements by rule, offline,
through the same matcher as the vocabulary check (`profile.RewriteVocabulary`).
A rule that names no replacement, and a match on a declared inflected form of a
term, stay in the text and are reported under `skipped` with the term, its list
and severity, the spellings matched and the reason, so a caller can tell an
unchanged text with nothing to fix from one that still carries violations. The
exit code stays 0. It does not call a model and does not touch tone, style or
phrasing; those are the caller's to rewrite.

### A vocabulary rule is a change-set entry

Fixing a recurring off-voice term at the *source* (adding a vocabulary rule so
every future draft is checked against it) is a `voice` entry in the same `kapi
apply` change-set, alongside the content fix that justifies it:

```json
{"kind":"voice","op":"add-rule","list":"forbidden","term":"utilize","replacement":"use","severity":"minor"}
```

The entry adds a term rule to the named vocabulary list (`forbidden`,
`competitor` or `preferred`) of the **committed** profile YAML the recipe binds,
creating and binding one if none exists, then re-imports that profile into the
voice store through the same `profile.LoadProfileYAML` path. The committed YAML
is the single source of truth and the diff is the review surface; the store is a
compiled cache written by the one importer. The operation is idempotent. A
binding that points at a starter pack or a store profile rather than a file is
rejected: `apply` edits a committed file, not a pack or a stored row.

### Built-in starter packs

The framework embeds a small set of starter packs (`core/profile/packs`, embedded
with `//go:embed`), each a complete profile YAML loaded through the same path as
any other profile. Packs are an on-ramp, not a special case: `kapi voice new
--pack <name>` emits one as an editable base. `kapi voice profiles` lists what is
installed.

### MCP surface

`host/mcp_voice.go` registers two offline voice tools on the shared `kapi mcp`
stdio server ([S-01](../surfaces/s-01-kapi-cli.md)) so non-CLI agents get parity:
`voice_check` scores text using the deterministic vocabulary rules, and
`voice_rewrite` substitutes forbidden and competitor terms and reports under
`skipped` what it matched and left in place.

These are hand-authored because each wraps a *resource* (a voice profile, a
terms store, a content memory) rather than a single processing tool. The
rendered guide is **not** a tool: it is reached by reading `context://<path>` or
`context://profile/<name>` ([C-06](c-06-retrieval.md)), because the guide is part
of what applies at a point rather than a thing to ask for separately.

## Consequences

- A voice profile is a portable YAML document that works with or without a store,
  reviewable in a diff and reusable across the CLI, MCP, flows and skills.
- The deterministic vocabulary check gives an instant, offline, reproducible
  signal; the model check is a bounded, credential-gated opt-in for the
  subjective dimensions.
- Findings are run-anchored and annotation-shaped, so they compose with the
  content model and surface uniformly rather than through a bespoke side channel.
- The MQM-style scoring is a single function over findings, so every surface
  computes the same score the same way.
- One term-rule shape serves the profile, the terms store and every governed
  step, so a rule declared anywhere is enforced and fingerprinted the same way.

## See also

- [C-02: Coordinates and governance](c-02-coordinates-and-governance.md): which
  profile governs which content.
- [C-08: Terms](c-08-terms.md): concept-level consistency the vocabulary rules
  intersect, and the one pass that locates declared terms.
- [E-03: Tool System](../engine/e-03-tool-system.md): the checks as
  registered tools.
- [E-07: AI Providers](../engine/e-07-model-providers.md):
  the provider behind the model-backed check.
- [F-02: Content Model](../foundations/f-02-content-model.md): run ranges
  and block annotations.
