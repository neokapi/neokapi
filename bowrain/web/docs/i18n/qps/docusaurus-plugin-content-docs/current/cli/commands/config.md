---
title: config
sidebar_position: 2
---

# ▒ Ƃöŵŕàîñ šéţţîñĝš (`ķàþî çöñƒîĝ`) ▒

▒ Ţĥéŕé îš öñé çöñƒîĝüŕàţîöñ çöḿḿàñđ, `ķàþî çöñƒîĝ`. Ţĥé ƃöŵŕàîñ þļüĝîñ đöéš ñöţ
šĥîþ îţš öŵñ: îţ çļàîḿš ţĥé `ƃöŵŕàîñ.*` ķéý ñàḿéšþàçé îñ îţš ḿàñîƒéšţ
(`çàþàƃîļîţîéš.çöñƒîĝ_ñàḿéšþàçéš`), àñđ ķàþî ŕöüţéš ţĥöšé ķéýš ţö ţĥé þļüĝîñ'š
öŵñ çöñƒîĝ ƒîļé. ▒

▒ Ţŵö šçöþéš šĥàŕé ţĥé ṽéŕƃ, šþļîţ ƃý šĥàþé: ▒

| Shape | Scope | Stored in |
| --- | --- | --- |
| `kapi config <key> [value]` (positional) | the project recipe | `kapi.yaml`, committed |
| `kapi config set/get/unset <key> [value]` | per-machine app config | `kapi.yaml` in the user config directory, or a plugin's own file for a namespaced key |

▒ Ţĥé üšéŕ çöñƒîĝ đîŕéçţöŕý îš `~/.çöñƒîĝ/ķàþî` öñ Ļîñüẋ àñđ
`~/Ļîƃŕàŕý/Àþþļîçàţîöñ Šüþþöŕţ/ķàþî` öñ ḿàçÖŠ. À ĥàñđ-ŵŕîţţéñ ƒîļé àţ
`$ĤÖḾÉ/.çöñƒîĝ/ķàþî/ķàþî.ýàḿļ` îš ŕéàđ öñ éîţĥéŕ þļàţƒöŕḿ àš à
ļöŵéŕ-þŕéçéđéñçé ļàýéŕ, àñđ `ķàþî çöñƒîĝ þàţĥ` þŕîñţš ţĥé ļöçàţîöñ ţĥàţ îš
àçţüàļļý îñ ƒöŕçé. ▒

## ▒ Þéŕ-ḿàçĥîñé ƃöŵŕàîñ đéƒàüļţš ▒

```bash
kapi config get bowrain.server.url        # Read the default server URL
kapi config set bowrain.server.url https://app.bowrain.cloud
kapi config unset bowrain.server.url      # Restore the built-in default
kapi config path bowrain                  # bowrain/bowrain.yaml under the user config directory
```

▒ `ķàþî çöñƒîĝ ļîšţ` šĥöŵš éṽéŕý ñàḿéšþàçé àţ öñçé, ķàþî'š öŵñ ķéýš àļöñĝšîđé
éàçĥ îñšţàļļéđ þļüĝîñ'š, àñđ ţàķéš `-ö ĵšöñ` ļîķé àñý öţĥéŕ ļîšţîñĝ. ▒

| Key | Description | Example |
| --- | --- | --- |
| `bowrain.server.url` | Default server URL `kapi init` offers for new projects | `https://app.bowrain.cloud` |

## ▒ Þŕöĵéçţ šéţţîñĝš ▒

▒ À þŕöĵéçţ'š öŵñ šéŕṽéŕ ƃîñđîñĝ ļîṽéš îñ ţĥé ŕéçîþé, ñöţ îñ þéŕ-ḿàçĥîñé çöñƒîĝ,
šö îţ îš çöḿḿîţţéđ àñđ ţŕàṽéļš ŵîţĥ ţĥé ŕéþöšîţöŕý: ▒

```bash
kapi config name                # Read the project name
kapi config name "My Project"   # Set the project name
kapi config server.url          # Read the compound server URL
kapi config server.url https://app.bowrain.cloud/my-team/proj_abc123
kapi config server.stream '$auto'
```

| Key | Description | Example |
| --- | --- | --- |
| `name` | Project name | `My App` |
| `defaults.source_language` | Source locale (BCP 47) | `en` |
| `defaults.target_languages` | Target locales (list) | `[fr, de]` |
| `server.url` | Compound server URL (encodes server / workspace / project) | `https://app.bowrain.cloud/my-team/proj_abc123` |
| `server.stream` | Server stream (`$auto` for auto-detect) | `$auto` |

## ▒ Þŕéçéđéñçé ▒

▒ Ţĥé þéŕ-ḿàçĥîñé đéƒàüļţ îš à šţàŕţîñĝ þöîñţ ƒöŕ ñéŵ þŕöĵéçţš; ţĥé ŕéçîþé îš
àüţĥöŕîţàţîṽé ƒöŕ à þŕöĵéçţ ţĥàţ àļŕéàđý éẋîšţš. Šéţ ţĥé đéƒàüļţ öñçé: ▒

```bash
kapi config set bowrain.server.url https://app.bowrain.cloud
```

▒ àñđ éṽéŕý šüƃšéǫüéñţ `ķàþî îñîţ` öƒƒéŕš îţ, ŵĥîļé àñý îñđîṽîđüàļ þŕöĵéçţ çàñ
šţîļļ þöîñţ éļšéŵĥéŕé: ▒

```bash
kapi config server.url https://staging.bowrain.cloud
```
