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
assistant (Claude Code and compatible tools) how to use kapi to keep generated
content on-brand and terminologically consistent and to publish it
multilingually. A skill is a directory: a `SKILL.md` router plus
progressive-disclosure reference files that the assistant reads only when the
task calls for them. Skills are embedded in the binary as the single source of
truth; `kapi skills` installs them into a `.claude/skills` directory at project
or user scope, offline and byte-identical across distribution paths. The skills
drive the kapi CLI directly (not the MCP server), and rely on the CLI's
exit-code contract ([AD-013](013-kapi-cli.md)) to distinguish a failed
quality/brand gate from an operational error.

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
covers brand, terminology, localization, i18n, the toolbox, and a project model,
with one reference file per concern — brand, localize (translation and
terminology), i18n, project, and the toolbox. Terminology folds into the
brand and localize references rather than a standalone file. The split is the organizing
principle for the surface a skill targets; the framework binary bundles the kapi
side, and `kapi skills list` reflects exactly what is embedded.

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

## Related

- [AD-013: Kapi CLI](013-kapi-cli.md) — the command surface the skills drive and
  the exit-code contract they consume
- [AD-022: Brand Voice](022-brand-voice.md) — the brand commands a skill invokes,
  including the `--min-score` gate
- [AD-023: Toolbox Utilities](023-toolbox-utilities.md) — the toolbox a skill
  drives and its grep-style exit codes
