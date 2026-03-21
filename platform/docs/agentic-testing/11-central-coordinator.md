# Central Coordinator Architecture

## Motivation

The decentralized self-scheduling model described in [03-orchestration.md](03-orchestration.md) works well for isolated per-project agent teams, but breaks down when the testing fleet grows across multiple workspaces and projects. Problems with decentralized scheduling at scale:

- **No fleet-wide visibility.** Each agent only sees its own workspace. Nobody knows that Excalidraw's translator is idle while Docusaurus is backed up with 500 untranslated blocks.
- **Rigid cron schedules.** Agent sessions fire at fixed times regardless of whether there's work to do, wasting container execution time and AI spend.
- **No cross-workspace learning.** If a format parser bug surfaces in one project, other projects using the same format don't benefit until a human intervenes.
- **Scaling is manual.** Adding a new project means defining cron jobs for every agent, tuning schedules to avoid overlap, and monitoring each independently.
- **Release walkthroughs require separate tooling.** The `release-walker` service is a one-off script that doesn't integrate with the scheduling system.

The central coordinator replaces rigid per-agent cron scheduling with a single intelligent agent that has fleet-wide visibility and dispatches work based on actual need.

## Architecture Overview

A single **coordinator agent** runs on a regular schedule (e.g., every 30 minutes) in an Azure Container App Job. It connects to **two MCP servers**:

1. **Bowrain MCP** (existing, `/mcp/`) — the 24+ content management tools every agent uses
2. **Agentic Testing MCP** (new) — fleet management tools for orchestrating the testing system

The coordinator reads fleet state, decides what work needs doing, and dispatches agent sessions by placing messages on Service Bus queues (Azure) or Redis channels (local). Worker agents remain unchanged — they still run as single-shot ZeroClaw sessions triggered by queue messages.

```
┌─────────────────────────────────────────────────────────────────┐
│  Azure Container Apps Environment                                │
│                                                                   │
│  ┌───────────────────────────────────────────────────────┐       │
│  │  Coordinator Job (scheduled: every 30min)              │       │
│  │                                                        │       │
│  │  ZeroClaw agent -m "Run coordination cycle"            │       │
│  │  SOUL.md: Fleet Coordinator persona                    │       │
│  │                                                        │       │
│  │  MCP connections:                                      │       │
│  │    ├── Bowrain MCP (/mcp/)    — content & project ops  │       │
│  │    └── Agentic Testing MCP    — fleet management ops   │       │
│  └────────────┬──────────────────────┬────────────────────┘       │
│               │                      │                            │
│       ┌───────▼────────┐    ┌────────▼──────────┐                │
│       │  Bowrain MCP   │    │  Agentic Testing  │                │
│       │  (24+ tools)   │    │  MCP (9 tools)    │                │
│       │  /mcp/         │    │  /agentic-mcp/    │                │
│       └───────┬────────┘    └────────┬──────────┘                │
│               │                      │                            │
│       ┌───────▼──────────────────────▼──────────┐                │
│       │          bowrain-server                   │                │
│       │  (serves both MCP endpoints)              │                │
│       └──────────────────┬───────────────────────┘                │
│                          │                                        │
│          ┌───────────────┼───────────────┐                       │
│          │               │               │                       │
│    ┌─────▼─────┐  ┌──────▼──────┐  ┌────▼──────┐               │
│    │ Service   │  │ PostgreSQL  │  │ Agent     │               │
│    │ Bus       │  │ (state,     │  │ Memory    │               │
│    │ Queues    │  │  audit log) │  │ (git)     │               │
│    └─────┬─────┘  └─────────────┘  └───────────┘               │
│          │                                                       │
│    ┌─────▼──────────────────────────────────────────┐           │
│    │  Worker Agent Jobs (event-triggered via KEDA)   │           │
│    │                                                  │           │
│    │  ┌──────────┐ ┌──────────┐ ┌──────────┐        │           │
│    │  │ alex-dev │ │ sophie-  │ │ thomas-  │  ...   │           │
│    │  │ (push/   │ │ translator│ │ qa       │        │           │
│    │  │  pull)   │ │ (fr-FR)  │ │ (checks) │        │           │
│    │  └──────────┘ └──────────┘ └──────────┘        │           │
│    └─────────────────────────────────────────────────┘           │
└──────────────────────────────────────────────────────────────────┘
```

## Coordinator Persona

The coordinator is a ZeroClaw agent with its own SOUL.md, but its role is fundamentally different from worker agents. It doesn't translate, push content, or review terminology — it observes fleet state and dispatches work.

**`agents/coordinator/SOUL.md`:**

```markdown
# Fleet Coordinator

You are the central coordinator for Bowrain's agentic testing fleet. Your job is
to keep all testing workspaces productive by dispatching the right agent sessions
at the right time.

## Your Role

- Monitor all testing workspaces for pending work
- Dispatch agent sessions when work is available (not on fixed schedules)
- Stagger sessions across workspaces to avoid resource contention
- Detect cross-workspace patterns (recurring failures, format issues)
- Walk through release history for new projects (accelerated mode)
- File feedback issues when platform bugs are detected
- Commit fleet-wide observations to memory

## Your Decision Framework

On each coordination cycle:

1. Call `get_fleet_summary` to see the state of all workspaces
2. For each workspace with pending work:
   a. Determine which agent role is needed (developer, translator, qa, etc.)
   b. Check if that agent recently ran (avoid over-scheduling)
   c. If work is needed, call `trigger_agent_session` with the appropriate persona and task
3. Check for cross-workspace patterns:
   a. Same error across multiple projects → file a feedback issue
   b. Translation quality dropping → trigger extra QA sessions
   c. New project onboarded → start accelerated release walkthrough
4. Commit any observations to memory for continuity across cycles

## Scheduling Philosophy

- Only dispatch work when there's actually something to do
- Respect inter-session gaps: don't trigger the same agent twice in 2 hours
- Stagger across workspaces: if Excalidraw and Docusaurus both need translation,
  schedule them 15 minutes apart to spread AI API load
- Prioritize freshness: newer pushes get translated before older backlog
- Budget awareness: track daily session count against limits

## Tools

You have two MCP servers:

### Bowrain MCP (content awareness)

- `list_projects` — see all projects across workspaces
- `list_blocks` — check untranslated block counts
- `connector_status` — see if upstream has changes

### Agentic Testing MCP (fleet management)

- `get_fleet_summary` — aggregated status of all workspaces and agents
- `trigger_agent_session` — dispatch a specific agent for a specific task
- `list_agent_executions` — recent session history (who ran, when, outcome)
- `walk_release` — advance a project to the next release tag
- `file_feedback_issue` — create GitHub issue for platform bugs
- `commit_memory` — persist observations across coordinator cycles
- `list_workspaces` — all testing workspaces with their projects
- `onboard_project` — set up a new project for agentic testing
- `get_workspace_status` — detailed status for a single workspace
```

## Agentic Testing MCP Server

A new MCP endpoint served by bowrain-server alongside the existing `/mcp/` endpoint. It exposes fleet management operations that only the coordinator needs.

### Endpoint

```
POST /agentic-mcp/    — MCP protocol (same transport as /mcp/)
```

Authentication: coordinator's JWT token, same mechanism as other agents. The coordinator user has a special role that grants access to the agentic testing MCP tools.

### Tool Catalog

#### `get_fleet_summary`

Returns aggregated status across all testing workspaces. This is the coordinator's primary decision-making input.

```json
{
  "workspaces": [
    {
      "slug": "excalidraw-l10n",
      "project_count": 1,
      "pending_pushes": 0,
      "untranslated_blocks": { "fr-FR": 142, "de-DE": 89, "ja-JP": 203 },
      "last_agent_session": {
        "agent": "sophie-translator",
        "role": "translator",
        "started_at": "2026-03-20T14:00:00Z",
        "status": "completed",
        "blocks_translated": 28
      },
      "active_sessions": [],
      "health": "healthy",
      "mode": "real-time"
    },
    {
      "slug": "docusaurus-l10n",
      "project_count": 1,
      "pending_pushes": 1,
      "untranslated_blocks": { "fr-FR": 0, "de-DE": 412, "ja-JP": 0 },
      "last_agent_session": {
        "agent": "alex-developer",
        "role": "developer",
        "started_at": "2026-03-20T09:00:00Z",
        "status": "completed",
        "blocks_pushed": 87
      },
      "active_sessions": [],
      "health": "healthy",
      "mode": "accelerated"
    }
  ],
  "global": {
    "total_workspaces": 4,
    "active_sessions": 0,
    "sessions_today": 12,
    "daily_budget": 50,
    "ai_spend_today_usd": 3.42
  }
}
```

#### `get_workspace_status`

Detailed status for a single workspace, including per-project and per-locale breakdowns.

**Parameters:** `workspace_slug` (string)

```json
{
  "slug": "excalidraw-l10n",
  "projects": [
    {
      "id": "proj_abc123",
      "name": "Excalidraw",
      "source_language": "en-US",
      "target_languages": ["fr-FR", "de-DE", "ja-JP"],
      "streams": ["main"],
      "block_stats": {
        "total": 1847,
        "translated": { "fr-FR": 1705, "de-DE": 1758, "ja-JP": 1644 },
        "reviewed": { "fr-FR": 1680, "de-DE": 1700, "ja-JP": 1600 }
      },
      "last_push": "2026-03-19T09:15:00Z",
      "upstream_status": "1 new release (v0.18.1)"
    }
  ],
  "recent_activity": [
    {
      "timestamp": "2026-03-20T14:32:00Z",
      "actor": "agent-sophie",
      "action": "translated 28 blocks (fr-FR)",
      "project": "Excalidraw"
    }
  ],
  "agent_history": [
    {
      "agent": "alex-developer",
      "last_session": "2026-03-19T09:00:00Z",
      "sessions_this_week": 5,
      "avg_duration_minutes": 8
    }
  ]
}
```

#### `trigger_agent_session`

Dispatches a worker agent session by placing a message on the appropriate queue. The worker agent picks it up via KEDA (Azure) or Redis subscription (local).

**Parameters:**

| Parameter        | Type    | Description                                                 |
| ---------------- | ------- | ----------------------------------------------------------- |
| `workspace_slug` | string  | Target workspace                                            |
| `agent_role`     | string  | `developer`, `translator`, `brand_manager`, `qa`, `pm`      |
| `persona`        | string  | Agent persona name (e.g., `sophie-translator`)              |
| `task`           | string  | Natural language task description for the agent's `-m` flag |
| `locale`         | string? | Target locale (for translators)                             |
| `priority`       | string? | `normal`, `high` (high = skip inter-session gap)            |

**Example:**

```json
{
  "workspace_slug": "excalidraw-l10n",
  "agent_role": "translator",
  "persona": "sophie-translator",
  "task": "Translate the 142 untranslated fr-FR blocks in Excalidraw. Focus on UI strings first, then documentation.",
  "locale": "fr-FR",
  "priority": "normal"
}
```

The task message is passed directly to `zeroclaw agent -m "<task>"`, so the coordinator can give context-rich, specific instructions rather than generic "do your job" messages.

#### `list_agent_executions`

Returns recent agent session history across the fleet, for scheduling decisions (avoiding over-triggering) and pattern detection.

**Parameters:** `workspace_slug` (string?), `agent` (string?), `since` (ISO timestamp?), `limit` (int, default 50)

```json
{
  "executions": [
    {
      "id": "exec_xyz789",
      "workspace": "excalidraw-l10n",
      "agent": "sophie-translator",
      "role": "translator",
      "started_at": "2026-03-20T14:00:00Z",
      "completed_at": "2026-03-20T14:22:00Z",
      "status": "completed",
      "task": "Translate 30 fr-FR blocks in Excalidraw",
      "result_summary": "Translated 28/30 blocks, 2 skipped (ambiguous source)",
      "ai_tokens_used": 45200,
      "error": null
    }
  ]
}
```

#### `walk_release`

Advances a project to the next release tag in its accelerated walkthrough. The coordinator calls this sequentially, waiting for all agents to finish processing one release before advancing to the next.

**Parameters:** `workspace_slug` (string), `project_id` (string), `tag` (string?)

If `tag` is omitted, advances to the next unprocessed tag. Returns the tag that was applied and the blocks that changed.

#### `file_feedback_issue`

Creates a GitHub issue in the `neokapi/agent-feedback` repo when the coordinator detects a platform problem. The coordinator provides richer context than individual agents because it sees cross-workspace patterns.

**Parameters:** `title` (string), `body` (string), `labels` (string[])

#### `commit_memory`

Commits changes to the fleet repo (`bowrain-l10n/agent-fleet`). Used to persist coordinator observations, update workspace status, or modify agent SOUL.md overrides.

**Parameters:** `path` (string) — file path within the fleet repo, `content` (string) — content to write, `message` (string) — commit message

#### `list_workspaces`

Lists all testing workspaces registered in the agentic testing system, with basic metadata.

#### `onboard_project`

Sets up a new project for agentic testing: creates the workspace, forks the upstream repo, configures languages, and schedules the first developer push.

**Parameters:** `upstream_repo` (string), `name` (string), `source_language` (string), `target_languages` (string[]), `content_paths` (object[])

## How the Coordinator Replaces Per-Agent Cron

### Before: Decentralized Scheduling

```
09:00  Alex push job fires (cron)         — maybe no upstream changes → wasted session
10:00  Lisa PM job fires (cron)           — maybe no new pushes → wasted session
10:00  Maria brand job fires (cron)       — maybe no new terms → wasted session
14:00  Sophie translator job fires (cron) — maybe no assigned tasks → wasted session
14:00  Thomas QA job fires (cron)         — maybe nothing to check → wasted session
```

Each agent runs regardless of whether there's work. With 7 agents × 5 days × 1-2 sessions/day, that's 35-70 sessions/week, many of which accomplish nothing.

### After: Coordinator-Dispatched

```
09:00  Coordinator wakes up
       → get_fleet_summary: Excalidraw has new upstream release
       → trigger_agent_session: alex-developer, "Push Excalidraw v0.18.1"
       → get_fleet_summary: Docusaurus has 412 untranslated de-DE blocks
       → trigger_agent_session: thomas-translator, "Translate de-DE backlog in Docusaurus"
       → No other work pending. Done. (2 dispatches instead of 7 blind cron fires)

09:30  Coordinator wakes up
       → get_fleet_summary: Alex finished pushing 87 new blocks to Excalidraw
       → trigger_agent_session: mei-pm, "Create tasks for new Excalidraw blocks"
       → Thomas still translating Docusaurus. No other changes. Done.

10:00  Coordinator wakes up
       → get_fleet_summary: Mei created tasks, 142 blocks need fr-FR translation
       → trigger_agent_session: sophie-translator, "Translate fr-FR blocks in Excalidraw"
       → Thomas finished Docusaurus de-DE batch
       → trigger_agent_session: thomas-qa, "Run QA on Docusaurus de-DE translations"
```

The coordinator fires every 30 minutes but only dispatches work when it exists. Total sessions are demand-driven, not schedule-driven.

## Git-Ops State Model

All persistent state for the agentic testing system lives in a single git repository (`bowrain-l10n/agent-fleet`). This is the **single source of truth** for the entire fleet — persona definitions, workspace plans, coordinator config, and agent memory. There is no database, no external config store, no baked-in container state.

### Repository Structure

```
bowrain-l10n/agent-fleet/
│
├── personas/                        # Persona templates (shared across workspaces)
│   ├── developer/
│   │   ├── SOUL.md                  # Alex Chen — Senior DevOps Engineer
│   │   └── config.toml              # Base ZeroClaw config (provider, tools, security)
│   ├── translator/
│   │   ├── SOUL.md                  # Base translator persona (parameterized)
│   │   └── config.toml
│   ├── brand-manager/
│   │   ├── SOUL.md
│   │   └── config.toml
│   ├── qa/
│   │   ├── SOUL.md
│   │   └── config.toml
│   ├── pm/
│   │   ├── SOUL.md
│   │   └── config.toml
│   └── coordinator/
│       ├── SOUL.md                  # Fleet Coordinator persona
│       └── config.toml
│
├── workspaces/                      # Per-workspace state
│   ├── excalidraw-l10n/
│   │   ├── plan.yaml                # Workspace plan (research output, release strategy)
│   │   ├── status.yaml              # Current progress (updated by coordinator)
│   │   └── agents/                  # Workspace-specific agent overrides
│   │       ├── alex-developer/
│   │       │   ├── SOUL.md          # Extends personas/developer/SOUL.md with project context
│   │       │   └── memory/          # Agent memory files (ZeroClaw markdown)
│   │       ├── sophie-translator/
│   │       │   ├── SOUL.md          # French translator with Excalidraw-specific guidelines
│   │       │   └── memory/
│   │       └── ...
│   ├── docusaurus-l10n/
│   │   ├── plan.yaml
│   │   ├── status.yaml
│   │   └── agents/
│   │       └── ...
│   └── home-assistant-l10n/
│       └── ...
│
├── observers.yaml                   # Global observers added to every workspace
│
├── coordinator/
│   └── memory/                      # Coordinator's cross-workspace observations
│
└── README.md                        # Fleet overview and onboarding instructions
```

### How It Works

The entrypoint script (`entrypoint-with-memory.sh`) clones/pulls the fleet repo at the start of each session and pushes changes back at the end. The script assembles the agent's working config by layering:

1. **Base persona** from `personas/{role}/` — shared SOUL.md and config.toml
2. **Workspace override** from `workspaces/{workspace}/agents/{agent}/` — project-specific SOUL.md additions, memory files

This means:

- **Edit `personas/translator/SOUL.md`** → all translator agents across all workspaces pick up the change on their next session
- **Edit `workspaces/excalidraw-l10n/agents/sophie-translator/SOUL.md`** → only Sophie's Excalidraw sessions change
- **Edit `workspaces/excalidraw-l10n/plan.yaml`** → coordinator picks up the new strategy on next cycle
- **`git log personas/translator/SOUL.md`** → full history of how the translator persona evolved
- **`git log workspaces/excalidraw-l10n/`** → complete history of one workspace's lifecycle

### Persona Templates vs Workspace Overrides

Base personas in `personas/` are generic — they define the role, tools, working style, and quality standards without reference to any specific project. Workspace agent directories extend the base with project-specific context:

**`personas/translator/SOUL.md`** (template):

```markdown
# Translator

You are a professional translator working on localization projects through Bowrain.

## Your Role

- Review AI-generated translations for accuracy and fluency
- Edit translations that don't meet quality standards
- Add high-quality translations to Translation Memory
- Flag ambiguous source text or terminology issues

## Your Tools

- `bowrain.listTasks` — See assigned translation tasks
- `bowrain.translate` — Submit your translation for a block
  ...
```

**`workspaces/excalidraw-l10n/agents/sophie-translator/SOUL.md`** (workspace override):

```markdown
# Sophie Martin — French Translator (Excalidraw)

You are Sophie Martin, translating Excalidraw from English to French (fr-FR).

## Project Context

- Excalidraw is a virtual whiteboard for sketching hand-drawn diagrams
- UI strings are in flat JSON files under packages/excalidraw/locales/
- Keys are descriptive (e.g., "toolBar.eraser") — use them for context
- ~40 strings use ICU plural syntax — follow the pluralization guidelines in the termbase
- Use formal register (vous) for UI labels, informal (tu) for tooltips and hints

## Inherited Behavior

Follow all guidelines from the base translator persona.
```

The entrypoint script concatenates the base and override SOUL.md files (or the override can explicitly reference the base — ZeroClaw supports `#include` directives).

### Observers

Human users who need to log in and monitor agent activity are defined at two levels:

**`observers.yaml`** (fleet-wide — added to every workspace):

```yaml
# People who monitor the entire fleet
observers:
  - email: asgeir@bowrain.com
    role: admin
```

**`workspaces/{slug}/plan.yaml`** (per-workspace — added only to that workspace):

```yaml
observers:
  - email: demo@bowrain.com
    role: viewer # Read-only for demo purposes
```

During onboarding, both lists are merged. Observers get Keycloak accounts (if they don't already exist) and are added as workspace members with the specified role. This lets them log in to the web UI to browse the activity feed, review translations, inspect dashboards, and see agent work in progress — without being part of the agent team.

### Workspace Onboarding in Git

Onboarding a new workspace is a git operation. The coordinator (or a human) creates the workspace directory structure and commits it:

```bash
# Coordinator creates this via the onboard_project MCP tool,
# or a human creates it manually and pushes:

workspaces/excalidraw-l10n/
├── plan.yaml          # Output of planning phase (see Workspace Planning Phase)
├── status.yaml        # Initial: { phase: "planned", current_release: null }
└── agents/
    ├── alex-developer/
    │   └── SOUL.md    # Project-specific developer context
    ├── sophie-translator/
    │   └── SOUL.md    # French translator context for this project
    └── thomas-qa/
        └── SOUL.md    # QA context for this project
```

When onboarding executes (either via the coordinator's `onboard_project` tool or manually), it:

1. Creates the Bowrain workspace and project via the Bowrain API
2. Provisions Keycloak users for each agent in the team
3. Adds **observers** listed in `plan.yaml` as workspace members so they can log in to the web UI and monitor agent activity, review translations, and inspect dashboards
4. Generates per-agent JWT tokens

The coordinator won't dispatch agents to a workspace until `plan.yaml` exists and `status.yaml` shows `phase: "approved"`. A human reviews the plan and sets the phase, or the coordinator auto-approves after validation passes.

### Coordinator State

The coordinator is stateless between cycles by design — it queries fleet state fresh each time via `get_fleet_summary` and `list_agent_executions`, and reads plans from the fleet repo. This avoids state synchronization issues and makes the coordinator crash-safe (if a cycle fails, the next cycle picks up naturally).

Persistent observations (patterns noticed over multiple cycles) are stored via `commit_memory` in `coordinator/memory/`. Examples:

- "Excalidraw fr-FR translations consistently score lower on quality — consider scheduling extra QA"
- "Format parser fails on nested MDX in Docusaurus docs section — filed issue #47"
- "Thomas works faster on shorter batches (20 blocks) than longer ones (50 blocks)"

## Fleet-Wide Intelligence

The central coordinator enables capabilities impossible with decentralized scheduling:

### Cross-Workspace Resource Allocation

When multiple workspaces need translation simultaneously, the coordinator decides priority:

```
Excalidraw: 142 untranslated fr-FR blocks (pushed 30min ago)
Docusaurus: 412 untranslated de-DE blocks (pushed 2 days ago)
Home Assistant: 23 untranslated ja-JP blocks (pushed 1 hour ago)

Decision: Excalidraw fr-FR first (freshest push, moderate volume),
then Home Assistant ja-JP (small batch, quick win),
then Docusaurus de-DE (large batch, stale — schedule over multiple sessions)
```

### Staggered AI API Usage

Instead of 5 translators hitting Azure AI Foundry simultaneously at 14:00:

```
14:00  Sophie (Excalidraw fr-FR) — 142 blocks
14:15  Thomas (Home Assistant ja-JP) — 23 blocks
14:45  Sophie (Docusaurus de-DE, batch 1/3) — 140 blocks
```

The coordinator spaces sessions to stay within API rate limits and budget.

### Cross-Workspace Pattern Detection

The coordinator sees all workspaces, so it can detect systemic issues:

- **Same parse error in 3 projects** → file one feedback issue with all examples
- **Quality scores dropping fleet-wide** → might be an LLM provider issue, not a translation issue
- **Specific locale consistently slower** → persona tuning needed (see [10-persona-evolution.md](10-persona-evolution.md))

### Release Walkthrough Orchestration

The coordinator subsumes the release-walker service. Instead of a separate script, the coordinator uses `walk_release` to advance projects through their release history:

```
Cycle 1: walk_release(excalidraw, v0.17.0) → trigger alex push → wait
Cycle 2: translations in progress, skip excalidraw. Advance docusaurus instead.
Cycle 3: excalidraw v0.17.0 done → walk_release(excalidraw, v0.17.1) → trigger alex push
```

This is more flexible than the release-walker's rigid sequential loop because the coordinator can interleave releases across projects based on completion status.

## Azure Deployment

### Coordinator Job

```bicep
resource coordinatorJob 'Microsoft.App/jobs@2024-03-01' = {
  name: 'job-agent-coordinator'
  location: location
  properties: {
    environmentId: containerAppsEnvironmentId
    configuration: {
      triggerType: 'Schedule'
      replicaTimeout: 300           // 5min max per coordination cycle
      replicaRetryLimit: 1
      scheduleTriggerConfig: {
        cronExpression: '*/30 * * * *'  // Every 30 minutes
        parallelism: 1
        replicaCompletionCount: 1
      }
    }
    template: {
      containers: [{
        name: 'coordinator'
        image: 'ghcr.io/neokapi/bravo-agent:latest'
        command: ['/bin/bash', '/bravo/entrypoint-with-memory.sh']
        resources: { cpu: json('0.25'), memory: '0.5Gi' }
        env: [
          { name: 'AGENT_NAME', value: 'coordinator' }
          { name: 'AGENT_TASK_MESSAGE', value: 'Run coordination cycle' }
          { name: 'BRAVO_MCP_ENDPOINT', value: bravoMcpEndpoint }
          { name: 'AGENTIC_MCP_ENDPOINT', value: agenticMcpEndpoint }
          { name: 'BRAVO_AGENT_TOKEN', secretRef: 'coordinator-token' }
          { name: 'FLEET_REPO', value: fleetRepoUrl }
          { name: 'AZURE_AI_FOUNDRY_KEY', secretRef: 'ai-foundry-key' }
        ]
      }]
    }
  }
}
```

### Worker Jobs (Unchanged)

Worker agents remain event-triggered Container App Jobs. The only difference is that messages now come from the coordinator (via `trigger_agent_session`) rather than from rigid cron schedules or the ChannelEventBus queue sink adapter.

```bicep
resource workerJob 'Microsoft.App/jobs@2024-03-01' = {
  name: 'job-agent-${agentName}'
  location: location
  properties: {
    environmentId: containerAppsEnvironmentId
    configuration: {
      triggerType: 'Event'
      replicaTimeout: 1800          // 30min max per session
      eventTriggerConfig: {
        scale: {
          minExecutions: 0
          maxExecutions: 1
          pollingInterval: 30
          rules: [{
            name: 'servicebus'
            type: 'azure-servicebus-queue'
            metadata: {
              queueName: 'agent-${agentName}'
              messageCount: '1'
            }
            auth: [{ triggerParameter: 'connection', secretRef: 'sb-connection' }]
          }]
        }
      }
    }
    template: {
      containers: [{
        name: agentName
        image: 'ghcr.io/neokapi/bravo-agent:latest'
        command: ['/bin/bash', '/bravo/entrypoint-with-memory.sh']
        resources: { cpu: json('0.25'), memory: '0.5Gi' }
        env: [
          { name: 'AGENT_NAME', value: agentName }
          // AGENT_TASK_MESSAGE comes from the Service Bus message body
          { name: 'BRAVO_MCP_ENDPOINT', value: bravoMcpEndpoint }
          { name: 'BRAVO_AGENT_TOKEN', secretRef: '${agentName}-token' }
          { name: 'FLEET_REPO', value: fleetRepoUrl }
        ]
      }]
    }
  }
}
```

### Service Bus Queue Layout

```
bowrain-sb-d (existing namespace)
  ├── bravo-jobs          (existing — Bravo async jobs)
  ├── translation-jobs    (existing — MT pipeline)
  ├── agent-coordinator   (new — coordinator dispatch output isn't a queue;
  │                         coordinator writes to per-agent queues directly)
  ├── agent-alex          (new — developer sessions)
  ├── agent-sophie        (new — fr-FR translator sessions)
  ├── agent-thomas        (new — QA + de-DE translator sessions)
  ├── agent-mei           (new — PM sessions)
  └── agent-generic       (new — overflow for dynamically assigned work)
```

## Local Development

For local development, the coordinator runs as a Docker container alongside the workers, using Redis channels instead of Service Bus:

```yaml
# docker-compose.yaml additions
coordinator:
  image: ghcr.io/neokapi/bravo-agent:latest
  command: daemon # In local mode, coordinator runs as a daemon with cron
  environment:
    BRAVO_MCP_ENDPOINT: http://bowrain-server:8080/mcp/
    AGENTIC_MCP_ENDPOINT: http://bowrain-server:8080/agentic-mcp/
    BRAVO_AGENT_TOKEN: ${COORDINATOR_TOKEN}
    AZURE_AI_FOUNDRY_KEY: ${AZURE_AI_FOUNDRY_KEY}
  volumes:
    - ./agents/coordinator:/root/.zeroclaw
```

The `trigger_agent_session` tool publishes to Redis channels locally (same `agentic:*` namespace) instead of Service Bus queues.

## Agentic Testing MCP Implementation

The Agentic Testing MCP server is implemented in `platform/server/agentic_mcp/` as a separate MCP endpoint within bowrain-server. It shares the same server process, database connections, and event bus as the main Bowrain MCP.

### Data Sources

The MCP tools aggregate data from existing platform components:

| Tool                    | Primary Data Source                                                                       |
| ----------------------- | ----------------------------------------------------------------------------------------- |
| `get_fleet_summary`     | ContentStore (block stats), audit_log (recent activity), Container Apps API (active jobs) |
| `get_workspace_status`  | ContentStore, audit_log, git connector status                                             |
| `trigger_agent_session` | Service Bus SDK (Azure) or Redis PUBLISH (local)                                          |
| `list_agent_executions` | audit_log filtered by agent actors + Container Apps execution history                     |
| `walk_release`          | git tags on upstream fork + ContentStore push history                                     |
| `file_feedback_issue`   | GitHub API via `gh` CLI or octokit                                                        |
| `commit_memory`         | git operations on fleet repo                                                              |
| `list_workspaces`       | Fleet repo `workspaces/` directory listing + `plan.yaml` metadata                         |
| `onboard_project`       | Creates workspace directory in fleet repo, forks upstream, generates plan                 |

### Workspace Discovery

Testing workspaces are discovered from the fleet repo, not the Bowrain database. The `list_workspaces` and `get_fleet_summary` tools scan `workspaces/*/plan.yaml` in the fleet repo to enumerate workspaces and read their configuration. This keeps the agentic testing system fully decoupled from production Bowrain — no metadata flags on production workspaces, no database queries for fleet state.

## Workspace Planning Phase

Before the coordinator dispatches any agents to a workspace, it runs a **planning phase** that researches the upstream project and produces a concrete execution plan. This is not automated setup — it's the coordinator reasoning about the project and making strategic decisions that shape the entire testing engagement.

### Planning Workflow

The coordinator (or a human operator) triggers planning for a new workspace via `onboard_project`. The planning phase proceeds in stages:

#### Stage 1: Upstream Research

The coordinator examines the upstream repository to understand what it's working with:

- **Release history**: How many releases? How frequent? What's the semver pattern?
- **Content structure**: Where are the localizable files? What formats (JSON, Markdown, HTML, YAML)?
- **Content volume**: How many strings/blocks per release? How much changes between releases?
- **Existing l10n**: Does the project already use Crowdin/Weblate/Transifex? What languages? What's the translation coverage?
- **Content types**: UI strings, documentation, blog posts, changelogs, API docs?
- **Complexity signals**: Nested formats (HTML in JSON), ICU message syntax, pluralization, gendered content?

This research uses the Bowrain MCP tools (`connector_pull` to fetch content, `list_blocks` to count extracted segments) plus git operations to inspect the repo.

#### Stage 2: Strategy Decision

Based on the research, the coordinator produces a **workspace plan** — a structured document that defines how the testing engagement will run:

```yaml
# Workspace plan: excalidraw-l10n
# Generated by coordinator, reviewed by operator

upstream:
  repo: excalidraw/excalidraw
  fork: bowrain-l10n/excalidraw

project:
  name: Excalidraw
  source_language: en-US
  target_languages: [fr-FR, de-DE, ja-JP]

content:
  paths:
    - path: "packages/excalidraw/locales/*.json"
      format: json
      estimated_blocks: 1200
    - path: "dev-docs/**/*.md"
      format: markdown
      estimated_blocks: 450
  total_estimated_blocks: 1650
  complexity: medium # No nested formats, some ICU plurals

release_strategy:
  mode: accelerated-then-realtime
  start_tag: v0.14.0 # First release with stable i18n structure
  skip_tags: [v0.14.1-rc*] # Skip release candidates
  end_tag: latest
  estimated_releases: 28 # v0.14.0 through v0.18.1
  pace: 2_hours_between_releases
  switch_to_realtime_at: latest # After catching up, track main

agent_team:
  developer: alex-developer
  translators:
    fr-FR: sophie-translator
    de-DE: thomas-translator # thomas doubles as QA
    ja-JP: mei-translator # mei doubles as PM
  brand: maria-brand
  qa: thomas-qa
  pm: mei-pm

observers: # Human users added to the workspace for monitoring
  - email: asgeir@bowrain.com
    role: admin # Can view everything, edit settings
  - email: demo@bowrain.com
    role: viewer # Read-only access for demos

notes: |
  - Excalidraw's i18n JSON uses flat key-value pairs, no nesting.
  - Keys are descriptive (e.g., "toolBar.eraser") which helps translation context.
  - The project had partial French translations via Crowdin up to v0.16.
    We ignore existing translations and start fresh to test the full workflow.
  - Dev docs use MDX with React components — need to verify MDX parser handles
    the custom components correctly before starting the walkthrough.
  - ICU plural syntax appears in ~40 strings. Brand manager should add
    pluralization guidelines to the termbase early.
```

#### Stage 3: Validation

Before committing to the plan, the coordinator validates it:

1. **Format test**: Push a sample of content from the start tag through Bowrain's parser. Verify all formats extract cleanly. Flag any parse errors.
2. **Volume check**: Confirm estimated block counts are realistic. Adjust `blocks_per_session` if the project is larger/smaller than expected.
3. **Release continuity**: Walk the tag list and verify each tag builds on the previous (no massive refactors that would invalidate translations mid-walkthrough).
4. **Team fit**: Verify the assigned agents have the right language pairs and capabilities.

If validation surfaces issues (e.g., MDX parser fails on custom components), the plan is updated with workarounds or the issue is filed before proceeding.

#### Stage 4: Plan Approval

The workspace plan is committed to the fleet repo as `workspaces/{slug}/plan.yaml`, and `status.yaml` is created with `phase: "planned"`:

```
bowrain-l10n/agent-fleet/
└── workspaces/
    └── excalidraw-l10n/
        ├── plan.yaml       # Research output + strategy (committed by coordinator)
        ├── status.yaml     # { phase: "planned", current_release: null }
        └── agents/
            ├── alex-developer/
            │   └── SOUL.md  # Project-specific developer context (generated from plan)
            ├── sophie-translator/
            │   └── SOUL.md  # French translator context (generated from plan notes)
            └── ...
```

A human operator reviews the plan in git and sets `phase: "approved"` in `status.yaml` (or the coordinator auto-approves after validation passes). The coordinator won't dispatch agents until the phase is approved.

### How the Coordinator Uses the Plan

During regular coordination cycles, the coordinator loads the workspace plan and uses it to make dispatch decisions:

- **Release strategy** determines when to call `walk_release` and what tag to advance to
- **Agent team** mapping determines which persona to dispatch for each role
- **Content paths** inform the task messages given to agents ("Translate the JSON UI strings, not the MDX docs — those are phase 2")
- **Notes** provide context the coordinator passes through to agents in their task messages
- **Pace** controls the inter-release interval during accelerated mode

### Plan Evolution

Plans are living documents. The coordinator updates them as the engagement progresses:

- After completing accelerated mode, updates `release_strategy.mode` to `realtime`
- If a format issue is discovered, adds it to `notes` and adjusts `content.paths`
- If an agent consistently underperforms, the coordinator notes this for persona tuning

Changes are committed to the plans directory with descriptive messages, creating a history of strategic decisions for each workspace.

## Clean-Slate Deployment

This architecture is deployed from scratch — there is no migration from the decentralized per-agent cron model. The previous agent jobs (alex-push, alex-pull, lisa, maria, jp, katrin, yuki, taylor) are removed from the Azure infrastructure and replaced entirely by the coordinator + worker job model.

Starting clean avoids the complexity of running two scheduling systems in parallel and ensures the Agentic Testing MCP is the single source of truth for fleet state from day one.

## Relationship to Other Documents

- [03-orchestration.md](03-orchestration.md) — describes the decentralized model that this architecture evolves from. The operating modes (real-time, accelerated, hybrid) and failure handling remain valid. The scheduling mechanism changes.
- [04-implementation.md](04-implementation.md) — ZeroClaw containers, Bowrain MCP, and Azure deployment remain unchanged. The Agentic Testing MCP is additive. The release-walker is subsumed by the coordinator.
- [09-agent-routines.md](09-agent-routines.md) — agent daily routines still define _what_ each persona does. The coordinator decides _when_ they do it.
- [10-persona-evolution.md](10-persona-evolution.md) — the coordinator's cross-workspace visibility feeds the tuning loop with richer data than per-agent metrics alone.
