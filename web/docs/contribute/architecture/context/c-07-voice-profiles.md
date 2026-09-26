---
id: c-07-voice-profiles
sidebar_position: 7
title: "C-07: Voice profiles"
description: "Architecture decision: a voice-profile subsystem with portable YAML profiles, built-in starter packs, a deterministic word-rule and pattern check and an LLM-based voice check, one resolution chain, and a kapi voice command tree that works fully offline."
keywords: [voice profile, voice check, voice rewrite, term rules, tone, starter packs, MCP, architecture decision, neokapi]
---

# C-07: Voice profiles

## Summary

The voice-profile subsystem keeps generated and translated content in voice. Its
core type, `profile.VoiceProfile`, is a portable YAML document describing tone,
style measures, pattern rules, guidance, examples, and locale, channel and
persona overrides. Word rules ("write this, not that") are terms
([C-08](c-08-terms.md)), and a profile holds none. Two
registered tools evaluate text against a profile: a deterministic, offline
`voice-vocab-check` and an LLM-based `voice-check`. Each finding says whether it
fails a check, carries a run-anchored position, and rolls up into a reported
0–100 compliance score.

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
- **Offline by default, AI-optional.** The check of word rules and
  regular-expression patterns is deterministic and needs no network. A model-backed check for the subjective
  dimensions is opt-in and credential-gated.
- **Composable with the rest of the engine.** Voice evaluation runs as registered
  tools ([E-03](../engine/e-03-tool-system.md)) so it composes into flows, reuses
  the schema and config machinery, and writes findings as block annotations other
  tools and the UI can read.
- **Several surfaces.** The same capability is reachable from the CLI, from an
  MCP client, and from the bundled agent skill
  ([S-03](../surfaces/s-03-agent-surfaces.md)).

Terms ([C-08](c-08-terms.md)) hold every word rule; a voice profile is the
broader, prose-level guardrail. The two meet in the deterministic check, which
applies the terms that govern the text beside the voice's patterns.

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
  and prohibited/required regular-expression patterns. A `Pattern` carries an
  optional `advisory` flag, an optional `rate` (`max` matches per `per_words` words, which turns
  a prohibition into a ceiling; under the ceiling nothing is reported, over it
  every match is), and an optional `scope` (`prose`, `code` or `heading`; empty
  means everywhere). The style enums stay closed, because code reads them.
- **`VoiceExample`s**: before/after rewrites with explanations.
- **`LocaleOverride`, `ChannelOverride` and persona maps**: adjustments resolved
  on top of the base profile, in that order. A locale adjusts formality, humour,
  point of view, cultural notes and examples; a channel's or persona's tone and
  style replace the resolved ones. None of them carries word rules.

The profile also carries versioning fields (a version snapshot per update, and
named tag references) for stores that track history.

### Word rules are terms

A word rule is a `profile.TermRule`: the form to reject (`term` and its
`forms`), the form to use (`replacement`), a note, `advisory`, `competitor` for
a rival's name, `case_sensitive` (unset by default, see below), `scope` (the
same values a pattern takes), `do_not_translate` (what gives a bare term with no
replacement its meaning), and `concept_id`, which ties the rule to a concept in
the terms store and the graph. A rule with a `replacement` and no `term` names a
preferred form and rejects nothing. `forms` are declared rather than derived:
`kapi terms expand` fills them in by asking a model once in the term's own
language.

The rules live in the terms store. A voice file may carry word rules beside the
voice under a top-level `terms:` list, as the starter packs do.
`profile.ParseVoiceFile` splits the two: the profile holds the voice, and the
file's rules ride beside it in memory (`CarriedTerms`, with the source they come
from) and are never stored with the profile. `kapi voice import` and
`kapi context import` move a file's rules into the project's terms store and
report how many. A file that lists its words under `vocabulary:` is read the
same way: a forbidden term becomes a rule, a competitor term a rule marked
`competitor`, and a preferred term a preferred form, or an advisory rule when it
names a different replacement. A bound starter pack's terms apply beside the
project's own, and a finding they raise names the pack (`pack technical-docs`)
in its `from` metadata.

`profile.TermRule` is also the one shape every governed step takes its
terminology in. `term-check`, `translate`, `recycle` and `dnt-check` all read
`term_rules:` as `[]profile.TermRule`, whether the rules come from the terms
store, a voice file or a recipe, and
`profile.TermRuleMap` is the single projection of that list into the map a
prompt renders and the context fingerprint hashes, so the staleness gate and the
producers cannot disagree about what governed a target. A violation of a rule
or a pattern fails a check unless the rule is marked `advisory: true`, which
makes it report; an unset marking fails. A terms-store concept carries the same
marking, so a rule resolved from the store reports only when a person marked the
concept advisory. A rule with an empty
replacement is skipped by the translation tools unless it is marked
do-not-translate, because "say this instead" needs a this; the check reads the
same bare term as "avoid this".

`TermRule.MatchesCase` decides whether a rule matches in its own casing. A rule
whose preferred form is capitalised, as a product name is, matches case
sensitively, so "write Quickcast, not QuickCast" flags `QuickCast` and leaves
the lower-case slug `quickcast` alone. A rule whose rejected form differs from
the preferred one only in case (`term: Ripgrep, replacement: ripgrep`) is case
sensitive too. Every other rule matches regardless of case, and
`case_sensitive: true|false` overrides the default in either direction.
`term-check` compares a source term with a replacement in another language, so
it reads only the explicit field. How the store and the rules are located in a
text is [C-08](c-08-terms.md)'s subject.

### Shared constraints and factual guidance

A profile's top-level `constraints` list is independent of its tone and style
sections. Channel and persona overrides replace presentation preferences;
shared constraints remain attached to the selected profile. Profile selection
still chooses one governing profile, with no implicit merge between profiles.

Each constraint carries an ID, version, source reference and statement. A
`prohibited_pattern` has an RE2 expression and emits failing findings through
`profile.PatternFindings`, the shared style-and-constraint path used by the
voice tool and `profile.Findings`. Metadata preserves the constraint's ID,
version and source. A `guidance` record carries a factual requirement into full
and compact guides. Deterministic checks do not verify its meaning, and report
coverage identifies semantic analysis as unsupported. A deterministic gate can
pass while factual guidance still needs review.

Optional scope matches exact locale, channel and persona coordinates. Locale
spelling is normalized without expanding a language to regional variants. A
constraint can carry an exception with a nonempty scope, reason, `approved_by`
and `approval_ref`. These fields record the local author's asserted provenance;
they do not authenticate an approval. Exceptions belong to the shared constraint
and cannot be introduced by a presentation override. Context retrieval exposes
applicable, out-of-scope and excepted records, including matching exceptions.

Constraint decoding rejects unknown keys, and validation rejects duplicate IDs,
invalid kinds, missing provenance, invalid patterns and incomplete exceptions.
Locale scopes cross the shared locale validation boundary; malformed locale
values are configuration errors. A guidance record cannot carry a regex, and a
prohibited pattern must contain a valid expression that cannot match empty text.
The deterministic path reports invalid in-memory constraints as configuration
findings. Semantic disagreement between two guidance statements requires review;
no conflict detector is implied by structural validation.

SQLite and PostgreSQL stores persist constraints in their own JSON column and
include them in archived profile versions. An update that omits constraints
preserves the stored value for clients without the field. An explicit empty
array removes it. The desktop profile-file writer follows the same update rule.
Author constraints in profile YAML; a visual editor may preserve the records
without exposing editing controls for them.

### Findings and scoring

A finding is `profile.VoiceFinding`, a type alias to `check.Finding` from the
framework's content-verification core (`core/check`). It carries a free-form
`Category` (a voice finding sets it to one of the fixed dimensions: tone, style,
vocabulary, clarity, compliance), `Fails`, `Suggested`, a human message, an
optional suggestion, the original text, optional metadata, and a **`Position
model.Anchor`**, so a finding is anchored to the runs it concerns, the same
run-range model overlays and redaction use
([F-02](../foundations/f-02-content-model.md)).

Tools attach findings to a block as a `VoiceAnnotation` (annotation type
`voice`), which also carries the profile id, the overall score and its own
position.

`Fails` is decided by the rule that raised the finding. A term rule, a
prohibited or required pattern, and a shared constraint fail unless the rule is
marked advisory. A finding raised by a suggested rule, one recorded by
`kapi context observe` or `correct` and not yet kept, carries `Suggested` and
never fails. The style measures report: the voice-similarity
check, the model-backed `voice-check`, and the comment limits under
`style.comments`, which fail only when the profile sets `style.comments.fails:
true`.

`profile.CalculateScore` rolls findings up per dimension using the weights in
`core/check.Weight`: a failing finding weighs 25, a reported one 1, a suggested
one 0. Each dimension starts at 100 and is reduced by its penalty, clamped at
0; the overall score is 100 minus the total penalty. The dimensions are fixed, so
a compliance score always has a consistent shape. The score is reported beside
the findings and gates nothing.

This finding and scoring path is shared across every checker
(terminology, do-not-translate, placeholder, register, voice) rather than being
bespoke to voice. Voice is one checkset over the generic core.

### The tools

- **`voice-vocab-check`** (`core/tools`): deterministic and offline. It scans
  source text for the forbidden, competitor and retired terms of a terms store,
  the word rules a caller holds (a starter pack's terms, the rules established
  across the workspace) and the voice's prohibited patterns, emitting findings
  with positions. It is an annotate-class tool: it writes the annotation, never
  the content. This is the fast first pass.
- **`voice-check`** (`core/ai/tools`): model-backed. It asks a provider
  ([E-07](../engine/e-07-model-providers.md)) to score the subjective dimensions
  against the rendered voice guide. It declares that it requires credentials and
  has an API-call side effect, produces the `voice` annotation, and runs with
  bounded per-block parallelism.
- **`voice-infer`** (`core/ai/tools`): model-backed inference of a profile from
  existing content, for a team that has a body of writing and no written-down
  voice.

`kapi check` reports the two halves as separate analyzers. Every word rule,
wherever it is held, is the `terms` analyzer with rule id `terms.vocabulary` and
one message shape (`Forbidden term "x" found`, `Competitor term …`,
`Retired term …`); a finding from a pack or a workspace rule carries
`metadata.from`. The voice's pattern rules are the `voice.rules` analyzer
(`voice.style` and its siblings), and the bilingual term check is `terms.target`.

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
`profile.Findings` matches it per block beside the word rules. A required
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
host loads it (a recipe binds a profile in the project's store, or a starter
pack, which a store id cannot name) and hands it over as the already-loaded
collection profile. So the
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
that depends on where the loop happened to run.

What crosses a push is every declared collection, the point it sits at and the
voice governing it, so both venues resolve the same voice for the same content.
What does not cross is a profile's `termstore:`, which names a store on this
machine, and a local store means nothing to a service that governs terminology
from a shared vocabulary. That divergence is real and is reported rather than hidden: a run
over a project that binds a terms store per profile *and* binds a venue prints a
warning to stderr and proceeds. A recipe field that is not readable at the other
venue is not a reason to refuse the run.

### Profile sources and the command tree

`kapi voice` resolves a profile from one of three mutually exclusive sources:

- `--profile <name>`: a profile in the voice store. Inside a project that store
  is the `voice_profiles` table of the project's context store
  ([C-03](c-03-context-store-and-graph.md)), so the same recipe resolves the
  same profile from any directory in the tree; an explicit `--name`, `--local`
  or `--file` selects a standalone store file instead, mirroring the terms
  store and the content memory, and outside a project the standalone `voice.db`
  is the default.
- `--profile-file <path>`: a standalone, reviewable profile YAML.
- `--pack <name>`: a built-in starter pack.

With no source flag, resolution falls back to the project in scope: the voice
governing the content collection that claims the file, else the recipe's
`defaults.voice`. Every rung answers out of the project's voice store, and
`kapi voice check DRAFT.md` therefore works flag-free inside a project.
Locale and channel overrides apply on top via `--locale`/`--channel`; an explicit
`--channel` wins over the channel the recipe declares.

| Command | Purpose |
| --- | --- |
| `new` | Scaffold a commented, schema-valid profile YAML, optionally seeded from a pack. |
| `guide` / `show` | Render the profile as a markdown voice guide to inject into an assistant's context. |
| `check` | Score text against the profile: its patterns and the word rules its file carries always, `--ai` adds the model check. Exits with the quality-gate code when a finding fails. |
| `rewrite` | Substitute the forbidden and competitor terms the profile's file carries for their approved replacements: deterministic, offline, no model. A rule that matches without a replacement is reported under `skipped`. |
| `validate` | Check a profile document against the schema; blocking problems fail, advisory notes print after the verdict. |
| `profiles` | List profiles: the voice store plus the built-in packs. |
| `import` | Import a profile YAML into the voice store, and the word rules it carries into the project's terms store. |
| `pack` | Install a built-in starter pack into the voice store. |
| `pointer` | Write the marker-delimited section into the project's assistant file (`CLAUDE.md`, or an `AGENTS.md` already at the root) that tells an assistant the voice is held by kapi and that `guide` retrieves it. |

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
It returns the quality-gate sentinel when at least one finding fails, which the
CLI maps to a distinct exit code ([S-01](../surfaces/s-01-kapi-cli.md)) so
skills and CI can tell a failed check from an operational error. Each finding
prints as `[fails/<category>]` or `[reports/<category>]`, and the JSON output
carries `passed`, `failing` and `score`. `kapi check --voice` is the project-level style
gate, with `--voice-min` setting the similarity cutoff.

### Fixing off-voice content

kapi does not send content to a model to rewrite it. An in-voice fix is
caller-supplied: the assistant reads what applies at the point
([C-06](c-06-retrieval.md)), rewrites the off-voice text itself, and applies the
result through the one write verb, `kapi apply`. The edits land through the
byte-faithful round-trip with **no provider involved**: structure and inline
codes are preserved, each block is drift-guarded by its content hash, and an edit
that would corrupt markup is rejected.

`kapi voice rewrite` is a separate, deterministic helper: it substitutes the
forbidden and competitor terms of the word rules a voice file carries (a starter
pack's terms, a file's `terms:` list) for their approved replacements by rule,
offline, through the same matcher as the word-rule check
(`profile.RewriteTermRules`). A profile read from the voice store carries no word
rules, so the project's terms are applied by `kapi check` and fixed through
`kapi apply`.
A rule that names no replacement, and a match on a declared inflected form of a
term, stay in the text and are reported under `skipped` with the term, its list,
whether it fails, the spellings matched and the reason, so a caller can tell an
unchanged text with nothing to fix from one that still carries violations. The
exit code stays 0. It does not call a model and does not touch tone, style or
phrasing; those are the caller's to rewrite.

### A word rule is a change-set entry

Fixing a recurring off-voice term at the *source* (adding a word rule so every
future draft is checked against it) is a `term` entry in the same `kapi apply`
change-set, alongside the content fix that justifies it:

```json
{"kind":"term","op":"upsert","term":"utilize","locale":"en","status":"forbidden","replacement":"use","advisory":true}
```

The entry writes the concept into the project's terms store. `advisory` makes a
use of the term report without failing, and `competitor` records the term as a
competitor's name. The context policy refuses an entry that names an agent as its
actor before anything is written. A change-set has no `voice` kind; an entry
that uses one is refused with a message that gives the `term` form.

A rule somebody notices while working is a term rule.
`kapi context observe --term use --instead-of utilise` records a suggestion,
which checks report and fail nothing on; a person keeping
it writes the rule into the project's terms store, where every check reads it.
See [C-11](c-11-context-operations.md).

### Authoring a profile

`kapi voice new` scaffolds a commented, schema-valid YAML file and
`kapi voice import` loads it into the project's voice store. Use
`kapi voice edit` for subsequent changes: it writes the
stored profile to a temporary file through the snapshot serializer, opens
`$VISUAL` or `$EDITOR`, validates what comes back the way `kapi voice validate`
validates a file, and imports it as one recorded change. The edited document
replaces the full profile, including any deleted sections. An editor error or
an unchanged document leaves the stored profile intact.

### Binding a profile

A recipe binds a voice by name and never by file: `voice: {profile: <id>}`, or
the short form `voice: <id>`, names a profile the project's store holds, and
`voice: {pack: <name>}` a starter pack. A recipe that names a file
(`profile_file:`, or a `voice:` that is a path) fails to load, and the message
identifies the import command and the required name binding. All surfaces
resolve profiles from the store.

`kapi context import` reads the profiles a layout carries: `voice.yaml` at the
top of the layout is the project's, and `profiles/<name>/voice.yaml` belongs to
that profile. When the recipe binds no voice under `defaults:`, the import binds
the project's profile there (or the only profile the layout carries) and says
so. It writes the two lines of the binding into `kapi.yaml` as a text insertion
(`project.BindVoice`), so every other byte of the recipe stays as authored. With
several profiles and none at the top, it binds none and lists them with the line
to add. An agent's import is refused, since reading context in is a person's
decision.

A binding to a missing profile produces an error. This can happen when a clone
has not imported its context or when using a new data directory. The error names the
binding and the two commands that bring the context, `kapi context pull` when
the project shares its context and `kapi context import` in a checkout carrying
the profile.
`kapi voice pack <name>` is suggested only when a starter pack carries that name,
because for any other name it would install something else.

`kapi context import` reads a profile's YAML from the layout path a directory
keys it by ([C-11](c-11-context-operations.md)). The store metadata key
`context.voiceBindings` ties a stored profile's id to the path a layout keys it
by, which is the one place the two are joined.

### Built-in starter packs

The framework embeds a small set of starter packs (`core/profile/packs`, embedded
with `//go:embed`), each a complete profile YAML loaded through the same path as
any other profile. Packs are an on-ramp, not a special case: `kapi voice new
--pack <name>` emits one as an editable base. `kapi voice profiles` lists what is
installed.

### MCP surface

`host/mcp_voice.go` registers two offline voice tools on the shared `kapi mcp`
stdio server ([S-01](../surfaces/s-01-kapi-cli.md)) so non-CLI agents get parity:
`voice_check` scores text using the voice's patterns and the word rules its file
carries, and `voice_rewrite` substitutes forbidden and competitor terms and
reports under `skipped` what it matched and left in place.

These are hand-authored because each wraps a *resource* (a voice profile, a
terms store, a content memory) rather than a single processing tool. The
rendered guide is **not** a tool: it is reached by reading `context://<path>` or
`context://profile/<name>` ([C-06](c-06-retrieval.md)), because the guide is part
of what applies at a point rather than a thing to ask for separately.

## Consequences

- A voice profile is a portable YAML document that works with or without a store,
  reviewable in a diff and reusable across the CLI, MCP, flows and skills.
- The deterministic word-rule and pattern check gives an instant, offline, reproducible
  signal; the model check is a bounded, credential-gated opt-in for the
  subjective dimensions.
- Findings are run-anchored and annotation-shaped, so they compose with the
  content model and surface uniformly rather than through a bespoke side channel.
- The MQM-style scoring is a single function over findings, so every surface
  computes the same score the same way.
- There is one word list: every word rule is a term, and one term-rule shape
  serves the terms store, a voice file and every governed step, so a rule
  declared anywhere is enforced and fingerprinted the same way.

## See also

- [C-02: Coordinates and governance](c-02-coordinates-and-governance.md): which
  profile governs which content.
- [C-08: Terms](c-08-terms.md): the store that holds every word rule, and the
  one pass that locates declared terms.
- [E-03: Tool System](../engine/e-03-tool-system.md): the checks as
  registered tools.
- [E-07: AI Providers](../engine/e-07-model-providers.md):
  the provider behind the model-backed check.
- [F-02: Content Model](../foundations/f-02-content-model.md): run ranges
  and block annotations.
