# Use a kapi project for standing context

A `.kapi` project binds the things that don't change between requests — source and
target locales, which files are content, the brand voice, and the glossary — so
that ordinary requests need no flags. kapi finds the project by walking up from the
current directory, like git.

## When to set one up

Set up a project when the work is ongoing: many files or a whole app, the same
target locales repeatedly, a brand voice or glossary to keep consistent, recurring
runs (CI, re-translate on change), or translation memory to reuse. For a true
one-off, skip it and run the command directly.

## Create it

```bash
kapi init --name my-app --source-locale en --target-locale fr --target-locale de
# --framework react-i18next | nextjs | vue-i18n | flutter | angular  pre-fills content paths
```

This writes `my-app.kapi` (the recipe, committed) and a `.kapi/` state directory
(gitignored: tm.db, termbase.db, caches).

## What the recipe binds

```yaml
version: v1
name: my-app
source_language: en
target_languages: [fr, de]
content:
  - path: src/locales/en.json
    format: json
    target: src/locales/{lang}.json
defaults:
  brand_voice:
    profile_file: brand.yaml   # or: profile: <store name> | pack: marketing-blog
  termbase: .kapi/termbase.db  # bound glossary (this is also the default location)
```

- **Brand voice** — bind it under `defaults.brand_voice`, or just keep a
  `brand.yaml` (or `.kapi/brand.yaml`) in the project; `kapi brand check <file>`,
  `brand rewrite`, and `brand guide` then resolve it with no flag.
- **Glossary / termbase** — import terms into the project termbase
  (`kapi termbase import glossary.csv -s en -t fr`); `kapi term-check <file>` and
  the translation flow enforce it with no `--termbase` flag.
- **Locales + content** — `kapi run <flow>`, `kapi extract`, and `kapi merge`
  apply the project's locales and content globs without `-i` / `--target-lang`.

## Translate within the project (you are the translator)

You don't need a separate translation model — kapi extracts the text and the
guardrails, you translate, kapi merges it back and checks it:

```bash
kapi extract --target-lang fr        # writes out/<...>-to-fr.xliff (source + empty targets)
kapi brand guide                     # the voice to follow (project-bound)
kapi termbase lookup "<term>" -t fr  # the approved wording
```

Fill the `<target>` of each unit in the bilingual file, following the brand guide
and the glossary and preserving placeholders; reuse any targets kapi pre-filled
from translation memory. Then:

```bash
kapi merge -i out/*.xliff            # writes translations into the target files + project TM
```

## Verify, and fix until it passes

Treat your output as a draft until kapi passes it. `kapi verify` runs the project's
gates together — brand voice score, terminology against the bound glossary, and
translation QA (placeholders preserved, nothing left untranslated) — and reports
the exact findings:

```bash
kapi verify --json                   # exit 0 = green; exit 3 = findings to fix
```

Read the findings, fix them, and run it again — loop until it passes. This is the
gate that makes the result trustworthy regardless of how you produced it.

For unattended runs (CI, no assistant), `kapi ai-translate` / `kapi run ai-translate-qa`
call a configured provider instead — the project's brand voice and glossary still apply,
and `kapi verify` is the same gate in the pipeline.
