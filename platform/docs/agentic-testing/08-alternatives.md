# Alternatives & Decision Log

## Architecture Decisions

### AD-1: Agent Implementation Language

**Decision:** TypeScript hybrid (Option C) for Phase 1-3, evaluate Go migration for Phase 5+.

| Option | Pros | Cons |
|--------|------|------|
| **A: Pure Go** | Same lang as Bowrain, shared types, single binary | Weaker LLM SDK, no Playwright, slower iteration |
| **B: Pure TypeScript** | Existing e2e infra, Playwright, rich AI SDKs | Separate from Go codebase, type drift |
| **C: TypeScript Hybrid** ✓ | Best of both: reuse e2e infra + LLM SDKs + Playwright | Two languages, subprocess overhead for CLI |
| **D: Python** | Strongest AI/ML ecosystem, LangChain/AutoGen | Third language, no existing infra, type safety concerns |

**Rationale:** The existing e2e infrastructure (`platform/e2e/shared/`) provides a BowrainAPI client, auth flows, and Playwright setup. Reusing this saves weeks. LLM SDKs in TypeScript are mature. CLI calls are infrequent enough that subprocess overhead is negligible.

**Revisit when:** If agent logic becomes complex enough to benefit from Go's type system and Bowrain's internal APIs, consider migrating the core orchestrator to Go while keeping Playwright in TypeScript.

---

### AD-2: LLM Provider for Agent Decision-Making

**Decision:** Claude (Anthropic) via Claude API / Anthropic SDK.

| Option | Pros | Cons |
|--------|------|------|
| **Anthropic Claude** ✓ | Best tool-use, long context, high quality | Cost at scale |
| **OpenAI GPT-4** | Mature ecosystem, function calling | Less reliable tool use |
| **Local LLM (Ollama)** | Free, private, no rate limits | Lower quality decisions, slower |
| **Mixed: Claude for decisions, Ollama for translation review** | Cost optimization | Complexity, inconsistent quality |

**Rationale:** Agent decisions (terminology review, translation critique, task creation) require high reasoning quality. Claude's tool-use is the most reliable for structured outputs. Cost is manageable with session budgets.

**Cost optimization for later:** Use Claude Haiku for simple accept/reject decisions, Sonnet for complex reviews, Opus only for brand strategy.

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

**Decision:** SQLite for orchestrator state, Bowrain server for platform state.

| Option | Pros | Cons |
|--------|------|------|
| **SQLite** ✓ | Zero infrastructure, portable, ACID | Single-node only |
| **PostgreSQL** | Scalable, shared across instances | Infra overhead, overkill for orchestrator |
| **JSON files** | Simplest possible | No ACID, corruption risk, slow queries |
| **Redis** | Fast, pub/sub for events | Volatile, another dependency |
| **Git repo as state** | Version-controlled, auditable | Slow, merge conflicts |

**Rationale:** The orchestrator runs on a single node. SQLite provides ACID guarantees, zero dependencies, and easy backup (copy the file). Platform state (projects, translations, TM) lives in Bowrain's own database — the orchestrator only tracks its own scheduling/agent state.

---

### AD-5: Scheduling Model

**Decision:** Cron + event-driven hybrid with dependency chains.

| Option | Pros | Cons |
|--------|------|------|
| **Pure cron** | Simple, predictable | Can't react to events, wasteful polling |
| **Pure event-driven** | Responsive, efficient | Complex, needs event bus infrastructure |
| **Hybrid** ✓ | Cron for regular tasks, events for reactions | Two mechanisms to maintain |
| **Manual/on-demand** | Zero automation overhead | Not "agentic", requires human triggers |

**Rationale:** Some tasks are naturally periodic (check upstream daily, review terminology weekly). Others are reactive (translate after push, QA after translation). The hybrid model handles both naturally.

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

## Alternative Architectures Considered

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

**Verdict:** These frameworks add abstraction we don't need. Our agents have well-defined tools (Bowrain API, CLI, git) and well-defined workflows (push → translate → pull). A custom lightweight orchestrator gives more control with less overhead. Reconsider if agent interaction patterns become significantly more complex.

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

**Verdict:** Good supplement for the Developer Agent's CI-focused tasks. Not suitable as the primary runtime for all agents due to cold starts and limited compute.

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
