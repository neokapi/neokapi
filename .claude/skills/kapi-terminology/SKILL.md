---
name: kapi-terminology
description: Build, look up, and enforce consistent terminology with a local termbase (glossary). Import terms from CSV/JSON/TBX, look up the approved translation or spelling of a term, and check that content uses approved terms. Use when the user wants consistent product/domain terminology across content or translations. Triggers on "check terminology", "what's the approved term", "import our glossary", "build a termbase", "TBX".
---

# kapi-terminology

Manages a local termbase (SQLite glossary) with the `kapi termbase` command. Keeps product names, domain terms, and their approved translations consistent across everything you write.

## When to use

- The user has a glossary / term list and wants it imported or enforced.
- You need the canonical term or its translation before writing/translating.
- You want to verify content uses approved terminology.

## Import a glossary

```bash
kapi termbase import glossary.csv --format csv --source-locale en --target-locale fr --local
kapi termbase import terms.tbx   --format tbx  --local      # ISO 30042 TBX (both TBX-Basic and MARTIF)
kapi termbase import terms.json  --format json --local
```

`--local` uses `./termbase.db`; `--name <n>` uses a named termbase in `~/.config/kapi/`; `--file <path>` is explicit.

## Look up a term

```bash
kapi termbase lookup "checkout" --source-locale en --target-locale fr --json
kapi termbase search "pay" --json
```

## Enforce / check terminology in content

Run the `term-check` tool over a file (it flags missing/wrong target terms):

```bash
kapi term-check ./locales/fr.json --json
```

To enforce terminology during AI translation, the termbase feeds `kapi ai-translate` and `kapi-translate`.

## Output

Lookup returns matches with `term`, `locale`, `status` (preferred/approved/deprecated/forbidden), and `match_type`. Use the preferred/approved term; avoid deprecated/forbidden ones.

## How to apply

Look up terms before writing or translating so you use the approved form everywhere. After producing content, run `term-check` (or `kapi-brand-check`, which also covers brand vocabulary) and fix any flagged inconsistencies.
