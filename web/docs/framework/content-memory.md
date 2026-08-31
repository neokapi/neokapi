---
sidebar_position: 10
title: Content memory
description: "Content memory is neokapi's store of previously settled content. It holds multilingual entries as Run sequences with inline markup, matches them in three tiers (plain, structural, and source-entity) so high-quality matches are returned first, and hands a block's previously approved translation to the translate step as governed reference."
keywords: [content memory, reuse, leverage, fuzzy matching, runs, inline markup, SQLite]
---

# Content memory

neokapi's **content memory** lives in the `memory/` package. It works with the
full content model: each entry holds multilingual variants as `Run` sequences
(text plus inline markup) and is matched in three tiers with entity-aware
adaptation. The same engine backs the `kapi memory` commands, the `recycle`
pipeline tool, the translate step's prior-version reuse, and the Go library.

In the CLI, content memory is the engine under `kapi exec recycle` (the
single-tool leverage pass) and under the first step of `kapi up`'s default flow,
which recycles from memory before any AI translation runs. See
[Understanding the CLI layers](/kapi/direct-execution-layer) for how the
single-tool, flow, and project-loop surfaces relate.

## Content-aware matching

Each entry is indexed under three keys, tried in order, so the highest-quality
match is returned first:

| Tier | Match type      | Normalizes                          | Example                                    |
| ---- | --------------- | ----------------------------------- | ------------------------------------------ |
| 1    | **Generalized** | Named entities → typed placeholders | "Welcome, John" → "Welcome, \{PERSON\}"    |
| 2    | **Structural**  | Inline markup → normalized codes    | "Click **here**" → "Click \{1\}here\{/1\}" |
| 3    | **Plain**       | Nothing (raw text)                  | Levenshtein fuzzy matching                 |

Each tier yields exact (100%) or fuzzy matches. When a generalized exact match
is found, entity values from the current source are adapted into the stored
target, so "Welcome, Bob" → "Bienvenue, Bob" adapts to "Welcome, Alice" →
"Bienvenue, Alice" at 100%. This ordering mirrors how a translator evaluates
matches: entity differences matter less than structural ones, which matter less
than textual changes.

The typed placeholders the generalized tier keys on (`{PERSON}`, `{PRODUCT}`, …)
come from entity detection, a fast local model or an LLM that recognizes the
named things in a block. You don't run detection as a separate task: it happens
as part of preparing content, and the same detection also powers
[redaction](/framework/redaction). Annotate entities once and both generalized
memory reuse and redaction follow.

## Storage backends

Two backends ship in the `memory/` package, both implementing the
`ContentMemory` interface with full tier support:

1. **In-memory** (`memory.NewInMemoryStore`): fast and ephemeral, used for
   session-scoped batch processing.
2. **SQLite** (`memory.NewSQLiteStore`): persistent file-based storage for CLI
   workflows.

The interface also accommodates server-side backends for multi-user
deployments with project scoping, streams, and workspace isolation. Fuzzy
matching uses Levenshtein edit distance with a configurable threshold (default
0.70); results are sorted by score and then by tier.

## CLI usage

### Resource location

All `kapi memory` commands (except `list`) accept these mutually exclusive flags:

| Flag            | Resolves to                   | Example                    |
| --------------- | ----------------------------- | -------------------------- |
| `--name <n>`    | `~/.config/kapi/memory/<n>.db`    | `--name project-memory`    |
| `--local`       | `./memory.db` (current directory) | `--local`                  |
| `--file <path>` | Explicit file path            | `--file /shared/memory.db` |
| _(no flag)_     | Same as `--local`             |                            |

Databases are created on demand if they don't exist. `memory/` and `memory.db`
are the on-disk names for a named or loose content memory. `~/.config/kapi` is
the user config directory on Linux, and resolves to
`~/Library/Application Support/kapi` on macOS. `kapi config path` prints the
resolved location.

With no flag inside a project, the content memory is instead a set of tables in
the project's `.kapi/work/store.db`; see
[Memory & terms storage](/kapi/recipes/memory-and-terms-storage).

```bash
kapi memory import translations.memory.json --name project-memory
kapi memory export --name project-memory -o project.memory.json
kapi memory lookup "Welcome to our platform" --name project-memory -s en -t fr
kapi memory search "welcome" --name project-memory -s en
kapi memory stats --name project-memory
kapi memory list
```

## Pipeline integration

The `recycle` tool queries content memory for each Block's source segments and
applies matches. Every match, exact or fuzzy, is recorded as an
`AltTranslation` annotation (matched source/target runs, score, match type),
and a filled target is committed with provenance
(`Origin{Kind: "memory", Tool: "recycle"}`), its score, and `draft` status, so
the leverage is auditable rather than an opaque overwrite. Exact matches skip AI
translation, reducing cost and latency.

**Segment-aware leverage.** When a block carries a multi-segment
[segmentation](/framework/segmentation) overlay (a prose paragraph split into
sentences), `recycle` looks up content memory **per sentence**. This recovers
leverage for multi-sentence blocks that would never match the sentence-keyed
memory as a single unit. A single-segment block (most UI strings) takes the
whole-block path unchanged.

The result is recorded so it is **auditable**:

- Each matching sentence is attached as an `AltTranslation` annotation (matched
  source and target runs, score, exact/fuzzy match type), kept
  whether or not the block target is filled, so **partial** leverage (some
  sentences matched, some new) is preserved for a reviewer or a later
  translation stage rather than discarded.
- The block records `tm-segment-matches` (e.g. `3/5`) for quick gating.
- The block target is filled only when **every** sentence matched and the
  segments are contiguous; when it is, the committed target carries
  provenance (`Origin{Kind: "memory", Tool: "recycle"}`), the roll-up `Score`,
  and `draft` status: a reviewable pre-fill rather than a signed-off translation.

Run [segmentation](/framework/segmentation) before `recycle` to enable this.

```bash
kapi exec recycle input.html -o output.html --source-lang en --target-lang fr --memory project-memory
```

```yaml
steps:
  - tool: recycle
    config:
      fuzzyThreshold: 70 # minimum score for fuzzy matches (0-100)
      fillTarget: true # copy the best candidate into the target
      fillTargetThreshold: 95 # minimum score required to fill the target
```

The two thresholds default to 70 and 95. The fuzzy path below an exact match is
retiring in favour of the governed reuse below, and the defaults stay where
they are on the paths that run without a translate step.

## Governed prior-version reuse

Recycle answers by *source text*: the same sentence, or one close to it, gets
the same translation. The translate step asks a second question by *unit*: what
was the previously approved translation of this block, before its source was
edited? The answer goes into the prompt as reference, so the model redrafts
from wording a person already accepted rather than from nothing.

The read is gated on governance. A `VersionRequest` names the unit (the block's
identity across edits), the point the content sits at, the source and target
locales, and `GovernedBy`, the fingerprint of the voice guidance and term rules
about to reach the model. An answer approved under any other context is
withheld, because a translation accepted under a different voice or vocabulary
is a wrong reference rather than a helpful one. The translate tool's `reuse`
setting (`prior`, the default, or `none`) turns the read on or off; the
`context:` setting is separate, because a prior version costs a corpus read
where a block's neighbours come free from the document in hand.

```go
v, ok := corpus.PriorVersion(ctx, memory.VersionRequest{
    Unit:       block.ChainUnit(),
    Point:      "kapimart/web",
    Source:     "en",
    Target:     "nb",
    GovernedBy: contextFingerprint,
})
```

## Go library

### Interface

```go
type ContentMemory interface {
    Add(entry Entry) error
    Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]Match, error)
    LookupText(source string, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]Match, error)
    Delete(id string) error
    Count() int
    Close() error
}
```

(Methods, and the TMX helpers below, take a `context.Context` as their first
argument in the real API; it is elided here for readability.)

`Lookup` takes a full `*model.Block` and uses its Run content (and entity
annotations) for tiered matching; `LookupText` takes a plain string and
performs plain-tier matching only. `LookupSegment` matches a single segment of
a block for sentence-level leverage. Both SQLite and in-memory backends also
implement `EntryProvider` (`Entries()`), which is how TMX export enumerates a
store, and offer paginated `SearchEntries(...)` for browsing.

### Key types

```go
type Entry struct {
    ID          string
    ProjectID   string
    Variants    map[model.LocaleID][]model.Run // peer language variants
    HintSrcLang model.LocaleID                 // locale the author treated as canonical
    Entities    []EntityMapping                // entity placeholders
    Properties  map[string]string
    Origins     []Origin
    Note        string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type Match struct {
    Entry             Entry
    Score             float64 // 0.0-1.0
    MatchType         MatchType
    ProjectID         string             // provenance of the matched entry
    EntityAdaptations []EntityAdaptation // entity value substitutions
}

type LookupOptions struct {
    MinScore     float64      // minimum match score (default 0.7)
    MaxResults   int          // max results to return (default 10)
    MatchModes   []MatchMode  // which tiers to use (default: all)
    ProjectID    string       // project context for scoring boost
    ProjectScope ProjectScope // project filtering mode (default: all)
}
```

An entry is multilingual: there is no authoritative source at the persistence
layer; each language is a peer `Variants[locale]` Run sequence, and the lookup
direction is supplied at the call site. `MatchType` ranges from
`generalized-exact` (highest reuse) through `structural-exact`, `exact`, the
corresponding fuzzy variants, down to `fuzzy`. `Entry` helpers:
`Variant(locale)`, `VariantText(locale)`, `VariantStructural(locale)`,
`VariantGeneralized(locale)`. The `EntityAdaptations` field on a match lists
each substitution with its position so consumers can apply adaptations
precisely.

### Example

```go
package main

import (
    "fmt"

    "github.com/neokapi/neokapi/core/model"
    "github.com/neokapi/neokapi/memory"
)

func main() {
    tm := memory.NewInMemoryStore()
    defer tm.Close()

    tm.Add(memory.Entry{
        ID: "e1",
        Variants: map[model.LocaleID][]model.Run{
            "en": {{Text: &model.TextRun{Text: "Welcome to our platform"}}},
            "fr": {{Text: &model.TextRun{Text: "Bienvenue sur notre plateforme"}}},
        },
        HintSrcLang: "en",
    })

    block := model.NewBlock("b1", "Welcome to our platform")
    matches, err := tm.Lookup(block, "en", "fr", memory.DefaultLookupOptions())
    if err != nil {
        panic(err)
    }
    for _, m := range matches {
        fmt.Printf("Score: %.0f%% Type: %s Target: %s\n",
            m.Score*100, m.MatchType, m.Entry.VariantText("fr"))
    }
}
```

### TMX import / export

```go
// Importing keeps only the named bilingual pair; ImportTMXLocalePairs keeps an
// arbitrary set of locales, and an empty set keeps every <tuv> present.
count, err := memory.ImportTMXWithOptions(tm, reader, "en", "fr",
    memory.ImportTMXOptions{OriginKey: "corpus.tmx"})

// ExportTMX emits one <tu> per entry with a <tuv> per selected locale; an
// empty locale list keeps every variant present.
err = memory.ExportTMX(tm, writer, []model.LocaleID{"en", "fr"})
```

Import requires a backend that supports import sessions, that is, one
implementing the persistent `Store` interface.

## Content memory and terminology

Content memory and [terminology](/framework/terminology) are deliberately
separate systems with different data shapes: memory stores segment pairs,
terminology stores multi-locale concepts. They share the `Block` annotation
system as their integration point, so both kinds of match are available to any
downstream tool or editor.
