# Orchestration & Scheduling

## Overview

The agentic testing system uses a **distributed, self-scheduling architecture** with two
deployment models:

- **Local (docker-compose):** ZeroClaw daemon containers with built-in cron + heartbeat
  polling. Simple, always-on, good for development.
- **Azure (Container Apps Jobs):** Scheduled and event-driven jobs. Agents start, do work,
  and stop. Pay only for execution time. Event-driven handoffs via Azure Service Bus
  give instant reaction instead of poll delays.

Both models use identical SOUL.md personas and Bravo MCP tools. Only the scheduling
and triggering mechanism differs.

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

## Agent Scheduling Architecture

Each ZeroClaw agent container runs in **daemon mode** with two scheduling mechanisms:

1. **Cron** — Regular tasks on a schedule (defined in `config.toml`)
2. **Heartbeat** — Periodic checks for new work (defined in `HEARTBEAT.md`)

There is no central scheduler or event router. Agents discover work by **polling Bowrain's activity feed** on their heartbeat cycle.

```
┌─────────────────────────────────────────────────────────────┐
│  ZeroClaw Daemon (per agent container)                       │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │  Cron         │  │  Heartbeat   │  │  Agent Loop      │  │
│  │  (config.toml)│  │ (HEARTBEAT.md│  │  (SOUL.md +      │  │
│  │  Fixed tasks  │  │  Check for   │  │   MCP tools)     │  │
│  │  at set times │  │  new work)   │  │                  │  │
│  └──────┬───────┘  └──────┬───────┘  └────────┬─────────┘  │
│         │                  │                    │             │
│         └──────────────────┼────────────────────┘             │
│                            │ MCP                              │
│                     ┌──────▼───────┐                         │
│                     │ Bowrain MCP  │                         │
│                     │ Server       │                         │
│                     └──────────────┘                         │
└─────────────────────────────────────────────────────────────┘
```

### Schedule Configuration (per agent)

Schedules live in each agent's `config.toml`:

```toml
# agents/alex-developer/config.toml
[daemon.cron]
"check-upstream" = "0 9 * * 1-5"     # Weekdays 9am: check for new releases
"pull-translations" = "0 17 * * 1-5"  # Weekdays 5pm: pull completed work

# agents/maria-brand/config.toml
[daemon.cron]
"review-terms" = "0 10 * * 1,3,5"    # Mon/Wed/Fri 10am: review terminology
"brand-audit" = "0 10 * * 4"         # Thursday: brand compliance audit

# agents/jeanpierre-fr/config.toml
[daemon.cron]
"translate-batch" = "0 14 * * 1-5"   # Weekdays 2pm: translate assigned blocks

# agents/lisa-pm/config.toml
[daemon.cron]
"morning-check" = "0 10 * * 1-5"     # Weekdays 10am: review dashboard (after Developer 9am push)
```

### Event Discovery via Activity Feed (Heartbeat Polling)

The simplest coordination model: agents discover events by polling Bowrain's activity
feed on their heartbeat cycle. No external dependencies beyond Bowrain itself.

| What the PM sees in activity feed  | What PM does               |
| ---------------------------------- | -------------------------- |
| "Alex Chen pushed 142 blocks"      | Creates translation tasks  |
| "Jean-Pierre translated 28 blocks" | Updates progress tracking  |
| "Taylor Kim: QA passed for fr-FR"  | Marks language as complete |

| What the Translator sees                                   | What Translator does         |
| ---------------------------------------------------------- | ---------------------------- |
| "Lisa Chen created task: Translate docs (fr-FR)"           | Starts translating           |
| "Maria Santos updated termbase: 'deploy' is now preferred" | Checks affected translations |

This is more realistic than event-driven triggers — humans check dashboards, not webhooks.
It works well for local development but has 1-2 hour latency between handoffs.

### Local: Redis Pub/Sub for Instant Handoffs

For faster local handoffs, agents can subscribe to Redis pub/sub channels. Redis is
**already in the platform compose stack** — Bravo uses it for SSE relay
(`bravo:sse:{conversationID}` channels). The agentic testing system reuses the same
Redis instance.

```
Developer pushes content (cron: 9am)
  → ChannelEventBus fires ContentPushed event
  → Queue sink adapter publishes to Redis channel: agentic:content-pushed
  → PM agent's Redis subscription triggers immediately

PM creates tasks
  → ChannelEventBus fires TasksCreated event
  → Queue sink adapter publishes to Redis channel: agentic:tasks-created-fr
  → French Translator agent reacts immediately
```

This gives Azure-like instant handoffs locally, without needing Service Bus.
ZeroClaw agents can subscribe to Redis channels directly (in addition to their
heartbeat cycle), so both coordination modes coexist.

### Azure: Event-Driven Handoffs via Service Bus

In Azure, agents use **Container Apps Jobs** with KEDA scalers monitoring Azure Service Bus
queues. The same ChannelEventBus events that go to Redis locally are routed to Service Bus
queues in Azure. Service Bus is **already deployed** for `bravo-jobs` and
`translation-jobs` — the agentic system adds queues to the same namespace.

```
content-pushed queue    → triggers PM job (create tasks)
tasks-created-fr queue  → triggers French Translator job
tasks-created-de queue  → triggers German Translator job
tasks-created-ja queue  → triggers Japanese Translator job
translation-complete    → triggers QA job
qa-passed queue         → triggers Developer pull job
```

Scheduled agents (Developer push, Brand Manager) use Azure-managed cron instead of
ZeroClaw's daemon cron. See `04-implementation.md` → "Azure Deployment: Container Apps Jobs"
for the full Bicep templates and agent job matrix.

### ChannelEventBus → Queue Sink Adapter

The key architectural piece is a **queue sink adapter** that subscribes to the existing
in-process ChannelEventBus (50+ event types) and forwards selected events to external
queues. This is a thin subscriber — not a new event system.

```
Bowrain Server Process
  │
  ├── ChannelEventBus (in-process, Go channels)
  │     │
  │     ├── existing subscribers (automation, SSE, webhooks, ...)
  │     │
  │     └── Queue Sink Subscriber (NEW)
  │           │
  │           ├── [local]  → Redis PUBLISH agentic:{event-type}
  │           └── [azure]  → Service Bus send to {queue-name}
```

**Event routing rules** (configured, not hardcoded):

| ChannelEventBus Event | Redis Channel (local)            | Service Bus Queue (Azure) |
| --------------------- | -------------------------------- | ------------------------- |
| `ContentPushed`       | `agentic:content-pushed`         | `content-pushed`          |
| `TasksCreated`        | `agentic:tasks-created-{locale}` | `tasks-created-{locale}`  |
| `TranslationComplete` | `agentic:translation-complete`   | `translation-complete`    |
| `QAPassed`            | `agentic:qa-passed`              | `qa-passed`               |

The adapter backend is selected by environment variable:

- `BOWRAIN_AGENT_RUNTIME=local` → Redis pub/sub (default for docker-compose)
- `BOWRAIN_AGENT_RUNTIME=queue` → Service Bus (Azure deployments)

This is the same `BOWRAIN_AGENT_RUNTIME` flag that already controls whether Bowrain runs
API and worker in the same process or as separate processes connected via Service Bus + Redis.

**When to use which:**

| Environment            | Scheduling           | Coordination                        | Latency   | Notes                          |
| ---------------------- | -------------------- | ----------------------------------- | --------- | ------------------------------ |
| Local (simple)         | ZeroClaw daemon cron | Poll activity feed (heartbeat)      | 1-2 hours | No Redis needed                |
| Local (instant)        | ZeroClaw daemon cron | Redis pub/sub (agentic:\* channels) | Seconds   | Same Redis as Bravo SSE        |
| Azure (Container Apps) | Azure scheduled jobs | Event-driven (Service Bus + KEDA)   | Seconds   | Same Service Bus as bravo-jobs |

## Failure Handling

### Retry Strategy

```yaml
retry:
  max_attempts: 3
  backoff: exponential # 5min, 15min, 45min
  on_failure: pause_agent # Don't retry indefinitely
  alert_after: 2 # Alert after 2 failures
```

### Common Failure Scenarios

| Failure               | Detection              | Recovery                                            |
| --------------------- | ---------------------- | --------------------------------------------------- |
| Server unreachable    | MCP tool returns error | ZeroClaw retries; agent skips task if persistent    |
| Auth token expired    | 401 from MCP           | MCP server refreshes token automatically            |
| Push conflict         | 409 from MCP           | Agent pulls latest, resolves, retries               |
| AI provider down      | LLM timeout            | ZeroClaw falls back per config; agent skips session |
| Agent container crash | Docker restart policy  | `restart: unless-stopped` auto-recovers             |
| Rate limit            | 429 from MCP           | Agent respects Retry-After; next heartbeat retries  |

### Container Restart Policy

Docker's restart policy handles agent crashes:

```yaml
# In docker-compose.yaml, each agent has:
restart: unless-stopped
```

If an agent container crashes, Docker restarts it automatically. The agent's cron and heartbeat resume from the daemon's schedule. Workspace state persists in mounted volumes.

## Timeline Simulation

### Walking Through Release History

For accelerated mode, a thin **release-walker** service (the only centralized component) sequences releases and signals the Developer agent:

```
Release Walker flow:
1. Get upstream tags: v3.0.0, v3.0.1, v3.1.0, ...
2. For each release:
   a. Merge upstream tag into fork
   b. Write marker file (.zeroclaw-release-ready) in fork workspace
   c. Developer agent's heartbeat detects marker → pushes to Bowrain
   d. Other agents process normally via their schedules/heartbeats
   e. Wait for activity feed to show completion (or timeout)
   f. Remove marker, advance to next release
3. Pace: configurable interval between releases (default: 2h)
```

The release-walker is optional — only started with `docker compose --profile accelerated up`. In real-time mode, the Developer agent's cron schedule handles upstream tracking naturally.

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

Project-level config defines what to localize and how — shared across agents via their workspace mounts:

```yaml
# config/projects/docusaurus.yaml
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

accelerated:
  start_release: v3.0.0
  end_release: latest
  release_interval_minutes: 120
```

### Agent Configuration (ZeroClaw config.toml)

Each agent's behavior is configured in its `config.toml` and `SOUL.md`:

```toml
# agents/jeanpierre-fr/config.toml (same config locally and in Azure)
[llm]
default_provider = "custom"
default_model = "claude-sonnet-4-5-20250514"

[providers.custom]
name = "azure-claude"
base_url = "https://bowrain-foundry-d.services.ai.azure.com/v1"
api_key_env = "AZURE_AI_FOUNDRY_KEY"

[security]
allowed_commands = []    # Translator: API-only, no shell

[mcp]
[mcp.bowrain]
transport = "http"
url = "${BRAVO_MCP_ENDPOINT}"
headers = { Authorization = "Bearer ${BRAVO_AGENT_TOKEN}" }

[daemon]
[daemon.cron]
"translate-batch" = "0 14 * * 1-5"
```

Behavioral parameters (acceptance rate, blocks per session, communication style) live in `SOUL.md` as natural language instructions rather than configuration values. This is more flexible — the LLM interprets and applies them contextually.

**Same Azure AI config everywhere.** API keys work locally and in Azure — no
environment-specific overlays needed. See `04-implementation.md` for the provider matrix
(Azure OpenAI for simple agents, Azure AI Foundry/Claude for translators/brand).

### Environment Configuration

```bash
# .env — same keys work locally and in Azure
AZURE_OPENAI_API_KEY=...              # Developer, PM, QA (GPT-4o-mini/4o)
AZURE_AI_FOUNDRY_KEY=...              # Translators, Brand Manager (Claude Sonnet)
BOWRAIN_URL=http://bowrain-server:8080
KEYCLOAK_URL=http://keycloak:8080
```

## Concurrency Model

### Natural Serialization via Scheduling

Because each agent runs on its own cron schedule, conflicts are avoided by design:

```
Timeline (Docusaurus):
09:00  Alex (Developer) pushes     — sole writer at this time
10:00  Maria (Brand) reviews terms — read-heavy, no conflicts
10:00  Lisa (PM) creates tasks     — parallel with Brand Manager
14:00  Jean-Pierre (fr) translates — parallel with other translators
14:00  Katrin (de) translates      — parallel with other translators
20:00  Yuki (ja) translates        — different timezone, no overlap
17:00  Alex (Developer) pulls      — sole writer at this time
```

Push/pull operations are naturally serialized because only the Developer agent does them, and they run at distinct cron times.

### Cross-Project Parallelism

Each agent container is independent. Agents working on different projects run fully in parallel with zero coordination:

```
Docusaurus agents ──────────────────▶ (independent containers)
Gitea agents ───────────────────────▶ (independent containers)
Home Assistant agents ──────────────▶ (independent containers)
Excalidraw agents ──────────────────▶ (independent containers)
```

### Resource Limits

- Docker resource limits per container (CPU, memory)
- Anthropic API rate limits handled by provider SDK
- Max 1 git operation per fork (enforced by single Developer agent per project)
- Bowrain server handles concurrent API calls natively
