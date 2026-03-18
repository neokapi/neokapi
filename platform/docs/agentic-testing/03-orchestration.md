# Orchestration & Scheduling

## Overview

The orchestrator is the brain of the agentic testing system. It schedules agent work, manages timelines, controls pacing, handles failures, and ensures agents interact in realistic patterns. It runs as a long-lived service (or cron-scheduled batch) that drives the entire system.

## Operating Modes

### Mode 1: Real-Time (Default)

Agents operate on natural timescales — tasks take hours to days, activity accumulates organically.

```
Timeline:  Day 1          Day 2          Day 3          Day 4
           ────────────── ────────────── ────────────── ──────
Developer: push v3.1      -              push v3.1.1    -
PM:        create tasks   monitor        create tasks   review
Brand:     review terms   update terms   -              audit
Fr Trans:  -              translate 30%  translate 70%  review
De Trans:  -              translate 20%  translate 50%  translate 80%
QA:        -              -              -              run checks
```

**Pacing controls:**
- `agent_work_window`: Hours per day an agent is "active" (default: 4-6h)
- `session_duration`: How long a single work session lasts (default: 30-90min)
- `blocks_per_session`: Throughput per session (varies by persona)
- `inter_session_gap`: Minimum gap between sessions (default: 2-4h)

### Mode 2: Accelerated (Release Walkthrough)

Compress months of release history into days. Walk through tagged releases sequentially, with agents processing each release's changes before moving to the next.

```
Timeline:  Hour 0-2       Hour 2-4       Hour 4-6       Hour 6-8
           ────────────── ────────────── ────────────── ──────
Release:   v3.0.0         v3.0.1         v3.1.0         v3.1.1
Developer: push           push delta     push delta     push delta
All:       full workflow   full workflow  full workflow   full workflow
```

**Pacing controls:**
- `release_interval`: Time between processing releases (default: 2h)
- `max_releases_per_day`: Cap to prevent runaway (default: 6)
- `wait_for_completion`: Block until all agents finish before next release (default: true)

### Mode 3: Hybrid

Start with accelerated mode to build up history, then switch to real-time for ongoing activity.

```
Week 1 (accelerated): Walk v2.0 → v3.0 (12 releases in 3 days)
Week 2+ (real-time):  Track upstream main, process changes as they land
```

## Orchestrator Architecture

```
┌──────────────────────────────────────────────────┐
│                  Orchestrator                      │
│                                                    │
│  ┌────────────┐  ┌─────────────┐  ┌───────────┐  │
│  │  Scheduler  │  │   State     │  │  Event    │  │
│  │  (cron +    │  │   Manager   │  │  Router   │  │
│  │   triggers) │  │  (SQLite)   │  │           │  │
│  └──────┬─────┘  └──────┬──────┘  └─────┬─────┘  │
│         │               │               │         │
│  ┌──────▼───────────────▼───────────────▼──────┐  │
│  │              Agent Dispatcher                │  │
│  │  (launches agents, passes context, collects  │  │
│  │   results, handles retries)                  │  │
│  └──────┬──────────┬──────────┬────────────────┘  │
│         │          │          │                    │
└─────────┼──────────┼──────────┼───────────────────┘
          │          │          │
   ┌──────▼───┐ ┌───▼────┐ ┌──▼─────────┐
   │Developer │ │ Brand  │ │ Translator │
   │ Agent    │ │ Agent  │ │ Agent(s)   │
   └──────────┘ └────────┘ └────────────┘
```

### Components

#### Scheduler

Determines **when** agents run. Combines:

- **Cron schedules:** Regular intervals (e.g., Developer checks upstream daily at 09:00)
- **Event triggers:** React to platform events (e.g., new content pushed → create translation tasks)
- **Dependency chains:** Agent B runs after Agent A completes (e.g., translators wait for PM task creation)
- **Randomized jitter:** Add ±30min to avoid mechanical regularity

```yaml
# Example schedule definition
schedules:
  - agent: developer
    project: docusaurus
    cron: "0 9 * * 1-5"        # Weekdays 9am
    jitter_minutes: 30
    task: check_upstream_and_push

  - agent: pm
    project: docusaurus
    trigger: content_pushed      # Event-driven
    delay_minutes: 60            # Wait 1h after push
    task: create_translation_tasks

  - agent: brand_manager
    project: docusaurus
    cron: "0 10 * * 1,3,5"     # Mon/Wed/Fri 10am
    task: review_terminology

  - agent: translator_fr
    project: docusaurus
    cron: "0 14 * * 1-5"       # Weekdays 2pm
    task: translate_assigned_batch
    depends_on: pm.create_translation_tasks

  - agent: qa
    project: docusaurus
    trigger: translation_batch_complete
    delay_minutes: 30
    task: run_quality_checks
```

#### State Manager

Tracks the state of every project, agent, and workflow. Persisted in SQLite for durability.

**State dimensions:**
```sql
-- Project state
CREATE TABLE project_state (
    project_id TEXT PRIMARY KEY,
    upstream_ref TEXT,          -- Last synced upstream commit/tag
    bowrain_ref TEXT,           -- Last pushed commit
    stream_name TEXT,           -- Active Bowrain stream
    status TEXT,                -- active, paused, completed
    last_push_at TIMESTAMP,
    last_pull_at TIMESTAMP
);

-- Agent state
CREATE TABLE agent_state (
    agent_id TEXT PRIMARY KEY,
    persona TEXT,               -- developer, brand_manager, translator_fr, etc.
    project_id TEXT,
    status TEXT,                -- idle, working, blocked, error
    last_session_at TIMESTAMP,
    session_count INTEGER,
    blocks_processed INTEGER,
    current_task TEXT
);

-- Workflow state (tracks handoff chains)
CREATE TABLE workflow_state (
    workflow_id TEXT PRIMARY KEY,
    project_id TEXT,
    release_tag TEXT,
    phase TEXT,                 -- push, task_creation, translation, qa, pull
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    agents_involved TEXT        -- JSON array
);
```

#### Event Router

Routes platform events to agent triggers:

| Event | Source | Triggers |
|-------|--------|----------|
| `content_pushed` | Developer Agent | PM creates tasks |
| `tasks_created` | PM Agent | Translators begin work |
| `translation_batch_complete` | Translator Agent | QA runs checks |
| `qa_passed` | QA Agent | PM approves, Developer pulls |
| `upstream_release` | Release monitor | Developer merges and pushes |
| `terminology_updated` | Brand Manager | Translators check affected blocks |
| `brand_violation_found` | QA Agent | Brand Manager reviews |

## Failure Handling

### Retry Strategy

```yaml
retry:
  max_attempts: 3
  backoff: exponential     # 5min, 15min, 45min
  on_failure: pause_agent  # Don't retry indefinitely
  alert_after: 2           # Alert after 2 failures
```

### Common Failure Scenarios

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Server unreachable | HTTP timeout | Retry with backoff; pause if persistent |
| Auth token expired | 401 response | Re-authenticate via device flow |
| Push conflict | 409 response | Pull latest, resolve, retry push |
| AI provider down | Translation timeout | Skip AI, mark for manual translation |
| Agent crash | Process exit | Orchestrator restarts agent from last checkpoint |
| Rate limit | 429 response | Respect Retry-After header |

### Circuit Breaker

If an agent fails 3 consecutive sessions:
1. Mark agent as `error` state
2. Log failure details
3. Continue other agents (don't block the pipeline)
4. Alert via webhook (Slack, email, or dashboard notification)
5. Require manual reset or auto-retry after cooldown (default: 6h)

## Timeline Simulation

### Walking Through Release History

For the "accelerated" demo mode, the orchestrator replays release history:

```python
# Pseudocode for release walkthrough
releases = get_upstream_releases(project, since="v3.0.0")

for release in releases:
    # 1. Update fork to this release
    merge_upstream(release.tag)

    # 2. Developer agent pushes changes
    run_agent("developer", task="push_release", release=release)

    # 3. Wait for content to be indexed
    wait_for_event("content_indexed")

    # 4. Run full translation workflow
    run_workflow("translate_release", release=release)

    # 5. Wait for workflow completion (or timeout)
    wait_for_completion(timeout=release_interval)

    # 6. Developer pulls translations
    run_agent("developer", task="pull_translations")

    # 7. Log progress
    log_release_complete(release, metrics)

    # 8. Pace: wait before next release
    sleep(release_interval - elapsed)
```

### Timestamp Manipulation

For demo authenticity, optionally backdate activity:

**Option A: Natural timestamps (recommended)**
- Activity happens at real wall-clock time
- Accelerated mode compresses days into hours, but timestamps are real
- Simplest, no platform modifications needed

**Option B: Simulated timestamps (advanced)**
- Orchestrator passes `simulated_time` to agents
- Agents include timestamp in API calls (requires server-side support)
- Activity feed shows "historical" dates
- More complex but creates a more compelling demo narrative

**Recommendation:** Start with Option A. Add Option B later if demo storytelling requires it.

## Configuration

### Project Configuration File

```yaml
# agentic-testing/projects/docusaurus.yaml
project:
  name: Docusaurus
  upstream: facebook/docusaurus
  fork: bowrain-l10n/docusaurus
  source_language: en-US
  target_languages: [fr-FR, de-DE, ja-JP]
  content_paths:
    - path: "website/i18n/en/**/*.json"
      format: json
    - path: "website/docs/**/*.md"
      format: markdown
    - path: "website/blog/**/*.md"
      format: markdown

orchestration:
  mode: hybrid                  # accelerated → real-time
  accelerated:
    start_release: v3.0.0
    end_release: latest
    release_interval: 2h
  real_time:
    upstream_check_cron: "0 9 * * *"

agents:
  - persona: developer
    identity: alex_chen
    config:
      work_window: "08:00-12:00"

  - persona: brand_manager
    identity: maria_santos
    channels: [technical, community]
    config:
      work_window: "09:00-17:00"
      review_frequency: "Mon,Wed,Fri"

  - persona: translator
    identity: jean_pierre_dubois
    target_language: fr-FR
    config:
      work_window: "14:00-18:00"
      blocks_per_session: 30
      ai_acceptance_rate: 0.6   # Accepts 60% of AI translations as-is

  - persona: translator
    identity: katrin_weber
    target_language: de-DE
    config:
      work_window: "10:00-14:00"
      blocks_per_session: 25
      ai_acceptance_rate: 0.4   # More critical reviewer

  - persona: translator
    identity: yuki_tanaka
    target_language: ja-JP
    config:
      work_window: "20:00-00:00"  # Tokyo timezone
      blocks_per_session: 20
      ai_acceptance_rate: 0.3     # Very thorough, rewrites most

  - persona: pm
    identity: lisa_chen
    config:
      work_window: "08:00-10:00"
      check_cron: "0 8 * * 1-5"
```

### Global Configuration

```yaml
# agentic-testing/config.yaml
global:
  bowrain_server: https://bowrain.example.com
  github_org: bowrain-l10n
  max_concurrent_agents: 10
  ai_provider: anthropic        # For agent LLM calls
  ai_model: claude-sonnet-4-5-20250514

observability:
  metrics_endpoint: /metrics
  dashboard_refresh: 30s
  activity_log: true
  screenshot_interval: 1h       # Periodic screenshots for demo reel

cost_controls:
  daily_ai_budget: $50          # Total AI spend across all agents
  per_agent_session_limit: $5   # Max per agent session
  translation_provider: pseudo  # Use pseudo-translation for dry runs
```

## Concurrency Model

### Per-Project Serialization

Within a single project, agents that modify state run sequentially to avoid conflicts:

```
Project: Docusaurus
├── Developer: push → (wait) → pull     [serialized with other writers]
├── PM: create tasks                     [parallel with translators]
├── Translator-FR: translate             [parallel with other translators]
├── Translator-DE: translate             [parallel with other translators]
├── Translator-JA: translate             [parallel with other translators]
└── QA: read-only checks                 [parallel with translators]
```

### Cross-Project Parallelism

Different projects run fully in parallel:

```
Docusaurus agents ──────────────────▶ (independent)
Gitea agents ───────────────────────▶ (independent)
Home Assistant agents ──────────────▶ (independent)
Tolgee agents ──────────────────────▶ (independent)
```

### Resource Limits

- Max 10 concurrent agent sessions (across all projects)
- Max 3 concurrent API calls to Bowrain server (per project)
- Max 1 concurrent git operation (per fork)
- AI provider rate limits respected via token bucket
