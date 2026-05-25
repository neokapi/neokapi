# xcstrings corpus — provenance

Real-world Xcode String Catalog (`.xcstrings`) files vendored verbatim from
permissively-licensed open-source projects. Each file is byte-identical to its
source; `corpus_test.go` asserts a byte-faithful read→write round-trip.

All files round-trip byte-for-byte through the native reader/writer (no
divergences, no skips). The set is curated to be diverse: one full real app
catalog plus two purpose-built fixtures covering plural variations and
nested device+plural variations.

## `zeitgeist.xcstrings` (~39 KB)

- Source repo: <https://github.com/daneden/Zeitgeist>
- License: Apache-2.0
- Commit pinned at fetch time: `a5af83e9947c7534f487d8c5df9700e8207db495`
- Path: `Localizable.xcstrings`

A complete real shipping-app catalog (157 string entries, `sourceLanguage` "en",
many `stringUnit` localizations with `state` values). Xcode's exact JSON layout:
two-space indent and `" : "` colon spacing.

```sh
curl -sSL -o zeitgeist.xcstrings \
  "https://raw.githubusercontent.com/daneden/Zeitgeist/a5af83e9947c7534f487d8c5df9700e8207db495/Localizable.xcstrings"
```

## `xckit_plural_variations.xcstrings` (~0.9 KB)

- Source repo: <https://github.com/corrupt952/xckit>
- License: MIT
- Commit pinned at fetch time: `f1e4e0335e6a2dedd3137687f2cfa1e12db38ac3`
- Path: `fixtures/plural_variations.xcstrings`

A `xckit` test fixture exercising `variations` → `plural` (one/other) with
`%lld` placeholders, multiple locales, and `\uXXXX`-escaped non-ASCII (Japanese).
Uses the compact `": "` colon style (a different but valid JSON layout from
Xcode's) — confirms the byte-preserving rewriter is layout-agnostic.

```sh
curl -sSL -o xckit_plural_variations.xcstrings \
  "https://raw.githubusercontent.com/corrupt952/xckit/f1e4e0335e6a2dedd3137687f2cfa1e12db38ac3/fixtures/plural_variations.xcstrings"
```

## `xcstringseditor_variations.xcstrings` (~3.4 KB)

- Source repo: <https://github.com/xiles/XCStringsEditor>
- License: MIT
- Commit pinned at fetch time: `3b1e69ffb057b60ba1bb458eb58cb18a4527bde7`
- Path: `Sample Files/variations.xcstrings`

XCStringsEditor's sample file exercising the hardest nesting in the spec:
`variations` → `device` (applewatch/ipad/iphone/mac) → `variations` → `plural`
(zero/other and few/many/one/other for Russian), plus mixed `state` values
(`new`, `needs_review`) and empty-localization entries. Xcode `" : "` layout.

```sh
curl -sSL -o xcstringseditor_variations.xcstrings \
  "https://raw.githubusercontent.com/xiles/XCStringsEditor/3b1e69ffb057b60ba1bb458eb58cb18a4527bde7/Sample%20Files/variations.xcstrings"
```

## Note on a permissive *real-app* catalog with plurals

The two largest permissively-licensed real-app catalogs found in a broad GitHub
search — `gee1k/uPic` (Apache-2.0, ~241 KB, `version` 1.1, `comment` /
`shouldTranslate` / `extractionState` fields) and the Zeitgeist catalog above —
are both `stringUnit`-only (no plural/variation subtrees). Real shipping apps
that use the deeply-nested `variations` subtrees under a *permissive* license are
scarce, so the two MIT fixtures above (authored against the spec by xcstrings
tooling projects) supply that coverage. They are genuine published files, not
hand-built here.
