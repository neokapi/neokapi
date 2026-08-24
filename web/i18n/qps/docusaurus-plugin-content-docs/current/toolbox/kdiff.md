---
sidebar_position: 6
title: kdiff
description: kdiff compares the text of two files block by block, regardless of format — a reflowed Word document or a reordered JSON catalog shows only the prose that actually changed, not byte-level noise.
keywords: [kdiff, diff, compare, changeset, coverage, docx, json, xliff, multilingual content]
---

# ▒ ķđîƒƒ ▒

▒ Çöḿþàŕé ţĥé ĥüḿàñ-ŕéàđàƃļé ţéẋţ îñšîđé àñý šüþþöŕţéđ ƒöŕḿàţ, **ƃļöçķ ƃý ƃļöçķ**,
ŕàţĥéŕ ţĥàñ ƃýţé ƃý ƃýţé. À ŕéƒļöŵéđ Ŵöŕđ `.đöçẋ`, à ŕé-žîþþéđ çöñţàîñéŕ öŕ à
ŕéöŕđéŕéđ ĴŠÖÑ çàţàļöĝ đö ñöţ ŕéĝîšţéŕ àš à đîƒƒ — öñļý ţĥé þŕöšé ţĥàţ àçţüàļļý
çĥàñĝéđ đöéš. Ţĥîš îš ţĥé çĥàñĝéšéţ à ţŕàñšļàţöŕ (öŕ à ţŕàñšļàţîöñ éñĝîñé) çàŕéš
àƃöüţ: ŵĥàţ çöñţéñţ ḿöṽéđ, ŵàš àđđéđ, ŵàš ŕéḿöṽéđ, öŕ ŵàš ŕéŵŕîţţéñ. ▒

```bash
kdiff [flags] FILE_A [FILE_B]
```

▒ Ŵîţĥ ñö ƒîļé éẋţéñšîöñ ţö ĝö ƃý, ţĥé ƒöŕḿàţ îš šñîƒƒéđ ƒŕöḿ ţĥé çöñţéñţ. À ƒîļé
öƒ `-` ŕéàđš šţàñđàŕđ îñþüţ. ▒

## ▒ Ţŵö ḿöđéš ▒

### ▒ Ŕéṽîšîöñ đîƒƒ — ţŵö ƒîļéš ▒

▒ Ŵĥàţ ţéẋţ çĥàñĝéđ ƃéţŵééñ ţŵö ṽéŕšîöñš öƒ à đöçüḿéñţ. ▒

```bash
# What changed between two catalog versions
kdiff old.json new.json

# What changed in a Word proposal — ignoring re-save / formatting noise
kdiff proposal.docx proposal-v2.docx

# What changed in the French translation specifically
kdiff --target fr old.xliff new.xliff
```

### ▒ Çöṽéŕàĝé đîƒƒ — öñé ƒîļé àĝàîñšţ îţš öŵñ ţŕàñšļàţîöñ ▒

▒ Þàšš à šîñĝļé ƒîļé ŵîţĥ `--ţàŕĝéţ ĻÖÇÀĻÉ` ţö çöḿþàŕé à ţŕàñšļàţîöñ àĝàîñšţ ţĥé
šöüŕçé *ŵîţĥîñ ţĥàţ ƒîļé* — à ǫüîçķ çöṽéŕàĝé ŕéþöŕţ öƒ ŵĥîçĥ ƃļöçķš àŕé šţîļļ
üñţŕàñšļàţéđ öŕ àŕé à ṽéŕƃàţîḿ çöþý öƒ ţĥé šöüŕçé. ▒

```bash
kdiff --target fr messages.xliff
```

```text
app [fr]: 18 translated, 3 untranslated, 1 identical to source
@@ "settings.title" (untranslated) @@
  Settings
@@ "brand.name" (identical) @@
  Acme
```

## ▒ Ĥöŵ ƃļöçķš àŕé àļîĝñéđ ▒

▒ `ķđîƒƒ` đöéš ñöţ çöḿþàŕé ļîñé þöšîţîöñš — îţ àļîĝñš ţĥé đöçüḿéñţ'š **ƃļöçķš**.
Ţĥé àļîĝñḿéñţ šţŕàţéĝý îš çĥöšéñ àüţöḿàţîçàļļý: ▒

- ▒ **Ķéýéđ ƒöŕḿàţš** (ĴŠÖÑ, ẊĻÎƑƑ, ÞÖ, `.ŕéšẋ`, … — àñýţĥîñĝ ŵîţĥ à šţàƃļé ƃļöçķ
  ķéý) àļîĝñ **ƃý ķéý**. Ŕéöŕđéŕîñĝ ķéýš îš ţĥéŕéƒöŕé ñöţ à đîƒƒ; ŕéñàḿîñĝ ţĥé
  ṽàļüé üñđéŕ à ķéý îš à *çĥàñĝé*; à ñéŵ ķéý îš àñ *àđđîţîöñ*. Ŕéöŕđéŕéđ ķéýš àŕé
  ŕéþöŕţéđ àš `ḿöṽéđ`, ñöţ àš à ŵĥöļéšàļé ŕéŵŕîţé ţĥé ŵàý à ļîñé đîƒƒ ŵöüļđ šĥöŵ
  ţĥéḿ. ▒
- ▒ **Þŕöšé ƒöŕḿàţš** (Ŵöŕđ, Ḿàŕķđöŵñ, ĤŢḾĻ — ŵĥöšé ƃļöçķ îđéñţîţîéš àŕé
  þöšîţîöñàļ) àļîĝñ **ƃý çöñţéñţ**, üšîñĝ à ļöñĝéšţ-çöḿḿöñ-šüƃšéǫüéñçé ḿàţçĥ
  öṽéŕ ţĥé ƃļöçķ ţéẋţ. Îñšéŕţîñĝ à þàŕàĝŕàþĥ šĥöŵš üþ àš à šîñĝļé àđđéđ ƃļöçķ,
  ñöţ àš à çàšçàđé öƒ "éṽéŕýţĥîñĝ àƒţéŕ îţ çĥàñĝéđ". ▒

▒ Ƒöŕçé à šţŕàţéĝý ŵîţĥ `--ƃý îđ` öŕ `--ƃý çöñţéñţ` ŵĥéñ ţĥé àüţöḿàţîç çĥöîçé îš
ŵŕöñĝ ƒöŕ ýöüŕ ƒîļé. ▒

## ▒ Öüţþüţ ▒

▒ Ţĥé đéƒàüļţ öüţþüţ îš à ƃļöçķ-öŕîéñţéđ üñîƒîéđ đîƒƒ: à `@@ <ķéý> (ķîñđ) @@`
ĥéàđéŕ þéŕ çĥàñĝéđ ƃļöçķ, ţĥéñ ţĥé ŕéḿöṽéđ (`-`) àñđ àđđéđ (`+`) ţéẋţ. ▒

```text
--- a/old.json
+++ b/new.json
@@ "greeting" (changed) @@
- Hello
+ Hi there
@@ "welcome" (added) @@
+ Welcome!
@@ "farewell" (removed) @@
- Goodbye
```

▒ `--ĵšöñ` éḿîţš ţĥé šàḿé çĥàñĝéšéţ àš šţŕüçţüŕéđ đàţà ƒöŕ šçŕîþţš àñđ ţööļš, àñđ
`--šţàţ` þŕéƒîẋéš à öñé-ļîñé šüḿḿàŕý (`2 çĥàñĝéđ, 1 àđđéđ, 1 ŕéḿöṽéđ`). `-ǫ,
--ƃŕîéƒ` þŕîñţš öñļý ŵĥéţĥéŕ ţĥé îñþüţš đîƒƒéŕ. ▒

## ▒ Éẋîţ šţàţüš ▒

▒ `ķđîƒƒ` ƒöļļöŵš ţĥé çļàššîç `đîƒƒ` çöñṽéñţîöñ, šö šĥéļļ šçŕîþţš àñđ ÇÎ çàñ ƃŕàñçĥ
öñ ţĥé ŕéšüļţ: ▒

| Code | Meaning |
| ---- | ------- |
| `0` | The inputs are equivalent (or the translation is fully covered). |
| `1` | They differ (or there is untranslated / copied content). |
| `2` | An operational error (a file could not be read or parsed). |

## ▒ Éẋàḿþļéš ▒

```bash
# Review what changed in a release's source catalog
kdiff v1/en.json v2/en.json

# Gate CI on a fully-translated file (exit 1 if anything is pending)
kdiff --target de -q messages.xliff || echo "German is incomplete"

# Machine-readable changeset for a re-translation pipeline
kdiff --json old.xliff new.xliff > changes.json

# Compare two drafts of a Word document, prose only
kdiff --by content draft.docx final.docx
```

## ▒ Öþţîöñš ▒

| Flag | Meaning |
| ---- | ------- |
| `--by STRATEGY` | Alignment: `auto` (default), `id`, or `content`. |
| `--target LOCALE` | Compare the translation for `LOCALE`. With one file, a coverage report of source vs. translation. |
| `-q, --brief` | Report only whether the inputs differ, not the changes. |
| `--stat` | Print a one-line summary of the changes before the diff. |
| `--json` | Emit the diff as JSON. |
| `--color` | Colorize the diff: `auto`, `always`, `never`. |
| `-f, --format` | Override format detection (e.g. `-f json`). |
| `--source-lang` | Source language (default `en`). |
| `--encoding` | Input encoding (default `UTF-8`). |
