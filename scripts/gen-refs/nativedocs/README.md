# Native reference docs

Authored documentation for **built-in** (native) formats and tools, mirroring
the `doc.json` content the okapi-bridge ships for its filters and steps. The
`gen-refs` generator overlays these sidecars onto the built-in entries it reads
from the registries, so native cards on the website Format/Tool Reference reach
the same documentation richness as bridge cards.

```
nativedocs/
├── formats/<format-id>.yaml   # e.g. json.yaml, html.yaml, properties.yaml
├── tools/<tool-id>.yaml       # e.g. term-check.yaml, pseudo-translate.yaml
└── checks/<check-id>.yaml     # the source-side checks of `kapi check`
```

- The file name **must** be the registry id (the `id` in `formats.json` /
  `tools.json`), e.g. `json`, `voice-check`. `gen-refs` **fails** on a file name
  no built-in entry carries, naming the file and the id it failed to match: a
  sidecar that binds to nothing documents nothing, and read-and-dropped it leaves
  the entry it was written for shipping the registry's bare metadata. Renaming a
  format or a tool therefore means renaming its sidecar in the same change,
  and, when the rename went with a change of mechanism, rewriting the prose to
  describe the current one rather than attaching stale copy to a live entry.
- `gen-refs` merges the sidecar into the entry: `displayName` / `description`
  override the registry values; everything else becomes the entry's `doc`,
  which the website renders and whose `parameters` map feeds `SchemaForm`'s
  `paramDocs`.

### `checks/`: sidecars with no entry behind them

`content-lint`, `length-check` and `pattern-check` are the source-side checks
`kapi check` runs directly (`core/check/sourcechecks.go`). They are check
infrastructure rather than registry tools, so they carry no dataset entry and no
schema, and nothing overlays their dossiers onto a card. A user still meets
them by the rule ids their findings carry, and the behaviour is live, so they are
documented here and held to the same register as the rest.

The binding is `core/check.SourceCheckIDs`, and `gen-refs` holds `checks/` to it
in **both** directions: a dossier naming no check fails the build, and a check
with no dossier fails it too. Retiring a check therefore takes its dossier with
it, and adding one asks for its dossier, which an exemption list cannot
enforce.

These dossiers have no generated page today. The Format and Tool Reference is
built from the dataset, and a check is not in it; giving the check rules a
reference section of their own is a separate piece of work.

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
   `packages/reference-data/data/reference-gaps.json`. Your entry should
   produce no `property:*` or `doc.*` gaps.
3. Match the register in `docs/internals/brand-communication.md`: restrained,
   academic, no marketing superlatives, no hardcoded counts. These files are the
   `neokapi-docs-reference` collection in the repo-root `kapi.yaml`, so the
   project's voice profile grades them the way it grades the authored docs.
   Run `kapi check` (below) rather than reading the rendered page, which no one
   may edit.
4. Keep examples runnable and minimal, one idea per example.

Regenerate and check after editing:

```bash
make generate-reference-docs        # → packages/reference-data/data/*.json
make generate-reference-pages       # → web/docs/reference/{commands,formats,tools}
make check-reference-prose          # the register gate over these files
```

All three are gated by the *Reference Data Drift Gate* workflow, so an edit
here lands together with the dataset and the pages it produces, and it lands
clean: `check-reference-prose` fails on any critical, major or minor finding.
