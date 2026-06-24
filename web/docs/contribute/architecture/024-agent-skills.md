---
id: 024-agent-skills
sidebar_position: 24
title: "AD-024: Agent Skills"
description: "Architecture decision: kapi bundles Agent Skills (SKILL.md routers with progressive-disclosure reference files) embedded in the binary, installed into .claude/skills at project or user scope, that teach an AI coding assistant to drive the kapi CLI for brand, terminology, localization, and the toolbox."
keywords: [agent skills, SKILL.md, Claude, .claude/skills, progressive disclosure, AI assistant, CLI, architecture decision, neokapi]
---

# AD-024: Agent Skills

## Summary

kapi bundles **Agent Skills** — `SKILL.md` definitions that teach an AI coding
assistant (Claude Code and compatible tools) how to use kapi to author, edit, and
keep content on-brand and terminologically consistent and to publish it
multilingually. A skill is a directory: a `SKILL.md` router plus
progressive-disclosure reference files that the assistant reads only when the
task calls for them. Skills are embedded in the binary as the single source of
truth; `kapi skills` installs them into a `.claude/skills` directory at project
or user scope, offline and byte-identical across distribution paths. The skills
drive the kapi CLI directly (not the MCP server), and rely on the CLI's
exit-code contract ([AD-013](013-kapi-cli.md)) to distinguish a failed
quality/brand gate from an operational error.

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

- **Single source of truth, many install targets.** The same skill content must
  install via the `kapi` CLI, ship in the Claude Code plugin bundle, and seed
  the in-repo dogfood — all byte-identical. Drift between copies is a
  correctness bug.
- **Offline and self-contained.** Installing a skill must not require a network
  fetch; the binary already contains everything.
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

### Embedding and the single source of truth

The skill tree is embedded into the binary with `//go:embed all:data` in the
`cli/skills` package. Embedding makes the binary the canonical source: there is
no separate skills directory to keep in sync, install works offline, and every
distribution path emits identical bytes. `skills.List` enumerates the embedded
skills (name and description from frontmatter); `skills.InstallTo` copies a
skill's full directory tree — router plus references — preserving structure.

The same embedded source feeds every path:

- `kapi skills install` writes into `.claude/skills`;
- `make plugin-bundle` runs `kapi skills export --dir …` to populate the Claude
  Code plugin bundle;
- `make dev-skills` installs into the repo's own `.claude/skills` for
  dogfooding.

Because all three read the one embedded tree, the bundle, the user install, and
the dogfood copy never diverge.

### The `kapi skills` command tree

`NewSkillsCmd` (`cli/skills_cmd.go`) builds the `kapi skills` group:

| Command | Purpose |
|---|---|
| `list` | List the bundled skills (name + description). |
| `install [names…]` | Install skills into `.claude/skills`; install a subset by name, or all when none are named. |
| `uninstall <names…>` | Remove installed skills from `.claude/skills`. |

Install and uninstall take a `--target` selecting the scope:

- `--target project` (default) → `./.claude/skills/<name>/`
- `--target user` → `~/.claude/skills/<name>/`

Project scope keeps a skill local to one repository (committable, shared with a
team); user scope makes it available across every project for that user. A
hidden `export --dir` writes every skill to an arbitrary directory and backs the
plugin-bundle build.

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
the organizing principle for the surface a skill targets; the framework binary
bundles the kapi side, and `kapi skills list` reflects exactly what is embedded.

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

- A single embedded skill tree feeds the CLI install, the Claude Code plugin
  bundle, and the in-repo dogfood byte-identically, so the three never drift.
- Installation is offline and scope-aware: a skill can be committed to a project
  or installed once per user.
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
