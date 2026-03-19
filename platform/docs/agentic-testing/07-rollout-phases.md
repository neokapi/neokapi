# Rollout Phases

## Phase 0: Validation & Foundation (Week 1-2)

**Goal:** Validate ZeroClaw daemon mode works with the existing Bravo MCP server (PR #43),
then get the first agent persona running against a real project.

The Bowrain MCP server (24 tools) is already built via PR #43 (Bravo / AD-028). Phase 0
focuses on **connecting ZeroClaw agents to it** and proving the agentic testing concept.

### Prerequisites (validate before building anything)

- [ ] **PR #43 merged:** Bravo MCP server is deployed and functional on local dev stack
- [ ] **ZeroClaw smoke test:** Run a single ZeroClaw daemon for 48h connecting to the
      Bowrain MCP endpoint (`/mcp/`) with a valid agent token. Confirm: cron fires
      reliably, HTTP transport stays connected, no memory leaks, daemon survives
      container restarts. If this fails, the plan needs a different agent runtime.
- [ ] **MCP tool coverage check:** Verify the 24 existing Bravo MCP tools cover the
      agentic testing workflows. Key tools to validate:
      - `connector_pull` / `connector_push` — can these sync git-based content?
      - `update_block` — can this submit per-block translations?
      - `run_flow` — does an AI translation flow exist (not just pseudo-translate)?
      File tickets for any gaps.

### Deliverables

- [ ] Fork Excalidraw (smallest Tier 1 candidate) to `neokapi/agentic-excalidraw`
- [ ] Set up local Bowrain server + Keycloak via docker-compose (reuse existing `compose.yaml`)
- [ ] Create Keycloak users for agent personas (automate via `keycloak-admin.ts`)
- [ ] Create Developer Agent workspace (config.toml + SOUL.md), using Gemini locally
- [ ] Connect ZeroClaw daemon to Bowrain's `/mcp/` endpoint with agent JWT token
- [ ] Developer Agent: `connector_pull` → content in dashboard → `connector_push`

### What's NOT included

- No AI translation quality decisions (agent executes tools per SOUL.md —
  but LLM inference runs via Gemini to interpret instructions, so
  Phase 0 has non-zero AI cost)
- No scheduling (manual `zeroclaw agent` invocation)
- Single project, single language (fr-FR)
- No custom MCP tools (uses existing 24 Bravo tools only)

### Success Criteria

- ZeroClaw agent can call Bravo MCP tools via HTTP transport
- Content synced via `connector_pull`/`connector_push` appears in dashboard
- Agent can `list_blocks`, `update_block` to submit a translation
- ZeroClaw daemon ran stable for 48h in smoke test

---

## Phase 1: Multi-Agent Team (Week 3-5)

**Goal:** Multiple ZeroClaw agents running with distinct personas, using existing Bravo MCP tools.

### Deliverables

- [ ] Build standalone email MCP server (`agentic/email-mcp/`) wrapping Mailpit SMTP/API
- [ ] Add Mailpit + email-mcp to docker-compose
- [ ] Configure `gh` CLI + `GITHUB_TOKEN` for agents that file issues
- [ ] Create Translator Agent workspace (Jean-Pierre, fr-FR)
- [ ] Create Brand Manager Agent workspace (Maria Santos)
- [ ] Docker-compose with all three agents running in daemon mode
- [ ] Cron schedules active — agents self-schedule
- [ ] Add second project (Docusaurus), second language (de-DE)

### Agent Capabilities at This Phase

| Agent | ZeroClaw Container | Bravo MCP Tools Used |
|-------|-------------------|----------------------|
| Developer | alex-developer | connector_pull/push, create_version, list_streams, execute_script (git) |
| Brand Manager | maria-brand | term_add, term_search, check_vocabulary, list_profiles, get_voice_guide |
| Translator (fr) | jeanpierre-fr | list_blocks, get_block, update_block, tm_search, run_flow |
| Translator (de) | katrin-de | list_blocks, get_block, update_block, tm_search, run_flow |

### Success Criteria

- Complete push → brand review → translate → pull cycle runs with multiple agents
- Agents wake on their cron schedules and do real work autonomously
- Terminology appears in termbase, translations build TM
- Two projects running, two languages each

---

## Phase 2: Full Team & Heartbeat Coordination (Week 6-8)

**Goal:** Full agent team with heartbeat-based coordination. System runs autonomously.

### Deliverables

- [ ] PM Agent workspace (Lisa Chen) — creates tasks, monitors progress
- [ ] QA Agent workspace (Taylor Kim) — runs quality checks
- [ ] HEARTBEAT.md for all agents — poll-based event discovery
- [ ] Per-agent auth (separate Keycloak users per agent)
- [ ] Add Gitea (INI format — breadth test)
- [ ] Container restart policies and health monitoring
- [ ] Basic log aggregation (docker compose logs)

### Operating Patterns

```
Automated daily schedule (all via ZeroClaw daemon cron):
09:00  Alex checks upstream, pushes if changes
10:00  Lisa reviews dashboard, creates tasks (discovers Alex's push)
10:00  Maria reviews new terminology
14:00  Jean-Pierre translates French batch
14:00  Katrin translates German batch
~every 2h  Taylor runs QA checks (heartbeat discovers completed translations)
17:00  Alex pulls completed translations
```

### Success Criteria

- System runs for 1 week via `docker compose up -d` without intervention
- 7 agent containers running, each with distinct schedule
- Activity feed shows realistic multi-persona collaboration
- Three projects active: Excalidraw, Docusaurus, Gitea

---

## Phase 3: Scale & Quality (Week 9-12)

**Goal:** Add Japanese translator, then Home Assistant. Quality benchmarking.

Add CJK and scale complexity **sequentially** so failures can be attributed to one
variable at a time.

### Deliverables (Week 9-10: Japanese)

- [ ] Japanese translator (Yuki Tanaka, ja-JP) — tests CJK handling
- [ ] QA Agent gets CJK-specific checks (encoding, character limits, display width)
- [ ] Agent personality variation via SOUL.md tuning
- [ ] LLM-based translation quality evaluation

### Deliverables (Week 11-12: Home Assistant + Benchmarking)

- [ ] Add Home Assistant Frontend — **scoped to core UI strings only** (~2000 keys,
      not all 10,000+ integration strings). Full volume would take 300+ days at
      30 blocks/session; the subset is completable and produces meaningful progress.
- [ ] Benchmark against existing community translations (Docusaurus, Gitea)
- [ ] Screenshot capture via periodic Playwright jobs (separate from agents)

### Team at This Phase

```
Per project (Docusaurus example):
├── Alex Chen (Developer)           — ZeroClaw + git MCP tools
├── Maria Santos (Brand Manager)    — ZeroClaw + brand/termbase MCP tools
├── Lisa Chen (PM)                  — ZeroClaw + task MCP tools
├── Jean-Pierre Dubois (fr-FR)      — ZeroClaw + translate MCP tools
├── Katrin Weber (de-DE)            — ZeroClaw + translate MCP tools
├── Yuki Tanaka (ja-JP)             — ZeroClaw + translate MCP tools
└── Taylor Kim (QA)                 — ZeroClaw + brand check MCP tools
```

### Success Criteria

- Four projects running with full agent teams
- Japanese translations pass CJK-specific QA checks
- Home Assistant scoped subset shows meaningful progress (not stuck at 5%)
- Quality scores tracked and improving over time
- System has been running 4+ weeks with authentic activity history

---

## Phase 4: Azure Deployment & Accelerated Mode (Week 13-16)

**Goal:** Deploy to Azure as Container Apps Jobs with event-driven handoffs. Walk through
release history to build months of activity.

### Deliverables

- [ ] **Early validation:** Confirm `zeroclaw agent -m` (single-shot mode) works with
      Bravo MCP tools — agent processes task, calls tools, exits cleanly.
- [ ] **Early validation:** Confirm ZeroClaw can authenticate to Azure OpenAI via managed
      identity bearer token.
- [ ] Provision Azure AI Foundry endpoint for Claude Sonnet (serverless)
- [ ] Create `agent-job.bicep` module — Container Apps Jobs (scheduled + event-driven)
- [ ] Create Service Bus queues for agent handoffs (`content-pushed`, `tasks-created-{locale}`,
      `translation-complete`, `qa-passed`)
- [ ] Add Service Bus adapter to Bowrain's event bus (publish selected events to queues)
- [ ] Config overlays: `config.azure-dev.toml` per agent
- [ ] Deploy agent job fleet to `rg-bowrain-d-sdc` (dev environment)
- [ ] Verify event-driven handoff chain: push → tasks → translate → QA → pull
- [ ] Release walker as a scheduled job (accelerated mode)
- [ ] Stream creation per major version
- [ ] Hybrid mode: accelerated backfill → switch to real-time
- [ ] TM growth tracking and visualization

### Demo Flow

```
Day 1-3 (accelerated):  Walk Docusaurus v2.0 → v3.5 (20 releases)
Day 4+  (real-time):    Agents track upstream via normal cron schedules
Result: Dashboard shows 6+ months of "activity" with authentic metrics
```

### Success Criteria

- Release walker processes 20+ releases without errors
- TM grows visibly through accelerated history
- Quality scores improve measurably over simulated time

---

## Phase 5: Public Demo & Polish (Week 17-20)

**Goal:** Production-ready system with public-facing demo site.

### Deliverables

- [ ] Deploy agent fleet to Azure dev (`rg-bowrain-d-sdc`) via Container Apps
      (agents target `dev.bowrain.cloud`)
- [ ] Standalone activity dashboard (React app) deployed to Azure Static Web Apps
- [ ] Public demo site at `agents.dev.bowrain.cloud` (read-only)
- [ ] Agent profiles page showing each persona and their activity
- [ ] Metrics dashboard (quality, cost, throughput)
- [ ] Blog-ready case studies per project
- [ ] Documentation: "Run your own agentic testing setup" guide (local docker-compose)

### Success Criteria

- Public demo site live with 4 projects, 12+ agents
- Months of visible activity history
- Local dev reproducible via `docker compose up -d`
- Azure prod running on Container Apps with managed identity

---

## Phase 6: Expansion & Community (Week 21+)

**Goal:** Scale to Tier 2 projects. Open source the agent configurations.

### Deliverables

- [ ] Add Tier 2 projects (Excalidraw, Immich, Cal.com, Grafana)
- [ ] Cross-project TM sharing (workspace-level memory)
- [ ] Community-contributed SOUL.md personas and project configs
- [ ] Publish Bowrain MCP server as a standalone package
- [ ] Chaos engineering: randomly stop/restart agent containers
- [ ] Integration with Bowrain's upcoming features (real-time collab, etc.)

---

## Resource Requirements

### Compute

| Phase | Infra | Estimated Cost |
|-------|-------|---------------|
| 0-2 | Local machine (docker compose) | $0 |
| 3-4 | Local + cloud VM for Bowrain | ~$50/mo |
| 5+ | Cloud VM (4 CPU, 8GB RAM — ample for 20 ZeroClaw containers + Bowrain) | ~$100/mo |

### AI Spend

**Cost model assumptions:** Each translator session involves ~4-7 LLM calls per block
(list tasks, AI translate, list concepts, list TM, review decision, submit, optionally
add TM). At 30 blocks/session, that's 120-210 LLM calls. Developer/PM/QA sessions are
lighter (10-30 calls). Gemini 2.5 Flash is ~$0.15/1M input tokens locally; Azure Claude
Sonnet is ~$3/1M input tokens.

| Phase | Agents | Sessions/Day | Est. Daily Cost (Gemini local) | Est. Daily Cost (Azure mixed) |
|-------|--------|-------------|-------------------------------|-------------------------------|
| 0 | 1 | 2 | ~$0.50 | N/A (local only) |
| 1 | 4 | 8 | ~$2-4 | N/A (local only) |
| 2 | 8 | 16 | ~$4-8 | N/A (local only) |
| 3 | 20 | 40 | ~$8-15 | N/A (local only) |
| 4 (accel) | 20 | 100+ | ~$20-40 | ~$40-80 (Azure mixed) |
| 5+ | 20 | 40 | N/A | ~$15-30 (Azure mixed) |

Note: Previous estimates ($5-25/day) were optimistic. Real cost depends on block length
and model choice. Gemini Flash locally is 10-20x cheaper than Azure Claude Sonnet.
Monitor actual spend from Phase 1 and adjust.

### Human Time

| Phase | Effort | Focus |
|-------|--------|-------|
| 0 | 1 week | Validate ZeroClaw + Bravo MCP, first agent workspace, smoke test |
| 1 | 1-2 weeks | Multi-agent docker-compose, 5 new MCP tools (github + email) |
| 2 | 1-2 weeks | Full team SOUL.md files, heartbeat tuning, Keycloak user automation |
| 3 | 2 weeks | Scale testing, quality benchmarking, SOUL.md refinement |
| 4 | 1-2 weeks | Azure deployment, release walker, accelerated mode |
| 5 | 2-3 weeks | Dashboard, demo site, polish |
| 6+ | Ongoing | Expansion, new personas and projects |

Note: Phase 0 is dramatically faster because the Bravo MCP server (PR #43) already
provides 24 tools. The main Phase 0 work is validating ZeroClaw daemon mode and writing
the first SOUL.md persona — not building infrastructure. The overall timeline compresses
by ~2 weeks compared to the pre-Bravo plan.

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| AI costs spiral | Daily budget caps, cost-per-session limits, Gemini Flash (cheap) for local |
| Agents get stuck in loops | Circuit breaker after 3 failures, session timeouts |
| Bowrain server instability | Health checks, graceful degradation, retry with backoff |
| Upstream repo changes break fork | Automated merge conflict detection, manual resolution queue |
| Translation quality too low | Increase human-eval frequency, tighten brand/term enforcement |
| System complexity overwhelming | Phase gates — each phase must be stable before proceeding |
| Auth token management | Token refresh automation, long-lived API tokens as fallback |
| ZeroClaw daemon instability (local) | 48h smoke test in Phase 0; Azure uses Jobs (no daemon) |
| Bowrain API gaps (missing endpoints) | REST API audit in Phase 0 prerequisites; file tickets before building MCP |
| MCP transport version mismatch | Pin ZeroClaw + MCP SDK versions; validate Streamable HTTP in smoke test |
| Azure managed identity auth failure | Validate ZeroClaw + Azure OpenAI token auth early in Phase 4 |
| Home Assistant volume too large | Scoped to core UI subset (~2000 keys); expand only if throughput allows |
