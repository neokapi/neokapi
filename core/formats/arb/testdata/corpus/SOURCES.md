# ARB corpus — provenance

Real-world Flutter Application Resource Bundle (`.arb`) files vendored verbatim
from permissively-licensed open-source projects. Each file is byte-identical to
its source; `corpus_test.go` asserts a byte-faithful read→write round-trip.

All files round-trip byte-for-byte through the native reader/writer (no
divergences, no skips).

## `flutter_gallery_intl_en.arb` (~134 KB) and `flutter_gallery_intl_es.arb` (~54 KB)

- Source repo: <https://github.com/flutter/gallery>
- License: BSD-3-Clause (`.license.spdx_id` == `BSD-3-Clause`)
- Branch: `main`
- Commit pinned at fetch time: `66a69803cc63dfc02878fae1959a2555f26ea25f`
- Paths: `lib/l10n/intl_en.arb`, `lib/l10n/intl_es.arb`

These are the canonical English source bundle and the Spanish translation bundle
for Google's Flutter Gallery demo app. They exercise the breadth of the format:
plain messages, `@<id>` description/placeholder attribute objects, `@@locale`
globals, ICU `{placeholder}` substitutions, ICU `plural`/`select` constructs,
nested ICU, and escaped characters — all within Flutter `gen_l10n`'s exact
two-space pretty-printed JSON layout.

Fetch commands (commit-pinned):

```sh
curl -sSL -o flutter_gallery_intl_en.arb \
  "https://raw.githubusercontent.com/flutter/gallery/66a69803cc63dfc02878fae1959a2555f26ea25f/lib/l10n/intl_en.arb"
curl -sSL -o flutter_gallery_intl_es.arb \
  "https://raw.githubusercontent.com/flutter/gallery/66a69803cc63dfc02878fae1959a2555f26ea25f/lib/l10n/intl_es.arb"
```
