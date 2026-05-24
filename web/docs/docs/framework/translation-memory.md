---
sidebar_position: 9
title: Translation Memory
description: Sievepen is neokapi's built-in translation memory. It stores multilingual entries as Run sequences with inline markup and matches them in three tiers — plain, structural, and source-entity — so high-quality matches are returned first.
keywords: [translation memory, Sievepen, TM leverage, fuzzy matching, runs, inline markup, SQLite]
---

# Translation Memory

neokapi's translation memory is **Sievepen** (`sievepen/`). Unlike traditional
TMs that store plain strings, Sievepen works with the full content model — each
entry holds multilingual variants as `Run` sequences (text plus inline markup)
and matches them in three tiers with entity-aware adaptation. The same engine
backs the `kapi tm` commands, the `tm-leverage` pipeline tool, and the Go
library.

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
target — so "Welcome, Bob" → "Bienvenue, Bob" adapts to "Welcome, Alice" →
"Bienvenue, Alice" at 100%. This ordering mirrors how a translator evaluates
matches: entity differences matter less than structural ones, which matter less
than textual changes.

## Storage backends

Two backends ship in the `sievepen/` package, both implementing the
`TranslationMemory` interface with full tier support:

1. **In-memory** (`sievepen.NewInMemoryTM`) — fast and ephemeral, used for
   session-scoped batch processing.
2. **SQLite** (`sievepen.NewSQLiteTM`) — persistent file-based storage for CLI
   workflows.

The interface also accommodates server-side backends for multi-user
deployments with project scoping, streams, and workspace isolation. Fuzzy
matching uses Levenshtein edit distance with a configurable threshold (default
0.70); results are sorted by score and then by tier.

## CLI usage

### Resource location

All TM commands (except `list`) accept these mutually exclusive flags:

| Flag            | Resolves to                   | Example                    |
| --------------- | ----------------------------- | -------------------------- |
| `--name <n>`    | `~/.config/kapi/tm/<n>.db`    | `--name project-tm`        |
| `--local`       | `./tm.db` (current directory) | `--local`                  |
| `--file <path>` | Explicit file path            | `--file /shared/memory.db` |
| _(no flag)_     | Same as `--local`             |                            |

Databases are created on demand if they don't exist.

```bash
kapi tm import translations.tmx --name project-tm -s en -t fr
kapi tm export --name project-tm -s en -t fr -o output.tmx
kapi tm lookup "Welcome to our platform" --name project-tm -s en -t fr
kapi tm search "welcome" --name project-tm -s en
kapi tm stats --name project-tm
kapi tm list
```

## Pipeline integration

The `tm-leverage` tool queries the TM for each Block's source segments and
applies matches. Exact matches skip AI translation, reducing cost and latency;
fuzzy matches are attached as `AltTranslation` annotations for translator
review.

```bash
kapi ai-translate -i input.html -o output.html -s en -t fr --tm project-tm
```

```yaml
steps:
  - tool: tm-leverage
    config:
      fuzzyThreshold: 70 # minimum score for fuzzy matches (0-100)
      fillTarget: true # copy the best candidate into the target
      fillTargetThreshold: 95 # minimum score required to fill the target
```

## Go library

### Interface

```go
type TranslationMemory interface {
    Add(entry TMEntry) error
    Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]TMMatch, error)
    LookupText(source string, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]TMMatch, error)
    Delete(id string) error
    Count() int
    Close() error
}
```

`Lookup` takes a full `*model.Block` and uses its Run content (and entity
annotations) for tiered matching; `LookupText` takes a plain string and
performs plain-tier matching only. `LookupSegment` matches a single segment of
a block for sentence-level leverage. Both SQLite and in-memory backends also
implement `EntryProvider` (`Entries()` and paginated `SearchEntries(...)`) for
export and browsing.

### Key types

```go
type TMEntry struct {
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

type TMMatch struct {
    Entry             TMEntry
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
layer — each language is a peer `Variants[locale]` Run sequence, and the lookup
direction is supplied at the call site. `MatchType` ranges from
`generalized-exact` (highest reuse) through `structural-exact`, `exact`, the
corresponding fuzzy variants, down to `fuzzy`. `TMEntry` helpers:
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
    "github.com/neokapi/neokapi/sievepen"
)

func main() {
    tm := sievepen.NewInMemoryTM()
    defer tm.Close()

    tm.Add(sievepen.TMEntry{
        ID: "e1",
        Variants: map[model.LocaleID][]model.Run{
            "en": {{Text: &model.TextRun{Text: "Welcome to our platform"}}},
            "fr": {{Text: &model.TextRun{Text: "Bienvenue sur notre plateforme"}}},
        },
        HintSrcLang: "en",
    })

    block := model.NewBlock("b1", "Welcome to our platform")
    matches, err := tm.Lookup(block, "en", "fr", sievepen.DefaultLookupOptions())
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
count, err := sievepen.ImportTMX(tm, reader, "en", "fr")
err = sievepen.ExportTMXBilingual(tm, writer, "en", "fr") // src/tgt pair
// or, for all locales in the TM:
err = sievepen.ExportTMX(tm, writer, []model.LocaleID{"en", "fr", "de"})
```

## Translation memory and terminology

TM and [terminology](/framework/terminology) are deliberately separate systems
with different data shapes — TM stores segment pairs, terminology stores
multi-locale concepts. They share the `Block` annotation system as their
integration point, so both kinds of match are available to any downstream tool
or editor.
