# Persona Evolution & Tuning

## Overview

The agentic testing system is not a "set and forget" deployment. Personas need ongoing
tuning as you learn what works, agents encounter new scenarios, and the Bowrain platform
evolves. This document describes the feedback loop for monitoring agent performance,
diagnosing problems, evolving SOUL.md files, and scaling the system across projects.

## The Tuning Loop

```
     ┌──────────┐
     │ Observe  │ ← Dashboard, activity feed, email threads, GitHub Issues
     └────┬─────┘
          │
     ┌────▼─────┐
     │ Diagnose │ ← Agent logs, LLM decision traces, quality metrics
     └────┬─────┘
          │
     ┌────▼─────┐
     │ Adjust   │ ← Edit SOUL.md, config.toml, HEARTBEAT.md
     └────┬─────┘
          │
     ┌────▼─────┐
     │ Deploy   │ ← Hot-reload (ZeroClaw picks up config changes)
     └────┬─────┘
          │
     ┌────▼─────┐
     │ Measure  │ ← Compare before/after metrics
     └──────────┘
          │
          └──→ (back to Observe)
```

The key insight: because agent behavior is defined in **markdown files** (SOUL.md) rather
than code, tuning is a writing task, not a programming task. You edit prose, not logic.

## Observation Layer

### What to Watch

| Signal              | Where to Find It                    | What It Tells You                                      |
| ------------------- | ----------------------------------- | ------------------------------------------------------ |
| Activity feed       | Bowrain dashboard                   | Are agents doing work? At the right times?             |
| Email threads       | Mailpit UI (localhost:8025)         | Are agents communicating usefully or spamming?         |
| GitHub Issues       | bowrain-l10n/feedback repo          | Are bug reports real? Are feature requests reasonable? |
| Translation quality | QA reports, LLM evaluations         | Is quality improving over time?                        |
| TM growth           | Bowrain TM explorer                 | Are agents building useful translation memory?         |
| Termbase            | Bowrain termbase                    | Is terminology growing organically?                    |
| Agent logs          | `docker compose logs {agent}`       | Are agents erroring, retrying, or stuck?               |
| Cost dashboard      | Azure Cost Management / API billing | Is spend within budget?                                |

### Daily Check (5 minutes)

```
1. Glance at Bowrain dashboard — are progress bars moving?
2. Check Mailpit — any interesting email threads?
3. Check `docker compose ps` — all agents running?
```

### Weekly Review (30 minutes)

```
1. Read PM's weekly status email (Lisa compiles this automatically)
2. Review GitHub Issues filed this week — genuine or noise?
3. Compare quality metrics week-over-week
4. Check cost trends
5. Note any patterns: agents stuck, repeating themselves, filing duplicates
```

## Diagnosis

### Common Problems and Root Causes

#### "Agent does nothing"

```
Symptom:  No activity from agent for 24h+
Check:    docker compose logs {agent} | tail -50
Causes:
  - Container crashed → docker compose ps shows "exited"
    Fix: docker compose up -d {agent}
  - Auth token expired → 401 errors in logs
    Fix: refresh Keycloak user, restart agent
  - Cron not firing → no log entries at scheduled times
    Fix: verify config.toml [daemon.cron] syntax
  - SOUL.md too vague → agent doesn't know what to do
    Fix: add more specific instructions (see "Sharpening Routines" below)
```

#### "Agent does too much / spams"

```
Symptom:  Agent files 20 GitHub Issues per day, sends 15 emails
Check:    GitHub Issues feed, Mailpit volume
Causes:
  - SOUL.md encourages filing issues for minor things
    Fix: raise the threshold — "only file issues for problems that block your work
         or affect multiple files"
  - Heartbeat too frequent → agent re-discovers same events
    Fix: increase heartbeat interval, verify "since" timestamp tracking
  - No duplicate detection → same issue filed repeatedly
    Fix: add "always search before filing" to SOUL.md
```

#### "Translation quality is low"

```
Symptom:  QA pass rate below 80%, many edits needed
Check:    QA weekly report, translator session logs
Causes:
  - AI acceptance rate too high → translator accepts bad translations
    Fix: lower threshold in SOUL.md ("accept only if you'd publish this unchanged")
  - Termbase too sparse → inconsistent term usage
    Fix: Brand Manager needs more aggressive term extraction
         Adjust Maria's SOUL.md: "extract at least 5 new terms per session"
  - Wrong model for task → Ollama producing poor translations locally
    Fix: switch to Azure Claude Sonnet for translation agents, keep GPT-4o-mini for Developer
```

#### "Agents talk past each other"

```
Symptom:  Translator emails Brand Manager about term, no response for days
Check:    Mailpit threads, agent schedules
Causes:
  - Schedule mismatch → Brand Manager works MWF, translator daily
    Fix: add "check email" to Brand Manager's heartbeat (not just cron sessions)
  - SOUL.md doesn't emphasize email responsiveness
    Fix: add "Always check email.listInbox at session start. Reply to any
         messages before starting your main work."
```

#### "GitHub Issues are noise"

```
Symptom:  Most filed issues are not real bugs, or are duplicates
Check:    Review issues, check agent decision logs
Causes:
  - Agents misinterpret normal behavior as bugs
    Fix: add known-behaviors section to SOUL.md:
         "These are NOT bugs: slow response after large push (indexing),
          empty activity feed before first push, 404 on new stream before push"
  - Duplicate detection not working
    Fix: strengthen search instructions: "Use github.searchIssues with at least
         3 keywords from your title. If any result matches, comment on existing
         issue instead of creating new."
```

## Adjustment Techniques

### Sharpening Routines

When an agent's behavior is too vague, add specificity to SOUL.md:

**Before (vague):**

```markdown
## Daily Routine

1. Check for new content
2. Translate assigned blocks
3. Report any issues
```

**After (sharp):**

```markdown
## Daily Routine

1. Check `email.listInbox` — reply to any messages first
2. Check `bowrain.listTasks` (assignee: me, status: open, sort: priority)
   - If no tasks: email PM "No tasks assigned, available for work"
   - If tasks exist: process up to 30 blocks
3. For EACH block:
   a. Check `bowrain.listConcepts` for any terms in the source text
   b. Check `bowrain.listTMEntries` — if >90% match, use TM
   c. Get AI suggestion via `bowrain.aiTranslate`
   d. Compare AI vs TM vs termbase — decide: accept, edit, reject
   e. Submit via `bowrain.translate`
   f. If edited with high confidence → `bowrain.addTMEntry`
4. After batch: `bowrain.updateTask` with progress
5. If ANY block has ambiguous source → email Brand Manager (don't guess)
6. If ANY tool returns an error → file GitHub Issue with tool name + params
```

### Adjusting Personality

Personality affects decision patterns. Tune via SOUL.md:

```markdown
## Your Translation Philosophy

# More conservative (for production-quality language pairs):

You are meticulous. You accept AI translations only when they are publication-ready.
You edit roughly 40% and reject 15%. You always verify against the termbase.
You add TM entries only for translations you're proud of.

# More aggressive (for rapid-coverage language pairs):

You move fast. Accept AI translations unless they have obvious errors.
Edit only when meaning is wrong or terms are inconsistent.
Add TM entries for all accepted translations to build memory quickly.
```

### Adding Situational Behaviors

When you notice agents don't handle a scenario, add it explicitly:

```markdown
## Situations to Handle

When upstream renames a feature (e.g., "Plugins" → "Extensions"):
→ Check termbase for the old term
→ Email Brand Manager: "Upstream renamed '{old}' to '{new}' — should I update translations?"
→ Wait for Brand Manager response before changing existing translations
→ New blocks with the new term: translate using the new term immediately

When you encounter a very long block (>500 words):
→ Translate in logical segments (paragraph by paragraph)
→ Pay extra attention to internal consistency
→ Add a TM entry only for the best segment, not the whole block

When the same source text appears in multiple files:
→ Translate it once, add to TM
→ Use TM match for subsequent occurrences
→ Verify context is the same (same term can mean different things in different files)
```

### Evolving the Team

As the system matures, evolve the team structure:

**Adding a specialist:**

```
Problem: Japanese translations are slow because Yuki handles both technical and UI content
Solution: Add a second Japanese translator — Kenji (UI specialist)
  1. Create agents/kenji-ja-ui/ workspace
  2. Write SOUL.md with UI focus (concise, button labels, menu items)
  3. Adjust Yuki's SOUL.md: "You focus on documentation and long-form content.
     Kenji handles UI strings and short content."
  4. PM assigns tasks accordingly
  5. docker compose up -d kenji-ja-ui
```

**Retiring a language:**

```
Problem: Portuguese (pt-BR) not needed anymore
Solution: Stop the translator container
  1. docker compose stop carlos-pt
  2. PM's SOUL.md updated: remove pt-BR from language list
  3. Keep workspace files for history
```

**Cross-project transfer:**

```
Problem: Gitea needs more attention, Excalidraw is mostly complete
Solution: Adjust SOUL.md to shift focus
  1. Edit translator SOUL.md: "Primary project: Gitea. Secondary: Excalidraw (maintenance only)"
  2. PM creates tasks accordingly
  3. No container changes needed — same agent, different focus
```

## Metrics-Driven Evolution

### Tracking Persona Effectiveness

Maintain a simple spreadsheet or dashboard tracking per-agent metrics over time:

```
┌─────────────────┬────────┬────────┬────────┬────────┐
│ Agent           │ Week 1 │ Week 4 │ Week 8 │ Week 12│
├─────────────────┼────────┼────────┼────────┼────────┤
│ JP (fr-FR)      │        │        │        │        │
│   Blocks/day    │ 15     │ 22     │ 28     │ 30     │
│   QA pass rate  │ 72%    │ 81%    │ 88%    │ 92%    │
│   TM entries    │ 12     │ 45     │ 120    │ 210    │
│   AI accept %   │ 70%    │ 58%    │ 52%    │ 48%    │
│   Issues filed  │ 5      │ 3      │ 1      │ 1      │
├─────────────────┼────────┼────────┼────────┼────────┤
│ Maria (Brand)   │        │        │        │        │
│   Terms added   │ 25     │ 15     │ 8      │ 5      │
│   Compliance %  │ 78%    │ 85%    │ 91%    │ 94%    │
│   Brand checks  │ 4      │ 8      │ 12     │ 12     │
├─────────────────┼────────┼────────┼────────┼────────┤
│ Alex (Dev)      │        │        │        │        │
│   Push success  │ 85%    │ 95%    │ 98%    │ 99%    │
│   Pull success  │ 90%    │ 96%    │ 99%    │ 100%   │
│   Issues filed  │ 8      │ 4      │ 2      │ 1      │
└─────────────────┴────────┴────────┴────────┴────────┘

Reading: JP's quality improves as TM grows (fewer AI translations accepted
blindly). AI accept rate DROPS as the translator gets more critical — this
is good, it means quality is going up. Maria adds fewer terms over time
because the termbase is maturing. Alex files fewer issues as platform
stabilizes.
```

### When to Tune

| Signal                       | Action                                                            |
| ---------------------------- | ----------------------------------------------------------------- |
| QA pass rate drops below 80% | Tighten translator SOUL.md, add term review step                  |
| Agent files >5 issues/week   | Raise filing threshold in SOUL.md                                 |
| Agent files 0 issues/week    | Lower threshold or add "proactive testing" routine                |
| TM reuse rate plateaus       | Encourage more TM contributions in translator SOUL.md             |
| Brand compliance drops       | Brand Manager needs more frequent audit schedule                  |
| Email volume > 20/day total  | Reduce email triggers, move more to Bowrain tasks                 |
| Email volume < 2/day total   | Agents aren't coordinating; add more email touchpoints            |
| Translator blocked >24h      | PM needs faster turnaround; adjust PM schedule                    |
| Cost trending up             | Switch simple agents to cheaper model, reduce heartbeat frequency |

### A/B Testing Personas

For subtle tuning, run two variants of the same role:

```
Experiment: Does a stricter translator produce better quality?

agents/jeanpierre-fr/SOUL.md (variant A — current):
  "Accept about 60% of AI translations as-is"

agents/jeanpierre-fr-strict/SOUL.md (variant B):
  "Accept only translations you'd publish unchanged. Edit anything imperfect."

Run both on different projects for 2 weeks. Compare:
  - QA pass rate (B should be higher)
  - Throughput (B should be lower)
  - TM quality (B entries should have fewer corrections later)
  - Cost (B uses more LLM tokens per block)

Pick the variant that best balances quality vs. throughput.
```

## Version Control for Personas

SOUL.md files are checked into git alongside the rest of the agentic system. This means:

- **Full history** of persona evolution via `git log agents/jeanpierre-fr/SOUL.md`
- **Diff-based review** — see exactly what changed and why
- **Rollback** — if a SOUL.md change makes quality worse, revert it
- **Branching** — experiment with persona variants on a git branch

### Commit Convention

```
persona(jeanpierre): tighten AI acceptance threshold from 60% to 45%

QA pass rate dropped to 76% after Week 6. Translator was accepting too many
mediocre AI translations. Tightened acceptance criteria in SOUL.md and added
explicit termbase verification step before accepting.

Metrics to watch: QA pass rate (target >85%), throughput (acceptable drop to 22 blocks/day)
```

## Scaling Across Projects

### Shared vs. Project-Specific Personas

When the same agent works across multiple projects, their SOUL.md contains both
shared behavior and project-specific sections:

```markdown
## General Translation Guidelines

(same for all projects — accuracy, fluency, terminology, formatting)

## Project-Specific Notes

### Docusaurus

- Heavy use of MDX — preserve JSX interpolation
- Audience: developers reading documentation
- Tone: technical but approachable
- Key terms: "sidebar", "plugin", "preset", "deployment"

### Gitea

- INI format — watch for escaped characters and line continuations
- Audience: sysadmins and developers
- Tone: neutral, professional
- Key terms: "repository", "pull request", "issue", "milestone"

### Home Assistant

- Deeply nested JSON — maintain structural hierarchy
- Audience: home automation enthusiasts (not all technical)
- Tone: friendly, helpful, accessible
- Key terms: "automation", "entity", "integration", "dashboard"
```

### When to Split Agents

If a single agent handles too many projects and quality suffers, split:

```
Before:
  jeanpierre-fr handles Docusaurus + Gitea + Home Assistant

After:
  jeanpierre-fr handles Docusaurus + Excalidraw (technical docs)
  sophie-fr handles Gitea + Home Assistant (UI-heavy content)

Sophie's SOUL.md is cloned from Jean-Pierre's with adjustments:
- Faster acceptance rate (UI strings are simpler)
- Focus on character limits (UI has space constraints)
- Different personality (more concise communication style)
```

### Template Personas

Create reusable templates for common roles that get customized per instantiation:

```
templates/
├── translator-template.md     # Base translator SOUL.md
├── brand-manager-template.md  # Base brand manager SOUL.md
├── developer-template.md      # Base developer SOUL.md
├── pm-template.md             # Base PM SOUL.md
└── qa-template.md             # Base QA SOUL.md
```

New agent = copy template → fill in: name, language, personality, project-specific notes.
