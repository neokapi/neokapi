# Use a kapi project for standing context

A `.kapi` project binds the things that don't change between requests — source and
target locales, which files are content, the brand voice, and the terms store — so
that ordinary requests need no flags. kapi finds the project by walking up from the
current directory, like git.

## When to set one up

Set up a project when the work is ongoing: many files or a whole app, the same
target locales repeatedly, a brand voice or terminology to keep consistent, recurring
runs (CI, re-translate on change), or content memory to reuse. For a true
one-off, skip it and run the command directly.

## Create it

```bash
kapi init --name my-app --source-locale en --target-locale fr --target-locale de
# --framework <preset>  pre-fills content paths; kapi init --list-presets shows them all
```

This writes `kapi.yaml` (the recipe, committed) and a `.kapi/` state directory
(gitignored: the content memory `memory.db`, the terms store `terms.db`, caches).

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
  terms: .kapi/terms.db  # the bound terms store (also the default location)
```

- **Brand voice** — bind it under `defaults.brand_voice`, or just keep a
  `brand.yaml` (or `.kapi/brand.yaml`) in the project; `kapi brand check <file>`,
  `brand rewrite`, and `brand guide` then resolve it with no flag.
- **Terms** — import terms into the project terms store
  (`kapi terms import terms.csv -s en -t fr`); `kapi exec term-check <file>` and
  the translation flow then enforce it with no `--termstore` flag.
- **Locales + content** — `kapi run <flow>`, `kapi extract`, and `kapi merge`
  apply the project's locales and content globs without `-i` / `--target-lang`.

## Translate within the project (you are the translator)

You don't need a separate translation model — kapi extracts the text and the
guardrails, you translate, kapi merges it back and checks it. Route it through
kapi rather than editing the target file by hand, so terminology, placeholders,
and format stay enforced:

```bash
kapi extract --target-lang fr        # writes out/<name>.en-to-fr.xliff (source + empty targets)
kapi brand guide                     # the voice to follow (project-bound)
kapi terms lookup "<term>" -t fr  # the approved wording
```

Fill the `<target>` of each unit in the bilingual file, following the brand guide
and the approved terminology, and preserving placeholders; reuse any targets kapi
pre-filled from content memory. Then:

```bash
kapi merge -i out/*.xliff            # writes translations into the target files + project content memory
```

## Verify, and fix until it passes

Treat your output as a draft until kapi passes it. `kapi check --ship` runs the project's
gates together — brand voice score, terminology against the bound terms store, and
translation QA (placeholders preserved, nothing left untranslated) — and reports
the exact findings:

```bash
kapi check --ship --json --no-fail         # report: read `pass` + findings; always exits 0
```

Exit 3 from `kapi check --ship` means "not on-spec yet", not a crash — it's the gate giving
you findings to act on. While you're iterating, pass `--no-fail` so it always exits 0
and you read the `pass` field; drop `--no-fail` in CI, where the non-zero exit blocks
the build. Read the findings, fix them, and run it again — loop until it passes. This is the
gate that makes the result trustworthy regardless of how you produced it.

For unattended runs (CI, no assistant), `kapi translate` / `kapi run translate-qa`
call a configured provider instead — the project's brand voice and terms still apply,
and `kapi check --ship` is the same gate in the pipeline.
