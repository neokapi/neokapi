---
sidebar_position: 2
title: kgrep
description: kgrep searches the text inside any supported format for a regular expression, skipping markup and structure. It mirrors grep's options and exit status.
keywords: [kgrep, grep, search, regex, docx, json, xliff, multilingual content]
---

# ▒ ķĝŕéþ ▒

▒ Šéàŕçĥ ţĥé ĥüḿàñ-ŕéàđàƃļé ţéẋţ îñšîđé àñý šüþþöŕţéđ ƒöŕḿàţ ƒöŕ à ŕéĝüļàŕ
éẋþŕéššîöñ — ţĥé þŕöšé öƒ à Ŵöŕđ `.đöçẋ`, ţĥé ṽàļüéš öƒ à ĴŠÖÑ çàţàļöĝ, ţĥé
šéĝḿéñţš öƒ àñ ẊĻÎƑƑ ƒîļé — šķîþþîñĝ ḿàŕķüþ àñđ šţŕüçţüŕé. Öüţþüţ ḿîŕŕöŕš
`ĝŕéþ`: öñé ḿàţçĥîñĝ ƃļöçķ þéŕ ļîñé, öþţîöñàļļý þŕéƒîẋéđ ŵîţĥ ţĥé ƒîļé ñàḿé àñđ
ţĥé ƃļöçķ ñüḿƃéŕ. ▒

```bash
kgrep [flags] PATTERN [FILE...]
```

▒ Ţĥé þàţţéŕñ îš à [Ĝö ŕéĝüļàŕ éẋþŕéššîöñ](https://pkg.go.dev/regexp/syntax). Ŵîţĥ
ñö ƒîļé, öŕ ŵĥéñ ţĥé ƒîļé îš `-`, šţàñđàŕđ îñþüţ îš ŕéàđ. Éẋîţ šţàţüš îš `0` îƒ
àñý ƃļöçķ ḿàţçĥéđ, `1` îƒ ñöñé đîđ, `2` öñ éŕŕöŕ — ţĥé šàḿé çöñṽéñţîöñ àš
`ĝŕéþ`, šö `ķĝŕéþ` çöḿþöšéš îñ šĥéļļ çöñđîţîöñàļš. ▒

## ▒ Éẋàḿþļéš ▒

```bash
# Find a word inside a Word document
kgrep "Tervetuloa" report.docx

# Case-insensitive search across JSON catalogs
kgrep -i todo locales/*.json

# Recurse a content tree, searching French translations
kgrep -r --target fr "déconnexion" ./content

# Count occurrences per file
kgrep -c "©" *.md

# Use kgrep's exit status in a script
if kgrep -q "DRAFT" manual.docx; then
  echo "manual still contains a DRAFT marker"
fi
```

## ▒ Öþţîöñš ▒

| Flag | Meaning |
| ---- | ------- |
| `-i, --ignore-case` | Case-insensitive matching. |
| `-v, --invert-match` | Select blocks that do **not** match. |
| `-c, --count` | Print a count of matching blocks per file. |
| `-n, --line-number` | Prefix each match with its block number. |
| `-o, --only-matching` | Print only the matched text, not the whole block. |
| `-l, --files-with-matches` | Print only the names of files containing matches. |
| `-L, --files-without-match` | Print only the names of files with no match. |
| `-w, --word-regexp` | Match only whole words. |
| `-F, --fixed-strings` | Treat the pattern as a literal string, not a regexp. |
| `-r, --recursive` | Recurse into directory arguments. |
| `-H, --with-filename` | Print the file name for each match. |
| `--no-filename` | Suppress file-name prefixes. |
| `-e, --regexp PATTERN` | Add a pattern (repeatable); a block matches if any matches. |
| `-q, --quiet` | Suppress output; report the result through exit status only. |
| `--color MODE` | Highlight matches: `auto`, `always`, `never`. |
| `--target LOCALE` | Search the translation for `LOCALE` instead of the source. |
| `--source-lang LOCALE` | Source language of the content (default `en`). |
| `-f, --format` | Override format detection (e.g. `-f json`). |
| `--encoding NAME` | Input encoding (default `UTF-8`). |
| `--json` | Emit matches as JSON. |
