# Agent Routines & Communication

## Overview

Each agent persona follows structured daily/weekly routines that generate authentic
platform activity. Beyond Bowrain workflows, agents communicate via **email** (Mailpit
locally, real SMTP in Azure) and file **GitHub Issues** for bug reports and feature
requests — exercising the full surface area of how real teams interact around
localization.

## Communication Channels

### 1. Bowrain Platform (Primary)

Tasks, activity feed, translation editor, brand dashboard — as already defined.
This is where the core localization work happens.

### 2. Email (Mailpit / SMTP)

Agents send and receive email for communication that happens _outside_ the
platform — the kind of messages real team members exchange:

| Email Type           | From          | To              | Example                                                       |
| -------------------- | ------------- | --------------- | ------------------------------------------------------------- |
| Weekly status digest | PM            | All             | "Week 12 summary: 78% fr-FR, 62% de-DE, 41% ja-JP"            |
| Terminology proposal | Brand Manager | Translators     | "Proposing 'déploiement' as preferred term for 'deployment'"  |
| Release coordination | Developer     | PM              | "Upstream v3.2.1 merged, 47 new blocks pushed"                |
| Quality concern      | QA            | Translator + PM | "3 placeholder mismatches in fr-FR release-notes.md"          |
| Escalation           | Translator    | Brand Manager   | "Need guidance: 'serverless' — transliterate or translate?"   |
| Welcome/onboarding   | PM            | New translator  | "Welcome Yuki! Here's your first batch of ja-JP translations" |

Agents never touch Mailpit directly. They call `email.send` and `email.listInbox` MCP
tools on the standalone email MCP server (`agentic/email-mcp/`), which connects to Mailpit
over the compose network (`mailpit:1025` for SMTP, `mailpit:8025` for the inbox API).
This is separate from the Bowrain MCP server — email is agentic testing infrastructure,
not a Bowrain platform feature.

**Local dev:** Mailpit catches all email inside the compose network. The operator can
browse captured emails at `http://localhost:8025` (port-forwarded to host).
No real email leaves the machine.

**Azure:** Replace Mailpit with Azure Communication Services or SendGrid. The MCP
server's `SMTP_HOST` / `SMTP_PORT` env vars point to the real SMTP endpoint.
Agent SOUL.md files stay identical — they only know about MCP tools, not SMTP details.

### 3. GitHub Issues (Bug Reports & Feature Requests)

Agents file GitHub Issues against the Bowrain platform repo when they encounter
problems or identify missing features. This creates a realistic feedback loop and
generates authentic issue history.

**Issue types agents file:**

| Category                 | Filed By       | Example                                                     |
| ------------------------ | -------------- | ----------------------------------------------------------- |
| **Bug: CLI**             | Developer      | "kapi push fails with BOM-encoded JSON files"            |
| **Bug: API**             | Translator, PM | "listTasks returns 500 when filtering by assignee + status" |
| **Bug: Web UI**          | Brand Manager  | "Brand profile tone slider resets to 0 after save"          |
| **Bug: Format**          | Developer, QA  | "Markdown reader strips frontmatter during round-trip"      |
| **Feature: CLI**         | Developer      | "Add --dry-run flag to kapi push"                        |
| **Feature: API**         | PM             | "Batch task creation endpoint (create N tasks in one call)" |
| **Feature: Translation** | Translator     | "Show TM match percentage in task list view"                |
| **Feature: Brand**       | Brand Manager  | "Export brand profile as shareable template"                |
| **Improvement: DX**      | Developer      | "kapi status should show per-language completion %"      |
| **Improvement: UX**      | Translator     | "Keyboard shortcut to accept AI translation and advance"    |

**Labels:** Agents apply appropriate labels (`bug`, `enhancement`, `ux`, `cli`, `api`, `format`).

**Important:** Issues are filed against a dedicated `bowrain-l10n/feedback` repo (not the
main neokapi repo) to keep agent-generated issues separate from real development. The
human operator reviews and triages — promoting genuine findings to the real issue tracker.

## Detailed Agent Routines

### Developer Agent (Alex Chen)

#### Daily Routine (Weekdays)

```
09:00 ── Morning Push Session ──────────────────────────────────────
│
├─ 1. Check upstream repos for new releases
│     Tool: git.checkUpstream for each project
│     If no changes → log "no upstream changes" → skip to step 5
│
├─ 2. Merge upstream changes
│     Tool: git.merge
│     If merge conflict → file GitHub Issue (label: needs-attention)
│                       → email PM: "Merge conflict on {project} {tag}"
│                       → skip this project
│
├─ 3. Push new content to Bowrain
│     Tool: bowrain.push
│     If push fails → file GitHub Issue (label: bug, cli)
│                   → retry once after 5 min
│     Log: "{N} blocks pushed for {project} {tag}"
│
├─ 4. Create stream if major version
│     Decision: LLM evaluates if tag warrants a new stream
│     Tool: bowrain.createStream (if yes)
│     Email PM: "New content pushed for {project} {tag}, stream created"
│
└─ 5. Check activity feed for completed translations
      Tool: bowrain.listActivities (since: last session)
      If QA passed events found → proceed to evening pull

17:00 ── Evening Pull Session ──────────────────────────────────────
│
├─ 1. Check which languages have completed QA
│     Tool: bowrain.listActivities
│
├─ 2. Pull translations for completed languages
│     Tool: bowrain.pull (per locale)
│     If pull fails → file GitHub Issue (label: bug, cli)
│
├─ 3. Commit and push to fork
│     Tool: git.commit, git.push
│     Commit message: "l10n({locale}): pull translations for {project} {tag}"
│
└─ 4. Verify round-trip integrity
      Compare pulled files against expected format
      If format corruption → file GitHub Issue (label: bug, format)
                           → email QA: "Round-trip integrity issue in {file}"
```

#### Weekly Routine (Friday)

```
├─ 1. Review CI pipeline health
│     Check GitHub Actions for bowrain-action failures this week
│     If recurring failures → file GitHub Issue with pattern analysis
│
├─ 2. Review stream inventory
│     Tool: bowrain.listStreams per project
│     Close completed streams, note orphaned ones
│
└─ 3. Email PM: "Weekly DevOps summary"
      Content: projects synced, blocks pushed/pulled, CI status, issues filed
```

---

### Brand Manager Agent (Maria Santos)

#### Mon/Wed/Fri Routine

```
10:00 ── Terminology Review Session ────────────────────────────────
│
├─ 1. Check activity feed for recent pushes
│     Tool: bowrain.listActivities (type: content_push, since: last session)
│     If no new content → skip to step 4
│
├─ 2. Analyze new content for terminology candidates
│     LLM reviews pushed blocks for:
│     - New technical terms not in termbase
│     - Terms used inconsistently across files
│     - Brand-sensitive vocabulary (product names, features)
│
├─ 3. Add/update terminology
│     Tool: bowrain.addConcept for each new term
│     Include: definition, domain, preferred status, usage notes
│     Email translators: "New terms added: {term_list}"
│
├─ 4. Run brand compliance check on recent translations
│     Tool: bowrain.checkBrand on recently translated files
│     If violations found:
│       Tool: bowrain.createTask (assign to translator, priority: high)
│       Email translator: "Brand compliance issue in {file} ({locale})"
│
└─ 5. Review translator terminology questions (from email)
      If escalation emails received → research and respond
      Update termbase if decision made
```

#### Thursday Routine (Brand Audit)

```
10:00 ── Weekly Brand Audit ────────────────────────────────────────
│
├─ 1. Comprehensive compliance scan across all projects
│     Tool: bowrain.checkBrand for each project × each locale
│     Generate compliance score matrix
│
├─ 2. Review brand profile evolution
│     Are current profiles still appropriate?
│     Any upstream project changes that affect brand voice?
│     If profile update needed → Tool: bowrain.updateBrandProfile
│
├─ 3. Audit term coverage
│     Compare active terms vs. terms appearing in content
│     Identify gaps (terms in content but not in termbase)
│     File feature request if termbase tooling could be improved
│
└─ 4. Email PM: "Weekly brand health report"
      Content: compliance scores per project/locale, new terms, violations
```

#### Reactive Behaviors

```
On email from translator ("what's the right term for X?"):
  → Research term in context
  → Respond with recommendation + reasoning
  → Add to termbase if not already present

On activity "new project added":
  → Create brand profile from starter pack
  → Review first batch of content for terminology
  → Email PM: "Brand profile created for {project}"

On pattern of repeated brand violations:
  → File GitHub Issue (label: enhancement, ux)
    "Brand compliance warnings should be shown inline in the translation editor"
```

---

### Translator Agent (Jean-Pierre Dubois, fr-FR)

#### Daily Routine (Weekdays, 14:00-18:00)

```
14:00 ── Translation Session ───────────────────────────────────────
│
├─ 1. Check email for messages from Brand Manager or PM
│     Terminology updates? Note and apply during translation
│     Priority changes? Adjust task order
│
├─ 2. Get assigned tasks
│     Tool: bowrain.listTasks (assignee: me, status: open)
│     Sort by priority → deadline
│     If no tasks → check activity feed for unassigned work
│                  → email PM: "No tasks assigned, available for work"
│
├─ 3. For each task (up to blocks_per_session):
│     a. Get AI translation suggestion
│        Tool: bowrain.aiTranslate
│
│     b. Look up relevant terminology
│        Tool: bowrain.listConcepts (filter by terms in source)
│
│     c. Look up translation memory
│        Tool: bowrain.listTMEntries (source text, en→fr)
│        If high TM match (>90%) → use TM translation, skip AI review
│
│     d. Review AI translation (LLM decision)
│        Accept (confidence > 0.8, no term issues)
│        Edit (confidence 0.5-0.8, minor fixes needed)
│        Reject (confidence < 0.5, fundamentally wrong)
│        Tool: bowrain.translate with final text
│
│     e. Add to TM if high quality
│        If accepted or edited with high confidence:
│        Tool: bowrain.addTMEntry
│
│     f. Update task progress
│        Tool: bowrain.updateTask
│
├─ 4. Handle problem translations
│     If source text is ambiguous:
│       Email Brand Manager: "Need clarification on '{source_text}'"
│       Skip block, continue with next
│     If no termbase entry for domain term:
│       Email Brand Manager: "Missing term: '{term}' — suggest '{translation}'"
│
└─ 5. End-of-session update
      Tool: bowrain.updateTask (mark completed or note progress)
      If blocked on anything → email PM with details
```

#### Weekly Routine (Friday)

```
├─ 1. Review my translation quality
│     Check QA results from this week
│     Note recurring issues (terminology, formatting, tone)
│
├─ 2. TM contribution review
│     How many TM entries did I add this week?
│     Are they being reused? (check bowrain.listTMEntries)
│
└─ 3. Email PM: "Weekly translation summary for fr-FR"
      Content: blocks translated, QA issues, TM contributions, blockers
```

#### Reactive Behaviors

```
On email from Brand Manager ("new preferred term: X → Y"):
  → Acknowledge via email
  → In next session, check existing translations for old term
  → Update affected blocks

On email from QA ("placeholder mismatch in {file}"):
  → Fix immediately in next session (priority override)
  → Respond to QA via email confirming fix

On encountering a UI issue in Bowrain:
  → File GitHub Issue (label: bug, ux)
    e.g., "Translation editor loses cursor position after auto-save"
  → Continue working (don't block on UI issues)
```

---

### Project Manager Agent (Lisa Chen)

#### Daily Routine (Weekdays, 08:00-10:00)

```
08:00 ── Morning Dashboard Review ─────────────────────────────────
│
├─ 1. Check activity feed (overnight + yesterday)
│     Tool: bowrain.listActivities (since: last session)
│     Build mental model: who did what, what's pending
│
├─ 2. Review task board
│     Tool: bowrain.listTasks (all statuses)
│     Identify: overdue tasks, unassigned work, blocked items
│
├─ 3. Create tasks for new content
│     If Developer pushed since last check:
│       Calculate work per language (new blocks × estimated time)
│       Tool: bowrain.createTask per translator per file batch
│       Set priority based on: release urgency, file type, word count
│       Email translators: "New tasks assigned for {project} {tag}"
│
├─ 4. Handle escalations
│     Check email for translator blockers, QA escalations
│     Reassign tasks if translator is overloaded
│     Adjust deadlines if upstream schedule changed
│
├─ 5. Monitor quality
│     Check QA pass rates per language
│     If quality dropping:
│       Email translator + Brand Manager: "Quality concern in {locale}"
│       Consider adjusting translator workload
│
└─ 6. If project milestone approaching:
      Email Developer: "Release {tag} translation ETA: {date}"
      Email all: "Push for {project} completion by {deadline}"
```

#### Weekly Routine (Friday → Monday)

```
Friday 09:00 ── Weekly Status Report ───────────────────────────────
│
├─ 1. Compile metrics across all projects
│     Translation progress per locale (% complete)
│     Velocity (blocks/day per translator)
│     Quality (QA pass rate, brand compliance score)
│     TM growth (entries added, reuse rate)
│     Cost (estimated AI spend this week)
│
├─ 2. Send weekly status email to all team members
│     Subject: "L10n Weekly: {date_range}"
│     Body: progress table, highlights, blockers, next week priorities
│
├─ 3. Review and triage GitHub Issues filed by agents this week
│     Comment on issues with priority assessment
│     Close duplicates
│     Label issues that need human attention: "needs-triage"
│
└─ 4. Plan next week
      Identify which projects need attention
      Pre-create tasks for known upcoming content
      Adjust translator assignments if workload uneven
```

#### Reactive Behaviors

```
On email from Developer ("merge conflict on {project}"):
  → Assess impact: how much content is blocked?
  → Email Developer with priority guidance
  → If critical: create high-priority task for manual resolution

On email from Translator ("no tasks assigned"):
  → Check if there's unassigned work
  → If yes: create and assign tasks immediately
  → If no: email back "All caught up! Great work this week"

On stalled project (no activity for 48h):
  → Email assigned translators: "Checking in on {project}"
  → Review if technical issue (check Developer logs)
  → File GitHub Issue if platform problem suspected
```

---

### QA Agent (Taylor Kim)

#### Routine (Heartbeat-Driven, Every 2 Hours)

```
Heartbeat ── Quality Check Cycle ───────────────────────────────────
│
├─ 1. Discover completed translation batches
│     Tool: bowrain.listActivities (type: translation_complete, since: last_check)
│     If nothing new → exit
│
├─ 2. Run automated QA checks
│     For each completed batch:
│       a. Placeholder consistency (critical)
│          Verify all {variables}, %s, {{tokens}} preserved
│       b. Format validation
│          Verify translated file parses in original format
│       c. Terminology compliance
│          Tool: bowrain.listConcepts → verify usage
│       d. Brand compliance
│          Tool: bowrain.checkBrand on translated content
│       e. Whitespace and punctuation
│          Locale-specific rules (French spacing, German capitalization)
│       f. Empty translations
│          Flag any blocks with no target text
│
├─ 3. Categorize and report issues
│     Critical (placeholder, format): Create task immediately
│       Tool: bowrain.createTask (assign to translator, priority: urgent)
│       Email translator: "URGENT: {issue} in {file} ({locale})"
│     High (terminology, brand): Create task
│       Tool: bowrain.createTask (priority: high)
│     Medium (whitespace, punctuation): Create task
│       Tool: bowrain.createTask (priority: medium)
│     Low (stylistic): Note for weekly report
│
├─ 4. Verify previously reported issues
│     Check if issues from prior cycles were fixed
│     If fixed: update task status → email translator "Confirmed fixed"
│     If recurring: escalate to PM
│       Email PM: "Recurring QA issue: {pattern}"
│
└─ 5. Platform health checks
      If QA tools behave unexpectedly:
        File GitHub Issue (label: bug, api)
        e.g., "bowrain.checkBrand returns 500 for files > 100 blocks"
      Track response times — if degrading:
        File GitHub Issue (label: performance)
```

#### Weekly Routine (Monday)

```
├─ 1. Generate weekly quality report
│     QA pass rate per locale per project
│     Most common issue types
│     Translators with highest/lowest pass rates
│     Trend: improving or degrading?
│
├─ 2. Email PM + all: "Weekly QA Report"
│     Content: metrics table, top issues, recommendations
│
└─ 3. File feature requests based on patterns
      If same issue type keeps recurring:
        File GitHub Issue (label: enhancement)
        e.g., "Add pre-translation placeholder validation to prevent issues"
      If QA workflow is manual when it could be automated:
        File GitHub Issue (label: enhancement, automation)
```

---

## GitHub Issues — via `gh` CLI

Agents file GitHub Issues using the `gh` CLI directly — not through the Bowrain MCP
server. GitHub Issues are agentic testing infrastructure, not a Bowrain platform feature.

ZeroClaw's `allowed_commands` grants access to `gh`. Agents call it as a shell command:

```bash
# Search before filing (dedup)
gh issue list --repo neokapi/agent-feedback --search "BOM-encoded JSON" --json number,title

# File a bug report
gh issue create --repo neokapi/agent-feedback \
  --title "[Bug] cli: kapi push fails with BOM-encoded JSON files" \
  --body "..." \
  --label bug,cli

# Comment on existing issue
gh issue comment 42 --repo neokapi/agent-feedback --body "Still reproducing on v3.2.1"
```

**Config:** `GITHUB_TOKEN` env var passed to agent containers. Only agents with `gh` in
their `allowed_commands` can file issues (Developer, QA, PM by default).

### Issue Guidelines in SOUL.md

Each agent's SOUL.md includes guidance on when and how to file issues:

```markdown
## Filing GitHub Issues

When you encounter a platform problem or have an improvement idea, file a GitHub Issue
using the `gh` CLI against the `neokapi/agent-feedback` repo.

**Before filing:**

- Search existing issues: `gh issue list --repo neokapi/agent-feedback --search "keywords"`
- Only file if the issue is reproducible or the improvement is clearly valuable

**Bug report format:**
gh issue create --repo neokapi/agent-feedback \
 --title "[Bug] {component}: {short description}" \
 --body "**What I was doing:** ...\n**What happened:** ...\n**Steps:** ...\n**Error:** ..." \
 --label bug,{component}

**Feature request format:**
gh issue create --repo neokapi/agent-feedback \
 --title "[Feature] {component}: {short description}" \
 --body "**Goal:** ...\n**Current gap:** ...\n**Suggestion:** ..." \
 --label enhancement,{component}
```

---

## Email — Standalone MCP Server

Email is handled by a lightweight standalone MCP server in `agentic/email-mcp/` — not
part of the Bowrain MCP server. This keeps Bowrain clean (platform tools only) while
giving agents email capability.

Agents connect to it as a second MCP endpoint in their `config.toml`:

```toml
[mcp.email]
transport = "http"
url = "http://email-mcp:3001/mcp"
```

### Tools

| Tool              | Used By | Description                                     |
| ----------------- | ------- | ----------------------------------------------- |
| `email.send`      | All     | Send email (to, subject, body) via Mailpit SMTP |
| `email.listInbox` | All     | Query received emails via Mailpit API           |

### Email Patterns in SOUL.md

```markdown
## Email Communication

You communicate with team members via email for coordination that doesn't
belong in the Bowrain task system:

- Status updates and summaries (weekly reports)
- Questions and escalations
- Terminology discussions
- Release coordination

Use `email.send` with the recipient's role:

- "pm" → Lisa Chen
- "brand-manager" → Maria Santos
- "developer" → Alex Chen
- "translator-fr" → Jean-Pierre Dubois
- "translator-de" → Katrin Weber
- "translator-ja" → Yuki Tanaka
- "qa" → Taylor Kim
- "all" → everyone

**Check your inbox** at the start of each session with `email.listInbox`.
Respond to messages that need a reply before starting your main work.
```

---

## Docker Compose Additions

```yaml
# Add to docker-compose.yaml

# === Email ===
mailpit:
  image: axllent/mailpit:latest
  ports:
    - "8025:8025" # Web UI for operator (browse captured emails)
  environment:
    MP_SMTP_AUTH_ACCEPT_ANY: 1
    MP_SMTP_AUTH_ALLOW_INSECURE: 1

# Standalone email MCP server (wraps Mailpit SMTP + API)
email-mcp:
  build: ./agentic/email-mcp
  environment:
    SMTP_HOST: mailpit
    SMTP_PORT: 1025
    MAILPIT_API_HOST: mailpit
  depends_on: [mailpit]
```

### Environment Variables

```bash
# .env — add to existing
GITHUB_TOKEN=ghp_...                # GitHub PAT with issues write access to neokapi/agent-feedback
```
