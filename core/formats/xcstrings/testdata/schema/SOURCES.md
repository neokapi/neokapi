# xcstrings JSON Schema — provenance

`xcstrings.schema.json` is a **de-facto** JSON Schema, not an official one.

Apple has not published a JSON Schema (or even a formal written spec) for the
String Catalog format — it is intentionally an Xcode-managed format, and
json.schemastore.org carries no `xcstrings` entry (verified against the
SchemaStore catalog API). The schema here was therefore reconstructed faithfully
from the documented and community-reverse-engineered structure and validated
against the real-world corpus under `../corpus/` plus the package fixtures under
`../`.

The structure it encodes (top-level `sourceLanguage` / `version` / `strings`;
per-entry `comment` / `extractionState` / `shouldTranslate` / `localizations`;
per-localization `stringUnit` or `variations`; `variations` →
`plural` (CLDR categories) / `device` (device classes) / `substitutions`
(named, each with `argNum` / `formatSpecifier` and its own `variations`)) is
described in:

- Apple Developer Forums, "Xcode 15 beta strings catalog"
  <https://developer.apple.com/forums/thread/732120>
- Tolgee — Apple String Catalog (.xcstrings)
  <https://docs.tolgee.io/platform/formats/apple_xcstrings>
- Phrase — .XCSTRINGS Apple Strings Catalog
  <https://support.phrase.com/hc/en-us/articles/18807352792604>

The schema is deliberately strict where Apple's vocabulary is closed (the
`state` enum, CLDR plural categories, device classes) so that it catches
malformed output, but open where the format is open (the message-key set,
language identifiers, substitution names). It rejects bad `state` values,
non-CLDR plural categories, and unexpected structural keys (verified with
negative cases), and accepts every file in the corpus and the package fixtures.
