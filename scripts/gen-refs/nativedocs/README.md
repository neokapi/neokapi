# Native reference docs

Authored documentation for **built-in** (native) formats and tools, mirroring
the `doc.json` content the okapi-bridge ships for its filters and steps. The
`gen-refs` generator overlays these sidecars onto the built-in entries it reads
from the registries, so native cards on the website Format/Tool Reference reach
the same documentation richness as bridge cards.

```
nativedocs/
├── formats/<format-id>.yaml   # e.g. json.yaml, html.yaml, properties.yaml
└── tools/<tool-id>.yaml       # e.g. word-count.yaml, pseudo-translate.yaml
```

- The file name **must** be the registry id (the `id` in `formats.json` /
  `tools.json`), e.g. `json`, `word-count`.
- `gen-refs` merges the sidecar into the entry: `displayName` / `description`
  override the registry values; everything else becomes the entry's `doc`,
  which the website renders and whose `parameters` map feeds `SchemaForm`'s
  `paramDocs`.

## Schema

See [`_TEMPLATE.yaml`](_TEMPLATE.yaml) for the full annotated structure. Fields:

| field | purpose |
|---|---|
| `displayName` | optional override of the card title |
| `description` | one-line summary shown on the card and in search |
| `overview` | markdown; the lead explainer (what it is, when to use it) |
| `parameters.<schemaPropPath>` | per-parameter `help` (markdown), `values`, `notes`, `examples`, `dependsOn` |
| `limitations` | known limits, one bullet each |
| `processingNotes` | behavioural notes (segmentation, part types, ordering) |
| `examples` | worked `config` snippets (fenced YAML) with a `title` + `description` |
| `wikiUrl` | optional upstream/spec link |

`parameters` keys are the schema property paths. Use the **dotted path** for
nested object properties (e.g. `inlineCodes.enabled`), matching the keys in the
generated `schema.properties`.

## Authoring rules

1. **Ground every statement in the code and the format spec.** Read the
   format/tool source (`core/formats/<id>/`, `core/tools/`, `core/ai/tools/`)
   and the relevant spec; never invent behaviour, defaults, or parameters. If a
   native format mirrors an Okapi filter, cross-check the Okapi semantics.
2. **Cover every schema property.** Run `make generate-reference-docs` and check
   `packages/reference-data/data/reference-gaps.json` — your entry should
   produce no `property:*` or `doc.*` gaps.
3. Match the brand voice in `docs/internals/brand-communication.md`: restrained,
   academic, no marketing superlatives, no hardcoded counts.
4. Keep examples runnable and minimal — show one idea per example.

Regenerate after editing:

```bash
make generate-reference-docs        # → packages/reference-data/data/*.json
```
