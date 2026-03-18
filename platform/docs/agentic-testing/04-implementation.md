# Technical Implementation

## Architecture Decision: ZeroClaw Containers

Each agent persona runs as an independent **ZeroClaw** instance in its own Docker container. ZeroClaw is an ultra-lightweight Rust-based AI agent runtime (~8.8MB binary, &lt;5MB RAM) with built-in scheduling, MCP tool integration, and workspace-scoped identity files.

### Why ZeroClaw

| Concern | Custom TS Orchestrator (Previous) | ZeroClaw Containers |
|---------|----------------------------------|---------------------|
| Scheduling | Custom cron + event router code | Built-in daemon mode with cron |
| Agent identity | Prompt templates in TypeScript | SOUL.md / IDENTITY.md files |
| Tool integration | Custom API wrappers per agent | MCP native — declare once, use everywhere |
| State | Custom SQLite state manager | Workspace files + Bowrain platform state |
| Scaling | Code changes to add agents | Add container to docker-compose |
| Isolation | Shared process, manual sandboxing | Container-level isolation by default |
| Memory per agent | ~100MB+ (Node.js) | &lt;5MB (Rust binary) |
| Total for 20 agents | ~2GB+ | ~100MB |

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
│       └── tolgee.yaml
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
# Local: Gemini (default) or Ollama (free)
# Azure: GPT-4o-mini via Azure OpenAI (managed identity)
default_provider = "google"
default_model = "gemini-2.5-flash"

[security]
allowed_commands = ["git", "bowrain", "ls", "cat", "diff"]

[mcp]
[mcp.bowrain]
transport = "http"
url = "${BRAVO_MCP_ENDPOINT}"
headers = { Authorization = "Bearer ${BRAVO_AGENT_TOKEN}" }
tool_timeout_secs = 120

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
# Provider set by environment overlay
# Local: Gemini (default) or Ollama (free)
# Azure: Claude Sonnet via Azure AI Foundry (managed identity)
default_provider = "google"
default_model = "gemini-2.5-flash"

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
# Provider set by environment overlay
# Local: Gemini (default) or Ollama (free)
# Azure: Claude Sonnet via Azure AI Foundry (managed identity)
default_provider = "google"
default_model = "gemini-2.5-flash"

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

| Tool | Used By | Description |
|------|---------|-------------|
| `check_vocabulary` | Brand Manager, QA | Validate text against brand terms, flag violations |
| `list_profiles` | Brand Manager | List brand voice profiles in workspace |
| `get_voice_guide` | Brand Manager, Translator | Formatted brand guide for LLM consumption |

**Content Management (11 tools):**

| Tool | Used By | Description |
|------|---------|-------------|
| `list_projects` | PM, Developer | List projects in workspace |
| `get_project` | All | Get project details |
| `create_project` | Developer | Create a new project |
| `update_project` | Developer, PM | Update project settings |
| `list_blocks` | Translator, QA | List translatable blocks |
| `get_block` | Translator | Get block with source + targets |
| `update_block` | Translator | Submit translation for a block (per locale) |
| `create_version` | Developer | Create a new version/snapshot |
| `list_streams` | Developer, PM | List content streams |
| `diff_streams` | Developer, QA | Compare two streams |
| `merge_stream` | Developer | Merge a stream into parent |

**Flows & Automation (3 tools):**

| Tool | Used By | Description |
|------|---------|-------------|
| `list_flows` | Developer, QA | List available flows (AI translate, QA, etc.) |
| `run_flow` | Developer, Translator, QA | Execute a flow on project content |
| `get_flow_status` | Developer | Check flow execution status |

**Translation Memory & Terminology (4 tools):**

| Tool | Used By | Description |
|------|---------|-------------|
| `tm_search` | Translator | Search TM with fuzzy matching (min score 0.5) |
| `tm_import` | Developer | Bulk import TM entries |
| `term_search` | Translator, Brand Manager | Search termbase with locale filters |
| `term_add` | Brand Manager | Add new terminology concept |

**Connectors & Sync (3 tools):**

| Tool | Used By | Description |
|------|---------|-------------|
| `connector_pull` | Developer | Fetch content from Git/CMS into project |
| `connector_push` | Developer | Publish translations to external target |
| `connector_status` | Developer, PM | Check sync state (last sync, pending, errors) |

**Sandbox (1 tool):**

| Tool | Used By | Description |
|------|---------|-------------|
| `execute_script` | Developer, QA | Run Python/Bash/Node.js in isolated sandbox |

**Tools to add for agentic testing (5 tools):**

| Tool | Used By | Where to Add |
|------|---------|--------------|
| `github_create_issue` | All | `platform/server/mcp/tools_github.go` |
| `github_search_issues` | All | `platform/server/mcp/tools_github.go` |
| `github_comment_issue` | PM, QA | `platform/server/mcp/tools_github.go` |
| `email_send` | All | `platform/server/mcp/tools_email.go` |
| `email_list_inbox` | All | `platform/server/mcp/tools_email.go` |

**Key workflow mapping:**
- AI translation → use `run_flow` with an AI translation flow (not pseudo-translate)
- Push/pull content → use `connector_pull` / `connector_push` with Git connector
- Submit translation → use `update_block` to set target text per locale
- Git operations → Developer agent uses ZeroClaw's `allowed_commands` for direct git access

### Per-Agent Auth

Each ZeroClaw agent connects directly to the Bowrain server's MCP endpoint (`/mcp/`)
using a per-agent JWT token — the same auth mechanism Bravo uses for interactive
conversations.

**No MCP sidecar needed.** The Bravo MCP server is built into bowrain-server itself
(`platform/server/mcp/`). Each agent connects to the same server with its own token.

```toml
# agents/alex-developer/config.toml
[mcp.bowrain]
transport = "http"
url = "${BRAVO_MCP_ENDPOINT}"          # e.g., http://bowrain-server:8080/mcp/
headers = { Authorization = "Bearer ${BRAVO_AGENT_TOKEN}" }
tool_timeout_secs = 120
```

This is the exact config template from `platform/docker/bravo/config.toml.template`.
Agent tokens are workspace-scoped JWTs (30min TTL, auto-refreshed) created via
the Bravo conversation API.

**Key implication:** No per-agent MCP sidecar containers. Each agent has ONE container
(ZeroClaw daemon) that connects directly to bowrain-server. This halves the container
count from 14 to 7.

## Docker Compose (Local Development)

The local docker-compose runs the full stack on your machine. Because the Azure OpenAI
resource has `disableLocalAuth: true` (managed-identity-only), **local agents use
Google Gemini or Ollama** — not Azure endpoints.

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
    depends_on: [postgres, keycloak]

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: bowrain
      POSTGRES_PASSWORD: bowrain
      POSTGRES_DB: bowrain
    volumes:
      - pgdata:/var/lib/postgresql/data

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
  # No MCP sidecar needed — the MCP server is built into bowrain-server.

  alex-developer:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      GOOGLE_API_KEY: ${GOOGLE_API_KEY:-}
      BRAVO_MCP_ENDPOINT: http://bowrain-server:8080/mcp/
      BRAVO_AGENT_TOKEN: ${ALEX_AGENT_TOKEN}
    volumes:
      - ./agents/alex-developer:/root/.zeroclaw
      - ./forks:/root/.zeroclaw/workspace
    depends_on: [bowrain-server]

  jeanpierre-fr:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      GOOGLE_API_KEY: ${GOOGLE_API_KEY:-}
      BRAVO_MCP_ENDPOINT: http://bowrain-server:8080/mcp/
      BRAVO_AGENT_TOKEN: ${JEANPIERRE_AGENT_TOKEN}
    volumes:
      - ./agents/jeanpierre-fr:/root/.zeroclaw
    depends_on: [bowrain-server]

  # ... same pattern for: maria-brand, katrin-de, yuki-ja, lisa-pm, taylor-qa
  # Total: 7 agent containers + platform services (no sidecars)

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
GOOGLE_API_KEY=AIza...                 # All agents use Gemini locally
ALEX_AGENT_TOKEN=...                   # Per-agent JWT tokens
JEANPIERRE_AGENT_TOKEN=...            # (created via Bravo conversation API
MARIA_AGENT_TOKEN=...                  #  or Keycloak user setup)
KATRIN_AGENT_TOKEN=...
YUKI_AGENT_TOKEN=...
LISA_AGENT_TOKEN=...
TAYLOR_AGENT_TOKEN=...
```

## Event Coordination: Poll-Based via Activity Feed

ZeroClaw agents are autonomous — they self-schedule via cron and heartbeat. Cross-agent coordination happens through **polling Bowrain's activity feed**, not through a central event bus.

This is actually more realistic: real humans check their dashboard, they don't react to webhooks.

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
  upstream: string;       // e.g., "facebook/docusaurus"
  forkPath: string;       // e.g., "/app/forks/docusaurus"
  startRelease: string;   // e.g., "v3.0.0"
  endRelease: string;     // e.g., "latest"
  intervalMinutes: number; // e.g., 120
}

async function walkReleases(config: ReleaseConfig) {
  // 1. Get release tags
  const { stdout } = await execFileAsync("git", [
    "tag", "--list", "v*", "--sort=version:refname",
  ], { cwd: config.forkPath });

  const tags = stdout.trim().split("\n")
    .filter(t => t >= config.startRelease);

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
      JSON.stringify({ tag, timestamp: new Date().toISOString() })
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

# === Option A: Gemini (good quality, cheap, good tool-use) ===
echo "GOOGLE_API_KEY=AIza..." > .env
cd neokapi/agentic
docker compose up -d

# === Option B: Ollama (free, lower quality, good for MCP/workflow iteration) ===
cd neokapi/agentic
docker compose --profile ollama up -d
# Then override agents to use ollama provider (see config overlay below)

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
5. Run `bowrain init` in the fork (one-time setup)

## Model Provider Strategy

### The Problem: Azure OpenAI Has No API Keys

The Azure OpenAI resource (`oai-bowrain-{env}`) has `disableLocalAuth: true` in
`bowrain-infra/modules/openai.bicep`. Only managed identity Bearer tokens work — these
are only available from Azure resources (Container Apps, VMs with assigned identity).
A local docker-compose cannot authenticate to Azure OpenAI.

### The Solution: Environment-Specific Providers

Agent SOUL.md, HEARTBEAT.md, and MCP tools are **identical** across all environments.
Only the `[llm]` block in `config.toml` changes per environment. We use a config overlay
pattern — a base config.toml per agent, with environment-specific overrides.

### Three Environments

```
┌─────────────────────────────────────────────────────────────┐
│  Local (docker-compose)                                      │
│                                                              │
│  Provider: Google Gemini 2.5 Flash  — or —  Ollama (free)   │
│  Auth: GOOGLE_API_KEY in .env       — or —  no auth (local) │
│  Use for: MCP development, persona tuning, workflow testing  │
│  All agents use the same provider (simplicity)               │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  Azure Dev (rg-bowrain-d-sdc, dev.bowrain.cloud)            │
│                                                              │
│  Simple agents:  Azure OpenAI GPT-4o-mini  (capacity 60)    │
│  Complex agents: Azure AI Foundry Claude Sonnet (serverless) │
│  Auth: Managed identity (no keys)                            │
│  Bowrain target: dev.bowrain.cloud                           │
│  Demo dashboard: agents.dev.bowrain.cloud                    │
│  Use for: Long-running agents, public demo, sustained        │
│           activity generation                                │
└─────────────────────────────────────────────────────────────┘
```

### Config Overlay Pattern

Each agent workspace has a base `config.toml` and optional environment overrides:

```
agents/jeanpierre-fr/
├── config.toml              # Base: provider (Gemini), MCP, cron, security
├── config.azure-dev.toml    # Azure dev: Claude via Foundry + managed identity
└── config.azure-prod.toml   # Azure prod: same as dev, different endpoint
```

The base `config.toml` defaults to Gemini — this is what local docker-compose uses.
Azure overlays switch to Azure OpenAI / Azure AI Foundry with managed identity.

**Ollama override (optional, for zero-cost iteration):**
```toml
# Override in config.toml when using --profile ollama
[llm]
default_provider = "ollama"
default_model = "llama3.1:8b"
# Ollama runs as a sibling container, no auth needed
```

**Azure overlay — Translator (Claude via Foundry):**
```toml
# agents/jeanpierre-fr/config.azure-dev.toml
[llm]
default_provider = "custom"
default_model = "claude-sonnet-4-5-20250514"

[providers.custom]
name = "azure-claude"
base_url = "https://bowrain-foundry-d.services.ai.azure.com/v1"
# Auth via managed identity — no api_key_env needed
```

**Azure overlay — Developer (GPT-4o-mini via Azure OpenAI):**
```toml
# agents/alex-developer/config.azure-dev.toml
[llm]
default_provider = "custom"
default_model = "gpt-4o-mini"

[providers.custom]
name = "azure-openai"
base_url = "https://oai-bowrain-d.openai.azure.com/openai/deployments/gpt-4o-mini/v1"
# Auth via managed identity — no api_key_env needed
```

### Azure Provider Matrix (dev/prod)

| Agent | Task Complexity | Model | Azure Service | Est. Cost |
|-------|----------------|-------|---------------|-----------|
| Developer (Alex) | Low — push/pull/git | GPT-4o-mini | Azure OpenAI (existing) | ~$0.15/1M tok |
| PM (Lisa) | Medium — task creation | GPT-4o | Azure OpenAI (existing) | ~$2.50/1M tok |
| QA (Taylor) | Medium — quality checks | GPT-4o | Azure OpenAI (existing) | ~$2.50/1M tok |
| Brand Manager (Maria) | High — terminology, brand | Claude Sonnet 4.5 | Azure AI Foundry (new) | ~$3/1M tok |
| Translators (JP, Katrin, Yuki) | Medium-High — translation review | Claude Sonnet 4.5 | Azure AI Foundry (new) | ~$3/1M tok |

### Azure Infrastructure

**Already provisioned** (from `bowrain-infra/modules/openai.bicep`):
- Azure OpenAI: `oai-bowrain-d` / `oai-bowrain-p` in Sweden Central
- GPT-4o: capacity 30 (dev) / 150 (prod)
- GPT-4o-mini: capacity 60 (dev) / 300 (prod)
- Auth: `disableLocalAuth: true`, managed identity with `Cognitive Services OpenAI User` role

**New resource needed:**
- Azure AI Foundry workspace + Claude Sonnet serverless deployment
- Deploy via portal initially, codify in Bicep later
- Same managed identity gets `Cognitive Services User` role on the Foundry resource

### Azure Deployment: Container Apps Jobs

In Azure, agents run as **Container Apps Jobs** — not always-on containers. Each agent
session starts, does work, and stops. You pay only for execution time, not 24/7 uptime.

This uses the same Container Apps Environment already deployed for bowrain-server, with
the same user-assigned managed identity for Azure OpenAI and AI Foundry access.

**Two job trigger types:**

| Trigger | Agents | How It Works |
|---------|--------|-------------|
| **Scheduled** | Developer (push), Brand Manager | Azure-managed cron expression. Fires on schedule, runs ZeroClaw session, exits. |
| **Event-driven** | PM, Translators, QA, Developer (pull) | KEDA scaler monitors Azure Service Bus queue depth. New message → new job execution. |

**Why Jobs, not always-on Container Apps:**
- 7 agents × 24h = 168 container-hours/day if always-on. With Jobs, 7 agents × ~2h
  active/day = ~14 container-hours/day. **~12x cheaper.**
- ZeroClaw cold start is <10ms (3.4MB binary) — negligible for jobs that run minutes.
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

### Event-Driven Handoff via Azure Service Bus

Instead of polling the activity feed with heartbeat, agents communicate through Azure
Service Bus queues (already deployed in `bowrain-infra/core.bicep`):

```
Developer pushes content (scheduled: 09:00)
  → Bowrain server publishes to queue: "content-pushed"
  → KEDA detects message → spins up PM job execution (instant)

PM creates tasks
  → Bowrain server publishes to queue: "tasks-created"
  → KEDA detects → spins up Translator job executions (instant)

Translators complete batch
  → Bowrain server publishes to queue: "translation-complete"
  → KEDA detects → spins up QA job execution (instant)

QA passes
  → Bowrain server publishes to queue: "qa-passed"
  → KEDA detects → spins up Developer pull job (instant)
```

**Service Bus integration:** Bowrain's event bus (`platform/event/`) publishes events.
A Service Bus adapter routes selected events to agent queues. This is a new component
but uses the existing Service Bus resource.

### Agent Job Matrix (Azure)

| Agent | Job Name | Trigger | Cron / Queue | ZeroClaw Mode |
|-------|----------|---------|-------------|---------------|
| Developer (push) | `job-agent-alex-push` | Schedule | `0 9 * * 1-5` | `agent -m "Check upstream, merge, push"` |
| Developer (pull) | `job-agent-alex-pull` | Event | queue: `qa-passed` | `agent -m "Pull translations, commit"` |
| PM | `job-agent-lisa` | Event | queue: `content-pushed` | `agent -m "Review push, create tasks"` |
| Brand Manager | `job-agent-maria` | Schedule | `0 10 * * 1,3,5` | `agent -m "Review terminology"` |
| Translator (fr) | `job-agent-jp` | Event | queue: `tasks-created-fr` | `agent -m "Translate assigned blocks"` |
| Translator (de) | `job-agent-katrin` | Event | queue: `tasks-created-de` | `agent -m "Translate assigned blocks"` |
| Translator (ja) | `job-agent-yuki` | Event | queue: `tasks-created-ja` | `agent -m "Translate assigned blocks"` |
| QA | `job-agent-taylor` | Event | queue: `translation-complete` | `agent -m "Run quality checks"` |

### Benefits of Azure AI + Container Apps Jobs

- **Pay per execution** — ~12x cheaper than always-on containers
- **Instant handoffs** — Event-driven via KEDA, not 1-2h poll delays
- **Managed scheduling** — Azure handles cron, no daemon reliability risk
- **Managed identity** — No API key rotation; Entra ID authentication
- **Execution history** — Built-in job execution logs, status, retry tracking
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
7. **Local dev is free/cheap** — Gemini or Ollama, no Azure charges
