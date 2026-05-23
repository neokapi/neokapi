# ARB JSON Schema — provenance

`arb.schema.json` is a **de-facto** JSON Schema, not an official one.

There is no official JSON Schema for the Application Resource Bundle format on
json.schemastore.org. The schema here was written faithfully from the ARB
specification and Flutter's `gen_l10n` behaviour, then validated against the
real-world corpus under `../corpus/` (Flutter Gallery `intl_en.arb` /
`intl_es.arb`) plus the package fixtures under `../`.

Spec references:

- Google ARB specification (Application Resource Bundle)
  <https://github.com/google/app-resource-bundle/wiki/ApplicationResourceBundleSpecification>
- Flutter internationalization / `gen_l10n` ARB usage
  <https://docs.flutter.dev/ui/accessibility-and-internationalization/internationalization>

The schema encodes the flat-object structure: each property is a message
(`key` → ICU MessageFormat string), an attribute object (`@<id>` →
`{ description, type, placeholders, … }`), or a global (`@@<name>`, e.g.
`@@locale`, `@@last_modified`). It enforces that message values are strings and
that attribute/placeholder objects carry the documented shapes, while leaving
the open-ended message-key set unconstrained. It rejects non-string message
values (verified with a negative case) and accepts every corpus and fixture
file.
