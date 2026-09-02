# Use a kapi project for standing context

A kapi project binds the things that don't change between requests (which files
are content, the voice profile, the terms store, the declared coordinates, and
the source and target locales) so that ordinary requests need no flags. kapi
finds the project by walking up from the current directory, like git.

## When to set one up

Set up a project when the work is ongoing: many files or a whole app, the same
target locales repeatedly, a voice profile or terminology to keep consistent, recurring
runs (CI, re-translate on change), or content memory to reuse. For a true
one-off, skip it and run the command directly.

## Create it

```bash
kapi init --name my-app --source-locale en --target-locale fr --target-locale de
# --framework <preset>  pre-fills content paths; kapi init --list-presets shows them all
```

This writes `kapi.yaml` (the recipe) and a `.kapi/` directory. **`.kapi/` is
committed**: it is the project's context rather than scratch space. Git it like source.

- **`.kapi/`**: the context graph, all committed and flat: `terms.json`,
  `voice.yaml`, `memory/` (the content-memory bundles, `memory.json` the
  primary), `profiles/<name>/` (what a profile overrides), and `state/*.jsonl`,
  the unit-state record (one shard per document). `kapi commit` publishes staged
  unit state into `state/`; then `git add` it like any other source file.
- **`.kapi/work/`**: everything derived, and the only gitignored path.
  `store.db` is the local index over the recipe, the context sources and the
  content files. Never read or write it directly and never commit it; go through
  kapi commands, which are what keep it consistent with the sources.

The ignore rule `kapi init` writes is `.kapi/.gitignore` with two lines, `work/`
and `filters.local.json` (a developer's personal reader settings). If you see
a project ignoring more of `.kapi/` than that, it is stale.

Deleting:

- `rm -rf .kapi/work/cache` is **always** safe; everything under it rebuilds on
  the next run.
- `rm -rf .kapi/work` costs two things. Review unit state staged since the last
  `kapi commit` live only in `store.db`; run `kapi commit` first. And if the
  project uses redaction, `.kapi/work/vault/` holds the withheld originals, which
  are local-only by design and rebuild from nothing: merge any batch that is out
  with a translator before clearing it.

## What the recipe binds

```yaml
version: v1
name: my-app
defaults:
  source_language: en
  target_languages: [fr, de]
  voice:
    profile_file: .kapi/voice.yaml   # or: profile: <store name> | pack: marketing-blog
  terms_source: .kapi/terms.json    # the committed terms source
  memory_source: .kapi/memory/memory.json  # the committed content memory
  coordinates:
    brand: my-app                   # a declared axis; product/channel are never written here
collections:
  - path: src/locales/en.json
    format: json
    target: src/locales/{lang}.json
  - name: package-copy              # read and checked, never written back
    source_only: true
    content:
      - path: deploy/homebrew/*.rb
        format: { name: sourcecode, config: { language: ruby, nodePathPatterns: [desc, caveats] } }
```

- **Voice profile**: bind it under `defaults.voice`, or just keep a
  `.kapi/voice.yaml` (or a `voice.yaml` at the project root); `kapi
  voice check <file>`, `voice rewrite`, and `voice guide` then resolve it with no
  flag.
- **More than one voice in one repo**: declare one profile per product under
  `profiles:`, list the channels that product ships on, and bind each *named*
  collection to one of them with `channel:`. Runs split per distinct resolution,
  so each product's content is translated and checked under its own voice and
  vocabulary:

  ```yaml
  profiles:
    framework:
      channels: [docs]
      voice: .kapi/voice.yaml
    platform:                        # .kapi/profiles/platform/voice.yaml and
      channels: [docs, landing]      # terms.json answer by convention; bind
                                     # `voice:`/`termstore:` only to override them

  collections:
    - name: platform-docs
      channel: platform/docs
      content:
        - path: platform/docs/**/*.md
    - name: platform-landing
      channel: platform/landing
      content:
        - path: platform/web/pages/*.tsx
  ```

  Profile names and channels are slugs. The profile name is also the directory
  under `.kapi/profiles/<name>/`. An explicit `--profile` still beats the recipe.
  The channel additionally picks the override inside the selected profile's
  voice, so a landing register lives beside the voice it varies rather than in a
  second file. A channel no profile declares, and a bare channel two profiles
  declare, both fail the load; kapi will not quietly translate that content in
  the wrong voice.

  The recipe is the authoring surface for governance. A push carries every
  collection, its point and the governing voice, so a connected project resolves
  the same voice on the server. A profile's `termstore:` is the exception: it names
  a local path, so a project binding terms per profile and also bound to a server
  warns on every run that the binding applies to local runs only.
- **Declared axes**: `defaults.coordinates` names the axes the project varies
  along beyond product and channel (`brand`, `mode`), inherited by every
  collection; a collection that sits elsewhere sets the one axis it differs on
  in its own `coordinates:`. `product` and `channel` are derived from `channel:`
  and rejected here. `kapi context <path>` prints the point a file resolved to
  and everything governing it; `kapi context search <query>` asks every bound
  store at once.
- **Source-only collections**: `source_only: true` declares content kapi reads
  and checks but never writes back (package descriptions, installer strings);
  `kapi up` skips it and target-coverage gates exclude it. A collection that
  also names a target fails to load.
- **Terms**: import terms into the project terms store
  (`kapi terms import terms.csv -s en -t fr`); `kapi exec term-check <file>` and
  the translation flow then enforce it with no `--termstore` flag. Rules without
  a store go under a flow step's `term_rules:` (one `term`, its `replacement`,
  a `severity`, optionally a `concept_id`), the same shape a voice profile's
  vocabulary uses; `term-check`, `translate`, `recycle`, `dnt-check` and
  `pseudo-translate` all take it.
- **Locales + content**: `kapi run <flow>`, `kapi extract`, and `kapi merge`
  apply the project's locales and content globs without `-i` / `--target-lang`.

## Translate within the project (you are the translator)

You don't need a separate translation model: kapi extracts the text and the
guardrails, you translate, kapi merges it back and checks it. Route it through
kapi rather than editing the target file by hand, so terminology, placeholders,
and format stay enforced:

```bash
kapi extract --target-lang fr        # writes out/<name>.en-to-fr.xliff (source + empty targets)
kapi voice guide                     # the voice to follow (project-bound)
kapi terms lookup "<term>" -t fr  # the approved wording
```

Fill the `<target>` of each unit in the bilingual file, following the voice guide
and the approved terminology, and preserving placeholders; reuse any targets kapi
pre-filled from content memory. Then:

```bash
kapi merge -i out/*.xliff            # writes translations into the target files + project content memory
```

## Verify, and fix until it passes

Treat your output as a draft until kapi passes it. `kapi check --ship` runs the project's
gates together (voice profile score, terminology against the bound terms store, and
translation checks: placeholders preserved, nothing left untranslated) and reports
the exact findings:

```bash
kapi check --ship --json --no-fail         # report: read `pass` + findings; always exits 0
```

Exit 3 from `kapi check --ship` means "not on-spec yet" rather than a crash; it's the gate giving
you findings to act on. While you're iterating, pass `--no-fail` so it always exits 0
and you read the `pass` field; drop `--no-fail` in CI, where the non-zero exit blocks
the build. Read the findings, fix them, and run it again; loop until it passes. This is the
gate that makes the result trustworthy regardless of how you produced it.

For unattended runs (CI, no assistant), `kapi translate` / `kapi run translate-qa`
call a configured provider instead; the project's voice profile and terms still apply,
and `kapi check --ship` is the same gate in the pipeline.
