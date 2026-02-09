---
id: 017-locale-handling
sidebar_position: 17
title: "ADR-017: Locale Handling and BCP-47"
---
# ADR-017: Locale handling and BCP-47 validation

## Context

Localization inherently revolves around language tags. gokapi uses `model.LocaleID`
(a `string` typedef in `core/model/locale.go`) throughout the codebase: CLI flags,
KAZ manifest files, TM entries, terminology terms, Bowrain project creation, and
format readers/writers. The type's doc comment says "BCP 47 language tag" but no
validation, normalization, or display name logic existed.

Meanwhile, the Bowrain desktop app used free-text `<input>` fields for language
selection (e.g., typing "fr, de, ja") and displayed raw locale codes everywhere
("en → fr, de"). Translators work with dozens of languages and expect to select
from a searchable list with friendly names like "French" rather than remembering
ISO codes.

The `golang.org/x/text` package (already in `go.mod`) provides the
`language` and `language/display` subpackages for BCP-47 parsing and display
name resolution, but they were unused.

## Decision

### LocaleID canonical form

`model.LocaleID` remains a `string` typedef. Values must be valid BCP-47 tags
in their shortest canonical form — e.g., `en` not `en-Latn-US`, `pt-BR` not
`pt-Latn-BR`. The `language.Make(tag).String()` function from `golang.org/x/text`
produces this form.

### Validation and parsing

A new `core/locale` package provides:

```go
func Parse(s string) (model.LocaleID, error)   // validate + normalize
func MustParse(s string) model.LocaleID         // panics on invalid
func DisplayName(id model.LocaleID) string      // "French", "German"
func WellKnownLocales() []LocaleInfo            // curated list for UI
```

`Parse` delegates to `language.Parse()` which rejects garbage input and
normalizes casing. The returned tag is converted to its canonical string form.

`DisplayName` uses `display.English.Tags()` to produce English display names.
For unknown or custom tags it falls back to the raw code.

### Well-known locales

`WellKnownLocales()` returns a curated list of ~50 common BCP-47 tags sorted
by display name. This powers Bowrain's locale selector dropdowns. The list
covers the languages translators commonly work with — not a full CLDR dump.

```go
type LocaleInfo struct {
    Code        string `json:"code"`
    DisplayName string `json:"display_name"`
}
```

### Bowrain integration

The Bowrain backend exposes `GetKnownLocales()` and `GetLocaleDisplayName(code)`
methods. The frontend uses a `useLocales()` hook to fetch and cache the list,
and a `<LocaleSelect>` component for searchable single-select and multi-select
locale pickers. All locale displays show friendly names: "French (fr)" in
dropdowns, "English → French, German" on project cards.

### Backward compatibility

Existing projects using bare codes like "fr", "de", "ja" already hold valid
BCP-47 subtags. No migration is needed. The `Parse` function normalizes these
to the same canonical form they already use.

## Where this applies

| Subsystem | Usage |
|---|---|
| `core/model/locale.go` | `LocaleID` type definition |
| CLI flags (`--source-lang`, `--target-lang`) | User input, validated on entry |
| KAZ manifest (`manifest.yaml`) | Source and target locale fields (ADR-011) |
| TM entries (`lib/sievepen/`) | Source/target locale columns (ADR-010) |
| Terminology (`lib/termbase/`) | Term locale field (ADR-016) |
| Bowrain project creation | Locale selectors with friendly names (ADR-012) |
| Format readers/writers | Source/target language properties |

## Consequences

- Invalid locale codes are rejected early (at CLI parse time, project creation,
  API boundaries) rather than propagating silently
- Locale codes are always in canonical form; no case mismatches between "FR"
  and "fr"
- Bowrain users see searchable dropdowns with friendly names instead of
  memorizing ISO codes
- The well-known list is curated, not auto-generated — adding new locales is
  a deliberate code change
- `golang.org/x/text/language` handles the complexity of BCP-47 subtag
  parsing, script inference, and canonicalization
