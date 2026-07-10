---
name: refresh-i18n-framework
description: >-
  Audit one i18n framework in the kapi skill's registry against the Toil Index
  rubric — re-verify versions/stewardship on the web, re-score the five toil
  axes with citations, and update the registry + playbook — or add a brand-new
  framework/ecosystem to the skill. Use when the user asks to "refresh
  <framework> in the i18n skill", "re-score <framework>", "is our <framework>
  guidance current?", "add <framework> to the i18n skill", or "add <language>
  ecosystem support to the kapi skill". Run per-framework as ecosystems shift;
  the fleet-wide counterpart is the i18n-triage workflow.
---

# Refresh i18n Framework

Keep the kapi skill's i18n framework knowledge honest: one framework at a
time, re-verified on the web, re-scored against the Toil Index, and updated in
both the machine-readable registry and the human playbook. The rubric is
[`docs/internals/i18n-toil.md`](../../docs/internals/i18n-toil.md); the
research base is `strategy/i18n-skill/research-*.md` (sibling `strategy/`
checkout).

## When to use

- **One framework**: this skill — interactive audit or addition.
- **All frameworks at once**: trigger the `i18n-triage` workflow
  (`.claude/workflows/i18n-triage.js`) — sweeps every registry entry for
  drift, suppresses uncited score moves, and (mode=apply) lands the updates.
- **Building/adopting i18n in a user project**: that is the *kapi skill's*
  job (`cli/skills/data/kapi/references/i18n.md`), not this one.

## The artifacts (all must stay in agreement)

| Artifact | Role |
|---|---|
| `cli/skills/data/kapi/references/i18n/frameworks.yaml` | Registry: detection signals, catalog layout, kapi preset/flags, toil vectors (stock + recommended), grade, status, `buys`, watch watermarks |
| `cli/skills/data/kapi/references/i18n/<ecosystem>.md` | Playbook: grade table, opinionated defaults, per-framework Idiom / Recommended config / kapi / Footguns |
| `cli/skills/data/kapi/references/i18n.md` | Router: detection table + Toil Index summary |
| `docs/internals/i18n-toil.md` | The rubric (change-controlled — see Footguns) |
| `cli/skills/EVALS.md` | Trigger evals — extend when adding ecosystems |

## Mode A — refresh an existing framework

1. **Read the current claim.** The framework's registry entry and its
   playbook section. Note `watch.verified`, `watch.latest_verified`, the two
   toil vectors, grade, status, and `buys`.
2. **Re-verify on the web** (do not trust training data): latest stable
   version vs the watermark; maintenance signals (deprecation, abandonment,
   revival, stewardship/org moves, license changes); breaking changes touching
   the playbook (extraction tooling, catalog formats, package renames); new
   competing tools that would change the `buys` list or the ecosystem's
   recommended default.
3. **Re-score** the five axes (W/X/S/R/E, 0–3) for both vectors per rubric
   §2, applying the §3 rules (score the recommended configuration; source-as-
   key must earn S; no pseudo-loc path caps R at 1; "TMS without repo-side
   automation is S2"). Recompute the grade per §1. **Every changed axis needs
   a citation**; an uncited move is reverted — same sticky-anchor rule the
   triage workflow enforces.
4. **Check the kapi side is still true.** Preset ids against
   `kapi init --list-presets` / `core/preset/builtins.go`; format ids and
   `--format` flags against `core/formats/`; any kapi commands quoted in the
   playbook against the current CLI. A CLI change and its skill update land in
   one PR (AD-024 lockstep).
5. **Update** the registry entry (vectors, grade, status, buys, notes,
   `watch.latest_verified`, `watch.verified` = today) and the playbook section
   (grade table row, prose, setup steps). Keep them in exact agreement — the
   registry is the source of truth for numbers.
6. **Validate**:
   `node -e "require('./node_modules/js-yaml').load(require('fs').readFileSync('cli/skills/data/kapi/references/i18n/frameworks.yaml','utf8'))"`
   (run at the repo root), then grep the playbook grade table against the
   registry grades. If the
   change alters when the skill should *trigger* (rare), update the SKILL.md
   description and re-run the EVALS.md checklist.

## Mode B — add a new framework or ecosystem

1. **Research first** (web, cited): current version + cadence + stewardship;
   key model (source-as-key vs IDs); marking cost; extraction story; catalog
   format + layout; sync/churn behavior on rename/delete/source-edit;
   plural/ICU correctness; type safety; SSR/platform quirks; known community
   pain points; the tools that reduce each toil axis. Save the harvest as
   `strategy/i18n-skill/research-<topic>.md` if it's a whole new ecosystem.
2. **Score** stock and recommended vectors per the rubric; derive the grade;
   pick a status (`recommended` needs an argument versus the ecosystem's
   current default; `caution` for T3+ or E≥2; `avoid` for dormant/dead with a
   named migration path).
3. **Registry entry** — copy an existing entry as the template; every field
   is required. Detection signals must be real file/dependency markers an
   agent can check. `kapi.preset`/`kapi.flags`: only presets and format ids
   that exist in the CLI; if kapi lacks the catalog format entirely, say so in
   `notes` (that gap is itself a signal to the format roadmap — see how the
   `fluent` entry handles it).
4. **Playbook** — extend the matching `<ecosystem>.md`, or create a new one
   following `react.md`'s structure (grade table → opinionated defaults →
   per-framework sections → Verify). New file ⇒ add a routing row to
   `references/i18n.md`.
5. **Evals** — new ecosystem ⇒ add a positive trigger prompt to
   `cli/skills/EVALS.md` ("Add i18n to this <X> app") and re-run the
   checklist if the SKILL.md description changed.
6. **Validate** as in Mode A, and run `make dev-skills` so the in-repo
   dogfood copy picks the change up.

## Footguns

- **Change control** (rubric §3.7): a change that improves a framework's
  published grade may not edit the rubric or the triage workflow in the same
  change. Fix the reference, not the gate.
- **The formula is the grade.** Don't hand-pick a grade that "feels right" —
  recompute from the recommended vector per §1 (including the W=0 requirement
  for T0). If the formula feels wrong, that's a rubric issue to raise
  separately, not to route around.
- **Skill files ship standalone.** The plugin bundle copies only
  `cli/skills/data/kapi/` — playbooks must not reference repo paths outside
  the skill tree (URLs are fine). The rubric stays in `docs/internals/`; the
  playbooks carry only grades and the compact axis summary.
- **Don't let `recommended` vectors assume tools the playbook never tells the
  user to install.** Every improved axis needs its `buys` line and a matching
  setup step in the playbook.
- **kapi facts drift too.** kapi retires spellings outright rather than
  carrying aliases: `kapi presets list`/`presets show` are gone (use `kapi init
  --list-presets`) and so is `kapi verify` (use `kapi check --ship`). Re-check
  the CLI surface each audit, not just the ecosystem.

## References

- Rubric: [`docs/internals/i18n-toil.md`](../../docs/internals/i18n-toil.md)
- Registry + playbooks: `cli/skills/data/kapi/references/i18n/`
- Fleet sweep: `.claude/workflows/i18n-triage.js`
- Research base: `strategy/i18n-skill/research-*.md`
- Skill architecture + lockstep rule: `web/docs/contribute/architecture/024-agent-skills.md`
- Trigger evals: `cli/skills/EVALS.md`
