# Technical Implementation

## Architecture Decision: ZeroClaw Containers

Each agent persona runs as an independent **ZeroClaw** instance in its own Docker container. ZeroClaw is an ultra-lightweight Rust-based AI agent runtime (~8.8MB binary, &lt;5MB RAM) with built-in scheduling, MCP tool integration, and workspace-scoped identity files.

### Why ZeroClaw

| Concern             | Custom TS Orchestrator (Previous) | ZeroClaw Containers                       |
| ------------------- | --------------------------------- | ----------------------------------------- |
| Scheduling          | Custom cron + event router code   | Built-in daemon mode with cron            |
| Agent identity      | Prompt templates in TypeScript    | SOUL.md / IDENTITY.md files               |
| Tool integration    | Custom API wrappers per agent     | MCP native — declare once, use everywhere |
| State               | Custom SQLite state manager       | Workspace files + Bowrain platform state  |
| Scaling             | Code changes to add agents        | Add container to docker-compose           |
| Isolation           | Shared process, manual sandboxing | Container-level isolation by default      |
| Memory per agent    | ~100MB+ (Node.js)                 | &lt;5MB (Rust binary)                     |
| Total for 20 agents | ~2GB+                             | ~100MB                                    |

### What ZeroClaw Provides

- **SOUL.md** — Agent personality and instructions (our persona prompts)
- **Daemon mode** — Long-running with cron scheduler + heartbeat
- **MCP integration** — Connect to external MCP servers; tools appear native to the agent
- **Workspace scoping** — File access restricted to agent's workspace directory
- **Command allowlist** — Only explicitly allowed commands (git, bowrain) can execute
- **Provider support** — 22+ providers including Anthropic, OpenAI, and any OpenAI-compatible endpoint (Azure OpenAI, Azure AI Foundry)
- **Encrypted secrets** — API keys encrypted at rest
- **Hot-reloadable config** — Change provider/model without restart

## System Architecture

```
┌──────────────────── docker-compose.yaml ──────────────────────┐
│                                                                │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  bowrain-server  (platform + Bravo MCP at /mcp/)         │  │
│  │  keycloak        (OIDC authentication)                   │  │
│  │  24 tools: content, brand, flows, TM, connectors, sandbox│  │
│  └─────────────────────────┬───────────────────────────────┘  │
│                             │ HTTP + Bearer JWT                 │
│         ┌───────────────────┼───────────────────┐              │
│         │                   │                   │              │
│  ┌──────▼──────┐  ┌────────▼────────┐  ┌──────▼──────┐      │
│  │ alex-dev    │  │ maria-brand     │  │ jp-fr       │      │
│  │ ZeroClaw    │  │ ZeroClaw        │  │ ZeroClaw    │      │
│  │ (Developer) │  │ (Brand Manager) │  │ (Translator)│      │
│  └─────────────┘  └─────────────────┘  └─────────────┘      │
│  ┌─────────────┐  ┌─────────────────┐  ┌─────────────┐      │
│  │ katrin-de   │  │ lisa-pm         │  │ taylor-qa   │      │
│  │ ZeroClaw    │  │ ZeroClaw        │  │ ZeroClaw    │      │
│  │ (Translator)│  │ (PM)            │  │ (QA)        │      │
│  └─────────────┘  └─────────────────┘  └─────────────┘      │
│                                                                │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  release-walker  (thin coordinator for accelerated mode) │  │
│  │  Only needed for release walkthrough; optional otherwise  │  │
│  └─────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

## Bowrain MCP Server (Already Built)

PR #43 (Bravo / AD-028) implements a comprehensive MCP server in `platform/server/mcp/`
with **24 tools** covering brand voice, content management, flows, TM/terminology,
connectors, and sandbox execution. This will be merged before agentic testing starts.

The agentic testing system **uses the existing Bravo MCP server directly** — no custom
MCP server is needed. Each ZeroClaw agent connects to the Bowrain server's `/mcp/`
endpoint with a per-agent JWT token, using the same infrastructure Bravo uses for
interactive conversations.

The only new MCP tools needed for agentic testing are `github.*` and `email.*` (5 tools),
which can be added to the existing `platform/server/mcp/` as new tool files.

## Repository Structure

```
neokapi/agentic/
├── docker-compose.yaml          # Full stack: Bowrain + agents
├── Makefile                     # Convenience targets
│
├── agents/                      # Agent workspace definitions
│   ├── shared/                  # Shared files across agents
│   │   └── AGENTS.md            # Team roster (all personas)
│   │
│   ├── alex-developer/          # Developer Agent workspace
│   │   ├── config.toml          # ZeroClaw config (provider, model, cron)
│   │   ├── SOUL.md              # Persona: Alex Chen, DevOps engineer
│   │   ├── HEARTBEAT.md         # Periodic check: "any upstream changes?"
│   │   └── workspace/           # Git fork mount point
│   │
│   ├── maria-brand/             # Brand Manager workspace
│   │   ├── config.toml
│   │   ├── SOUL.md              # Persona: Maria Santos, Head of Content
│   │   ├── HEARTBEAT.md         # Periodic: "any new terms to review?"
│   │   └── workspace/
│   │
│   ├── jeanpierre-fr/           # French Translator workspace
│   │   ├── config.toml
│   │   ├── SOUL.md              # Persona: Jean-Pierre Dubois
│   │   ├── HEARTBEAT.md         # Periodic: "any assigned tasks?"
│   │   └── workspace/
│   │
│   ├── katrin-de/               # German Translator workspace
│   │   ├── config.toml
│   │   ├── SOUL.md
│   │   ├── HEARTBEAT.md
│   │   └── workspace/
│   │
│   ├── yuki-ja/                 # Japanese Translator workspace
│   │   ├── config.toml
│   │   ├── SOUL.md
│   │   ├── HEARTBEAT.md
│   │   └── workspace/
│   │
│   ├── lisa-pm/                 # Project Manager workspace
│   │   ├── config.toml
│   │   ├── SOUL.md              # Persona: Lisa Chen, Program Manager
│   │   ├── HEARTBEAT.md         # Periodic: "check dashboard, any blockers?"
│   │   └── workspace/
│   │
│   └── taylor-qa/               # QA Specialist workspace
│       ├── config.toml
│       ├── SOUL.md              # Persona: Taylor Kim, QA Engineer
│       ├── HEARTBEAT.md
│       └── workspace/
│
├── config/                      # Project-level configuration
│   └── projects/
│       ├── docusaurus.yaml
│       ├── gitea.yaml
│       ├── home-assistant.yaml
│       └── excalidraw.yaml
│
├── email-mcp/                   # Standalone email MCP server (Mailpit wrapper)
│   ├── package.json
│   └── src/
│       └── index.ts             # email.send + email.listInbox via Mailpit SMTP/API
│
├── release-walker/              # Accelerated mode coordinator (thin)
│   ├── package.json
│   └── src/
│       └── index.ts             # Walk releases, trigger developer agents
│
└── dashboard/                   # Activity visualization (Phase 5)
    ├── package.json
    └── src/
        └── ...
```

## ZeroClaw Agent Configuration

### Example: Developer Agent (Alex Chen)

**`agents/alex-developer/config.toml`** (base — provider-agnostic):

```toml
[llm]
# Provider set by environment overlay (config.local.toml or config.azure-dev.toml)
# Azure OpenAI — same config works locally and in Azure (API key auth)
default_provider = "custom"
default_model = "gpt-4o-mini"

[providers.custom]
name = "azure-openai"
base_url = "https://oai-bowrain-d.openai.azure.com/openai/deployments/gpt-4o-mini/v1"
api_key_env = "AZURE_OPENAI_API_KEY"

[security]
allowed_commands = ["git", "gh", "bowrain", "ls", "cat", "diff"]

[mcp]
[mcp.bowrain]
transport = "http"
url = "${BRAVO_MCP_ENDPOINT}"
headers = { Authorization = "Bearer ${BRAVO_AGENT_TOKEN}" }
tool_timeout_secs = 120

[mcp.email]
transport = "http"
url = "http://email-mcp:3001/mcp"     # Standalone email MCP (agentic/email-mcp/)

[daemon]
# Check for upstream changes daily at 9am (with jitter handled by heartbeat)
[daemon.cron]
"check-upstream" = "0 9 * * 1-5"
"pull-translations" = "0 17 * * 1-5"
```

**`agents/alex-developer/SOUL.md`:**

```markdown
# Alex Chen — Senior DevOps Engineer

You are Alex Chen, a senior DevOps engineer responsible for the localization
infrastructure of open source projects managed through the Bowrain platform.

## Your Role

- Manage the Bowrain CLI integration and GitHub Actions workflows
- Push source content when upstream projects release new versions
- Pull completed translations and commit them to the fork
- Create Bowrain streams for major release branches
- Troubleshoot format issues and sync problems

## Your Working Style

- You prefer the CLI and scripts over the web UI
- You're methodical: check status before pushing, verify after pulling
- You write clear commit messages mentioning localization context
- You create streams for each major release branch
- You're responsive to upstream changes but don't rush

## Your Tools

You have access to the Bowrain MCP server with these tools:

- `bowrain.push` — Push source content to Bowrain
- `bowrain.pull` — Pull translated content from Bowrain
- `bowrain.sync` — Push then pull in one operation
- `bowrain.status` — Check sync state
- `bowrain.createStream` — Create a stream for a release
- `bowrain.listStreams` — See existing streams
- `bowrain.listActivities` — Check recent team activity
- `git.*` — Git operations (fetch, merge, commit, push, checkUpstream)

## Daily Routine

1. Check if upstream has new releases or significant changes
2. If changes found: merge upstream, then push to Bowrain
3. Check activity feed — have translators completed anything?
4. If translations are ready: pull and commit to the fork
5. Report any issues (format errors, sync failures)

## Current Projects

{project_list}
```

**`agents/alex-developer/HEARTBEAT.md`:**

```markdown
Check if there are upstream changes to process. Use `git.checkUpstream` for
each project. If changes are found, merge and push. Also check
`bowrain.listActivities` for any completed translation batches — if found,
pull translations and commit.
```

### Example: French Translator (Jean-Pierre Dubois)

**`agents/jeanpierre-fr/config.toml`** (base — provider-agnostic):

```toml
[llm]
# Azure AI Foundry (Claude) — same config works locally and in Azure (API key auth)
default_provider = "custom"
default_model = "claude-sonnet-4-5-20250514"

[providers.custom]
name = "azure-claude"
base_url = "https://bowrain-foundry-d.services.ai.azure.com/v1"
api_key_env = "AZURE_AI_FOUNDRY_KEY"

[security]
# Translator has no shell access — API only via MCP
allowed_commands = []

[mcp]
[mcp.bowrain]
transport = "http"
url = "${BRAVO_MCP_ENDPOINT}"
headers = { Authorization = "Bearer ${BRAVO_AGENT_TOKEN}" }
tool_timeout_secs = 120

[daemon]
[daemon.cron]
"translate-batch" = "0 14 * * 1-5"   # Weekday afternoons
```

**`agents/jeanpierre-fr/SOUL.md`:**

```markdown
# Jean-Pierre Dubois — French Translator

You are Jean-Pierre Dubois, a professional French translator working on
localization projects through the Bowrain platform. You translate from
English (en-US) to French (fr-FR).

## Your Role

- Review AI-generated translations for accuracy and fluency
- Edit translations that don't meet quality standards
- Add high-quality translations to Translation Memory
- Flag ambiguous source text or terminology issues
- Ensure brand voice compliance for French content

## Your Working Style

- You prefer formal register (vous over tu) for technical content
- You verify terminology against the project termbase before translating
- You add TM entries for translations you're especially confident about
- You flag ambiguous source text rather than guessing
- You review AI translations critically — you accept about 60% as-is,
  edit about 30%, and reject about 10%

## Your Tools

- `bowrain.listTasks` — See assigned translation tasks
- `bowrain.translate` — Submit your translation for a block
- `bowrain.aiTranslate` — Get AI translation suggestion for a file
- `bowrain.listConcepts` — Check termbase for correct terminology
- `bowrain.addTMEntry` — Add a translation to memory
- `bowrain.listTMEntries` — Look up existing translations
- `bowrain.listActivities` — Check recent team activity

## Translation Guidelines

- Technical terms: Check the termbase first. Use preferred terms only.
- Brand voice: Follow the project's brand profile for French.
- Placeholders: Never modify {variables}, %s, or {{tokens}}.
- Numbers and dates: Use French conventions (1 000, 31/12/2026).
- Gender: Default to masculine when the subject is ambiguous in tech docs.

## Daily Routine

1. Check `bowrain.listTasks` for assigned work
2. For each task:
   a. Get AI translation suggestion via `bowrain.aiTranslate`
   b. Review against termbase (`bowrain.listConcepts`)
   c. Accept, edit, or reject each block
   d. For excellent translations, add to TM (`bowrain.addTMEntry`)
3. Check `bowrain.listActivities` for any terminology changes
4. Process up to 30 blocks per session

## Quality Standards

- Accuracy: Must convey identical meaning to source
- Fluency: Must read naturally to a native French speaker
- Consistency: Same term → same translation throughout
- Completeness: All information preserved, nothing omitted
```

### Example: Brand Manager (Maria Santos)

**`agents/maria-brand/config.toml`** (base — provider-agnostic):

```toml
[llm]
# Azure AI Foundry (Claude) — same config works locally and in Azure (API key auth)
default_provider = "custom"
default_model = "claude-sonnet-4-5-20250514"

[providers.custom]
name = "azure-claude"
base_url = "https://bowrain-foundry-d.services.ai.azure.com/v1"
api_key_env = "AZURE_AI_FOUNDRY_KEY"

[security]
allowed_commands = []

[mcp]
[mcp.bowrain]
transport = "http"
url = "${BRAVO_MCP_ENDPOINT}"
headers = { Authorization = "Bearer ${BRAVO_AGENT_TOKEN}" }
tool_timeout_secs = 120

[daemon]
[daemon.cron]
"review-terminology" = "0 10 * * 1,3,5"   # Mon/Wed/Fri mornings
"brand-audit" = "0 10 * * 4"              # Thursday morning audit
```

**`agents/maria-brand/SOUL.md`:**

```markdown
# Maria Santos — Head of Content

You are Maria Santos, Head of Content for the localization projects.
You own the English brand voice and terminology across all projects.

## Your Role

- Maintain brand voice profiles per project
- Curate the termbase — add, update, deprecate terms
- Review content for brand compliance after translations
- Define channel-specific voice (technical, marketing, UI, community)
- Ensure terminology consistency across all target languages

## Your Tools

- `bowrain.listActivities` — See what content was recently pushed
- `bowrain.addConcept` — Add terminology concepts to the termbase
- `bowrain.listConcepts` — Review existing terminology
- `bowrain.createBrandProfile` — Create a brand voice profile
- `bowrain.checkBrand` — Check content against brand rules
- `bowrain.createTask` — Create tasks for translators when issues found
- `bowrain.listTasks` — Check outstanding tasks

## Daily Routine (Mon/Wed/Fri)

1. Check `bowrain.listActivities` for recent content pushes
2. Review new content for terminology candidates
3. Add new terms to termbase with definitions and status
4. Check brand compliance on recently translated content
5. Create tasks for translators if brand violations found

## Terminology Guidelines

- Every technical term must have a termbase entry
- Status: preferred (use this), approved (acceptable), deprecated (stop using)
- Include definition and domain (software, ui, marketing, legal)
- Consider all target languages when choosing terms
```

## Bowrain MCP Server

The single biggest piece of new work. This server wraps Bowrain's REST API as MCP tools, making them available to all ZeroClaw agents.

### MCP Tool Catalog (from PR #43 / Bravo)

PR #43 implements **24 MCP tools** in `platform/server/mcp/tools_*.go`. These are
already built — the agentic testing system uses them directly.

**Brand Voice (3 tools):**

| Tool               | Used By                   | Description                                        |
| ------------------ | ------------------------- | -------------------------------------------------- |
| `check_vocabulary` | Brand Manager, QA         | Validate text against brand terms, flag violations |
| `list_profiles`    | Brand Manager             | List brand voice profiles in workspace             |
| `get_voice_guide`  | Brand Manager, Translator | Formatted brand guide for LLM consumption          |

**Content Management (11 tools):**

| Tool             | Used By        | Description                                 |
| ---------------- | -------------- | ------------------------------------------- |
| `list_projects`  | PM, Developer  | List projects in workspace                  |
| `get_project`    | All            | Get project details                         |
| `create_project` | Developer      | Create a new project                        |
| `update_project` | Developer, PM  | Update project settings                     |
| `list_blocks`    | Translator, QA | List translatable blocks                    |
| `get_block`      | Translator     | Get block with source + targets             |
| `update_block`   | Translator     | Submit translation for a block (per locale) |
| `create_version` | Developer      | Create a new version/snapshot               |
| `list_streams`   | Developer, PM  | List content streams                        |
| `diff_streams`   | Developer, QA  | Compare two streams                         |
| `merge_stream`   | Developer      | Merge a stream into parent                  |

**Flows & Automation (3 tools):**

| Tool              | Used By                   | Description                                   |
| ----------------- | ------------------------- | --------------------------------------------- |
| `list_flows`      | Developer, QA             | List available flows (AI translate, QA, etc.) |
| `run_flow`        | Developer, Translator, QA | Execute a flow on project content             |
| `get_flow_status` | Developer                 | Check flow execution status                   |

**Translation Memory & Terminology (4 tools):**

| Tool          | Used By                   | Description                                   |
| ------------- | ------------------------- | --------------------------------------------- |
| `tm_search`   | Translator                | Search TM with fuzzy matching (min score 0.5) |
| `tm_import`   | Developer                 | Bulk import TM entries                        |
| `term_search` | Translator, Brand Manager | Search termbase with locale filters           |
| `term_add`    | Brand Manager             | Add new terminology concept                   |

**Connectors & Sync (3 tools):**

| Tool               | Used By       | Description                                   |
| ------------------ | ------------- | --------------------------------------------- |
| `connector_pull`   | Developer     | Fetch content from Git/CMS into project       |
| `connector_push`   | Developer     | Publish translations to external target       |
| `connector_status` | Developer, PM | Check sync state (last sync, pending, errors) |

**Sandbox (1 tool):**

| Tool             | Used By       | Description                                 |
| ---------------- | ------------- | ------------------------------------------- |
| `execute_script` | Developer, QA | Run Python/Bash/Node.js in isolated sandbox |

#### Non-Bowrain Tools (agentic testing infrastructure, NOT in bowrain-server)

These are not Bowrain platform features — they're agentic testing infrastructure handled
via ZeroClaw's native capabilities:

| Capability         | How                                          | Details                                                                     |
| ------------------ | -------------------------------------------- | --------------------------------------------------------------------------- |
| **GitHub Issues**  | `gh` CLI via `allowed_commands`              | Agents call `gh issue create`, `gh issue list`, `gh issue comment` directly |
| **Email**          | Standalone email MCP in `agentic/email-mcp/` | Lightweight Node.js MCP server wrapping Mailpit SMTP/API                    |
| **Git operations** | `git` CLI via `allowed_commands`             | Developer agent runs git directly in its workspace                          |

This keeps the Bowrain MCP server clean (platform tools only) while giving agents the
external communication channels they need.

**Key workflow mapping:**

- AI translation → use `run_flow` with an AI translation flow (not pseudo-translate)
- Push/pull content → use `connector_pull` / `connector_push` with Git connector
- Submit translation → use `update_block` to set target text per locale
- Git operations → `allowed_commands: ["git", "gh"]` in agent config.toml
- GitHub Issues → `gh issue create --repo neokapi/agent-feedback ...` via command execution
- Email → `email.send` / `email.listInbox` via standalone email MCP overlay

### Per-Agent Auth

Each ZeroClaw agent connects directly to the Bowrain server's MCP endpoint (`/mcp/`)
using a per-agent JWT token — the same auth mechanism Bravo uses for interactive
conversations.

**No MCP overlay needed.** The Bravo MCP server is built into bowrain-server itself
(`platform/server/mcp/`). Each agent connects to the same server with its own token.

```toml
# agents/alex-developer/config.toml
[mcp.bowrain]
transport = "http"
url = "${BRAVO_MCP_ENDPOINT}"          # e.g., http://bowrain-server:8080/mcp/
headers = { Authorization = "Bearer ${BRAVO_AGENT_TOKEN}" }
tool_timeout_secs = 120
```

This is the exact config template from `platform/docker/bravo-agent/config.toml.template`.
Agent tokens are workspace-scoped JWTs (30min TTL, auto-refreshed) created via
the Bravo conversation API.

**Key implication:** No per-agent MCP overlay containers. Each agent has ONE container
(ZeroClaw daemon) that connects directly to bowrain-server. This halves the container
count from 14 to 7.

## Docker Compose (Local Development)

The local docker-compose runs the full stack on your machine. Agents use the **same
Azure AI API keys** as the Azure deployment — no separate local provider needed.

The agent SOUL.md files, MCP tools, and scheduling are identical across environments.
Only the `[llm]` provider block in `config.toml` differs.

```yaml
# docker-compose.yaml — local development stack
services:
  # === Platform ===
  bowrain-server:
    image: bowrain/server:latest
    ports: ["8080:8080"]
    environment:
      DATABASE_URL: postgres://bowrain:bowrain@postgres:5432/bowrain
      KEYCLOAK_URL: http://keycloak:8080
      REDIS_URL: redis://redis:6379 # Same Redis used by Bravo SSE relay
      BOWRAIN_AGENT_RUNTIME: local # Queue sink adapter → Redis pub/sub
    depends_on: [postgres, keycloak, redis]

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: bowrain
      POSTGRES_PASSWORD: bowrain
      POSTGRES_DB: bowrain
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
    # Already present in platform compose.yaml for Bravo SSE relay.
    # Agentic events use agentic:* channels on the same instance.

  keycloak:
    image: quay.io/keycloak/keycloak:latest
    ports: ["8180:8080"]
    environment:
      KEYCLOAK_ADMIN: admin
      KEYCLOAK_ADMIN_PASSWORD: admin
    command: start-dev

  # === Optional: Ollama for zero-cost local dev ===
  ollama:
    image: ollama/ollama:latest
    profiles: ["ollama"]
    ports: ["11434:11434"]
    volumes:
      - ollama-data:/root/.ollama

  # === Agents ===
  # Each agent connects directly to bowrain-server's /mcp/ endpoint
  # using a per-agent JWT token (same auth as Bravo interactive sessions).
  # No MCP overlay needed — the MCP server is built into bowrain-server.

  alex-developer:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      AZURE_OPENAI_API_KEY: ${AZURE_OPENAI_API_KEY}
      BRAVO_MCP_ENDPOINT: http://bowrain-server:8080/mcp/
      BRAVO_AGENT_TOKEN: ${ALEX_AGENT_TOKEN}
      GITHUB_TOKEN: ${GITHUB_TOKEN:-}
    volumes:
      - ./agents/alex-developer:/root/.zeroclaw
      - ./forks:/root/.zeroclaw/workspace
    depends_on: [bowrain-server]

  jeanpierre-fr:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      AZURE_AI_FOUNDRY_KEY: ${AZURE_AI_FOUNDRY_KEY}
      BRAVO_MCP_ENDPOINT: http://bowrain-server:8080/mcp/
      BRAVO_AGENT_TOKEN: ${JEANPIERRE_AGENT_TOKEN}
    volumes:
      - ./agents/jeanpierre-fr:/root/.zeroclaw
    depends_on: [bowrain-server]

  # ... same pattern for: maria-brand, katrin-de, yuki-ja, lisa-pm, taylor-qa
  # Total: 7 agent containers + platform services (no overlays)

  # === Optional: Release Walker ===
  release-walker:
    build: ./release-walker
    profiles: ["accelerated"]
    environment:
      BOWRAIN_URL: http://bowrain-server:8080
    volumes:
      - ./forks:/app/forks
      - ./config:/app/config
    depends_on: [bowrain-server]

volumes:
  pgdata:
  ollama-data:
```

**Environment variables (`.env`):**

```bash
AZURE_OPENAI_API_KEY=...              # For Developer, PM, QA agents (GPT-4o-mini/4o)
AZURE_AI_FOUNDRY_KEY=...              # For Translator, Brand Manager agents (Claude Sonnet)
GITHUB_TOKEN=ghp_...                  # For gh CLI (filing issues)
ALEX_AGENT_TOKEN=...                  # Per-agent Bowrain JWT tokens
JEANPIERRE_AGENT_TOKEN=...            # (created via Bravo conversation API
MARIA_AGENT_TOKEN=...                 #  or Keycloak user setup)
KATRIN_AGENT_TOKEN=...
YUKI_AGENT_TOKEN=...
LISA_AGENT_TOKEN=...
TAYLOR_AGENT_TOKEN=...
```

## Event Coordination

ZeroClaw agents are autonomous — they self-schedule via cron and heartbeat. Cross-agent
coordination can happen through two mechanisms:

1. **Heartbeat polling** (simple) — Agents poll Bowrain's activity feed. More realistic
   (humans check dashboards, not webhooks) but has 1-2h latency. Works without Redis.
2. **Redis pub/sub** (instant) — Agents subscribe to `agentic:*` Redis channels for
   immediate handoffs. Uses the same Redis instance that Bravo already uses for SSE relay.

Both mechanisms coexist. An agent can use heartbeat polling as a fallback while also
subscribing to Redis channels for faster reaction when Redis is available.

### How Handoffs Work

```
Developer pushes content (cron: 9am)
  → Activity: "Alex Chen pushed 142 blocks"

PM checks activity feed (cron: 10am)
  → Sees new push → creates translation tasks
  → Activity: "Lisa Chen created 4 tasks"

Translator checks tasks (cron: 2pm)
  → Sees assigned tasks → translates batch
  → Activity: "Jean-Pierre translated 28 blocks"

QA checks activity feed (heartbeat: every 2h)
  → Sees translation batch → runs quality checks
  → Activity: "Taylor Kim: QA passed"

Developer checks activity feed (cron: 5pm)
  → Sees QA passed → pulls translations → commits
```

### HEARTBEAT.md Pattern

Each agent's `HEARTBEAT.md` defines what to check on each heartbeat cycle:

```markdown
# Heartbeat Check (runs every 2 hours)

1. Call `bowrain.listActivities` with since=last_check
2. If any "content_pushed" events: I have new work to review
3. If any "terminology_updated" events: check my translations for affected terms
4. If any "task_assigned" events where assignee is me: process immediately
5. Update last_check timestamp
```

ZeroClaw's daemon runs heartbeat at a configurable interval (default varies; we'd set it to 1-2 hours).

## Release Walker (Accelerated Mode)

The one component that remains a thin custom service. It walks through release history and triggers the Developer agent to process each release.

```typescript
// release-walker/src/index.ts
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

interface ReleaseConfig {
  upstream: string; // e.g., "facebook/docusaurus"
  forkPath: string; // e.g., "/app/forks/docusaurus"
  startRelease: string; // e.g., "v3.0.0"
  endRelease: string; // e.g., "latest"
  intervalMinutes: number; // e.g., 120
}

async function walkReleases(config: ReleaseConfig) {
  // 1. Get release tags
  const { stdout } = await execFileAsync("git", ["tag", "--list", "v*", "--sort=version:refname"], {
    cwd: config.forkPath,
  });

  const tags = stdout
    .trim()
    .split("\n")
    .filter((t) => t >= config.startRelease);

  for (const tag of tags) {
    console.log(`Processing release: ${tag}`);

    // 2. Merge upstream to this tag
    await execFileAsync("git", ["merge", `upstream/${tag}`, "--no-edit"], {
      cwd: config.forkPath,
    });

    // 3. Signal the Developer agent by writing a marker file
    // (Developer's heartbeat checks for this file)
    await writeFile(
      `${config.forkPath}/.zeroclaw-release-ready`,
      JSON.stringify({ tag, timestamp: new Date().toISOString() }),
    );

    // 4. Wait for all agents to process this release
    await waitForCompletion(config, tag);

    // 5. Pace
    await sleep(config.intervalMinutes * 60 * 1000);
  }
}
```

Alternatively, the release walker can use `zeroclaw agent -m` to send a one-shot message to the Developer agent container, triggering an immediate push.

## Local Development

```bash
# Prerequisites
brew install docker
cargo install zeroclaw   # For local testing outside Docker

# Configure Azure AI keys (same keys work locally and in Azure)
cat > .env << 'EOF'
AZURE_OPENAI_API_KEY=...
AZURE_AI_FOUNDRY_KEY=...
GITHUB_TOKEN=ghp_...
EOF

# Start the full stack
cd neokapi/agentic
docker compose up -d

# Or use Ollama for zero-cost iteration (override provider in config.toml)
docker compose --profile ollama up -d

# === Common commands ===
docker compose logs -f alex-developer       # Watch agent logs
docker compose logs -f jeanpierre-fr
docker compose run --rm alex-developer agent # Interactive session (chat with Alex)
docker compose --profile accelerated up -d   # Accelerated release walkthrough
docker compose down                          # Stop everything
```

## Adding a New Agent

Adding a new persona is pure configuration — no code changes:

1. Create workspace directory: `agents/new-agent/`
2. Write `config.toml` (provider, model, cron schedule, MCP connection)
3. Write `SOUL.md` (persona, tools, routines, guidelines)
4. Write `HEARTBEAT.md` (what to check periodically)
5. Add service to `docker-compose.yaml` (copy an existing agent, change volume mount)
6. Create Keycloak user for the agent
7. `docker compose up -d new-agent`

No TypeScript, no Go, no compilation. The agent runtime (ZeroClaw) and tools (Bowrain MCP) are shared infrastructure.

## Adding a New Project

1. Fork the upstream repo to `bowrain-l10n/project-name`
2. Clone into `forks/project-name`
3. Create `config/projects/project-name.yaml` with content paths and languages
4. Update agent SOUL.md files to include the new project
5. Run `kapi init` in the fork (one-time setup)

## Model Provider Strategy

### Azure AI with API Keys

Azure AI services (Azure OpenAI + Azure AI Foundry) support access keys, so the **same
provider config works everywhere** — local docker-compose and Azure Container Apps Jobs.
No environment-specific overlays needed.

Each agent's `config.toml` points to Azure AI endpoints with an API key from `.env`.
The same config runs locally and in Azure.

### Provider Matrix

| Agent                          | Task Complexity                  | Model             | Azure Service    | Est. Cost     |
| ------------------------------ | -------------------------------- | ----------------- | ---------------- | ------------- |
| Developer (Alex)               | Low — push/pull/git              | GPT-4o-mini       | Azure OpenAI     | ~$0.15/1M tok |
| PM (Lisa)                      | Medium — task creation           | GPT-4o            | Azure OpenAI     | ~$2.50/1M tok |
| QA (Taylor)                    | Medium — quality checks          | GPT-4o            | Azure OpenAI     | ~$2.50/1M tok |
| Brand Manager (Maria)          | High — terminology, brand        | Claude Sonnet 4.5 | Azure AI Foundry | ~$3/1M tok    |
| Translators (JP, Katrin, Yuki) | Medium-High — translation review | Claude Sonnet 4.5 | Azure AI Foundry | ~$3/1M tok    |

### Config Examples

**Developer (GPT-4o-mini via Azure OpenAI):**

```toml
# agents/alex-developer/config.toml
[llm]
default_provider = "custom"
default_model = "gpt-4o-mini"

[providers.custom]
name = "azure-openai"
base_url = "https://oai-bowrain-d.openai.azure.com/openai/deployments/gpt-4o-mini/v1"
api_key_env = "AZURE_OPENAI_API_KEY"
```

**Translator (Claude Sonnet via Azure AI Foundry):**

```toml
# agents/jeanpierre-fr/config.toml
[llm]
default_provider = "custom"
default_model = "claude-sonnet-4-5-20250514"

[providers.custom]
name = "azure-claude"
base_url = "https://bowrain-foundry-d.services.ai.azure.com/v1"
api_key_env = "AZURE_AI_FOUNDRY_KEY"
```

**Ollama (optional, zero-cost local iteration):**

```toml
# Override for free local dev
[llm]
default_provider = "ollama"
default_model = "llama3.1:8b"
```

### Environment Variables

```bash
# .env — same keys work locally and in Azure
AZURE_OPENAI_API_KEY=...           # For Developer, PM, QA agents
AZURE_AI_FOUNDRY_KEY=...           # For Translator, Brand Manager agents
```

### Azure Infrastructure

**Already provisioned** (from `bowrain-infra/modules/openai.bicep`):

- Azure OpenAI: `oai-bowrain-d` in Sweden Central
- GPT-4o: capacity 30 (dev)
- GPT-4o-mini: capacity 60 (dev)
- API key access enabled

**New resource needed:**

- Azure AI Foundry workspace + Claude Sonnet serverless deployment
- Deploy via portal initially, codify in Bicep later

### Azure Deployment: Container Apps Jobs

In Azure, agents run as **Container Apps Jobs** — not always-on containers. Each agent
session starts, does work, and stops. You pay only for execution time, not 24/7 uptime.

This uses the same Container Apps Environment already deployed for bowrain-server.
Agents authenticate to Azure AI via API keys (same keys as local dev).

**Two job trigger types:**

| Trigger          | Agents                                | How It Works                                                                         |
| ---------------- | ------------------------------------- | ------------------------------------------------------------------------------------ |
| **Scheduled**    | Developer (push), Brand Manager       | Azure-managed cron expression. Fires on schedule, runs ZeroClaw session, exits.      |
| **Event-driven** | PM, Translators, QA, Developer (pull) | KEDA scaler monitors Azure Service Bus queue depth. New message → new job execution. |

**Why Jobs, not always-on Container Apps:**

- 7 agents × 24h = 168 container-hours/day if always-on. With Jobs, 7 agents × ~2h
  active/day = ~14 container-hours/day. **~12x cheaper.**
- ZeroClaw cold start is under 10ms (3.4MB binary) — negligible for jobs that run minutes.
- Azure manages scheduling and retries natively — no ZeroClaw daemon reliability risk.
- Event-driven jobs give **instant handoffs** instead of 1-2h heartbeat polling delays.

**Scheduled job (e.g., Developer push):**

```bicep
// modules/agent-job.bicep
resource agentJob 'Microsoft.App/jobs@2024-03-01' = {
  name: 'job-agent-${agentName}'
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: { '${managedIdentityId}': {} }
  }
  properties: {
    environmentId: containerAppsEnvironmentId
    configuration: {
      triggerType: 'Schedule'
      replicaTimeout: 1800          // 30min max per session
      replicaRetryLimit: 1
      scheduleTriggerConfig: {
        cronExpression: cronExpression  // e.g., '0 9 * * 1-5'
        parallelism: 1
        replicaCompletionCount: 1
      }
    }
    template: {
      containers: [{
        name: agentName
        image: 'ghcr.io/zeroclaw-labs/zeroclaw:latest'
        command: ['zeroclaw', 'agent', '-m', agentTaskMessage]
        resources: { cpu: json('0.25'), memory: '0.5Gi' }
        env: [
          { name: 'BRAVO_MCP_ENDPOINT', value: bravoMcpEndpoint }
          { name: 'BRAVO_AGENT_TOKEN', secretRef: '${agentName}-token' }
          { name: 'BRAVO_MODEL_PROVIDER', value: modelProvider }
          { name: 'BRAVO_MODEL_NAME', value: modelName }
          { name: 'BRAVO_MODEL_API_BASE', value: modelApiBase }
        ]
      }]
    }
  }
}
```

**Event-driven job (e.g., Translator reacting to tasks-created):**

```bicep
resource translatorJob 'Microsoft.App/jobs@2024-03-01' = {
  name: 'job-agent-${agentName}'
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: { '${managedIdentityId}': {} }
  }
  properties: {
    environmentId: containerAppsEnvironmentId
    configuration: {
      triggerType: 'Event'
      replicaTimeout: 1800
      eventTriggerConfig: {
        scale: {
          minExecutions: 0
          maxExecutions: 3           // Max 3 concurrent translator sessions
          pollingInterval: 30
          rules: [{
            name: 'servicebus'
            type: 'azure-servicebus-queue'
            metadata: {
              queueName: '${agentName}-tasks'
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
        image: 'ghcr.io/zeroclaw-labs/zeroclaw:latest'
        command: ['zeroclaw', 'agent', '-m', agentTaskMessage]
        resources: { cpu: json('0.25'), memory: '0.5Gi' }
      }]
    }
  }
}
```

**Key: `zeroclaw agent -m` not `zeroclaw daemon`** — Jobs use single-shot mode. The agent
receives a task message, processes it via MCP tools, and exits. Azure manages the lifecycle.

### Git-Backed Agent Memory

Container Apps Jobs are ephemeral — the filesystem is destroyed when the job exits. To
give agents persistent memory across sessions, each agent's ZeroClaw memory is stored in
a shared git repo (`bowrain-l10n/agent-memory`). The job entrypoint pulls memory before
execution and pushes it back after.

**Repo structure:**

```
bowrain-l10n/agent-memory/
├── alex-developer/
│   └── memory/              # ZeroClaw markdown memory files
├── jeanpierre-fr/
│   └── memory/
├── maria-brand/
│   └── memory/
├── katrin-de/
│   └── memory/
├── yuki-ja/
│   └── memory/
├── lisa-pm/
│   └── memory/
└── taylor-qa/
    └── memory/
```

**What you get for free:**

- `git log alex-developer/memory/` — full history of what Alex "remembers"
- `git diff HEAD~1` — exactly what changed in the last session
- `git revert` — roll back bad memory (agent learned something wrong)
- Memory diffs visible alongside SOUL.md changes during persona tuning (see `10-persona-evolution.md`)

**Entrypoint wrapper** (`docker/bravo-agent/entrypoint-with-memory.sh`):

```bash
#!/bin/bash
set -euo pipefail

AGENT_NAME="${AGENT_NAME:?}"
MEMORY_REPO="${MEMORY_REPO:?}"
MEMORY_DIR="/tmp/agent-memory"
ZEROCLAW_MEMORY="$HOME/.zeroclaw/memory"
MAX_PUSH_RETRIES=5

# --- Pull memory ---
if [ -d "$MEMORY_DIR/.git" ]; then
  git -C "$MEMORY_DIR" fetch origin
  git -C "$MEMORY_DIR" reset --hard origin/main
else
  git clone --depth 1 --filter=blob:none --sparse "$MEMORY_REPO" "$MEMORY_DIR"
  git -C "$MEMORY_DIR" sparse-checkout set "$AGENT_NAME"
fi

mkdir -p "$ZEROCLAW_MEMORY"
cp -r "$MEMORY_DIR/$AGENT_NAME/memory/"* "$ZEROCLAW_MEMORY/" 2>/dev/null || true

# --- Run agent ---
zeroclaw agent -m "$AGENT_TASK_MESSAGE"
EXIT_CODE=$?

# --- Push memory (with pull/rebase/retry for concurrent execution races) ---
if [ $EXIT_CODE -eq 0 ]; then
  mkdir -p "$MEMORY_DIR/$AGENT_NAME/memory"
  cp -r "$ZEROCLAW_MEMORY/"* "$MEMORY_DIR/$AGENT_NAME/memory/" 2>/dev/null || true
  cd "$MEMORY_DIR"
  git add "$AGENT_NAME/"

  if ! git diff --cached --quiet; then
    git commit -m "$AGENT_NAME: session $(date -u +%Y-%m-%dT%H:%M:%SZ)"

    for i in $(seq 1 $MAX_PUSH_RETRIES); do
      if git push origin main; then
        break
      fi
      echo "Push failed (attempt $i/$MAX_PUSH_RETRIES), rebasing..."
      git pull --rebase origin main
      # If rebase has conflicts (shouldn't for separate agent dirs), abort and skip
      if [ $? -ne 0 ]; then
        echo "Rebase conflict — skipping memory push this session"
        git rebase --abort
        break
      fi
    done
  fi
fi

exit $EXIT_CODE
```

**How it handles races:** If two jobs for different agents push simultaneously, git
handles it naturally — they modify different directories so rebase always succeeds.
If the same agent somehow runs concurrently (shouldn't happen with `maxExecutions: 1`),
the retry loop pulls/rebases until the push goes through. After 5 attempts, it gives
up gracefully — memory for that session is lost but the agent's work (in Bowrain) is not.

**Bicep: job containers use the wrapper entrypoint:**

```bicep
template: {
  containers: [{
    name: agentName
    image: 'ghcr.io/zeroclaw-labs/zeroclaw:latest'
    command: ['/bin/bash', '/bravo/entrypoint-with-memory.sh']
    env: [
      { name: 'AGENT_NAME', value: agentName }
      { name: 'AGENT_TASK_MESSAGE', value: taskMessage }
      { name: 'MEMORY_REPO', value: 'https://x-access-token:${githubPat}@github.com/bowrain-l10n/agent-memory.git' }
      { name: 'BRAVO_MCP_ENDPOINT', value: bravoMcpEndpoint }
      { name: 'BRAVO_AGENT_TOKEN', secretRef: '${agentName}-token' }
      { name: 'BRAVO_MODEL_PROVIDER', value: modelProvider }
      { name: 'BRAVO_MODEL_NAME', value: modelName }
      { name: 'BRAVO_MODEL_API_BASE', value: modelApiBase }
    ]
  }]
}
```

**Local dev:** The same entrypoint works in docker-compose if you set `MEMORY_REPO`.
Or skip it entirely — local daemon mode uses the mounted volume for persistent memory
(no git needed).

### Event-Driven Handoff via ChannelEventBus Adapter

Agent handoffs use a **queue sink adapter** on the existing ChannelEventBus — not a
separate messaging system. The adapter forwards selected events to Redis (local) or
Service Bus (Azure), depending on `BOWRAIN_AGENT_RUNTIME`.

**Alignment with existing Bravo architecture:**

- **Same Redis instance** used for Bravo SSE relay (`bravo:sse:{conversationID}`) and agentic event handoffs (`agentic:*`)
- **Same Service Bus namespace** used for `bravo-jobs` / `translation-jobs` and agentic event queues
- **ChannelEventBus** is the single event source; the queue sink adapter routes to different sinks

**Local (Redis pub/sub):**

```
Developer pushes content (cron: 09:00)
  → ChannelEventBus fires ContentPushed
  → Queue sink adapter → Redis PUBLISH agentic:content-pushed
  → PM agent's Redis subscription triggers immediately

PM creates tasks
  → ChannelEventBus fires TasksCreated
  → Queue sink adapter → Redis PUBLISH agentic:tasks-created-fr
  → Translator agent reacts immediately
```

**Azure (Service Bus + KEDA):**

```
Developer pushes content (scheduled: 09:00)
  → ChannelEventBus fires ContentPushed
  → Queue sink adapter → Service Bus queue: content-pushed
  → KEDA detects message → spins up PM job execution (instant)

PM creates tasks
  → ChannelEventBus fires TasksCreated
  → Queue sink adapter → Service Bus queue: tasks-created-fr
  → KEDA detects → spins up Translator job executions (instant)

Translators complete batch
  → ChannelEventBus fires TranslationComplete
  → Queue sink adapter → Service Bus queue: translation-complete
  → KEDA detects → spins up QA job execution (instant)

QA passes
  → ChannelEventBus fires QAPassed
  → Queue sink adapter → Service Bus queue: qa-passed
  → KEDA detects → spins up Developer pull job (instant)
```

**Queue sink adapter** is a pluggable backend behind a common interface:

```go
// Conceptual interface — the adapter subscribes to ChannelEventBus
// and publishes to the configured backend.
type QueueSink interface {
    Publish(ctx context.Context, channel string, payload []byte) error
}

// RedisQueueSink — local dev, uses existing Redis from compose stack
// ServiceBusQueueSink — Azure, uses existing Service Bus namespace
```

The adapter registers as a subscriber on the existing ChannelEventBus (just like
the SSE relay, webhook dispatcher, and automation engine already do). It filters
events by type and forwards matching events to the configured sink.

### Agent Job Matrix (Azure)

| Agent            | Job Name              | Trigger  | Cron / Queue                  | ZeroClaw Mode                            |
| ---------------- | --------------------- | -------- | ----------------------------- | ---------------------------------------- |
| Developer (push) | `job-agent-alex-push` | Schedule | `0 9 * * 1-5`                 | `agent -m "Check upstream, merge, push"` |
| Developer (pull) | `job-agent-alex-pull` | Event    | queue: `qa-passed`            | `agent -m "Pull translations, commit"`   |
| PM               | `job-agent-lisa`      | Event    | queue: `content-pushed`       | `agent -m "Review push, create tasks"`   |
| Brand Manager    | `job-agent-maria`     | Schedule | `0 10 * * 1,3,5`              | `agent -m "Review terminology"`          |
| Translator (fr)  | `job-agent-jp`        | Event    | queue: `tasks-created-fr`     | `agent -m "Translate assigned blocks"`   |
| Translator (de)  | `job-agent-katrin`    | Event    | queue: `tasks-created-de`     | `agent -m "Translate assigned blocks"`   |
| Translator (ja)  | `job-agent-yuki`      | Event    | queue: `tasks-created-ja`     | `agent -m "Translate assigned blocks"`   |
| QA               | `job-agent-taylor`    | Event    | queue: `translation-complete` | `agent -m "Run quality checks"`          |

### Benefits of Azure AI + Container Apps Jobs

- **Pay per execution** — ~12x cheaper than always-on containers
- **Instant handoffs** — Event-driven via KEDA, not 1-2h poll delays
- **Managed scheduling** — Azure handles cron, no daemon reliability risk
- **Same config everywhere** — API keys work locally and in Azure (no overlays needed)
- **Execution history** — Built-in job execution logs, status, retry tracking
- **Git-backed memory** — Agent memory persisted in git with full history, diffs, rollback
- **Cost Management** — Azure Cost Management tracks per-job, per-agent spend
- **Auto-retry** — `replicaRetryLimit` handles transient failures natively
- **Monitoring** — Azure Monitor + Log Analytics for all job executions

### Cost Controls

1. **Container Apps Jobs** — Pay only for execution time (~14 container-hours/day vs 168)
2. **Model tiering** — GPT-4o-mini for simple tasks, Claude Sonnet for complex
3. **replicaTimeout: 1800** — 30min hard cap per session prevents runaway costs
4. **maxExecutions** — Cap concurrent job runs per agent type
5. **Azure Cost Management** — Budgets and alerts per resource group
6. **max_tokens in config.toml** — Cap LLM output per session
7. **Ollama for zero-cost iteration** — switch provider for MCP/workflow dev without AI spend
