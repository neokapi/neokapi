---
title: workspace
sidebar_position: 11
---

# ▒ ķàþî ŵöŕķšþàçé ▒

▒ Ļîšţ ţĥé ŵöŕķšþàçéš ýöü çàñ àççéšš öñ à Ƃöŵŕàîñ šéŕṽéŕ, öŕ çŕéàţé à ñéŵ ţéàḿ
ŵöŕķšþàçé. Ŕéǫüîŕéš àüţĥéñţîçàţîöñ (ŕüñ [`ķàþî àüţĥ ļöĝîñ`](/cli/commands/auth)
ƒîŕšţ). ▒

## ▒ Üšàĝé ▒

```bash
kapi workspace list
kapi workspace create --name "<name>" [--slug <slug>]
```

▒ Ţĥé šéŕṽéŕ îš ŕéšöļṽéđ ƒŕöḿ `--šéŕṽéŕ`, ţĥéñ `ƂÖŴŔÀÎÑ_ŠÉŔṼÉŔ_ÜŔĻ` /
`šéŕṽéŕ.üŕļ` îñ [`ƃöŵŕàîñ.ýàḿļ`](/cli/commands/config), ţĥéñ ýöüŕ šţöŕéđ ļöĝîñ,
ƒàļļîñĝ ƃàçķ ţö ţĥé ĥöšţéđ šéŕṽîçé (`ĥţţþš://àþþ.ƃöŵŕàîñ.çļöüđ`). ▒

## ▒ Šüƃçöḿḿàñđš ▒

### ▒ `ļîšţ` ▒

▒ Ļîšţš éṽéŕý ŵöŕķšþàçé ýöüŕ àççöüñţ çàñ àççéšš, ḿàŕķîñĝ ýöüŕ þéŕšöñàļ ŵöŕķšþàçé. ▒

```bash
kapi workspace list

# Example output:
# alice (Alice) [personal]
# acme (Acme Corp)
```

▒ Àđđ `--ĵšöñ` ƒöŕ ḿàçĥîñé-ŕéàđàƃļé öüţþüţ: ▒

```bash
kapi workspace list --json
```

### ▒ `çŕéàţé` ▒

▒ Çŕéàţéš à ñéŵ ţéàḿ ŵöŕķšþàçé. Ţĥé šļüĝ îš đéŕîṽéđ ƒŕöḿ `--ñàḿé` ŵĥéñ `--šļüĝ`
îš öḿîţţéđ. ▒

```bash
kapi workspace create --name "Acme Corp"
# Workspace created: acme-corp (Acme Corp)

kapi workspace create --name "Acme Corp" --slug acme
# Workspace created: acme (Acme Corp)
```

## ▒ Ƒļàĝš ▒

| Flag       | Applies to | Description                                      |
| ---------- | ---------- | ------------------------------------------------ |
| `--server` | both       | Server URL (overrides config and stored login)   |
| `--name`   | `create`   | Workspace name (required)                        |
| `--slug`   | `create`   | URL-friendly slug (derived from `--name` if omitted) |

## ▒ Éẋîţ Çöđéš ▒

- ▒ `0` — Šüççéšš ▒
- ▒ `1` — Éŕŕöŕ (ñöţ àüţĥéñţîçàţéđ, šéŕṽéŕ üñŕéàçĥàƃļé, šļüĝ çöñƒļîçţ, …) ▒

## ▒ Ŕéļàţéđ Çöḿḿàñđš ▒

- ▒ [`ķàþî àüţĥ`](/cli/commands/auth) — Àüţĥéñţîçàţé ƃéƒöŕé ļîšţîñĝ öŕ çŕéàţîñĝ ŵöŕķšþàçéš ▒
- ▒ [`ķàþî îñîţ`](/cli/commands/init) — Šçàƒƒöļđš à þŕöĵéçţ àñđ çàñ šéļéçţ öŕ çŕéàţé à ŵöŕķšþàçé îñţéŕàçţîṽéļý ▒
