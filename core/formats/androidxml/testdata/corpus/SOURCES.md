# Android string-resources real-world corpus — provenance

Every file in this directory is a GENUINE, unmodified Android `res/values/strings.xml`
fetched from a permissively-licensed public repository. The exact commit is
pinned so the fetch is reproducible. These files exercise the byte-faithful
round-trip of the native `androidxml` reader/writer against real Android
resource files (`<plurals>`, `<string-array>`, `translatable="false"`, CDATA,
`<xliff:g>` do-not-translate spans, `%1$s`/`%d` printf args, Android backslash
escapes, the `product=` qualifier, and many XML comments).

The files are used here verbatim, solely as test fixtures, with attribution
below.

| File | Upstream repo | License | Commit |
|---|---|---|---|
| `aosp-calendar-strings.xml` | [aosp-mirror/platform_packages_apps_Calendar](https://github.com/aosp-mirror/platform_packages_apps_Calendar) (AOSP) | Apache-2.0 | `aaa8ec6e4aeea00708d1637376f04c5b7391ec46` |
| `k9mail-message-list-strings.xml` | [thunderbird/thunderbird-android](https://github.com/thunderbird/thunderbird-android) (K-9 Mail) | Apache-2.0 | `d78aa2d5e613bb9a690073c34a5805144fcc0ef6` |

> The AOSP Calendar file carries an explicit `Licensed under the Apache License,
> Version 2.0` header in the file itself (the aosp-mirror repo ships a `NOTICE`
> file; all AOSP source is Apache-2.0). thunderbird-android is Apache-2.0 per its
> root `LICENSE`.

## Feature coverage

- `aosp-calendar-strings.xml` — the broad surface: `<plurals>` (CLDR quantity
  items), `<string-array>` items, `translatable="false"` (both `<string>` and
  `<string-array>`), `<xliff:g>` do-not-translate spans, `%s`/`%1$s` printf
  args, Android backslash escapes (`\'`, ` `), bare `>` in element content
  (`Do NOT check ->`), the `product="tablet"`/`product="default"` qualifier
  (two `<string>` entries sharing one `@name`), and hundreds of XML comments.
- `k9mail-message-list-strings.xml` — CDATA (`<![CDATA[Create & set new folder]]>`),
  `%1$s` printf args, Android backslash escapes (`Couldn\'t`, `\'%1$s\'`), and a
  preceding-comment developer note.

## Exact fetch commands

```bash
CAL_SHA="aaa8ec6e4aeea00708d1637376f04c5b7391ec46"
TB_SHA="d78aa2d5e613bb9a690073c34a5805144fcc0ef6"

curl -sSL "https://raw.githubusercontent.com/aosp-mirror/platform_packages_apps_Calendar/${CAL_SHA}/res/values/strings.xml" \
  -o aosp-calendar-strings.xml
curl -sSL "https://raw.githubusercontent.com/thunderbird/thunderbird-android/${TB_SHA}/feature/mail/message/list/internal/src/main/res/values/strings.xml" \
  -o k9mail-message-list-strings.xml
```

## Note on the `product=` qualifier (round-trip bug found & fixed)

`aosp-calendar-strings.xml` contains two `<string name="custom">` entries
distinguished only by `product="tablet"` vs `product="default"`. The native
reader/writer originally keyed string entries by `@name` alone, so the two
collapsed and the writer spliced the wrong text into the first. The reader now
suffixes a product-qualified entry's `Block.Name` as `name@product` and records
the `product` in `androidxml.product`; the writer keys by `{name, product}`.
This file is the regression fixture for that fix.
