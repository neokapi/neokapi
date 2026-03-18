# Rollout Phases

## Phase 0: Foundation (Week 1-2)

**Goal:** Build the Bowrain MCP server and prove one ZeroClaw agent can interact with Bowrain.

### Deliverables

- [ ] Fork Tolgee (smallest Tier 1 candidate) to `bowrain-l10n/tolgee-platform`
- [ ] Set up local Bowrain server + Keycloak via docker-compose
- [ ] Build **Bowrain MCP server** with core tools (push, pull, status, listActivities)
- [ ] Create Developer Agent workspace (config.toml + SOUL.md), using Gemini locally
- [ ] Run ZeroClaw container connected to Bowrain MCP
- [ ] Developer Agent: `bowrain.push` → content appears in dashboard → `bowrain.pull`

### What's NOT included

- No LLM decision-making (agent uses tools directly per SOUL.md instructions)
- No scheduling (manual `zeroclaw agent` invocation)
- Single project, single language (fr-FR)
- MCP server has minimal tool coverage

### Success Criteria

- ZeroClaw agent can call Bowrain MCP tools successfully
- Content pushed to Bowrain appears in web dashboard
- Pseudo-translations can be pulled back

---

## Phase 1: Multi-Agent + MCP Expansion (Week 3-5)

**Goal:** Multiple ZeroClaw agents running, MCP server covers full workflow.

### Deliverables

- [ ] Expand MCP server: translate, addConcept, listConcepts, createBrandProfile, createTask, listTasks, addTMEntry
- [ ] Add git MCP tools (checkUpstream, merge, commit, push)
- [ ] Create Translator Agent workspace (Jean-Pierre, fr-FR)
- [ ] Create Brand Manager Agent workspace (Maria Santos)
- [ ] Docker-compose with all three agents running in daemon mode
- [ ] Cron schedules active — agents self-schedule
- [ ] Add second project (Docusaurus), second language (de-DE)

### Agent Capabilities at This Phase

| Agent | ZeroClaw Container | MCP Tools Used |
|-------|-------------------|----------------|
| Developer | alex-developer | bowrain.push/pull, git.* |
| Brand Manager | maria-brand | bowrain.addConcept, bowrain.createBrandProfile |
| Translator (fr) | jeanpierre-fr | bowrain.listTasks, bowrain.translate, bowrain.addTMEntry |
| Translator (de) | katrin-de | bowrain.listTasks, bowrain.translate, bowrain.addTMEntry |

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
10:00  Maria reviews new terminology
08:00  Lisa reviews dashboard, creates tasks (heartbeat discovers pushes)
14:00  Jean-Pierre translates French batch
14:00  Katrin translates German batch
~every 2h  Taylor runs QA checks (heartbeat discovers completed translations)
17:00  Alex pulls completed translations
```

### Success Criteria

- System runs for 1 week via `docker compose up -d` without intervention
- 7 agent containers running, each with distinct schedule
- Activity feed shows realistic multi-persona collaboration
- Three projects active: Tolgee, Docusaurus, Gitea

---

## Phase 3: Scale & Quality (Week 9-12)

**Goal:** Add remaining Tier 1 projects, Japanese translator, quality benchmarking.

### Deliverables

- [ ] Add Home Assistant Frontend (large scale test)
- [ ] Japanese translator (Yuki Tanaka, ja-JP) — tests CJK handling
- [ ] Agent personality variation via SOUL.md tuning
- [ ] LLM-based translation quality evaluation
- [ ] Benchmark against existing community translations
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
- Quality scores tracked and improving over time
- System has been running 4+ weeks with authentic activity history
- 20+ agent containers running on &lt;500MB total RAM

---

## Phase 4: Azure Deployment & Accelerated Mode (Week 13-16)

**Goal:** Deploy to Azure infra with managed identity. Walk through release history.

### Deliverables

- [ ] Provision Azure AI Foundry endpoint for Claude Sonnet (serverless)
- [ ] Create `containerapp-agent.bicep` module (agents as Container Apps with managed identity)
- [ ] Config overlays: `config.azure-dev.toml` per agent (Azure OpenAI for simple, Foundry/Claude for complex)
- [ ] Deploy agent fleet to `rg-bowrain-d-sdc` (dev environment)
- [ ] Verify managed identity auth to both Azure OpenAI and Azure AI Foundry
- [ ] Release walker service (thin coordinator, `docker compose --profile accelerated`)
- [ ] Stream creation per major version
- [ ] Hybrid mode: accelerated backfill → switch to real-time
- [ ] TM growth tracking and visualization
- [ ] Time-lapse video generation from periodic screenshots

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

- [ ] Deploy full docker-compose stack to a cloud VM
- [ ] Standalone activity dashboard (React app)
- [ ] Public demo site at `demo.bowrain.io` (read-only)
- [ ] Agent profiles page showing each persona and their activity
- [ ] Metrics dashboard (quality, cost, throughput)
- [ ] Blog-ready case studies per project
- [ ] Documentation: "Run your own agentic testing setup" guide

### Success Criteria

- Public demo site live with 4 projects, 12+ agents
- Months of visible activity history
- `docker compose up -d` reproduces the entire setup

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

| Phase | Agents | Sessions/Day | Est. Daily Cost |
|-------|--------|-------------|-----------------|
| 0 | 1 | 2 | $0 (no LLM) |
| 1 | 4 | 8 | ~$5 |
| 2 | 8 | 16 | ~$10 |
| 3 | 20 | 40 | ~$25 |
| 4 (accel) | 20 | 100+ | ~$50 |
| 5+ | 20 | 40 | ~$25 |

### Human Time

| Phase | Effort | Focus |
|-------|--------|-------|
| 0 | 1-2 weeks | **Bowrain MCP server** (main new code), first ZeroClaw agent |
| 1 | 2 weeks | MCP tool expansion, multi-agent docker-compose |
| 2 | 1-2 weeks | Full team SOUL.md files, heartbeat tuning, auth setup |
| 3 | 2 weeks | Scale testing, quality benchmarking, SOUL.md refinement |
| 4 | 1-2 weeks | Release walker, accelerated mode |
| 5 | 2-3 weeks | Dashboard, demo site, polish |
| 6+ | Ongoing | Expansion, new personas and projects |

Note: Phases 1-3 are faster than the original plan because ZeroClaw eliminates the need to build a custom orchestrator, scheduler, and agent runtime. The main development effort is the Bowrain MCP server (Phase 0) and SOUL.md persona writing (Phase 1-2).

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| AI costs spiral | Daily budget caps, cost-per-session limits, pseudo-translation fallback |
| Agents get stuck in loops | Circuit breaker after 3 failures, session timeouts |
| Bowrain server instability | Health checks, graceful degradation, retry with backoff |
| Upstream repo changes break fork | Automated merge conflict detection, manual resolution queue |
| Translation quality too low | Increase human-eval frequency, tighten brand/term enforcement |
| System complexity overwhelming | Phase gates — each phase must be stable before proceeding |
| Auth token management | Token refresh automation, long-lived API tokens as fallback |
