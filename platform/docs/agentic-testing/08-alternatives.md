# Alternatives & Decision Log

## Architecture Decisions

### AD-1: Agent Runtime

**Decision:** ZeroClaw containers (one per persona) with a shared Bowrain MCP server.

| Option | Pros | Cons |
|--------|------|------|
| **A: Pure Go orchestrator** | Same lang as Bowrain, shared types | Must build scheduling, identity, tool dispatch from scratch |
| **B: Pure TypeScript orchestrator** | Existing e2e infra, Playwright, rich AI SDKs | Must build scheduling, identity, tool dispatch from scratch |
| **C: TypeScript Hybrid** | Best of both worlds | Still building a custom orchestrator |
| **D: Python (AutoGen/CrewAI)** | Strongest AI/ML ecosystem | Third language, heavy, opinionated frameworks |
| **E: ZeroClaw containers** ✓ | Built-in daemon/cron/heartbeat, SOUL.md identity, MCP native, <5MB RAM, container isolation | Rust binary (can't extend in Go/TS), newer project |

**Rationale:** ZeroClaw provides scheduling (daemon mode + cron), identity (SOUL.md), tool integration (MCP), and isolation (containers) out of the box. This eliminates the need for a custom orchestrator, scheduler, state manager, and event router. The main investment shifts to building a Bowrain MCP server — which has standalone value beyond the agentic testing system.

**Trade-off accepted:** Agent logic lives in SOUL.md (natural language) rather than code. This is less testable but more flexible — persona tuning is a markdown edit, not a code change.

**Revisit when:** If ZeroClaw's capabilities prove limiting (e.g., can't handle complex multi-step tool chains), consider supplementing with custom TypeScript agents for specific roles while keeping ZeroClaw for the majority.

---

### AD-2: LLM Provider for Agent Decision-Making

**Decision:** Mixed Azure AI providers — GPT-4o/mini (Azure OpenAI) for simple tasks, Claude Sonnet (Azure AI Foundry) for translation review and brand reasoning.

| Option | Pros | Cons |
|--------|------|------|
| **Direct Anthropic Claude** | Best tool-use, simplest setup | Separate billing, data leaves Azure |
| **Azure OpenAI only** | Already deployed, single provider | GPT weaker at multilingual review |
| **Azure AI Foundry Claude only** | Best quality, single provider | Overkill for simple git/task ops |
| **Mixed: Azure OpenAI + Azure AI Foundry** ✓ | Right model for each task, consolidated Azure billing, data residency | Two providers to configure |
| **Local LLM (Ollama)** | Free, private, no rate limits | Lower quality decisions, slower |

**Rationale:** The Azure OpenAI resource has `disableLocalAuth: true` (managed-identity-only), so local docker-compose can't use it. This naturally leads to a split: local dev uses Google Gemini (good quality, cheap, existing API key), while Azure deployment uses managed identity to access both Azure OpenAI and Azure AI Foundry. The SOUL.md, MCP tools, and scheduling are identical — only the `[llm]` provider block differs per environment.

In Azure, simple agent tasks use the already-deployed GPT-4o-mini (cheap), while translation and brand agents use Claude Sonnet via Azure AI Foundry (stronger multilingual). All data stays in Sweden Central (EU compliance), billing is consolidated, and managed identity eliminates key management.

**Local:** All agents → Gemini 2.5 Flash (or Ollama for free iteration)
**Azure model tier mapping:**
- GPT-4o-mini → Developer, simple decisions (~$0.15/1M tokens)
- GPT-4o → PM, QA (~$2.50/1M tokens)
- Claude Sonnet 4.5 → Translators, Brand Manager (~$3/1M tokens)

---

### AD-3: Agent Interaction with Bowrain

**Decision:** Mix of REST API (primary) and Web UI via Playwright (for visual workflows + screenshots).

| Option | Pros | Cons |
|--------|------|------|
| **A: API only** | Fast, reliable, easy to test | No screenshots, misses UI bugs |
| **B: Web UI only** | Most realistic, catches UI issues | Slow, brittle, expensive |
| **C: Mixed** ✓ | API for data ops, UI for visual workflows | More complex, two code paths |
| **D: CLI only** | Simplest, tests CLI thoroughness | Misses server/web entirely |

**Rationale:** The API is the reliable backbone — all state mutations go through it. The Web UI is used selectively for: brand profile editing, translation editor interaction, dashboard screenshots. This catches UI bugs while maintaining reliability.

**Rule of thumb:**
- Mutation + needs to be fast → API
- Visual workflow + screenshot needed → Web UI
- Git/file operations → CLI

---

### AD-4: State Management

**Decision:** ZeroClaw workspace files + Bowrain platform state. No separate state database.

| Option | Pros | Cons |
|--------|------|------|
| **SQLite (custom)** | ACID, queryable | Must build; another component to maintain |
| **ZeroClaw workspace** ✓ | Built-in, persists in mounted volumes | Less structured than SQL |
| **Bowrain as sole state** ✓ | Authoritative for platform state | Can't track orchestrator-specific state |
| **Redis** | Fast, pub/sub | Volatile, another dependency |

**Rationale:** With ZeroClaw, agent state lives in the workspace directory (mounted Docker volume). Platform state (projects, translations, TM, tasks, activities) lives in Bowrain's own database. There's no need for a separate orchestrator state database because there's no orchestrator.

---

### AD-5: Scheduling Model

**Decision:** ZeroClaw daemon cron + heartbeat polling (no central scheduler).

| Option | Pros | Cons |
|--------|------|------|
| **Central cron scheduler** | Single point of control | Must build; single point of failure |
| **Central event bus** | Responsive, efficient | Complex infrastructure |
| **ZeroClaw daemon cron** ✓ | Built-in per agent, zero custom code | No central visibility of schedules |
| **Heartbeat polling** ✓ | Agents discover events naturally via activity feed | Slightly delayed reaction (poll interval) |

**Rationale:** ZeroClaw's daemon mode provides cron scheduling natively. "Event-driven" coordination is replaced by heartbeat polling of Bowrain's activity feed — more realistic (humans check dashboards) and requires zero custom event infrastructure. The poll delay (1-2h heartbeat) is a feature, not a bug — it creates natural pacing.

---

### AD-6: Fork Management Strategy

**Decision:** Full GitHub fork with `bowrain-main` tracking branch.

See `02-project-candidates.md` → "Fork Strategy" for detailed analysis.

**Key reason:** Full forks enable authentic git workflows (branches, PRs, commit history) that partial approaches lose. This matters for demo authenticity and for testing the full Bowrain → git → CI pipeline.

---

### AD-7: Translation Strategy (AI vs. Pseudo vs. Real)

**Decision:** AI translation with LLM review (production mode). Pseudo-translation as dry-run/budget fallback.

| Option | Pros | Cons |
|--------|------|------|
| **Pseudo-translation only** | Free, deterministic, fast | Not real translations, can't evaluate quality |
| **AI translation (unreviewed)** | Fast, cheap-ish | Quality varies, no human-like review process |
| **AI translation + LLM review** ✓ | Realistic workflow, quality feedback loop | Higher cost (two LLM calls per block) |
| **Real human translators** | Highest quality, most authentic | Expensive, not scalable, defeats "agentic" purpose |

**Rationale:** The whole point is demonstrating AI-assisted translation with human-like review. The LLM reviewer adds cost but creates the most compelling workflow — it catches AI mistakes, builds TM deliberately, and generates realistic activity patterns (accept/edit/reject decisions).

**Budget mode:** Fall back to AI translation without LLM review. Still builds TM and generates activity, just less sophisticated decision-making.

---

## Alternative Architectures Considered (Superseded by ZeroClaw)

### Alt-A: Claude Code as the Agent Runtime

Instead of building a custom orchestrator, use Claude Code itself as the agent runtime:

```
Claude Code session → reads project config → uses bowrain CLI → interacts with server
```

**Pros:**
- No custom code to build — Claude Code IS the agent
- Tool use is native (bash, file editing, web fetch)
- Can be invoked via cron (new session each time)
- Prompt-driven behavior changes are instant

**Cons:**
- Each session starts fresh (no persistent state beyond memory files)
- Expensive (full Claude Code session per agent invocation)
- Limited browser automation (no Playwright equivalent)
- Harder to coordinate multiple agents

**Verdict:** Interesting for Phase 0 prototyping. A single Claude Code session could run the Developer Agent tasks (CLI + git) very naturally. Not suitable for the full multi-agent system due to coordination and cost.

### Alt-B: Agent SDK (Anthropic)

Use Anthropic's Agent SDK (if/when available) for structured agent orchestration:

**Pros:**
- First-party agent lifecycle management
- Built-in tool use, memory, handoffs
- Likely optimized for Claude

**Cons:**
- SDK maturity uncertain
- May not support the specific Bowrain tool integrations needed
- Lock-in to Anthropic

**Verdict:** Worth evaluating when the SDK matures. The custom orchestrator can be refactored to use the Agent SDK as its foundation if it provides sufficient flexibility.

### Alt-C: Multi-Agent Framework (AutoGen, CrewAI, LangGraph)

Use an existing multi-agent framework:

| Framework | Strengths | Weaknesses |
|-----------|-----------|------------|
| **AutoGen** | Mature, multi-agent conversation | Python-only, heavy, opinionated |
| **CrewAI** | Role-based agents, task delegation | Python-only, less control over tools |
| **LangGraph** | Graph-based workflows, state machines | Complex, Python-heavy |
| **Mastra** | TypeScript, good tool integration | Newer, less proven |

**Verdict:** These frameworks add abstraction we don't need. ZeroClaw's daemon mode with MCP integration provides the right level of agent autonomy without the overhead of a multi-agent conversation framework. Our agents have well-defined tools (Bowrain MCP) and well-defined workflows (push → translate → pull).

### Alt-D: GitHub Actions as Agent Runtime

Each agent is a GitHub Actions workflow:

```yaml
# .github/workflows/translator-fr.yml
on:
  schedule:
    - cron: "0 14 * * 1-5"
  workflow_dispatch:

jobs:
  translate:
    runs-on: ubuntu-latest
    steps:
      - uses: neokapi/setup-bowrain@v1
      - run: node agents/translator.js --locale fr-FR --project docusaurus
```

**Pros:**
- Free compute (GitHub-hosted runners)
- Built-in scheduling (cron)
- Logs and artifacts for debugging
- Natural fit with bowrain-action

**Cons:**
- Cold start each run (no persistent state without artifacts)
- Rate limited (2000 minutes/month free tier)
- No browser automation without extra setup
- Slow feedback loop (minutes to start each workflow)

**Verdict:** Good supplement for the Developer Agent's CI-focused tasks. Not suitable as the primary runtime for all agents due to cold starts and limited compute. With ZeroClaw, agents run as persistent daemons — GitHub Actions could still be used for the CI-specific workflows (setup-bowrain + bowrain-action) that the Developer agent configures.

---

### Alt-E: Custom TypeScript Orchestrator (Original Plan)

The original v1 of this plan proposed a custom TypeScript orchestrator with:
- Custom scheduler (cron + event-driven)
- Custom agent runtime (BaseAgent class with LLM integration)
- Custom state management (SQLite)
- Custom event router
- Playwright browser automation for Web UI agents

**Verdict:** Superseded by ZeroClaw. The custom orchestrator required building scheduling, identity management, tool dispatch, state persistence, and failure handling from scratch. ZeroClaw provides all of these as built-in features of the runtime. The Bowrain MCP server (which we need to build regardless) replaces the custom API wrappers. The only custom code that remains is the release-walker for accelerated mode — which is a thin, optional coordinator rather than a full orchestrator.

**What's preserved:** The agent personas (SOUL.md files) carry forward directly from the original prompt templates. The MCP tool definitions mirror the original tool schemas. The project configurations are unchanged. The scheduling patterns (cron times per agent) are identical — they just live in `config.toml` instead of a custom scheduler.

---

## Open Questions

### Q1: Should agents use real email addresses?

**Options:**
- `agent+alex@bowrain.io` — real addresses, receive notifications
- Keycloak test users — managed programmatically, no real email
- Shared test account — single account with multiple display names

**Current lean:** Keycloak test users (consistent with existing e2e pattern). Each agent gets their own Keycloak user with unique display name.

### Q2: How to handle upstream breaking changes?

When an upstream project restructures their i18n (e.g., renames locale files, changes JSON structure):

**Options:**
- Manual intervention (pause agents, fix config, resume)
- Agent detects and reports (creates issue, pauses itself)
- Auto-adapt (LLM analyzes the change and updates config)

**Current lean:** Agent detects and reports. Auto-adaptation is ambitious but worth exploring in Phase 5+.

### Q3: Should translated files be contributed back upstream?

**Options:**
- No (keep forks separate, this is testing infrastructure)
- Yes (contribute translations to upstream projects)
- Optional (maintainer can choose)

**Current lean:** No for now. Contributing back adds coordination overhead and may not be welcome. Revisit if translation quality is consistently high and upstream projects express interest.

### Q4: How many concurrent projects before diminishing returns?

**Hypothesis:** 4 projects (Tier 1) provide sufficient format/scale diversity. Each additional project adds cost without proportional insight.

**Plan:** Validate with Tier 1, measure marginal value of each Tier 2 addition.

### Q5: Real-time collaboration testing?

Bowrain supports real-time collaborative editing. Should agents simulate this?

**Options:**
- Two translator agents editing the same file simultaneously
- Sequential editing with handoff
- Skip (not critical for initial demo)

**Current lean:** Skip for Phase 1-4. Add in Phase 5+ as a dedicated stress test.
