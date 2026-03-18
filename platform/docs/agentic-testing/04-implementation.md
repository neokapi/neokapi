# Technical Implementation

## Architecture Decision: ZeroClaw Containers

Each agent persona runs as an independent **ZeroClaw** instance in its own Docker container. ZeroClaw is an ultra-lightweight Rust-based AI agent runtime (~8.8MB binary, <5MB RAM) with built-in scheduling, MCP tool integration, and workspace-scoped identity files.

### Why ZeroClaw

| Concern | Custom TS Orchestrator (Previous) | ZeroClaw Containers |
|---------|----------------------------------|---------------------|
| Scheduling | Custom cron + event router code | Built-in daemon mode with cron |
| Agent identity | Prompt templates in TypeScript | SOUL.md / IDENTITY.md files |
| Tool integration | Custom API wrappers per agent | MCP native — declare once, use everywhere |
| State | Custom SQLite state manager | Workspace files + Bowrain platform state |
| Scaling | Code changes to add agents | Add container to docker-compose |
| Isolation | Shared process, manual sandboxing | Container-level isolation by default |
| Memory per agent | ~100MB+ (Node.js) | <5MB (Rust binary) |
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
│  │  bowrain-server  (Bowrain platform + PostgreSQL)         │  │
│  │  keycloak        (OIDC authentication)                   │  │
│  └─────────────────────────┬───────────────────────────────┘  │
│                             │                                  │
│  ┌─────────────────────────▼───────────────────────────────┐  │
│  │  bowrain-mcp  (Bowrain MCP Server)                       │  │
│  │  Exposes Bowrain API as MCP tools:                       │  │
│  │  - bowrain.push / bowrain.pull / bowrain.sync            │  │
│  │  - bowrain.translate / bowrain.pseudoTranslate           │  │
│  │  - bowrain.addConcept / bowrain.listConcepts             │  │
│  │  - bowrain.createBrandProfile / bowrain.checkBrand       │  │
│  │  - bowrain.createTask / bowrain.listTasks                │  │
│  │  - bowrain.listActivities                                │  │
│  │  - bowrain.createStream / bowrain.listStreams             │  │
│  │  - bowrain.addTMEntry / bowrain.listTMEntries            │  │
│  └─────────────────────────┬───────────────────────────────┘  │
│                             │ MCP (stdio/SSE)                  │
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

## Repository Structure

```
neokapi/agentic/
├── docker-compose.yaml          # Full stack: Bowrain + agents
├── Makefile                     # Convenience targets
│
├── mcp-server/                  # Bowrain MCP Server (new work)
│   ├── package.json
│   ├── tsconfig.json
│   └── src/
│       ├── index.ts             # MCP server entry point
│       ├── tools/
│       │   ├── push-pull.ts     # bowrain.push, bowrain.pull, bowrain.sync
│       │   ├── translate.ts     # bowrain.translate, bowrain.pseudoTranslate
│       │   ├── termbase.ts      # bowrain.addConcept, bowrain.listConcepts
│       │   ├── brand.ts         # bowrain.createBrandProfile, bowrain.checkBrand
│       │   ├── tasks.ts         # bowrain.createTask, bowrain.listTasks
│       │   ├── activities.ts    # bowrain.listActivities
│       │   ├── streams.ts       # bowrain.createStream, bowrain.listStreams
│       │   ├── tm.ts            # bowrain.addTMEntry, bowrain.listTMEntries
│       │   └── git.ts           # git.checkUpstream, git.merge, git.commit, git.push
│       └── auth.ts              # Per-agent auth context
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
transport = "sse"
url = "http://bowrain-mcp:3001/sse"

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
transport = "sse"
url = "http://bowrain-mcp:3001/sse"

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
- `bowrain.pseudoTranslate` — Get AI translation suggestion for a file
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
   a. Get AI translation suggestion via `bowrain.pseudoTranslate`
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
transport = "sse"
url = "http://bowrain-mcp:3001/sse"

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

### Implementation

Built on the MCP TypeScript SDK, reusing the BowrainAPI client from `platform/e2e/shared/`:

```typescript
// mcp-server/src/index.ts
import { McpServer } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { BowrainAPI } from "../../platform/e2e/shared/api-client.js";

const server = new McpServer({
  name: "bowrain",
  version: "1.0.0",
});

// Each tool maps directly to a BowrainAPI method
server.tool("bowrain.push", "Push source content to Bowrain server", {
  projectId: { type: "string", description: "Project ID" },
  workspaceSlug: { type: "string", description: "Workspace slug" },
}, async ({ projectId, workspaceSlug }) => {
  const api = getAuthenticatedAPI(workspaceSlug);
  const result = await api.syncPush(workspaceSlug, projectId);
  return { content: [{ type: "text", text: JSON.stringify(result) }] };
});

server.tool("bowrain.listTasks", "List translation tasks", {
  workspaceSlug: { type: "string", description: "Workspace slug" },
  assignee: { type: "string", description: "Filter by assignee email", optional: true },
  status: { type: "string", description: "Filter by status", optional: true },
}, async ({ workspaceSlug, assignee, status }) => {
  const api = getAuthenticatedAPI(workspaceSlug);
  const tasks = await api.listTasks(workspaceSlug, { assignee, status });
  return { content: [{ type: "text", text: JSON.stringify(tasks) }] };
});

server.tool("bowrain.addConcept", "Add a terminology concept", {
  workspaceSlug: { type: "string", description: "Workspace slug" },
  term: { type: "string", description: "The term" },
  definition: { type: "string", description: "Term definition" },
  domain: { type: "string", description: "Domain: software, ui, marketing, legal" },
  status: { type: "string", description: "Status: preferred, approved, deprecated" },
}, async ({ workspaceSlug, term, definition, domain, status }) => {
  const api = getAuthenticatedAPI(workspaceSlug);
  const concept = await api.addConcept(workspaceSlug, { term, definition, domain, status });
  return { content: [{ type: "text", text: JSON.stringify(concept) }] };
});

server.tool("bowrain.addTMEntry", "Add a translation memory entry", {
  workspaceSlug: { type: "string", description: "Workspace slug" },
  source: { type: "string", description: "Source text" },
  target: { type: "string", description: "Target text" },
  sourceLang: { type: "string", description: "Source locale (e.g., en-US)" },
  targetLang: { type: "string", description: "Target locale (e.g., fr-FR)" },
}, async ({ workspaceSlug, source, target, sourceLang, targetLang }) => {
  const api = getAuthenticatedAPI(workspaceSlug);
  await api.addTMEntry(workspaceSlug, source, target, sourceLang, targetLang);
  return { content: [{ type: "text", text: "TM entry added" }] };
});

// Git tools (for Developer agent)
server.tool("git.checkUpstream", "Check for new upstream releases", {
  repoPath: { type: "string", description: "Path to git repo" },
}, async ({ repoPath }) => {
  // Uses execFile (safe, no shell) to run git commands
  const result = await checkUpstream(repoPath);
  return { content: [{ type: "text", text: JSON.stringify(result) }] };
});

// ... 15-20 tools total covering all Bowrain workflows

const transport = new StdioServerTransport();
await server.connect(transport);
```

### MCP Tool Catalog

| Tool | Used By | Description |
|------|---------|-------------|
| `bowrain.push` | Developer | Push source content to server |
| `bowrain.pull` | Developer | Pull translations from server |
| `bowrain.sync` | Developer | Push + pull in one operation |
| `bowrain.status` | Developer, PM | Check sync state |
| `bowrain.createStream` | Developer | Create a stream for a release |
| `bowrain.listStreams` | Developer, PM | List existing streams |
| `bowrain.translate` | Translator | Submit a translation for a block |
| `bowrain.pseudoTranslate` | Translator | Get AI translation suggestion |
| `bowrain.addConcept` | Brand Manager | Add terminology concept |
| `bowrain.listConcepts` | Brand Manager, Translator | Query termbase |
| `bowrain.createBrandProfile` | Brand Manager | Create brand voice profile |
| `bowrain.checkBrand` | Brand Manager, QA | Check text against brand rules |
| `bowrain.createTask` | PM, Brand Manager | Create a translation task |
| `bowrain.listTasks` | PM, Translator | List/filter tasks |
| `bowrain.updateTask` | PM, Translator | Update task status |
| `bowrain.listActivities` | All | Check recent platform activity |
| `bowrain.addTMEntry` | Translator | Add to translation memory |
| `bowrain.listTMEntries` | Translator | Query translation memory |
| `bowrain.uploadFile` | Developer | Upload a file to a project |
| `git.checkUpstream` | Developer | Check for new releases/commits |
| `git.merge` | Developer | Merge upstream changes |
| `git.commit` | Developer | Commit changes |
| `git.push` | Developer | Push to remote |

### Per-Agent Auth

Each ZeroClaw container runs as a specific Bowrain user. The MCP server handles auth context per connection:

```typescript
// mcp-server/src/auth.ts
// Each agent container passes its auth token via environment variable
// The MCP server creates an authenticated BowrainAPI instance per agent
function getAuthenticatedAPI(agentToken: string): BowrainAPI {
  return new BowrainAPI(process.env.BOWRAIN_URL!, agentToken);
}
```

Alternatively, the MCP server can be deployed per-agent (one instance per container) with the agent's token baked in. This is simpler but uses more resources.

## Docker Compose (Local Development)

The local docker-compose runs the full stack on your machine. Because the Azure OpenAI
resource has `disableLocalAuth: true` (managed-identity-only), **local agents use
Google Gemini or Ollama** — not Azure endpoints.

The agent SOUL.md files, MCP tools, and scheduling are identical across environments.
Only the `[llm]` provider block in `config.toml` differs.

```yaml
# docker-compose.yaml — local development stack
services:
  # === Platform (same everywhere) ===
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

  # === MCP Server (same everywhere) ===
  bowrain-mcp:
    build: ./mcp-server
    environment:
      BOWRAIN_URL: http://bowrain-server:8080
    depends_on: [bowrain-server]
    ports: ["3001:3001"]

  # === Optional: Ollama for zero-cost local dev ===
  ollama:
    image: ollama/ollama:latest
    profiles: ["ollama"]          # Only start with: docker compose --profile ollama up
    ports: ["11434:11434"]
    volumes:
      - ollama-data:/root/.ollama

  # === Agents (local: Gemini API or Ollama) ===
  alex-developer:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      GOOGLE_API_KEY: ${GOOGLE_API_KEY:-}
    volumes:
      - ./agents/alex-developer:/root/.zeroclaw
      - ./forks:/root/.zeroclaw/workspace
    depends_on: [bowrain-mcp]

  maria-brand:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      GOOGLE_API_KEY: ${GOOGLE_API_KEY:-}
    volumes:
      - ./agents/maria-brand:/root/.zeroclaw
    depends_on: [bowrain-mcp]

  jeanpierre-fr:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      GOOGLE_API_KEY: ${GOOGLE_API_KEY:-}
    volumes:
      - ./agents/jeanpierre-fr:/root/.zeroclaw
    depends_on: [bowrain-mcp]

  katrin-de:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      GOOGLE_API_KEY: ${GOOGLE_API_KEY:-}
    volumes:
      - ./agents/katrin-de:/root/.zeroclaw
    depends_on: [bowrain-mcp]

  yuki-ja:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      GOOGLE_API_KEY: ${GOOGLE_API_KEY:-}
    volumes:
      - ./agents/yuki-ja:/root/.zeroclaw
    depends_on: [bowrain-mcp]

  lisa-pm:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      GOOGLE_API_KEY: ${GOOGLE_API_KEY:-}
    volumes:
      - ./agents/lisa-pm:/root/.zeroclaw
    depends_on: [bowrain-mcp]

  taylor-qa:
    image: ghcr.io/zeroclaw-labs/zeroclaw:latest
    command: daemon
    restart: unless-stopped
    environment:
      GOOGLE_API_KEY: ${GOOGLE_API_KEY:-}
    volumes:
      - ./agents/taylor-qa:/root/.zeroclaw
    depends_on: [bowrain-mcp]

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
│  Use for: Long-running agent tests, integration validation   │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  Azure Prod (rg-bowrain-p-sdc, bowrain.cloud)               │
│                                                              │
│  Simple agents:  Azure OpenAI GPT-4o-mini  (capacity 300)   │
│  Complex agents: Azure AI Foundry Claude Sonnet (serverless) │
│  Auth: Managed identity (no keys)                            │
│  Use for: Public demo, sustained activity generation         │
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

### Azure Deployment

In Azure, agents run as **Container Apps** (or Container Instances) within the existing
Container Apps Environment, with the same user-assigned managed identity used by the
bowrain-server and worker containers. This gives them access to both Azure OpenAI and
Azure AI Foundry without any API keys.

```bicep
// modules/containerapp-agent.bicep (new, per agent)
resource agentApp 'Microsoft.App/containerApps@2024-03-01' = {
  name: 'ca-agent-${agentName}'
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${managedIdentityId}': {}
    }
  }
  properties: {
    environmentId: containerAppsEnvironmentId
    template: {
      containers: [{
        name: agentName
        image: 'ghcr.io/zeroclaw-labs/zeroclaw:latest'
        command: ['zeroclaw', 'daemon']
        resources: { cpu: '0.25', memory: '0.5Gi' }  // ZeroClaw is tiny
        volumeMounts: [{ volumeName: 'workspace', mountPath: '/root/.zeroclaw' }]
      }]
      scale: { minReplicas: 1, maxReplicas: 1 }  // Always-on daemon
    }
  }
}
```

### Benefits of Azure AI

- **Consolidated billing** — All AI costs on the existing Azure subscription
- **Data residency** — Sweden Central (EU compliance)
- **Managed identity** — No API key rotation; Entra ID authentication
- **Cost Management** — Azure Cost Management tracks per-model, per-agent spend
- **Network security** — VNet integration, private endpoints if needed
- **Existing monitoring** — Azure Monitor / App Insights for latency and error tracking

### Cost Controls

1. **Model tiering** — GPT-4o-mini ($0.15/1M) for simple tasks vs Claude Sonnet ($3/1M) for complex ones
2. **Cron frequency** — Agents only wake on schedule (not continuous)
3. **Container Apps scale** — `minReplicas: 1, maxReplicas: 1` (no auto-scaling, predictable cost)
4. **Azure OpenAI capacity limits** — Built-in TPM caps per deployment
5. **Azure Cost Management** — Set budgets and alerts per resource group
6. **max_tokens in config.toml** — Cap output length per agent session
7. **Local dev is cheap** — Ollama for workflow iteration, Anthropic only when testing quality
