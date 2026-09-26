# Use a kapi project for standing context

A kapi project binds the things that don't change between requests (which files
are content, the voice profile, the terms store, the declared coordinates, and
the source and target locales) so that ordinary requests need no flags. kapi
finds the project by walking up from the current directory, like git.

## Before a command: what do you need on this machine

- The `kapi` binary on PATH (`kapi version`).
- No AI provider credential when you write, edit or translate the text yourself
  within kapi's guardrails, including editing through `kapi apply`, which
  applies your edits with no model. A saved credential (`kapi credentials add`)
  is only needed for kapi to call a provider itself: unattended translation
  (`kapi translate`) or the optional `--ai` checks. The rule-based voice and
  terminology checks need none.
- `kapi formats --json` reports whether a format is editable and round-trips
  faithfully. Check that before relying on preservation.

## When to set one up

Judge whether the work is a one-off or ongoing before reaching for a command:

- **Ad hoc**: one file or a snippet, a one-time read, check or edit,
  exploration, one or no target language. Run the command; no setup. kapi works
  without a project.
- **Project**: many files or a whole app, the same target locales repeatedly, a
  voice profile or terminology to keep consistent, recurring runs (CI,
  re-translate on change), or content memory to reuse. Bind that context once,
  then issue plain requests: kapi applies the project's locales, content, voice
  profile and terms with no flags.

If a project already exists, use it. If the task is project-shaped and there is
none, offer to set one up; do not impose a project on a genuine one-off.

## The layers under the porcelain

The porcelain verbs compose a lower layer you can drive directly. `kapi exec
<tool>` runs one registry tool with nothing around it, `kapi run <flow>` runs one
named flow for one pass, and `kapi extract`/`kapi merge` carry the translator
hand-off. Reach for them only when a task needs exactly one tool or one custom
pipeline; the layer model is
[Understanding the CLI layers](https://neokapi.github.io/kapi/direct-execution-layer).

## Create it

```bash
kapi init --name my-app --source-locale en --target-locale fr --target-locale de
# --framework <preset>  writes a known stack's catalog layout; kapi init --list-presets shows them all
```

This writes `kapi.yaml` (the recipe) with a collection for each kind of content
kapi found in the tree, each under a comment saying what it matched. Read the
list back to the user and adjust it with them; `kapi add <pattern>` adds one. On
a project that already has a recipe, `kapi init` leaves the collections alone
and prints the content no collection reads yet, with the lines to add.

The recipe is the configuration, and it is committed. The project's **context**
(its terms, its voice profiles, its content memory and its decisions) lives in
the user's workspace, one store per project, shared by every checkout. Never
tell the user to commit their context.

When the project binds a voice, `kapi init` also writes a short section into the project's assistant file: an
existing `CLAUDE.md` or `AGENTS.md` at the root, or a new `CLAUDE.md`. It says
the voice is held by kapi and that `kapi voice guide` retrieves it, so the next
assistant in this tree asks before it writes. The section sits between
`<!-- kapi:voice -->` markers and is replaced in place on every run; the rest of
the file is never touched. `kapi init --no-pointer` skips it. On an existing
project, `kapi voice pointer` writes or refreshes the same section: run it after
you bind a voice under `defaults.voice`, and tell the user which file it went
into, since they commit it. A section that landed in `AGENTS.md` reaches an
assistant limited to `CLAUDE.md` through an import line, `@AGENTS.md`.

`kapi init` also writes the MCP entry that starts `kapi mcp` for this project
(`.mcp.json` for Claude Code, `.cursor/mcp.json`, `.vscode/mcp.json`,
`.codex/config.toml`), naming `--tools writing,translation` when the recipe
declares target languages, and the short kapi skill (one `SKILL.md`) in the
host's skills directory. It names every file it writes and leaves an entry
someone else put there alone. `--agents <list|all|none>` chooses; tell the user
which files landed, since they commit them.

- **`.kapi/`**: this checkout's cache, ignored by version control. `work/store.db`
  is the checkout's projection of the working tree: the block cache, the
  overlays a run wrote, the extraction stamps. Beside it sit the caches, the
  redaction vault and `filters.local.json`, the reader filters saved as
  personal to this checkout.
- **The workspace**, under the user's data directory
  (`<data dir>/workspaces/default/`), holds one context store per project: the
  terms, the voice profiles, the content memory, the decision ledger and the
  reader filters saved as shared, all shared by every checkout of that
  project. This is where every read goes: a gate, a
  lookup and `kapi context <path>` all answer from it, on any branch. Never read
  or write either database directly and never commit one; go through kapi
  commands.

The ignore rule `kapi init` writes is `.kapi/.gitignore` with one line, `*`. A
project that commits context files in `.kapi/` for `kapi context import` to read
keeps an ignore rule of its own, and kapi leaves it as it is.

Deleting:

- `rm -rf .kapi/work/cache` is **always** safe; everything under it rebuilds on
  the next run.
- `rm -rf .kapi/work` costs a re-extraction. If the project uses redaction,
  `.kapi/work/vault/` holds the withheld originals, which are local-only by
  design and rebuild from nothing: merge any batch that is out with a translator
  before clearing it. Decisions are in the workspace and survive it.
- Deleting the workspace costs every project's terms, voice profiles, content
  memory and decisions. Tell the user to push each project to its context
  backend (`kapi context push`) or take a copy first with
  `kapi context export -o backup.kpz` in each project.

## What the recipe binds

```yaml
version: v1
name: my-app
defaults:
  source_language: en
  target_languages: [fr, de]
  voice:
    profile: my-app-docs            # a profile in the project's voice store
                                    # or: pack: marketing-blog
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

- **Voice profile**: `kapi voice import <file.yaml>` files one in the project's
  voice store and prints its id; bind that id under `defaults.voice.profile`.
  `kapi voice check <file>`, `voice rewrite` and `voice guide` then resolve it
  with no flag, and `kapi voice edit` is how the user changes it afterwards.
  Then `kapi voice pointer`, so the assistant file names the voice.
- **More than one voice in one repo**: declare one profile per product under
  `profiles:`, list the channels that product ships on, and bind each *named*
  collection to one of them with `channel:`. Runs split per distinct resolution,
  so each product's content is translated and checked under its own voice and
  terms:

  ```yaml
  profiles:
    framework:
      channels: [docs]
      voice:
        profile: framework-docs
    platform:                        # no `voice:`; the profile the store holds
      channels: [docs, landing]      # under "platform" answers

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

  Profile names and channels are slugs. A profile that binds no `voice:` is
  answered by the profile the store holds under its own name. An explicit
  `--profile` still beats the recipe.
  The channel additionally picks the override inside the selected profile's
  voice (its tone, its style, or both),
  so a landing register lives beside the voice it varies rather than in a
  second file. A channel no profile declares, and a bare channel two profiles
  declare, both fail the load; kapi will not quietly translate that content in
  the wrong voice.

  The recipe is the authoring surface for governance. A push carries every
  collection, its point and the governing voice, so a connected project resolves
  the same voice on the server. A profile's `termstore:` is the exception: it names,
  by the name `--termstore` takes, a terms store this machine keeps, and a push
  does not carry it, so a project binding terms per profile and also bound to a
  server warns on every run that the binding applies to local runs only.
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
- **Comments in code**: `comments: true` on a content item checks the comments
  in its files as content. Go source has no format reader, so its comments are
  what kapi reads: `kapi check` and `check --ship` check them under the item's
  voice and terms, one block per comment named for its declaration
  (`func/Parse`), and a comment gofmt would rewrite fails the gate as
  `formatter.gofmt`. `kapi up` leaves the files alone. Directives, generated
  files and the code inside doc comments are never read as prose. `kapi check
  <file>.go` works on a single file with no recipe. TypeScript, TSX,
  JavaScript, Python, Bash, CSS, Rust, Java, C#, C, C++ and Ruby comments are
  read the same way once the sourcecode plugin is installed (`kapi plugins install sourcecode`); without it
  a project check warns `format.no_reader`, names the plugin, and counts those
  files as not checked.
  On a YAML item, or an item whose format supplies its files' comments (such as
  `androidxml`, `resx`, `xliff`, `html`, `markdown`, `mdx`, `po` or
  `properties`), the
  comments are checked beside the values its reader extracts, by `kapi check`,
  `--ship` and a diff-scoped check alike, and `kapi up` converges the file
  exactly as it does without the key. An XML or HTML comment is named for its
  element (`comment/resources/string[greeting]`), a Markdown or MDX comment for
  its section (`comment/install/from-homebrew`), and commented-out markup,
  conditional comments, code and tool suppressions are never read as prose.
  `comments: {only: true}` declares an item's files for their comments alone,
  for a file whose values another tool owns (a CI workflow, a build config):
  a check of the project reads the comments and none of the values, and
  `kapi up`, merge, extract, flow runs, `kapi stats`, coverage and the ship
  gates skip the values. The item governs only the comments: `kapi check
  <file>` naming the file checks its values too, and `kapi voice guide <file>`
  and `kapi context <file>` answer for them, at the next item that claims the
  file or the project's default point. Another item that matches the same
  files claims their values wherever either item is listed, so `kapi up`
  converges those values and the comments stay at the comments-only item's
  point. Such an item cannot also set `target`,
  `target_languages`, `redaction`, `format.config` or `format.preset`.
  Markers your own tools read in
  comments, such as `okapi-skip:`, go under `defaults.comments.directives` for
  the whole project or `comments: {directives: [...]}` on an item: a comment
  line that starts with one is set aside rather than checked, and a marker
  inside a comment splits it in two. `comments: {channel: profile/channel}` on
  an item, or `defaults.comments.channel`, checks the comments under that
  point's voice and terms while the rest of the file keeps the item's point;
  each finding and each `execution.contexts` entry carries the `point` it was
  checked at.
- **Terms**: import terms into the project terms store
  (`kapi terms import terms.csv -s en -t fr`); `kapi exec term-check <file>` and
  the translation flow then enforce it with no `--termstore` flag. Rules without
  a store go under a flow step's `term_rules:` (one `term`, its `replacement`,
  `advisory: true` for a rule that only reports, optionally a `concept_id`),
  the same shape as every word rule, including the `terms:` a voice file
  carries; `term-check`, `translate`, `recycle`, `dnt-check` and
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

## Verify content and release gates

During authoring, check each edited file with `kapi check <file> --json`. Review
the findings and analyzer coverage against the task's requirements.

For release verification, `kapi check --ship` runs the project's applicable gates
together: voice and terms, source content checks, translation checks for target
content, and any declared coverage requirements. It reports the findings:

```bash
kapi check --ship --json --no-fail         # report: read `verdict` + findings
```

Exit 3 means a gate failed. With `--no-fail`, a failed gate exits 0 and the
`verdict` field carries it; omit that flag when CI must enforce the release bar.
Exit 4 means a gate did not run, with or without `--no-fail`: read
`did_not_run_cause` before continuing. `checker_invalid` means a checker is
broken and the run cannot be trusted; `nothing_to_check` and
`content_not_checked` mean some content was never checked.
The top-level `warnings` list names configuration to fix, such as an unknown key
in a voice profile, and never changes a gate's verdict or the exit code.
Correct findings within the requested scope and re-check. If a finding persists
or conflicts with the governing guidance, report it for review. A passing gate
establishes its declared checks; meaning and unsupported writing guidance still
need review.

For unattended runs (CI, no assistant), `kapi translate` / `kapi run translate-qa`
call a configured provider instead; the project's voice profile and terms still apply,
and `kapi check --ship` is the same gate in the pipeline.
