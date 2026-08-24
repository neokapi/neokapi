---
sidebar_position: 4
title: kcat
description: kcat prints the text of files block by block, regardless of format — a Word document, a JSON catalog and an XLIFF file all print as plain prose.
keywords: [kcat, cat, print, extract text, docx, json, xliff, multilingual content]
---

# ▒ ķçàţ ▒

▒ Þŕîñţ ţĥé ĥüḿàñ-ŕéàđàƃļé ţéẋţ éẋţŕàçţéđ ƒŕöḿ éàçĥ ƒîļé, öñé ƃļöçķ þéŕ ļîñé,
ŕéĝàŕđļéšš öƒ ţĥé üñđéŕļýîñĝ ƒöŕḿàţ. À Ŵöŕđ `.đöçẋ`, à ĴŠÖÑ çàţàļöĝ àñđ àñ
ẊĻÎƑƑ ƒîļé àļļ þŕîñţ àš ţĥéîŕ þļàîñ þŕöšé, ŵîţĥ ţĥé ḿàŕķüþ àñđ šţŕüçţüŕé
šţŕîþþéđ. ▒

```bash
kcat [flags] [FILE...]
```

▒ Ŵîţĥ ñö ƒîļé, öŕ ŵĥéñ ţĥé ƒîļé îš `-`, šţàñđàŕđ îñþüţ îš ŕéàđ. ▒

## ▒ Éẋàḿþļéš ▒

```bash
# See the prose inside a Word document
kcat report.docx

# Number the blocks of a JSON catalog
kcat -n locales/en.json

# Print the French translations of an XLIFF file
kcat --target fr messages.xliff

# Pipe arbitrary text through, treating it as plain text
cat raw.txt | kcat -f plaintext
```

▒ `ķçàţ` þàîŕš ñàţüŕàļļý ŵîţĥ ţĥé šĥéļļ ţööļš ýöü àļŕéàđý ĥàṽé — þîþé îţš öüţþüţ
îñţö ţĥé ŕéàļ `ĝŕéþ`, `ŵç`, öŕ `šöŕţ` ŵĥéñ ýöü ŵàñţ ƃýţé-ļéṽéļ ļîñé ƃéĥàṽîöüŕ
ŕàţĥéŕ ţĥàñ ţĥé ƃļöçķ-àŵàŕé `ķĝŕéþ`. ▒

## ▒ Öþţîöñš ▒

| Flag | Meaning |
| ---- | ------- |
| `-n, --number` | Number the output blocks. |
| `--id` | Prefix each block with its source ID. |
| `-r, --recursive` | Recurse into directory arguments. |
| `--target LOCALE` | Print the translation for `LOCALE` instead of the source. |
| `-f, --format` | Override format detection (e.g. `-f json`). |
| `--source-lang` | Source language (default `en`). |
| `--encoding` | Input encoding (default `UTF-8`). |
| `--json` | Emit blocks as JSON instead of plain text. |
