---
sidebar_position: 18
title: Redaction
description: Redaction replaces sensitive spans (people, product names, internal roles, secrets) with protected placeholders before external translation, then restores originals afterward. Sensitive values never leave the local machine.
keywords: [redaction, sensitive content, privacy, placeholders, translation, data protection]
---

import { RedactionDiagram } from "@neokapi/docs-shared";

# Redaction

Redaction replaces sensitive content (people, unreleased product names,
internal roles, secrets) with protected placeholders before a document is
sent for AI translation or handed to an external translator, then restores the
originals once the translation comes back. The sensitive value never leaves the
local machine.

<RedactionDiagram
  original="Email Sarah Chen the Project Halcyon launch date."
  redact={[
    { text: "Sarah Chen", label: "<x id=1/>" },
    "Project Halcyon",
  ]}
  translated="Envoyez à Sarah Chen la date de lancement de Project Halcyon."
  notes={["", "what the model sees: opaque tokens", "originals restored locally"]}
/>

For the architecture and design decisions behind this, see
[C-10: Content Redaction](/contribute/architecture/context/c-10-redaction).

In the CLI, `redact` and `unredact` are pipeline steps: the built-in
`secure-translate` flow runs redact → translate → unredact
(`kapi run secure-translate`), a project flow can place the same steps around
its own pipeline, and `kapi extract --redact` protects a translator hand-off.
In a project that declares a redaction policy, `kapi exec <tool>` puts the
same two steps around a tool that calls a model, for each file under the
policy that covers it (see [Recipe configuration](#recipe-configuration)). They apply the policy's rules
to every block the tool reads, blocks marked not translatable included, and to
each translation beside the source, plural forms included. The policy covers
blocks only: the term rules, voice profile and content memory a tool sends
with them go as the project holds them. A policy that detects entities stops
the run, because no step ahead of the tool finds them. A tool with no declared
contract does not run under the policy, because nothing says whether it calls a
model. See
[Understanding the CLI layers](/kapi/direct-execution-layer) for the flow and
single-tool surfaces.

The policy also covers `external-command`, because the program it runs may
send what it reads anywhere. The program reads each withheld value as its
placeholder, and kapi puts the original back into the program's output. This
holds for `kapi exec external-command` and for a flow, which needs a `redact`
step ahead of the tool.

## How it stays local

A redacted span becomes a **placeholder**, the same protected inline-code
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

- **Rules** (default) are literal terms and regular expressions you declare.
  Fully offline and deterministic.
- **Entities** (opt-in) are named entities (people, organizations, products,
  locations, dates, …) recognized automatically and redacted by category. A fast
  local model keeps detection on the machine; a cloud model trades that for
  broader coverage during the detection step. Entity recognition is not a
  separate task: it is the same detection that powers entity-generalized
  [content-memory](/framework/content-memory) reuse, so entities
  annotated once serve both.

  The `entity-extract` step selects the detector with `engine`: `llm` (the
  configured AI provider; the default), `ner` (an on-device model, with no
  provider call, no credentials and nothing leaving the machine), or `hybrid`
  (both, merged).
  With `engine: ner` the step carries no remote-egress side effect, so the
  placement pass accepts it ahead of `redact` without qualification. The
  browser [Flow workspace lab](/learn/flow-workspace?c=redaction) runs this end to end with a GLiNER ONNX model loaded
  in the page.

Each match is assigned a category. The recommended categories are `person`,
`role`, `product`, `org`, `location`, and `custom`, but categories are
free-form, so use whatever the placeholder template should display.

## Rules file

The term list is itself sensitive, so it lives in a dedicated file (keep it
gitignored) rather than in the committed recipe:

```yaml
# redaction.yaml — bind it with defaults.redaction.rules, and gitignore it
version: v1
placeholder: "[REDACTED:{category}]"   # {category} and {n} are substituted
detectors: [rules]
rules:
  - term: "Sarah Chen"
    category: person
  - term: "Project Halcyon"            # unreleased codename
    category: product
  - pattern: "ACME-[A-Z]+-\\d+"        # regular expression — internal SKUs
    category: product
    flags: [ignorecase]                # also: wholeword (literal terms)
```

## Workflows

### In-process: secure-translate

The built-in `secure-translate` flow redacts, AI-translates against the
placeholders, and restores the originals, all in a single run, so the secret
is only ever in memory:

```bash
kapi run secure-translate -i src/locales/en.json --target-lang fr
```

The flow is `reader → redact → translate → unredact → writer`. `redact` is
an ordinary [transformer step](/framework/flows#transformers): the framework
applier rewrites the source, replacing sensitive spans with placeholders and
vaulting the originals fail-closed, before the next step observes it. Because
redaction's edit plan is structured, run-anchored overlays attached upstream
(terms, entities) survive the rewrite by rebasing; only spans overlapping a
redacted span are dropped. The flow's placement pass keeps the ordering safe:
it rejects a `redact` placed after any step that sends source to a remote
service. `unredact` is an ordinary late step that restores the originals on
the way out. You can compose both into your own flows as ordered steps:

```yaml
steps:
  - tool: redact
  - tool: translate
  - tool: unredact
```

### External: extract, translate, merge

When a human translator or CAT tool does the translation, redact on the way out
and restore on the way back. `kapi extract --redact` emits a bilingual file
containing only placeholders and writes the originals to a local vault sidecar;
`kapi merge` restores them after the translator returns the file.

```bash
# Emit redacted XLIFF: originals stay in .kapi/vault/batches/<batch>.json
kapi extract --redact

# ... translator fills in targets, preserving the placeholders ...

# Restore the originals into the merged target
kapi merge -i out/app.en-to-fr.xliff
```

Pass `--redact-rules <path>` to point at a rules file directly, or enable
redaction in the recipe (below) so plain `kapi extract` redacts. Use
`kapi merge --no-restore` to keep the placeholders in the merged output.

## Recipe configuration

Declare redaction in the project recipe, and kapi applies it wherever content
leaves the project: `kapi push`, `kapi exec`, `kapi extract`, **Deeper look
with AI** and the record kept of each edit. A policy can be declared in three
places:

- `defaults.redaction`, for every file of the project;
- `redaction:` on a collection, named or bare, for the files it holds;
- `redaction:` on a content item, for the files that item matches.

Each file is redacted under the most specific declaration that covers it: its
item's, else its collection's, else the defaults. That declaration replaces the
others for the file. Its fields are not merged with theirs, so a collection
that names its own `rules` file redacts with that file alone, and one that
needs the default rules as well lists them in its own rules file. A
`redaction:` at any of the three places that sets `rules`, `detectors` or
`placeholder` must set `enabled: true`, and the recipe fails to load if it
does not; `enabled: false` alone turns redaction off for those files.

```yaml
defaults:
  source_language: en
  target_languages: [fr, de]
  redaction:
    enabled: true
    rules: redaction.yaml
    detectors: [rules]
    placeholder: "[REDACTED:{category}]"

collections:
  - name: legal
    redaction:              # replaces defaults.redaction for legal/
      enabled: true
      rules: legal-redaction.yaml
    content:
      - path: "legal/*.md"

  - path: "src/locales/*.json"
    format: json
    redaction:
      enabled: false        # no redaction for this entry
```

Content that sits in no file, such as the text an MCP tool call passes, is
redacted under `defaults.redaction`. A `.kpz` package holds files of every
collection, so in a project that declares a collection's or an item's policy,
`kapi exec` refuses to run a tool that calls a model over a package; run it on
the project's files instead. A redacted package (below) that
`kapi extract --format kpz` wrote in this project is the exception: its files
already hold placeholders, so the tool runs and reads them. A flow places its
own `redact` step, and
kapi refuses to run a flow that sends source to a remote provider without one.
A project flow runs over every file of the run, so it needs the step when any
declaration in the recipe turns redaction on. A built-in flow run on one file
needs it when a policy covers that file.

## Where the originals live

| Workflow | Vault |
| --- | --- |
| In-process (`secure-translate`, custom flows, `kapi exec`) | In memory on the block; removed after restore, and never written to output |
| External (`extract --redact` → `merge`) | `.kapi/vault/batches/<batch-id>.json`, in a directory kept out of version control and written private (`0700`, files `0600`) |
| Package from source files (`extract <files> -o work.kpz --redact-rules <file>` → `merge work.kpz`) | `work.kpz.vault` beside the package, written private (`0600`) |

The vault sits in `.kapi/vault/`, beside `.kapi/work/` and deliberately *not*
inside it. Everything under `work/` is free to delete because it rebuilds; a
withheld original rebuilds from nothing. It is never committed and never synced
to a server, so the only copy is the one on your machine. `rm -rf .kapi/work`
stays safe at any time; deleting `.kapi/` wholesale strands any batch you have
out with a translator, because the merge that would restore the originals has
nothing left to read.

Restoration matches placeholders back to originals by token where the format
preserves inline structure (in-process pipelines and XLIFF), and by the unique
visible token text where the format flattens placeholders to plain text (such
as JSON). On `extract`, redaction runs before content-memory pre-fill, so
no sensitive value can reach the emitted file through a memory match.

A vault keeps each original under the file it came from, the block, and the
placeholder's token. Block IDs and tokens repeat from file to file, so two files
that each withhold a different name keep both names, and `kapi merge` gives each
file its own. A vault written by an earlier kapi records no file, and a merge
still restores from it by block and token.

A PO file has no inline codes, so `kapi extract --format po` writes each
withheld value as its visible placeholder (`[REDACTED:Person]`), and
`kapi merge` restores it by that text.

### Redacted `.kpz` packages

Under a redaction policy, or with `--redact-rules`, `kapi extract --format kpz`
and `kapi extract <files> -o work.kpz` write a redacted package. Each source
file in it holds the visible placeholder (`[REDACTED:Person]`) wherever the
policy withholds a value: the skeleton, the raw source that `--with-source`
adds, and the content-memory matches and terms that travel with the package. A
match or a term in which the policy's rules find a value stays out of the
package. The package's manifest records that it is redacted and the ID of the
vault that restores it.

The vault never travels inside the package:

- `kapi extract --format kpz` keeps it in the project, at
  `.kapi/vault/batches/<vault-id>.json`, one vault for every package of the run.
- `kapi extract <files> -o work.kpz --redact-rules <file>` writes it beside the
  package, as `work.kpz.vault`. Extract never replaces a vault already at that
  path: merge the package it restores first and remove the vault, or extract to
  another name.

Whoever receives the package works on the placeholders. A receiver who runs
`kapi exec` on the package needs the source text in it, so extract it with
`--with-source`: a package that carries only skeletons gives a tool no text to
work on, and merges empty values. `kapi merge` restores each file's values,
in every plural form, from the vault whose ID the manifest records: in the
project for a project package, and beside the package for `merge work.kpz`.
When the vault is not there, or its ID differs, the merge stops before it
writes a file and names the vault it looked for. Keep the placeholders intact
during translation: a translation that still holds a placeholder after the
restore stops the merge, which names the file and the block.

`kapi exec` runs a tool over a redacted package on the placeholders and never
reads the vault, so the package keeps holding placeholders until the merge. In
a project that declares a collection's or an item's policy, a tool that calls a
model runs over a package only when `kapi extract --format kpz` wrote the
package in this project, so its vault is among the project's batch vaults.

Extract refuses a file it cannot redact into a package: one in which the
policy's rules still find a value once the blocks are redacted (in its markup,
in a field kapi does not read as a block, or across an inline code), and one
whose block IDs change once the values are withheld. It also refuses a policy
that detects entities, since a `.kpz` extraction runs no entity-extract step.
A project's XLIFF 2 or PO extraction carries the blocks alone.
