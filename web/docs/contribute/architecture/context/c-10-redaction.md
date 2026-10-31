---
id: c-10-redaction
sidebar_position: 10
title: "C-10: Redaction and clearance"
description: "Architecture decision: redaction replaces sensitive spans with protected placeholders before content leaves the machine, keeping the original in a local vault; the policy is read at extract, at ingest, by single-tool runs, and by the flow placement gate."
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
| A per-batch JSON sidecar | `.kapi/vault/batches/<batch-id>.json` | the extract → external tool → merge round trip, a project's redacted `.kpz` packages among them |
| A package sidecar | `<package>.kpz.vault`, beside the package | a redacted `.kpz` extracted from source files outside a project |
| The project vault | `.kapi/vault/redaction.json` | continuous ingest, which has no batch id to look under |

The in-process annotation is keyed under a name no format writer serializes, and
`unredact` deletes it after restoring, so it cannot reach an output file. The
sidecars and the vault are written with restrictive permissions.

A sidecar or the project vault keys each original by document, block ID and
token (`redaction.RedactedValue.Doc`, the project-relative path). Block IDs
repeat across files and the token counter starts again in every block, so a
key without the document lets one file's original overwrite another's. Content
that sits in no file (an edit record, a note) names itself in its block ID and
records no document. `Flush` writes version 2 (`fileVaultVersion`), which
records the document and, for a vault a redacted package names, the vault's ID
(`redaction.FileVault.ID`, optional), and
`redaction.OpenFileVault` reads from version 2 (`oldestVaultVersion`) up to the
version it writes; the floor stays put when the written version moves. A file
with no version, or below the floor, was written by an earlier kapi, and it is
refused with an `*OlderVaultError` (`ErrOlderVault`) that names the version
found; one above the written version was written by a newer kapi, and it is
refused with `ErrNewerVault`. Either way merge fails for that file, and a
write to the workspace home and a read of a kept edition fail. The caller
names the command that writes a new vault (`redaction.WithRun`): `kapi
extract` for a batch sidecar or a package sidecar, whose entries belong to the
files that extraction wrote, and, for the project vault, a run after moving the file
aside, since the old file holds the only copy of every recorded edit's
originals. The vault is the only copy of the originals, never a cache, so kapi
leaves a refused file as it is.

`vault/` sits beside `work/`, outside it. **`work/` is defined by being
disposable**: losing it costs CPU. **The vault is defined by an exclusion**: a
named destination must never read it, and losing it means redacted content can
never be restored. So `rm -rf .kapi/work` stays free, and kapi never deletes the
vault on its own initiative. `core/project.Layout.PrepareVault` writes the
directory owner-only and adds it to `.kapi/.gitignore` when that file ignores
less than the whole of `.kapi/`, before anything is withheld into it.

For model calls the guarantee is *structural*, not advisory: a block with inline
codes is rendered through the placeholder protocol, which presents each
placeholder as an opaque token (the model sees neither the original nor even the
visible label), and the return is matched back to the source run by id, so the
placeholder survives the round trip with full fidelity
([M-05](../multilingual/m-05-prompts-and-batching.md)).

### Redacted packages

A `.kpz` carries each source file as a skeleton (whose entries hold the text of
every block, rendered and as written), with the raw bytes under
`--with-source`, and a skeleton has no placeholder form. So a package written
under a policy carries a different file: `host.redactSourceForKpz` runs the
`redact` tool over the source in external mode, a step that draws each
redaction placeholder as its visible token (`redaction.ShowTokens`), and the
format writer, which writes the token as text whatever the format. The
skeleton, the raw source and the content-memory lookups all read that file.
The policy's rules, compiled into a detector, read each memory match and each
terms concept as JSON, and one they find a value in is left out of the
package, whichever file the value comes from.

The package works on tokens from there on, so merge restores by visible token
within the block, in every plural form and select case
(`redaction.RestoreTree`), as it does for PO, keyed by the file's name in the
package and the block ID. The block IDs of the redacted file are those of the
original: extract reads both and refuses the file when they differ, since merge
matches by block. The same detector then reads the redacted bytes and the text
of every redacted block, and extract refuses the file when it finds a value:
one in the markup or in a field the reader does not extract, which the redact
step never sees, or one across an inline code, where `redaction.Redact` skips
a match. A policy that detects entities is refused before anything is read,
since extract runs no entity-extract step to find them.

The manifest records `redaction.vaultId` (`kpz.Redaction`), and the vault file
records the same ID. The vault stays where the route keeps its originals: the
project's batch vault, named by its ID, for `kapi extract --format kpz`, and
`<package>.vault` beside the package for an extract from source files, which
refuses to replace a vault already at that path. A workspace cache carries the
record in its metadata, with the package's kind, interchange task and each
file's project path, and a pack writes them back, so a package that a receiver
ran a tool over and packed still merges as what it is. `kapi merge` refuses a
redacted package before it writes anything when the vault is absent or its ID
differs (`host.packageVault`); a vault that records no ID is refused the same
way. Merge then restores through the batch-vault path for an interchange
package (`restoreRedactedBlocks`) and an `unredact` step in external mode for a
workspace. A translation that still holds a placeholder prefix after the
restore stops the merge with the file and the block: a translator changed the
placeholder. The workspace route merges once into the cache to find one
before it writes any file.

The interchange task's revisions in the manifest are unsalted hashes of the
original runs, as they are in XLIFF 2 and PO, because merge compares them with
the source the project holds.

`kapi exec` never reads the vault: a tool over a redacted package reads and
writes tokens, and the originals reach a file only at merge. The refusal of a
tool that sends blocks out over a package in a project with a collection's or
an item's policy does not apply to a package whose vault is among the project's
batch vaults (`host.packageVaultHere`): `kapi extract --format kpz` wrote it in
this project and withheld each file's values under that file's policy. A vault
beside a package proves nothing about which rules withheld its values, so any
other redacted package is refused like any other package.

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

### The policy and its readers

The recipe declares redaction under `defaults.redaction`, on a collection
(named or bare) and on a content item, each pointing at a separate rules file
so the sensitive term list stays out of the committed recipe:

```yaml
defaults:
  redaction:
    enabled: true
    rules: redaction.yaml         # binds any path; keep it out of version control
    detectors: [rules]            # opt in: entities
    placeholder: "[REDACTED:{category}]"
collections:
  - name: legal
    redaction:                    # replaces defaults.redaction for these files
      enabled: true
      rules: legal-redaction.yaml
    content:
      - path: "legal/*.md"
```

A file's policy is the most specific declaration that covers it: the item that
claims the file (the claim `ItemForPath` makes), else that item's collection,
else the defaults (`KapiProject.RedactionForPath`, which `host.RedactionFor`
wraps). The declaration replaces the others whole. Rule files, detectors and
placeholders are not merged across levels, so what a collection declares is
the complete policy for its files, and `enabled: false` takes them out of
redaction. One check (`RedactionSpec.validateAt`) holds every level to the
same shape: a declaration that sets `rules`, `detectors` or `placeholder`
without `enabled: true` fails the recipe load. Content in no file (an MCP call's text) falls under the defaults.

A policy with one reader is a policy that fails open everywhere else, so the
policy is read at every point content can leave, and each reader asks for the
policy of the file it holds:

1. **`kapi extract`** resolves it per source, before it reads any, and applies
   it, emitting a redacted bilingual file and writing the batch sidecar. The
   `--redact` and `--redact-rules` flags layer over the policy each source
   resolves. Redaction runs **before** memory pre-fill, so
   the content memory is queried with redacted text and pre-fills targets from
   it, and no sensitive value reaches the emitted file by way of a memory match.
   On `kapi merge` the incoming source is always restored, so per-block
   staleness compares original against original; the target is restored unless
   `--no-restore` is set.
2. **Ingest**, which is the push to a venue (`host/venue/source`).
   `host.RedactionFor` resolves each file's policy and `host.RedactAtIngest`
   applies it as the file is read in, vaulting into the project vault. Ingest
   is continuous rather than batched, which is why its vault is project-scoped:
   a later restore has no batch id to look under.
3. **The flow placement gate.** A project that declares redaction **rejects any
   flow that sends source to a remote destination without a `redact` step**
   (`FlowDefinition.CheckRedactionCoverage`, enforced in flow placement). This is
   a gate rather than an implicit rewrite: silently inserting a step into a
   user's flow would change what their flow means, while refusing to run it says
   so. A project flow's tools are built once for every file of a pass, so the
   gate holds it when any declaration turns redaction on
   (`KapiProject.DeclaresRedaction`); a built-in flow built for one input file
   is held when a policy covers that file. A project flow's input set splits
   by the policy that governs each file (`groupInputsByBinding`), and a
   `redact` step that names no `rulesPath` or `rules` of its own redacts each
   part under that part's policy (`App.applyFlowRedaction`); a step that names
   its own rules keeps them. `kapi run` and `kapi up` reach the step through
   the same assembly.
4. **A single-tool run.** `kapi exec <tool>`, and the MCP tools that run one
   registry tool over a caller's text, compose no flow of the user's, so the run
   itself places the steps (`host/toolrun_redact.go`). When the tool's contract,
   refined for its config, carries an API call, `redact` runs ahead of it with
   the rules of the policy that covers the file, settled per file before any
   file is read, and `unredact` after it. A `.kpz` package is one chain over
   every file it holds, so under a collection's or an item's policy such a run
   is refused. The originals ride on the
   block in between as they do in `secure-translate`. The tools a run reaches
   read the whole block, so the step is configured for it (`WholeBlock`): it
   redacts blocks marked not translatable, and each target locale's runs under
   tokens of their own. Detection reads a run sequence's own text runs, the
   coordinates the carve uses, and every plural form and select case is
   redacted the same way under a prefix of its own. Unredact puts back an
   edition whose restored content equals what redact read exactly as it was
   read, and re-stamps a translation's status and origin, so the two steps
   around a tool that left a translation alone change nothing about it. A
   collector that sends what it gathered to a model
   (voice-infer's draft) reads the blocks between the two steps. A policy that
   detects entities is refused, because entity detection reads annotations an
   extraction step leaves and a run of one tool has none. A tool with no
   declared contract (no registry entry, or a schema with no `toolMeta`) is
   refused before it reads a block, because nothing says whether it calls a
   provider: over a file a policy covers, and over a `.kpz` package in a
   project that declares any policy. A tool that calls no provider runs alone. The chain each run builds is held to
   `CheckRedactionCoverage` and `CheckPlacement`, so a run without the redact
   step fails the gate a flow fails.

   `external-command` hands each block to a program kapi runs, which may send
   it anywhere, so its contract declares the `process-exec` side effect and
   the policy covers it as it covers a provider call: a run places `redact`
   and `unredact` around it, a run over a `.kpz` package is refused under a
   collection's or an item's policy, and a flow that runs it needs a `redact`
   step ahead of it. The program reads each placeholder as its visible token
   (`redaction.VisibleText`). The tool compares the tokens of its input with
   those of the program's output (`redaction.CompareTokens`); a token the output
   drops, alters, repeats or adds leaves the block as it was and records a
   failing `external-command.redaction-placeholder` finding, which `kapi exec`
   reports with exit code 3 and a flow run reports beside its other findings.
   A flow step's finding appears in the run's output and is not part of the
   coverage count `kapi up` derives from the written files.
5. **The deeper look.** It sends chunks of the project's block texts to a model
   (`host/contextdeeper.go`). A chunk holds blocks of files under one policy
   only, and each chunk is redacted under that policy before the model reads
   it. Each chunk then carries that policy and the values read back from the
   vault, so the rest of its requests is redacted as below.
6. **The operation log.** The record of an applied edit travels to every
   context backend the project shares its context through, so the recorder
   redacts the runs and the note it keeps of each document under that
   document's policy, vaulting each run sequence under the edition and revision
   it belongs to, and leaves the change set as sent out of every record of a
   project that declares a policy
   ([C-03](c-03-context-store-and-graph.md#edits-are-recorded-as-content-edit)).
   A document the recorder knows by key alone is looked up in the document
   index. One whose file it still cannot name falls under every policy the
   recipe declares: under one, it is redacted with it; under several, its
   record keeps revisions and hashes only and the workspace home refuses to
   keep it, because successive passes of the redact tool number their tokens
   alike and would overwrite each other's originals in the vault.
   A translation with no file is kept in the log itself (the workspace home),
   so its record is redacted the same way and the workspace home restores the
   originals from the project vault where it reads the edition: on the machine
   that withheld them the edition reads whole, and anywhere else it reads with
   its placeholders. A policy that detects entities needs a read's entity
   annotations, which a record does not carry, so under one a record keeps
   revisions and hashes only and the workspace home keeps no edition.

### Everything a request carries

A request to a model carries more than the block, and the policy covers all
of it. The project strings a request can carry, by where the prompt takes
them from:

| String | Source | Prompts |
| --- | --- | --- |
| The block, its translations, its plural forms | the block | every prompt |
| The neighbouring blocks | the document | translate |
| The block's key | the block's name | translate |
| The previous source and approved translation | the content memory (`prompt.Context.Prior`) | translate |
| Each term and its replacement, the do-not-translate terms | the term rules (`profile.TermRuleMap`) | translate, review |
| Guidance, style, patterns, examples | the voice profile (`profile.RenderVoiceGuide`) | translate, review, voice-check |
| The instruction | `--instruction`, the recipe, a fix pass's findings | translate, segment |
| The domain | a tool preset | term-extract, voice-infer |
| The known terms | the terms store | entity-extract |
| The neighbouring lines | the document | media-refine |

The redact step leaves its policy (`redaction.Policy`: the rule detector and
the placeholder options) on every block it reads in-process, in the block's
secret annotation, whether or not the block holds a value to withhold. A
tool's view of a block puts a `redaction.Egress` over that entry on the
view's context (`tool.ContextBinder`). A tool that sends several blocks in one
request binds them together: the translate batch (`redaction.BindBlocks`),
and the voice-infer corpus and the media-refine neighbours
(`redaction.BindCorpus`). Each exported AI tool constructor and prompt
function in `core/ai/tools` wraps the provider it is given in
`aiprovider.Redacting`, whoever built the provider; the wrapper reports the
provider's limits and does nothing for a context with no Egress. It reads the
Egress from the call's context and redacts each text part and section of a
chat, and every field of a translate request, before the provider sends it.
The recorder sits inside the wrapper, so `--explain-prompts` shows the
request as it was sent.

The Egress matches the policy's rules, and every original the request's
blocks already withhold, the latter by literal text, so a value an entity
step found is redacted beside the block too. A JSON payload in a prompt (the
translate batch) holds each string escaped, so the Egress also reads the
unescaped text and replaces each match at its escaped span, with the token
escaped the same way. Entity detection itself reads the blocks only: a value
named only beside the block is redacted when a rule names it or a block of
the request withholds the same text. Tokens are shared within a request:

- A value a block withholds is drawn as the token the request already shows
  for it: its placeholder tag when the prompt carries the block's runs as
  tags (translate), its visible token (`[REDACTED:Person]`) otherwise. In a
  batch, the value is also kept in each other block's entry, so it restores
  in whichever block's answer the model copies it into.
- A value met only beside the block takes the next token in the block's
  sequence, under a visible token that no other value of the request uses and
  that the request's text does not already hold, and is kept in the block's
  entry. `unredact` restores it from there by its visible token, so a token
  that matched text the person wrote would replace that text.
- A span inside a token the request already shows is left alone.

Only the request that produced a translation fills the entry its tokens
restore from, so the translate tool neither serves a block that carries an
entry from the session's target cache nor keeps its translation there. A
later redact step keeps the values an earlier one left in the entry and
takes the entry's policy over.

A prompt that carries text alone (review, quality check, term-extract,
voice-check, voice-infer, entity-extract, the media-refine neighbours, the
deeper look) draws each placeholder as its visible token
(`redaction.VisibleText`), so the model reads a token where the value was.
entity-extract reports spans in the text it sent, and
`redaction.VisibleSource` maps each back to the runs; a span on a token names
a withheld value and is dropped. The LLM segmenter reads the block as every
segmentation engine does, with each inline code masked, because its chunks
must align with the runs.

The entry grows in memory only, by at most one value per distinct original
the rest of the request carries and the block does not hold, and `unredact`
discards it with the entry. On `samples/northsea`, a policy over the product
and company names adds 250 values to the 23 the redact step recorded, across
136 blocks, at most 2 in one block: the voice profile names both, and every
request carries it. The deeper look reads the project vault and writes
nothing back to it.

Two commands ask a model outside a run of a registry tool. `kapi voice check
--ai` runs its tool between redact and unredact (`host.VoiceCheckTools`)
under the policy of the file it reads, or `defaults.redaction` for text given
inline. `kapi terms expand` holds no block, so it redacts each request under
the rules of `defaults.redaction` (`host.TermsExpandRedaction`); a project
whose only policies sit on collections or items gives it none to apply. A
withheld term goes out as its token, and the forms the model returns for a
token name no term that was asked about, so none is written. The AI actions
on Kapi Desktop's review (Fix with AI, Retranslate, Explain, the AI
pre-review) run their tool through `host.RedactionChain`, under the policy of
the unit's file.

A machine-translation engine reads a redacted block through semantic HTML:
each withheld value is a placeholder element
(`<span data-type="redaction:person"/>`) that maps back to its placeholder by
position, and unredact restores it by id, as the translate tool's placeholder
tag does. The engine never reads the visible token as text: an engine
translates text, so `[REDACTED:Person]` could come back reworded and match
nothing, where it keeps markup in place.

Not covered:

| Path | What reaches the provider |
| --- | --- |
| Media payloads (media-refine's image crop, speech clip, video frame) | The media as recognised; the rules match text, and a crop or clip holds none |
| Machine-translation plugin engines (`translate` with an MT provider) | The redacted block, each withheld value a placeholder element; no term rules, voice profile or other project text. The request goes through the plugin's provider, not `aiprovider.Redacting`, so anything the plugin sends beyond it is outside the policy |

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
