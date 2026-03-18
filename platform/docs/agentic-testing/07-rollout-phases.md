# Rollout Phases

## Phase 0: Foundation (Week 1-2)

**Goal:** Prove the concept with a single project, single language, minimal agents.

### Deliverables

- [ ] Fork Tolgee (smallest Tier 1 candidate) to `bowrain-l10n/tolgee-platform`
- [ ] Set up local Bowrain server + Keycloak
- [ ] Create `neokapi/agentic/` package with base infrastructure
- [ ] Implement Developer Agent (CLI wrapper + git ops only, no LLM)
- [ ] Developer Agent: `bowrain init` → `bowrain add` → `bowrain push` → `bowrain pull`
- [ ] Verify: content appears in Bowrain dashboard, translations can be pulled

### What's NOT included

- No LLM decision-making (hardcoded actions)
- No browser automation
- No scheduling (manual invocation)
- Single project, single language (fr-FR)

### Success Criteria

- Developer agent can push Tolgee's translatable content to Bowrain
- Content appears in the web dashboard
- Pseudo-translations can be pulled back and committed to the fork

---

## Phase 1: Core Agent Loop (Week 3-5)

**Goal:** Add LLM-powered translation and brand management. Complete the basic handoff chain.

### Deliverables

- [ ] Integrate Anthropic SDK for agent decision-making
- [ ] Implement Translator Agent with LLM review of AI translations
- [ ] Implement Brand Manager Agent with terminology creation
- [ ] Implement basic orchestrator (sequential task execution)
- [ ] Add SQLite state management
- [ ] Wire up event routing (push → translate → pull cycle)
- [ ] Add second project (Docusaurus)
- [ ] Add second language (de-DE)

### Agent Capabilities at This Phase

| Agent | Capabilities |
|-------|-------------|
| Developer | Push, pull, git operations |
| Brand Manager | Create brand profile, add terminology concepts |
| Translator (fr) | Review AI translations, edit, accept/reject |
| Translator (de) | Review AI translations, edit, accept/reject |

### Success Criteria

- Complete push → brand review → translate → pull cycle runs end-to-end
- Translations are genuinely reviewed by LLM (not just accepted blindly)
- Terminology appears in termbase, used by translator agents
- Two projects running, two languages each

---

## Phase 2: Scheduling & Persistence (Week 6-8)

**Goal:** System runs autonomously on a schedule. Agents have work patterns.

### Deliverables

- [ ] Cron-based scheduler with jitter
- [ ] Event-driven triggers (content_pushed → create tasks)
- [ ] Agent work windows (configurable active hours)
- [ ] Session persistence (agents resume from where they left off)
- [ ] Error handling with retry and circuit breaker
- [ ] PM Agent implementation (task creation, progress monitoring)
- [ ] Cost tracking and daily budget enforcement
- [ ] Add Gitea (INI format — breadth test)

### Operating Patterns

```
Automated daily schedule:
09:00 ± 30min  Developer checks upstream, pushes if changes
10:00 ± 30min  Brand Manager reviews new terminology
10:30           PM creates translation tasks (event-driven)
14:00 ± 30min  French translator works on batch
15:00 ± 30min  German translator works on batch
```

### Success Criteria

- System runs for 1 week without manual intervention
- Agents respect work windows and pacing controls
- State survives restarts (stop → start → agents pick up where they left off)
- Cost stays within daily budget
- Three projects active: Tolgee, Docusaurus, Gitea

---

## Phase 3: Browser Automation & Full Personas (Week 9-12)

**Goal:** Agents use the Web UI for visual workflows. Full team composition.

### Deliverables

- [ ] Playwright browser automation layer
- [ ] Brand Manager uses brand dashboard (visual profile editing)
- [ ] Translator uses translation editor (visual block editing)
- [ ] PM uses task board and dashboard
- [ ] QA Agent implementation (quality checks, bug reporting)
- [ ] Screenshot capture on key events
- [ ] Add Home Assistant Frontend (large scale test)
- [ ] Add Japanese translator (ja-JP) — tests CJK handling
- [ ] Agent personality variation (speed, precision, communication style)

### Team at This Phase

```
Per project (Docusaurus example):
├── Alex Chen (Developer)           — CLI + git
├── Maria Santos (Brand Manager)    — Web UI + API
├── Lisa Chen (PM)                  — Web UI
├── Jean-Pierre Dubois (fr-FR)      — Web UI editor
├── Katrin Weber (de-DE)            — Web UI editor
├── Yuki Tanaka (ja-JP)             — Web UI editor
└── Taylor Kim (QA)                 — CLI + API
```

### Success Criteria

- Agents interact through the Web UI with captured screenshots
- Screenshots show realistic translation editor activity
- QA agent catches and reports real issues
- Four projects running with full teams
- System has been running 4+ weeks with authentic activity history

---

## Phase 4: Accelerated Mode & History (Week 13-16)

**Goal:** Walk through release history to build months of activity quickly.

### Deliverables

- [ ] Release walker: iterate through tagged releases chronologically
- [ ] Stream creation per major version
- [ ] Pacing controls (releases per day, wait-for-completion)
- [ ] Hybrid mode: accelerated backfill → real-time tracking
- [ ] TM growth tracking and visualization
- [ ] Translation quality benchmarking against existing community translations
- [ ] Time-lapse video generation from screenshots

### Demo Flow

```
Day 1-3 (accelerated):  Walk Docusaurus v2.0 → v3.5 (20 releases)
Day 4+  (real-time):    Track upstream main, process changes as they arrive
Result: Dashboard shows 6+ months of "activity" with authentic metrics
```

### Success Criteria

- Release walker processes 20+ releases without errors
- TM grows visibly through accelerated history
- Quality scores improve measurably over simulated time
- Time-lapse video shows compelling dashboard evolution

---

## Phase 5: Public Demo & Polish (Week 17-20)

**Goal:** Production-ready system with public-facing demo site.

### Deliverables

- [ ] Deploy orchestrator as a container (Docker)
- [ ] Standalone activity dashboard (React app)
- [ ] Public demo site at `demo.bowrain.io` (read-only)
- [ ] Per-project pages with embedded Bowrain views
- [ ] Agent profiles page
- [ ] Metrics dashboard (quality, cost, throughput)
- [ ] Blog-ready case studies per project
- [ ] Comparison reports (Bowrain agents vs. community translations)
- [ ] Documentation for running your own agentic testing setup

### Success Criteria

- Public demo site live with 4 projects, 12+ agents
- Months of visible activity history
- Compelling metrics: TM growth, cost savings, quality improvement
- Marketing team can use screenshots/recordings in content

---

## Phase 6: Expansion & Community (Week 21+)

**Goal:** Scale to Tier 2 projects. Explore community participation.

### Deliverables

- [ ] Add Tier 2 projects (Excalidraw, Immich, Cal.com, Grafana)
- [ ] Cross-project TM sharing (workspace-level memory)
- [ ] Agent personality randomization (different behavior each run)
- [ ] Chaos engineering: introduce artificial delays, failures
- [ ] Open source the agentic testing framework
- [ ] Community-contributed project configs
- [ ] Integration with Bowrain's upcoming features (real-time collab, etc.)

---

## Resource Requirements

### Compute

| Phase | Infra | Estimated Cost |
|-------|-------|---------------|
| 0-2 | Local machine | $0 |
| 3-4 | Local + cloud Bowrain | ~$50/mo (server) |
| 5+ | Dedicated VM + Bowrain cloud | ~$200/mo |

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
| 0 | 1 week | Scaffolding, CLI wrapper |
| 1 | 2 weeks | LLM integration, agent logic |
| 2 | 2 weeks | Orchestrator, scheduling |
| 3 | 3 weeks | Browser automation, personas |
| 4 | 2 weeks | Release walker, benchmarking |
| 5 | 3 weeks | Dashboard, demo site, polish |
| 6+ | Ongoing | Expansion, maintenance |

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
