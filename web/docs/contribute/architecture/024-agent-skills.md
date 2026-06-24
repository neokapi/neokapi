---
id: 024-agent-skills
sidebar_position: 24
title: "AD-024: Agent Skills"
description: "Architecture decision: kapi ships an Agent Skill (a SKILL.md router with progressive-disclosure reference files) whose source lives in the monorepo, in lockstep with the CLI surface, and is distributed to AI coding assistants as a Claude Code plugin through the neokapi-plugins marketplace — teaching the assistant to drive the kapi CLI for editing, brand, terminology, and localization."
keywords: [agent skills, SKILL.md, Claude, .claude/skills, progressive disclosure, AI assistant, CLI, architecture decision, neokapi]
---

# AD-024: Agent Skills

## Summary

kapi bundles **Agent Skills** — `SKILL.md` definitions that teach an AI coding
assistant (Claude Code and compatible tools) how to use kapi to author, edit, and
keep content on-brand and terminologically consistent and to publish it
multilingually. A skill is a directory: a `SKILL.md` router plus
progressive-disclosure reference files that the assistant reads only when the
task calls for them. The skill source tree is the single source of truth in the
monorepo (`cli/skills/data`), kept in lockstep with the CLI command surface it
documents; it is distributed to assistants as a **Claude Code plugin** through
the `neokapi-plugins` marketplace — there is no `kapi skills` CLI command and the
binary does not carry the skill. The skills drive the kapi CLI directly (not the
MCP server), and rely on the CLI's exit-code contract ([AD-013](013-kapi-cli.md))
to distinguish a failed quality/brand gate from an operational error.

The default Claude + kapi model is symmetric across writing, editing, and
translating: **the assistant produces the content; kapi supplies the context
(brand guide, terms), enforces a faithful format round-trip, drift-checks the
result, and is the checker — no second model is called.** Two first-class loops
realize this: an **edit** loop for existing content (`kapi inspect` → assistant
edits → `kapi apply`) and a **create** loop for content authored from scratch
(author a generative format → `kapi inspect`/`kapi stats` parse it as the first
check → `kapi check`/`verify` gate → revise). An AI provider is the unattended
fallback only.

## Context

neokapi's positioning is to plug into an AI assistant so the assistant's output
stays on-brand and ships in other languages. The assistant already writes the
prose and the code; kapi supplies the guardrails (brand voice, terminology) and
the format round-tripping. The connective tissue is an Agent Skill: a unit of
instruction that tells the assistant *when* to reach for kapi and *which*
command to run, without bloating its base context.

Requirements:

- **Single source of truth, in lockstep with the CLI.** The skill names specific
  kapi commands and flags, so its source lives beside the code it documents
  (`cli/skills/data`) and changes in the same PR — a command change and its skill
  update are reviewed together. The plugin bundle and the in-repo dogfood are
  copies of that one tree; drift between them is a correctness bug.
- **Distributed and self-updating.** Users add the plugin once; Claude Code's
  plugin manager keeps it current, so the marketplace — not a manual re-install —
  is the update path.
- **Progressive disclosure.** A skill's router must stay small so it can sit in
  the assistant's context cheaply; deeper how-to detail loads on demand.
- **Agent-actionable, not architectural.** Skill content carries only what an
  agent needs to act — when to trigger, which command, what footgun to avoid —
  not the implementation or architecture detail that belongs in the framework
  docs.

## Decision

### What a skill is

A skill is a directory under `cli/skills/data/`:

```
data/<skill>/
├── SKILL.md            the router: YAML frontmatter (name, description) + a
│                       short body that decides scope and points at references
└── references/         progressive-disclosure how-to files loaded on demand
    └── *.md
```

The `SKILL.md` frontmatter supplies the `name` and `description`; the body
routes the assistant — judging whether a request is ad hoc or ongoing, then
pointing at the relevant reference file. The reference files carry the
task-specific detail (one per concern), so the router stays small and the
assistant pulls deeper instruction only when a task matches.

### Source of truth and distribution

The skill source is the directory tree under `cli/skills/data` — plain files,
not embedded in the binary, with no CLI command to install them. Three consumers
**copy** that one tree, so they never diverge:

- `make plugin-bundle` assembles the Claude Code plugin bundle
  (`packages/kapi-claude-plugin`) from the source;
- `make publish-plugin` mirrors the assembled bundle to the `neokapi-plugins`
  marketplace repo (`neokapi/claude-plugins`), published on each kapi release;
- `make dev-skills` copies it into the repo's own `.claude/skills` for
  dogfooding.

Because the source lives next to the CLI it documents, a command change and its
skill update land in one reviewed PR (lockstep). The marketplace repo is a
**generated distribution artifact**, like the Homebrew tap — never hand-edited.

### Distribution: the Claude Code plugin

There is no `kapi skills` user command; the binary neither carries nor installs
the skill. Distribution is the plugin: a marketplace
(`packages/kapi-claude-plugin/.claude-plugin/marketplace.json`, name
`neokapi-plugins`) hosting the `kapi` plugin — the skill plus two project-scoped,
fail-open hooks: a **Stop** hook that runs `kapi verify` and keeps the assistant
working until the gates are green, and a **PreToolUse** hook that blocks direct
hand-edits of files the project generates as translation targets. Users install
with `/plugin marketplace add neokapi/claude-plugins` then
`/plugin install kapi@neokapi-plugins`, and the plugin self-updates through
Claude Code. The publish cadence is **on kapi release**, so the published skill
always matches a shipped CLI surface — a Claude Code plugin can't pin a CLI
version, so a skill that referenced an unreleased command would break an
up-to-date plugin against an older binary.

### kapi-* and bowrain-* skills

The skill set is split by which surface a skill drives. **kapi-\*** skills drive
the local, offline `kapi` CLI — brand checks, terminology, localization,
internationalization, and the format-aware toolbox. **bowrain-\*** skills drive
the governed Bowrain platform (project sync, automation). The framework module
owns and embeds the kapi side; a bowrain skill is contributed by the bowrain
plugin, so the platform's how-tos ship with the platform rather than with the
open framework.

In the framework today the embedded set is a single `kapi` skill whose router
covers editing, creating, brand, terminology, localization, i18n, the toolbox,
and a project model, with one reference file per concern — edit (the read → edit →
write → verify loop), create (the author → parse → check loop), brand, localize
(translation and terminology), i18n, project, and the toolbox. Terminology folds
into the brand and localize references rather than a standalone file. The split is
the organizing principle for the surface a skill targets; the framework owns the
kapi skill source (`cli/skills/data`), and a bowrain skill is contributed by the
bowrain plugin.

### The edit and create loops — Claude writes, kapi checks

The two productive loops a skill drives are both **provider-free by default**: the
assistant is the writer, and kapi is the format engine and the checker. This is
the same asymmetry-correction translation already made (the assistant is a capable
translator, so kapi routes its output through the round-trip rather than calling a
separate model) applied to editing and creation.

- **Edit existing content.** `kapi inspect` is the read leg: it parses any
  editable format into one record per content block — the block's `text` with
  inline codes rendered as `<x id="…"/>` placeholders so an edit can round-trip,
  its structural role, a stable `id`, and a `content_hash` (the canonical block
  identity, [AD-003](003-identity.md)). The assistant rewrites the `text` and
  sends back a typed change-set; `kapi apply` writes it.
- **Create new content.** When there is no frozen source, the assistant authors a
  **generative** format (one whose writer can produce a document from the content
  model alone) and uses `kapi inspect` / `kapi stats` to parse it back as the
  first check, then `kapi check` / `kapi verify` as the brand-and-terminology
  gate, revising until green. Binary formats (`.docx`, `.pptx`) are editable but
  not generative — authored elsewhere, edited in place.

### `kapi apply` — the one write verb

Every deliberate, **reviewed** change the assistant proposes — a content edit *or*
an asset edit — is one typed JSONL entry, discriminated by `kind`, and lands
through a single command, `kapi apply` (the write sibling of `kapi inspect`):

| `kind` | What it edits | How it lands |
|---|---|---|
| `content` | a block's text in a named `file` | byte-faithful format round-trip, drift- and inline-code guarded |
| `term` | a glossary term | committed `.klftb` source → termbase import → `.kapi/termbase.db` |
| `tm` | a translation-memory pair | committed `.klftm` source → TM import → `.kapi/tm.db` |
| `brand` | a brand vocabulary rule | committed brand profile YAML → brand-store import ([AD-022](022-brand-voice.md)) |
| `recipe` | an allowlisted recipe field | the `.kapi` recipe, via project load/save |

Two properties make this one verb rather than five:

- **Asset edits write the committed source, then compile the cache.** An asset is
  edited in the git-tracked artifact the recipe binds (the `.klftb`/`.klftm`,
  the brand YAML, the recipe), and the *existing* importer refreshes the
  gitignored SQLite cache from it. The backing store is therefore written by
  exactly one path, `git diff` is the uniform review surface for every kind, and
  the operation is idempotent — an entry already in the desired state is a
  no-op, so re-running a partly-applied change-set is safe.
- **Content edits carry their own guards.** Each `content` entry pins a
  `content_hash`; if the block drifted since the assistant inspected it the edit
  is **stale** and skipped, and an edit that drops, invents, or unbalances an
  inline code is **rejected** by the fidelity guard rather than written as broken
  markup. Either outcome maps to the dedicated **gate** exit code (distinct from
  an operational error, per the exit-code contract below) so the fix loop
  re-inspects and retries.

`kapi apply` is the one write verb. It lands content edits, asset edits (a `term`
or `brand` rule), or a mix of both, and spans several files. kapi never sends
content to a model to rewrite it: the assistant rewrites the text and `kapi apply`
round-trips it back. A mixed change-set — a content fix and the `term` or `brand`
rule that justifies it — lands atomically, so the draft and the rule that governs
future drafts move together.

The MCP server exposes the same loop for non-CLI agents: `extract_content` (read
leg) and `apply_edits` (the typed change-set, [AD-022](022-brand-voice.md)).

### Format editability is declarative

A skill needs to know, before it edits, whether a format can be written back.
`kapi formats list` carries an **Edit** column and the JSON adds `editable` and
`round_trip`. A format is *editable* when it has a reader and a writer and is not
a bilingual interchange format — **including binary office formats**, because the
faithful round-trip is exactly what makes editing a binary container safe.
*RoundTrip* (`faithful` in the table) means the writer reconstructs from a
skeleton, so an edit changes only the edited text. Authoring from scratch is
gated on the separate `generative` flag; binary is edit-existing only. These flags
are resolved declaratively, without loading a plugin.

### CLI, not MCP

The skills drive kapi through its **CLI**, not the MCP server. The CLI is the
richer surface (it has the LLM-backed brand check, the credential store, the
project resolution, the full toolbox), and an AI coding assistant already runs
shell commands. The MCP brand/terminology/TM tools ([AD-022](022-brand-voice.md),
[AD-013](013-kapi-cli.md)) exist for parity with non-CLI agents (Cursor,
generic MCP clients); the bundled skills themselves use the CLI.

### Exit-code contract consumption

Because a skill issues CLI commands, it relies on the command exit code to
decide what to do next. The CLI exit-code contract ([AD-013](013-kapi-cli.md))
gives skills and CI a distinct signal for a failed quality/brand gate: a command
like `kapi brand check --min-score` returns the `ErrQualityGate` sentinel, which
maps to a dedicated gate exit code distinct from the generic operational-error
code. A skill (or a CI step) can therefore branch — "the draft scored below the
threshold, rewrite it" versus "the command itself failed" — without parsing
output. The grep-style `ErrSilentExit` used by the toolbox
([AD-023](023-toolbox-utilities.md)) is part of the same contract.

## Consequences

- A single source tree (`cli/skills/data`) feeds the plugin bundle and the
  in-repo dogfood by copy, so they never drift, and the source stays in lockstep
  with the CLI surface it documents (one PR changes the command and its skill).
- Distribution is a Claude Code plugin via the `neokapi-plugins` marketplace,
  self-updating through Claude Code; the binary no longer carries or installs the
  skill.
- The skill's `description` — the sole triggering lever, loaded at startup across
  every `SKILL.md`-aware tool — is tracked against a maintainer eval checklist
  (`cli/skills/EVALS.md`): positive prompts that must fire the skill and negatives
  that must not, re-run after any change to the description.
- Progressive disclosure keeps the router small and loads deeper detail only
  when a task matches, so the assistant's context stays cheap.
- Skills lean on the CLI's exit-code contract to act on quality gates
  programmatically, which keeps the skill content about *when and what to run*
  rather than about parsing results.
- The default attended loops (edit, create) call no AI provider: the assistant
  writes and kapi enforces the round-trip, drift-checks, and gates — so the same
  one write verb (`kapi apply`) covers content and asset edits and a mixed
  change-set lands atomically, with `git diff` as the uniform review surface.

## Related

- [AD-013: Kapi CLI](013-kapi-cli.md) — the command surface the skills drive and
  the exit-code contract they consume
- [AD-022: Brand Voice](022-brand-voice.md) — the brand commands a skill invokes,
  including the `--min-score` gate
- [AD-023: Toolbox Utilities](023-toolbox-utilities.md) — the toolbox a skill
  drives and its grep-style exit codes; `kapi apply` is the deliberate,
  reviewed-edit sibling of the toolbox's `ksed` regex substitution
- [AD-017: Bilingual Format Interop](017-bilingual-format-interop.md) — the
  bilingual `extract` / `merge` round-trip that `kapi inspect` / `kapi apply`
  mirror on the monolingual source side, and why `apply` and `merge` stay distinct
- [AD-003: Identity](003-identity.md) — the block `content_hash` used as the
  change-set drift anchor and block identity
