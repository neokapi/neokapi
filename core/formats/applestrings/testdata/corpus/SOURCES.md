# applestrings corpus — provenance

Real-world legacy Apple `.strings` and `.stringsdict` files vendored verbatim
from permissively-licensed open-source projects. Each file is byte-identical to
its source; `corpus_test.go` asserts a byte-faithful read→write round-trip.

All files round-trip byte-for-byte through the native reader/writer (no
divergences, no skips). Each `.stringsdict` is additionally validated by
`plutil -lint` in the acceptance suite.

## `utm_it.strings` (~67 KB)

- Source repo: <https://github.com/utmapp/UTM>
- License: Apache-2.0
- Branch: `main`
- Commit pinned at fetch time: `c4eb8aad9e84e6c12f1e872c71f9518c0111756a`
- Path: `Platform/it.lproj/Localizable.strings`

A large real shipping-app `.strings` table (Italian localization of UTM). Covers
the line-based format end to end: block `/* … */` comments (including multi-line
comments listing several call sites), `%@` / `%lld` printf specifiers, positional
`%1$@` / `%2$lld` specifiers, emoji in keys and values, and UTF-8 throughout.

```sh
curl -sSL -o utm_it.strings \
  "https://raw.githubusercontent.com/utmapp/UTM/c4eb8aad9e84e6c12f1e872c71f9518c0111756a/Platform/it.lproj/Localizable.strings"
```

## `utm_it.stringsdict` (~0.6 KB)

- Source repo: <https://github.com/utmapp/UTM>
- License: Apache-2.0
- Commit pinned at fetch time: `c4eb8aad9e84e6c12f1e872c71f9518c0111756a`
- Path: `Platform/it.lproj/Localizable.stringsdict`

A minimal real `.stringsdict`: one `NSStringPluralRuleType` entry with a
`%#@cores@` format key and `one`/`other` plural categories, tab-indented in
Apple's canonical plist layout with the standard DOCTYPE.

```sh
curl -sSL -o utm_it.stringsdict \
  "https://raw.githubusercontent.com/utmapp/UTM/c4eb8aad9e84e6c12f1e872c71f9518c0111756a/Platform/it.lproj/Localizable.stringsdict"
```

## `playem_fr.stringsdict` (~3.2 KB)

- Source repo: <https://github.com/tillt/PlayEm>
- License: MIT
- Commit pinned at fetch time: `7e3eeb4a07f01a00aca5272deec39fce4014a343`
- Path: `PlayEm/fr.lproj/LocalizablePlural.stringsdict`

A richer real `.stringsdict` (French): several `NSStringPluralRuleType` entries
whose format keys mix positional `%1$ld` / `%2$#@var@` references with literal
text, and `one`/`other` categories carrying accented French (`entrée`,
`éléments`). Tab-indented Apple plist layout.

```sh
curl -sSL -o playem_fr.stringsdict \
  "https://raw.githubusercontent.com/tillt/PlayEm/7e3eeb4a07f01a00aca5272deec39fce4014a343/PlayEm/fr.lproj/LocalizablePlural.stringsdict"
```
