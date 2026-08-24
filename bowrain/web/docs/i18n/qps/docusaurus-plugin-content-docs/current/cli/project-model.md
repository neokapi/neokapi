---
sidebar_position: 3
title: Project Model
---

# ▒ Ƃöŵŕàîñ Þŕöĵéçţ Ḿöđéļ ▒

▒ À ƃöŵŕàîñ þŕöĵéçţ îš à `.ķàþî` þŕöĵéçţ ŵîţĥ à `ƃöŵŕàîñ:` ƃļöçķ öñ îţš ŕéçîþé. Ţĥéŕé îš öñé þŕöĵéçţ ḿöđéļ šĥàŕéđ ŵîţĥ ţĥé `ķàþî` ÇĻÎ: à šîñĝļé `ķàþî.ýàḿļ` ŕéçîþé ƒîļé àţ ţĥé þŕöĵéçţ ŕööţ àñđ à šîƃļîñĝ `.ķàþî/` đîŕéçţöŕý. ▒

## ▒ Đîŕéçţöŕý Šţŕüçţüŕé ▒

```
my-app/
├── kapi.yaml                   # the recipe (committed) — fixed, conventional filename
├── .kapi/                      # committed — the project's context
│   ├── manifest.yaml           # bookkeeping: block counts, fingerprints
│   ├── filters.json            # shared reader/writer configuration
│   ├── filters.local.json      # personal overrides (gitignored)
│   ├── flows/                  # optional file-per-flow definitions
│   │   └── pseudo.yaml
│   ├── terms.json              # terms (bound by defaults.terms_source)
│   ├── voice.yaml              # the voice profile
│   ├── memory/                 # content-memory bundles
│   │   └── memory.json         # the primary (bound by defaults.memory_source)
│   ├── profiles/               # per-profile governance overrides
│   │   └── bowrain/
│   │       └── voice.yaml
│   ├── state/                  # the unit-state record, one shard per document
│   │   └── src-locales-en-messages.jsonl
│   └── work/                   # gitignored — everything derived
│       ├── store.db            # the local index over everything committed
│       ├── vault/              # withheld redaction originals (local-only)
│       └── cache/              # free to delete, always
│           ├── sync-cache.json  # kapi push/pull state
│           ├── extractions/
│           └── collections/
└── src/
    └── locales/
        ├── en/
        │   └── messages.json
        └── fr/
            └── messages.json
```

▒ Öŵñéŕšĥîþ žöñéš àţ ţĥé þŕöĵéçţ ŕööţ: ▒

- ▒ **`ķàþî.ýàḿļ`** — ĥàñđ-éđîţéđ, çöḿḿîţţéđ ţö ĝîţ. Ţĥé ŕéçîþé îš ţĥé šîñĝļé šöüŕçé öƒ ţŕüţĥ ƒöŕ þŕöĵéçţ çöñƒîĝüŕàţîöñ. Îţš ƒîẋéđ, çöñṽéñţîöñàļ ƒîļéñàḿé ḿéàñš éṽéŕý éđîţöŕ àñđ çöđé ĥöšţ (ĜîţĤüƃ, ĜîţĻàƃ) àþþļîéš ÝÀḾĻ šýñţàẋ ĥîĝĥļîĝĥţîñĝ ţö đîƒƒš àñđ þŕéṽîéŵš ŵîţĥ ñö çöñƒîĝüŕàţîöñ. ▒
- ▒ **`.ķàþî/`** — ţĥé çöḿḿîţţéđ çöñţéẋţ ĝŕàþĥ, ƒļàţ: `ţéŕḿš.ĵšöñ`, `ḿéḿöŕý/` àñđ `ṽöîçé.ýàḿļ`, ŵîţĥ þéŕ-þŕöƒîļé öṽéŕŕîđéš üñđéŕ `þŕöƒîļéš/<ñàḿé>/`, ŕéṽîéŵéđ ţĥŕöüĝĥ `ĝîţ đîƒƒ` ļîķé àñý öţĥéŕ šöüŕçé ƒîļé. `.ķàþî/` îš çöḿḿîţţéđ îñ ƒüļļ; öñļý `.ķàþî/ŵöŕķ/` îš ĝîţîĝñöŕéđ. ▒
- ▒ **`.ķàþî/šţàţé/*.ĵšöñļ`** — ţĥé üñîţ-šţàţé ŕéçöŕđ, çöḿḿîţţéđ. `ķàþî çöḿḿîţ` þüƃļîšĥéš šţàĝéđ üñîţ šţàţé îñţö îţ. ▒
- ▒ **`.ķàþî/ŵöŕķ/šţöŕé.đƃ`** — ķàþî-öŵñéđ, ĝîţîĝñöŕéđ. Öñé ŠǪĻîţé ƒîļé ĥöļđîñĝ éṽéŕý šüƃšýšţéḿ'š ţàƃļéš — ƃļöçķ çàçĥé, ţéŕḿš šţöŕé, çöñţéñţ ḿéḿöŕý, ţĥé ŵöŕķîñĝ šéţ öƒ üñîţ šţàţé šţàĝéđ šîñçé ţĥé ļàšţ `ķàþî çöḿḿîţ`, àñđ ţĥé þŕöĵéçţ'š çöñţéẋţ ĝŕàþĥ. Îţ îš àñ îñđéẋ öṽéŕ ţĥé çöḿḿîţţéđ šöüŕçéš àƃöṽé àñđ ŕéƃüîļđš ƒŕöḿ ţĥéḿ. ▒
- ▒ **`.ķàþî/ŵöŕķ/çàçĥé/`** — ÇĻÎ-öŵñéđ, ĝîţîĝñöŕéđ. Éṽéŕýţĥîñĝ çĥéàþļý ŕéĝéñéŕàƃļé: ţĥé ķàþî šýñç çàçĥé, éẋţŕàçţîöñ îñţéŕḿéđîàţéš, öṽéŕļàý ļàýéŕš. Šàƒé ţö đéļéţé àţ àñý ţîḿé. ▒
- ▒ **`.ķàþî/ƒļöŵš/*.ýàḿļ`** — öþţîöñàļ ƒîļé-þéŕ-ƒļöŵ đéƒîñîţîöñš, ĥàñđ-éđîţéđ, çöḿḿîţţéđ. Ƃöŵŕàîñ ŕéàđš ţĥéšé îñ àđđîţîöñ ţö îñļîñé `ƒļöŵš:` đéçļàŕéđ öñ ţĥé ŕéçîþé. ▒

▒ Ļöçàļ àñđ šéŕṽéŕ çöñṽéŕĝé îñ šĥàþé. Ƃöŵŕàîñ àñšŵéŕš ĝŕàþĥ ǫüéšţîöñš öṽéŕ öñé Þöšţĝŕéš šþàññîñĝ ŵöŕķšþàçéš, þŕöĵéçţš àñđ šţŕéàḿš; à þŕöĵéçţ àñšŵéŕš ţĥé šàḿé ǫüéŕý šĥàþéš öṽéŕ `šţöŕé.đƃ` ŵîţĥ ţĥöšé đîḿéñšîöñš ƒîẋéđ ţö öñé ṽàļüé — šö ŵĥîçĥ ƃļöçķš üšé à ĝîṽéñ ţéŕḿ, ƃý çöļļéçţîöñ àñđ çööŕđîñàţé, îš àñšŵéŕàƃļé ŵîţĥ ñö šéŕṽéŕ. ▒

▒ Ţĥé þàîŕîñĝ ķééþš ţĥé ĝîţ-ļîķé šĥàþé öƒ à çöḿḿîţţéđ çöñƒîĝ ƒîļé ƃéšîđé à ţööļ-öŵñéđ đîŕéçţöŕý: `ķàþî.ýàḿļ` àļöñĝšîđé `.ķàþî/` àţ ţĥé šàḿé ŕööţ. Îţ îš ţĥé šàḿé ḿöđéļ ĝîţ îţšéļƒ üšéš — `.ĝîţ` îš ţĥé ţööļ'š đîŕéçţöŕý, àñđ îţš îñđéẋ ļîṽéš îñšîđé îţ. ▒

## ▒ Ŕéçîþé šçĥéḿà ▒

▒ Ţĥé ŕéçîþé îš à ÝÀḾĻ đöçüḿéñţ. Ƃöŵŕàîñ þŕöĵéçţš ļàýéŕ à `ƃöŵŕàîñ:` ƃļöçķ (àñđ öþţîöñàļ ţöþ-ļéṽéļ `ĥööķš`, `àüţöḿàţîöñš`, `àššéţš`, `ƃŕàñđ_ṽöîçé`) öñţö ţĥé ƒŕàḿéŵöŕķ'š `ĶàþîÞŕöĵéçţ` šçĥéḿà. ▒

```yaml
version: v1
name: My App

defaults:
  source_language: en
  target_languages: [fr, de, ja]
  collection: ui/strings
  exclude:
    - "**/*.test.json"
    - "node_modules/**"

collections:
  - path: src/locales/**/*.json
    format: json
  - path: content/docs/**/*.md
    format: markdown
    target: i18n/{lang}/docs/{path}/{filename}
  - path: src/es/**/*.json
    format: json
    source_language: es      # per-entry source language override
    collection: spanish-ui   # per-entry collection routing override

plugins:
  okapi-bridge: "^1.47.0"    # map form: name → version constraint

flows:
  pseudo:
    steps:
      - tool: pseudo-translate
        config: { method: extended }

# The bowrain: block depends on the bowrain plugin. init declares the
# requirement so a plain kapi binary (without the plugin) fails fast instead of
# silently ignoring the connection.
requires:
  bowrain: "*"

# Optional bowrain-server connection — presence enables push/pull and makes the
# server the default venue for `kapi up`.
bowrain:
  url: https://app.bowrain.cloud/my-team/abc123
  stream: $auto              # auto-detect from git branch / CI
  converge: on-push          # on-push (default) | manual

# Top-level lifecycle policy:
hooks:
  pre-push: [qa]
  post-pull: [update-stats]

automations:
  - name: notify-on-parked
    trigger: run-parked
    actions:
      - type: slack
        config: { channel: "#translation" }

# Top-level governance / asset policy:
assets:
  enabled: true
  max_size: 100MB

brand_voice:
  profile: company-profile
  channel: marketing
```

### ▒ Ţöþ-ļéṽéļ ƒîéļđš ▒

| Field          | Type           | Description                                                            |
| -------------- | -------------- | ---------------------------------------------------------------------- |
| `version`      | string         | Schema version (currently `v1`)                                        |
| `name`         | string         | Project display name                                                   |
| `defaults`     | object         | Project-wide language and execution defaults                           |
| `collections`  | list           | Content collections (see [Content Collections](#content-collections))  |
| `profiles`     | map            | Governance bound per product, keyed by profile name (see [Profiles and channels](#profiles-and-channels)) |
| `plugins`      | map            | Plugin dependencies as `name: version-constraint` (e.g. map form)      |
| `requires`     | map            | Plugin name → version constraint that gates loading; a `bowrain:` block adds `bowrain` so a plain kapi binary refuses the recipe |
| `flows`        | map            | Inline flow definitions (file-per-flow under `.kapi/flows/` also work) |
| `bowrain`      | object         | Optional bowrain-server connection coordinates                         |
| `hooks`        | map            | Flows that run at lifecycle points (`pre-push`, `post-pull`, ...)      |
| `automations`  | list           | Local automation rules (see [Automations](#automations))               |
| `assets`       | object         | Asset (image/binary) policy                                            |
| `voice`        | object         | Voice profile and channel                                              |

### ▒ `đéƒàüļţš` ƃļöçķ ▒

| Field              | Type   | Description                                              |
| ------------------ | ------ | -------------------------------------------------------- |
| `source_language`  | string | BCP-47 source language (e.g. `en`)                       |
| `target_languages` | list   | BCP-47 target languages                                  |
| `collection`       | string | Default collection name for organizing content           |
| `exclude`          | list   | Glob patterns to skip during scanning                    |
| `formats`          | map    | Per-format default presets and config overrides          |
| `terms_source`     | string | Path to the committed terms source (e.g. `.kapi/terms.json`) |
| `memory_source`    | string | Path to the committed content memory source (e.g. `.kapi/memory/memory.json`) |

### ▒ `ƃöŵŕàîñ` ƃļöçķ ▒

▒ Öñļý ţĥé çöññéçţîöñ çööŕđîñàţéš šîţ üñđéŕ `ƃöŵŕàîñ:`: ▒

| Field      | Description                                                                  |
| ---------- | ---------------------------------------------------------------------------- |
| `url`      | Compound URL: `<server>/<workspace>/<project-id>` or `<server>/projects/<id>` |
| `stream`   | Server-side stream to sync against; `$auto` auto-detects from CI / git branch |
| `converge` | Server-side convergence policy: `on-push` (default) or `manual`              |

▒ Ļîƒéçýçļé (`ĥööķš`, `àüţöḿàţîöñš`) àñđ çöñţéñţ/ĝöṽéŕñàñçé (`àššéţš`, `ƃŕàñđ_ṽöîçé`) ļîṽé àţ ţĥé **ţöþ ļéṽéļ** öƒ ţĥé ŕéçîþé, ñöţ üñđéŕ `ƃöŵŕàîñ:` — ţĥéý đéšçŕîƃé þŕöĵéçţ-öŵñéđ þöļîçý, ñöţ šéŕṽéŕ îđéñţîţý. ▒

▒ Ţĥé ƒŕàḿéŵöŕķ ĥàš ñö ƃüîļţ-îñ ñöţîöñ öƒ à šéŕṽéŕ: `ƃöŵŕàîñ:` (àñđ `ĥööķš:`, `àüţöḿàţîöñš:`, `àššéţš:`, `ƃŕàñđ_ṽöîçé:`) àŕé ƃöŵŕàîñ **ŕéçîþé éẋţéñšîöñš** đéçöđéđ öñļý ŵĥéñ ţĥé `ķàþî-ƃöŵŕàîñ` þļüĝîñ îš îñšţàļļéđ (ţĥé ƒŕàḿéŵöŕķ ŕöüñđ-ţŕîþš ţĥéḿ ṽéŕƃàţîḿ öţĥéŕŵîšé). Ţĥé ķéý îš ţĥé þļàţƒöŕḿ'š öŵñ ñàḿé ƃéçàüšé ţĥé ƃļöçķ *îš* ţĥé þļàţƒöŕḿ: ķàþî ƒîñđš îţ ţĥŕöüĝĥ à ṽéñüé ƒļàĝ öñ ţĥé þļüĝîñ'š šçĥéḿà ŕéĝîšţŕàţîöñ àñđ ŕéàđš öñļý `üŕļ:` àñđ `çöñṽéŕĝé:`, ñéṽéŕ šþéļļîñĝ ţĥé ķéý öüţ. Šö `ķàþî îñîţ` / `ķàþî îñîţ-çöññéçţ` (àñđ `ķàþî çöñƒîĝ šéŕṽéŕ.üŕļ …`) đéçļàŕé `ŕéǫüîŕéš: { bowrain: "*" }` ŵĥéñéṽéŕ ţĥéý ŵŕîţé à `ƃöŵŕàîñ:` ƃļöçķ. À þļàîñ `ķàþî` ƃîñàŕý ŵîţĥöüţ ţĥé þļüĝîñ ţĥéñ ŕéƒüšéš ţĥé ŕéçîþé ŵîţĥ àñ àçţîöñàƃļé "ŕéǫüîŕéš ţĥé ƃöŵŕàîñ þļüĝîñ" éŕŕöŕ ŕàţĥéŕ ţĥàñ šîļéñţļý îĝñöŕîñĝ ţĥé çöññéçţîöñ. Šéé [Ç-01: Ţĥé þŕöĵéçţ ḿöđéļ — ŕéçîþé éẋţéñšîöñ ḿéçĥàñîšḿ](https://neokapi.github.io/contribute/architecture/context/c-01-project-model). ▒

## ▒ Çöñţéñţ Çöļļéçţîöñš ▒

▒ Éàçĥ éñţŕý üñđéŕ `çöļļéçţîöñš:` îš à çöñţéñţ çöļļéçţîöñ. Ƃàŕé éñţŕîéš àŕé šîñĝļé-þàţţéŕñ çöļļéçţîöñš; ñàḿéđ çöļļéçţîöñš ĝŕöüþ ḿüļţîþļé îţéḿš ţöĝéţĥéŕ. ▒

▒ Ýöü çàñ éđîţ `çöļļéçţîöñš:` ƃý ĥàñđ, öŕ ŵîţĥ ţĥé çöŕé `ķàþî` çöḿḿàñđš (ñö ƃöŵŕàîñ þļüĝîñ ŕéǫüîŕéđ — ţĥéý öñļý ţöüçĥ ţĥé ļöçàļ ŕéçîþé): ▒

```bash
kapi add "src/**/*.json"                 # append a content pattern (format auto-detected)
kapi rm  "src/legacy/*.json"             # remove the mapping, or add to the exclude list
kapi ls                                  # list the files the content tracks
kapi add "src/**/*.md" --format markdown # …pass --format only to override detection
kapi ls --stats                          # …with per-file block and word counts
```

▒ `àđđ`/`ŕḿ`/`ļš` àŕé ƒŕàḿéŵöŕķ çöḿḿàñđš; šýñç šţàţé (çĥàñĝéđ-ṽš-šéŕṽéŕ) îš [`ķàþî šţàţüš`](/cli/commands/status). ▒

```yaml
collections:
  # Bare entry — single source pattern
  - path: src/locales/**/*.json
    format: json

  # With output path template
  - path: content/docs/**/*.md
    format: markdown
    target: i18n/{lang}/docs/{path}/{filename}

  # Per-entry overrides
  - path: legacy/**/*.properties
    format: java-properties
    source_language: en-GB
    collection: legacy

  # Named collection — its items live under content:, relative to base:
  - name: ui
    channel: app
    base: src
    content:
      - path: "**/*.tsx"
        format:
          name: exec
          config:
            command: "vp neokapi-i18n extract --stream"
      - path: "i18n/en/*.json"
        format: json
```

### ▒ Çöļļéçţîöñ ƒîéļđš ▒

| Field              | Type            | Description                                                                |
| ------------------ | --------------- | -------------------------------------------------------------------------- |
| `name`             | string          | Collection name; required to bind a `channel:`                             |
| `base`             | string          | The directory this collection lives in — every `path`, `target` and item `base` below is written relative to it |
| `channel`          | string          | The point in the context space this content sits at: `profile/channel`, or a bare `channel` (see [Profiles and channels](#profiles-and-channels)) |
| `content`          | list            | The collection's content items                                             |
| `collection`       | string          | Collection routing override                                                |
| `source_language`  | string          | Source language override                                                   |
| `target_languages` | list            | Target language override                                                   |

### ▒ Çöñţéñţ îţéḿ ƒîéļđš ▒

| Field              | Type            | Description                                                                |
| ------------------ | --------------- | -------------------------------------------------------------------------- |
| `path`             | string          | Glob pattern for source files (supports `{lang}` placeholder)              |
| `format`           | string / object | File format ID (e.g. `json`, `html`) or object with `name`/`config`/`preset` |
| `target`           | string          | Output path pattern for target files (supports `{lang}` and `{path}`)      |
| `base`             | string          | Directory a matched file's path is made relative to for target-token expansion; defaults to the glob's fixed prefix |
| `collection`       | string          | Collection routing override for this entry                                 |
| `source_language`  | string          | Source language override for this entry                                    |
| `target_languages` | list            | Target language override for this entry                                    |
| `assets`           | object          | Per-entry asset policy override                                            |
| `asset_max_size`   | string          | Per-entry asset max size override                                          |

▒ À ƃàŕé éñţŕý çàŕŕîéš `þàţĥ`, `ƒöŕḿàţ` àñđ `ţàŕĝéţ` đîŕéçţļý öñ ţĥé çöļļéçţîöñ àñđ ĥàš ñö `çöñţéñţ:` ļîšţ. ▒

## ▒ Þŕöƒîļéš àñđ çĥàññéļš ▒

▒ Çöñţéñţ îš ŵŕîţţéñ ƒöŕ à þöîñţ îñ à ţŵö-àẋîš šþàçé: ţĥé **þŕöđüçţ** îţ ƃéļöñĝš ţö àñđ ţĥé **çĥàññéļ** îţ šĥîþš öñ. Ţĥé šþàçé îš šţŕüçţüŕàļ — à ķéý üñđéŕ `þŕöƒîļéš:` îš à þŕöđüçţ, ţĥé çĥàññéļš ţĥàţ þŕöƒîļé đéçļàŕéš àŕé ţĥé çĥàññéļš ţĥàţ þŕöđüçţ šĥîþš öñ, àñđ à ñàḿéđ çöļļéçţîöñ ñàḿéš îţš þöîñţ ŵîţĥ öñé `çĥàññéļ:` ŕéƒéŕéñçé. ▒

```yaml
profiles:
  acme:
    channels: [app, docs]
    voice: .kapi/voice.yaml
  acme-labs:
    channels: [app]
    voice: .kapi/profiles/acme-labs/voice.yaml
    terms: .kapi/profiles/acme-labs/terms.json

collections:
  - name: acme-docs
    channel: docs        # only acme declares it — the bare form resolves
    content:
      - path: docs/**/*.md
  - name: labs-app
    channel: acme-labs/app   # both declare `app` — qualify it
    content:
      - path: labs/src/i18n/**/*.kbf.json
```

▒ Ţĥé þŕöƒîļé ñàḿé îš àļšö ţĥé đîŕéçţöŕý üñđéŕ `.ķàþî/þŕöƒîļéš/<ñàḿé>/` ĥöļđîñĝ ŵĥàţ ţĥàţ þŕöƒîļé öṽéŕŕîđéš. Þŕöƒîļé ñàḿéš àñđ çĥàññéļš àŕé šļüĝš — ļöŵéŕçàšé ļéţţéŕš, đîĝîţš àñđ ĥýþĥéñš — šţàƃļé îđéñţîƒîéŕš ţĥàţ çŕöšš ţĥé šýñç ŵîŕé àš ţĥé çöñţéñţ'š þŕöđüçţ àñđ çĥàññéļ çööŕđîñàţéš, ñéṽéŕ ṽöçàƃüļàŕý. À ƃàŕé `çĥàññéļ:` ţŵö þŕöƒîļéš đéçļàŕé îš à ļöàđ éŕŕöŕ ñàḿîñĝ ƃöţĥ ǫüàļîƒîéđ šþéļļîñĝš; à çöļļéçţîöñ ƃîñđîñĝ ñö çĥàññéļ îš ĝöṽéŕñéđ ƃý `đéƒàüļţš.ṽöîçé` àñđ ţĥé þŕöĵéçţ'š öŵñ ţéŕḿš. ▒

▒ À þŕöƒîļé'š `ţéŕḿš:` îš ţĥé öñé ƃîñđîñĝ ţĥàţ đöéš ñöţ çŕöšš ţö ţĥé šéŕṽéŕ, ŵĥîçĥ ĝöṽéŕñš ţéŕḿîñöļöĝý ƒŕöḿ ţĥé ŵöŕķšþàçé ṽöçàƃüļàŕý îñšţéàđ. À çöññéçţéđ þŕöĵéçţ ţĥàţ ƃîñđš ţéŕḿš þéŕ þŕöƒîļé ŵàŕñš öñ éṽéŕý ŕüñ ţĥàţ ţĥé ƃîñđîñĝ àþþļîéš ţö ļöçàļ ŕüñš öñļý. ▒

### ▒ Ƒöŕḿàţ öƃĵéçţ ƒöŕḿ ▒

▒ Ŵĥéñ ýöü ñééđ ţö çöñƒîĝüŕé à ƒöŕḿàţ (àþþļý à þŕéšéţ, þàšš öþţîöñš, ŕüñ à šüƃþŕöçéšš éẋţŕàçţöŕ) üšé ţĥé öƃĵéçţ ƒöŕḿ: ▒

```yaml
collections:
  - path: "src/**/*.tsx"
    format:
      name: exec
      config:
        command: "vp neokapi-i18n extract --stream"

  - path: "docs/**/*.html"
    format:
      name: html
      preset: strict-extraction
```

## ▒ Àüţöḿàţîöñš ▒

▒ Àüţöḿàţîöñš àŕé ŕüļéš ţĥàţ ŕüñ àüţöḿàţîçàļļý àţ ļîƒéçýçļé þöîñţš, đéçļàŕéđ àţ ţĥé ţöþ ļéṽéļ öƒ ţĥé ŕéçîþé: ▒

```yaml
automations:
  - name: qa-before-push
    trigger: pre-push
    actions:
      - type: run_flow
        config:
          flow: qa
      - type: wait_translate

  - name: auto-pull-after-push
    trigger: post-push
    actions:
      - type: pull
```

### ▒ Àüţöḿàţîöñ ƒîéļđš ▒

| Field     | Description                                                                                |
| --------- | ------------------------------------------------------------------------------------------ |
| `name`    | Rule name                                                                                  |
| `trigger` | Lifecycle point: `pre-push`, `post-push`, `pre-pull`, `post-pull`, `pre-flow`, `post-flow` |
| `actions` | List of actions (`run_flow`, `wait_translate`, `pull`, `push`)                             |
| `enabled` | Optional boolean (defaults to `true`)                                                      |

▒ Ƒöŕ ļîĝĥţŵéîĝĥţ þŕé/þöšţ ĥööķš ţĥàţ đö ñöţĥîñĝ ƃüţ çàļļ éẋîšţîñĝ ƒļöŵš, þŕéƒéŕ ţĥé ţöþ-ļéṽéļ `ĥööķš:` ḿàþ. ▒

## ▒ Þŕöĵéçţ Đîšçöṽéŕý ▒

▒ ķàþî šéàŕçĥéš ƒöŕ à `ķàþî.ýàḿļ` ŕéçîþé ƃý ŵàļķîñĝ üþ ţĥé đîŕéçţöŕý ţŕéé (ļîķé ĝîţ): ▒

```bash
cd my-app/src/locales/fr/
kapi status  # finds kapi.yaml at ../../../kapi.yaml
```

▒ Àļļ çöḿḿàñđš ŵöŕķ ƒŕöḿ àñý šüƃđîŕéçţöŕý ŵîţĥîñ ţĥé þŕöĵéçţ. À đîŕéçţöŕý ĥöļđš àţ ḿöšţ öñé `ķàþî.ýàḿļ`, šö đîšçöṽéŕý îš üñàḿƃîĝüöüš; àñ éẋþļîçîţ `-þ <þàţĥ>` šţîļļ öṽéŕŕîđéš îţ. ▒

## ▒ Ṽéŕšîöñ Çöñţŕöļ ▒

### ▒ Çöḿḿîţ ţö ĝîţ ▒

- ▒ `ķàþî.ýàḿļ` — ţĥé ŕéçîþé (šîñĝļé šöüŕçé öƒ ţŕüţĥ ƒöŕ çöñƒîĝüŕàţîöñ) ▒
- ▒ `.ķàþî/ţéŕḿš.ĵšöñ`, `.ķàþî/ḿéḿöŕý/ḿéḿöŕý.ĵšöñ`, `.ķàþî/ṽöîçé.ýàḿļ` — ţĥé çöñţéẋţ šöüŕçéš ţĥé ŕéçîþé ƃîñđš ▒
- ▒ `.ķàþî/šţàţé/*.ĵšöñļ` — ţĥé üñîţ-šţàţé ŕéçöŕđ ▒
- ▒ `.ķàþî/ƒļöŵš/*.ýàḿļ` — ƒîļé-þéŕ-ƒļöŵ đéƒîñîţîöñš, îƒ ýöü üšé ţĥéḿ ▒
- ▒ `.ķàþî/ḿàñîƒéšţ.ýàḿļ`, `.ķàþî/ƒîļţéŕš.ĵšöñ` — ƃööķķééþîñĝ àñđ šĥàŕéđ ŕéàđéŕ çöñƒîĝüŕàţîöñ ▒

### ▒ Đö ÑÖŢ çöḿḿîţ ▒

▒ `ķàþî îñîţ` ŵŕîţéš à ţŵö-ļîñé îĝñöŕé ŕüļé, ŵîţĥ ñö ñéĝàţîöñ: ▒

```gitignore
/.kapi/work/
/.kapi/filters.local.json
```

- ▒ `.ķàþî/ŵöŕķ/` — éṽéŕýţĥîñĝ đéŕîṽéđ: `šţöŕé.đƃ`, ţĥé çàçĥéš, àñđ ţĥé ŕéđàçţîöñ ṽàüļţ ▒
- ▒ `.ķàþî/ƒîļţéŕš.ļöçàļ.ĵšöñ` — ýöüŕ þéŕšöñàļ ŕéàđéŕ öṽéŕŕîđéš ▒

▒ Đéļéţîñĝ `.ķàþî/ŵöŕķ/çàçĥé/` çöšţš ñöţĥîñĝ. Đéļéţîñĝ `.ķàþî/ŵöŕķ/` çöšţš ţŵö ţĥîñĝš: ţĥé ŕéṽîéŵ üñîţ šţàţé šţàĝéđ šîñçé ţĥé ļàšţ `ķàþî çöḿḿîţ`, ŵĥîçĥ ļîṽé öñļý îñ `šţöŕé.đƃ` — ŕüñ `ķàþî çöḿḿîţ` ƃéƒöŕé ýöü ŕéḿöṽé îţ — àñđ, îƒ ţĥé þŕöĵéçţ üšéš ŕéđàçţîöñ, ţĥé ŵîţĥĥéļđ öŕîĝîñàļš îñ `.ķàþî/ŵöŕķ/ṽàüļţ/`, ŵĥîçĥ àŕé ļöçàļ-öñļý ƃý đéšîĝñ àñđ ŕéƃüîļđ ƒŕöḿ ñöţĥîñĝ. ▒

## ▒ Îñîţîàļîžàţîöñ ▒

▒ Çŕéàţé à ñéŵ ƃöŵŕàîñ þŕöĵéçţ: ▒

```bash
cd my-app/
kapi init
```

▒ Îñ îñţéŕàçţîṽé ḿöđé (đéƒàüļţ ŵĥéñ šţđîñ îš à ţéŕḿîñàļ), `ķàþî îñîţ` þŕéšéñţš à ĝüîđéđ šéţüþ ŵîžàŕđ ŵĥéŕé ýöü çàñ šîĝñ îñ, çĥööšé à ŵöŕķšþàçé, àñđ çöñƒîĝüŕé ýöüŕ þŕöĵéçţ. ▒

▒ Ƒöŕ ñöñ-îñţéŕàçţîṽé üšàĝé (é.ĝ. ÇÎ/ÇĐ), üšé ƒļàĝš: ▒

```bash
# Local-only project (no bowrain: block written)
kapi init --source en --targets fr,de,ja

# Connect to a server (anonymous claim)
kapi init --server https://app.bowrain.cloud --anonymous

# Apply a framework preset
kapi init --preset nextjs

# Connect to an existing project
kapi init --server https://app.bowrain.cloud --project abc123
```

### ▒ Îñîţ ƒļàĝš ▒

| Flag          | Description                                                       |
| ------------- | ----------------------------------------------------------------- |
| `--server`    | Server URL                                                        |
| `--project`   | Connect to an existing project by ID                              |
| `--name`      | Project name (default: current directory name)                    |
| `--source`    | Source locale (default: `en`)                                     |
| `--targets`   | Target locales, comma-separated (e.g. `nb,fr`)                    |
| `--anonymous` | Create a project without signing in                               |
| `--email`     | Create a project and email a link to claim it                     |
| `--preset`    | Apply a framework preset (e.g. `nextjs`, `react-intl`, `angular`) |

▒ `ķàþî îñîţ` ŵŕîţéš: ▒

1. ▒ `ķàþî.ýàḿļ` ŕéçîþé àţ ţĥé þŕöĵéçţ ŕööţ (ŵîţĥ à `ƃöŵŕàîñ:` ƃļöçķ ŵĥéñ à šéŕṽéŕ ŵàš šüþþļîéđ) ▒
2. ▒ `.ķàþî/` đîŕéçţöŕý ▒
3. ▒ `.ķàþî/ƒļöŵš/þšéüđö.ýàḿļ` — àñ éẋàḿþļé ƒļöŵ ▒
4. ▒ à ţŵö-ļîñé îĝñöŕé ŕüļé — `/.ķàþî/ŵöŕķ/` àñđ `/.ķàþî/ƒîļţéŕš.ļöçàļ.ĵšöñ` ▒

## ▒ Šéŕṽéŕ Çöññéçţîöñ ▒

▒ Ţĥé ƃļöçķ'š `üŕļ` ƒîéļđ îš à çöḿþöüñđ ÜŔĻ ţĥàţ éñçöđéš ţĥé šéŕṽéŕ àđđŕéšš, ŵöŕķšþàçé, àñđ þŕöĵéçţ ÎĐ: ▒

```yaml
bowrain:
  # Workspace project
  url: https://app.bowrain.cloud/my-team/abc123

  # Direct project (no workspace)
  # url: https://app.bowrain.cloud/projects/abc123

  stream: $auto
```

▒ Öñçé çöññéçţéđ, ýöü çàñ šýñç ŵîţĥ ţĥé šéŕṽéŕ: ▒

```bash
kapi push    # Upload local source blocks to server
kapi pull    # Fetch translated blocks from server
kapi status  # Show sync state (pending push/pull)
```

▒ Ţĥé àçţîṽé šéŕṽéŕ ÜŔĻ îš ŕéšöļṽéđ ƒŕöḿ (ƒîŕšţ ḿàţçĥ ŵîñš): ▒

1. ▒ `üŕļ:` üñđéŕ ţĥé ŕéçîþé'š `ƃöŵŕàîñ:` ƃļöçķ ▒
2. ▒ `--šéŕṽéŕ` ƒļàĝ ▒
3. ▒ `ƂÖŴŔÀÎÑ_ŠÉŔṼÉŔ_ÜŔĻ` éñṽîŕöñḿéñţ ṽàŕîàƃļé / `šéŕṽéŕ.üŕļ` îñ ţĥé þéŕ-ḿàçĥîñé [ƃöŵŕàîñ çöñƒîĝ](/cli/commands/config) ▒
4. ▒ Éẋîšţîñĝ àüţĥ šţàţé (ƒŕöḿ `ķàþî àüţĥ ļöĝîñ`) ▒
5. ▒ Ţĥé ĥöšţéđ šéŕṽîçé (`ĥţţþš://àþþ.ƃöŵŕàîñ.çļöüđ`) — çöḿḿàñđš ţĥàţ çöñţàçţ à
   šéŕṽéŕ ƒàļļ ƃàçķ ţö îţ; šéļƒ-ĥöšţéđ đéþļöýḿéñţš çöñƒîĝüŕé öñé öƒ ţĥé àƃöṽé ▒

## ▒ Ñéẋţ Šţéþš ▒

- ▒ [Îñîţîàļîžé à Þŕöĵéçţ](/cli/commands/init) ▒
- ▒ [Çüšţöḿ Ƒļöŵš](/cli/flows/custom-flows) ▒
- ▒ [Šéŕṽéŕ Šýñç](/cli/commands/push) ▒
