# Agentic Testing System for Bowrain

## Vision

Build a long-running, multi-agent testing system that **impersonates real localization teams** working on **real open source projects** through the Bowrain platform. Unlike traditional e2e tests that execute scripted assertions in seconds, this system operates over days, weeks, and months — generating authentic activity feeds, translation memory, terminology databases, and brand profiles that showcase Bowrain as a living platform.

The system forks/mirrors active open source projects and manages their localization entirely through Bowrain, ignoring any existing translation infrastructure (Crowdin, Weblate, Transifex). Each project gets a team of AI agent personas who collaborate through the platform exactly as human teams would.

## Goals

1. **Demonstrate Bowrain at scale** — Real projects, real formats, real translation workflows running continuously
2. **Generate authentic activity** — Activity feeds, dashboards, and metrics that show genuine platform usage
3. **Stress-test the platform** — Concurrent agents pushing/pulling/translating across multiple projects
4. **Showcase format breadth** — JSON, Markdown, HTML, YAML, INI, XML, MDX across different projects
5. **Build reusable assets** — Translation memories, termbases, and brand profiles that grow organically
6. **Validate the full workflow** — CLI → Server → Web UI → CI/CD → back to repo
7. **Regression detection** — Agents surface platform bugs, API changes, and UX friction naturally
8. **Marketing/demo material** — Screenshots, recordings, and live dashboards from real activity

## Architecture at a Glance

Each agent persona runs as an independent **ZeroClaw** container — a lightweight Rust-based AI agent runtime (~3.4MB binary, &lt;5MB RAM). Agents interact with Bowrain through the Bravo MCP server (24 tools, already built) and coordinate through the platform.

**Two deployment models, identical personas:**
- **Local (docker-compose):** ZeroClaw daemons with cron + heartbeat polling, optionally supplemented by Redis pub/sub for instant handoffs (Redis is already in the platform compose stack for Bravo SSE relay)
- **Azure (Container Apps Jobs):** Scheduled + event-driven jobs via KEDA/Service Bus — pay only for execution time, instant handoffs

**We are not building a new messaging system.** The agentic testing system adds a thin adapter to the existing ChannelEventBus that forwards events to Redis (local) or Service Bus (Azure) — the same infrastructure Bravo already uses for SSE relay and job queuing.

```
┌──────────────────── docker-compose ────────────────────────┐
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │  ZeroClaw     │  │  ZeroClaw     │  │  ZeroClaw     │   │
│  │  Alex Chen    │  │  Maria Santos │  │  Jean-Pierre  │   │
│  │  (Developer)  │  │  (Brand Mgr)  │  │  (fr-FR)      │   │
│  │  SOUL.md      │  │  SOUL.md      │  │  SOUL.md      │   │
│  │  cron: 9am    │  │  cron: MWF    │  │  cron: 2pm    │   │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘     │
│         │ MCP              │ MCP              │ MCP         │
│  ┌──────▼──────────────────▼─────────────────▼──────────┐  │
│  │              Bowrain MCP Server                        │  │
│  │  (exposes push/pull/translate/termbase/brand/tasks)    │  │
│  └──────┬───────────────────────────────────────────────┘  │
│         │                                                   │
│  ┌──────▼───────────────────────────────────────────────┐  │
│  │              Bowrain Platform                          │  │
│  │  (Server + ChannelEventBus + Web + CLI)               │  │
│  │         │                                              │  │
│  │   ChannelEventBus (50+ event types)                   │  │
│  │         │                                              │  │
│  │   Queue Sink Adapter                                   │  │
│  │     ├── Redis pub/sub (local) ─── agentic:* channels  │  │
│  │     └── Service Bus (Azure) ───── KEDA triggers       │  │
│  └──────┬───────────────────────────────────────────────┘  │
│         │                                                   │
│  ┌──────▼───────────────────────────────────────────────┐  │
│  │         Forked Open Source Projects                    │  │
│  │  (Docusaurus, Gitea, Home Assistant, etc.)            │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Document Index

| Document | Description |
|----------|-------------|
| [01-agent-personas.md](01-agent-personas.md) | Agent roles, behaviors, prompts, and interaction patterns |
| [02-project-candidates.md](02-project-candidates.md) | Open source projects to fork/mirror, evaluation criteria |
| [03-orchestration.md](03-orchestration.md) | Scheduling, pacing, timeline simulation, state management |
| [04-implementation.md](04-implementation.md) | ZeroClaw containers, Bowrain MCP server, infrastructure |
| [05-activity-visualization.md](05-activity-visualization.md) | Dashboards, feeds, metrics, and demo material |
| [06-evaluation-quality.md](06-evaluation-quality.md) | Translation quality assessment, platform health metrics |
| [07-rollout-phases.md](07-rollout-phases.md) | Phased rollout plan from MVP to full system |
| [08-alternatives.md](08-alternatives.md) | Alternative approaches, trade-offs, and decision log |
| [09-agent-routines.md](09-agent-routines.md) | Detailed daily/weekly routines, GitHub Issues, email communication |
| [10-persona-evolution.md](10-persona-evolution.md) | Tuning loop, diagnosis, metrics-driven evolution, scaling |

## Key Design Principles

- **Authenticity over speed** — Agents behave like humans with realistic timing, not bots that blast through workflows
- **Observable by default** — Every agent action generates platform activity that's visible in dashboards
- **Idempotent and resumable** — The system can be stopped and restarted without losing state
- **Composable agents** — Each persona is an independent ZeroClaw container; add/remove agents by editing docker-compose
- **Real projects, real formats** — No synthetic test data; everything comes from actual open source codebases
- **Progressive complexity** — Start with one project and three agents; scale to many projects and dozens of agents
