# Bowrain Agentic Testing System

Workspace-centric agentic testing for the Bowrain localization platform. Each
open-source project gets its own workspace with context-specific agent personas
that interact with Bowrain through its real interfaces — CLI, REST API, and MCP
tools.

See [issue #54](https://github.com/neokapi/neokapi/issues/54) for the roadmap.

## How It Works

The agentic testing system runs as a service, driven by GitOps. Four GitHub
repositories work together:

| Repo                                        | Role                                                |
| ------------------------------------------- | --------------------------------------------------- |
| [`neokapi/agentic-fleet`](#fleet-repo)      | Fleet control plane — plans, status, agent personas |
| [`neokapi/agentic-excalidraw`](#fork-repos) | Fork of upstream project (one per project)          |
| [`neokapi/agent-memory`](#agent-memory)     | Per-agent session memory (sparse git checkout)      |
| [`neokapi/agent-feedback`](#agent-feedback) | GitHub Issues filed by agents for platform bugs     |

### The GitOps Cycle

```
1. Commit plan.yaml to agentic-fleet
   └─ defines: upstream fork, target languages, content hint, release tags

2. Coordinator reads fleet repo, calls walk_release
   ├─ Clones neokapi/agentic-excalidraw (on demand, from plan.yaml)
   ├─ Merges upstream tag (e.g., v0.14.2)
   ├─ Strips target locale files (Bowrain owns translations)
   ├─ Discovers source content via glob pattern
   ├─ Pushes source blocks to bowrain-server
   └─ Commits status.yaml back to fleet repo (current_release: v0.14.2)

3. Coordinator dispatches translator agents
   └─ Agents translate via Bowrain MCP, persist to agent-memory

4. Coordinator checks completion, advances to next tag → repeat
```

### Fleet Repo

[`neokapi/agentic-fleet`](https://github.com/neokapi/agentic-fleet)
is the single source of truth for fleet state. The coordinator and agentic-server
read configuration from here and commit status updates back.

```
agentic-fleet/
├── workspaces/
│   └── excalidraw-l10n/
│       ├── plan.yaml       # What to do: fork, languages, content hint, tags
│       ├── status.yaml     # Where we are: current_release, phase
│       └── agents/
│           ├── alex/SOUL.md
│           └── maria/SOUL.md
├── coordinator/
│   └── memory/             # Coordinator's cross-workspace observations
└── observers.yaml          # Human users added to all workspaces
```

**plan.yaml** uses prompt-guided content discovery instead of hardcoded paths.
The walker discovers source files at each release tag via a glob pattern, so it
handles path changes across releases automatically:

```yaml
upstream:
  repo: excalidraw/excalidraw
  fork: neokapi/agentic-excalidraw

content:
  hint: >
    Excalidraw is a React whiteboard app. Locale files are flat JSON
    key-value pairs in a locales/ directory.
  format: json
  source_file_pattern: "**/locales/en.json"

release_strategy:
  tags: [v0.14.0, v0.14.2, v0.15.0, v0.16.0, v0.17.0, v0.18.0]
```

**status.yaml** tracks release progression:

```yaml
phase: active
current_release: v0.14.0
```

### Fork Repos

Each upstream project has a fork (e.g., `neokapi/agentic-excalidraw`). The
release walker clones it on demand, merges upstream tags, and strips target
language files so Bowrain and the agents are the sole authority for translations.

### Agent Memory

[`neokapi/agent-memory`](https://github.com/neokapi/agent-memory) stores
per-agent context across sessions. Each agent gets a sparse checkout of only
their directory. The entrypoint script pulls before a session and pushes after.

### Agent Feedback

[`neokapi/agent-feedback`](https://github.com/neokapi/agent-feedback) is a
GitHub Issues repo where agents file bugs when they hit platform problems (format
parser failures, API errors, UX issues). The coordinator also files here when it
detects cross-workspace patterns.

## Local Development

### Quick start (bootstrap with make)

```bash
cp .env.example .env         # Fill in keys
make up                      # Start all services (docker compose)
make onboard W=excalidraw    # Create users + workspace + walk releases
make logs-excalidraw         # Follow Excalidraw agent logs
```

### Day-to-day operations

```bash
make list                    # List all configured workspaces
make status W=excalidraw     # Show agents, languages, coverage
make content W=excalidraw    # Re-run release walker (picks up new releases)
make deploy                  # Rebuild + deploy agent containers
make down                    # Stop everything
```

### Adding a new project

1. **Configure** — copy the template and fill in project details:

   ```bash
   cp -r workspaces/_template workspaces/my-project
   # Edit workspaces/my-project/workspace.yaml
   # Create agents with SOUL.md + config.toml in agents/<name>/
   ```

2. **Create plan.yaml** — define content discovery and release strategy
   in `agentic-fleet/workspaces/<slug>/plan.yaml`.

3. **Onboard** — create users, workspace, and populate content:

   ```bash
   make onboard W=my-project
   ```

4. **Deploy** — trigger the persona-agent container build:

   ```bash
   make deploy
   ```

Each step is idempotent and can be run independently.

## Directory Structure

```
agentic/
├── workspaces/
│   ├── excalidraw/                 # Active workspace
│   │   ├── workspace.yaml          # Agent definitions (names, roles)
│   │   ├── plan.yaml               # Content discovery + release strategy
│   │   ├── status.yaml             # Current release tracking
│   │   ├── agents/
│   │   │   ├── alex/               # Quality Reviewer
│   │   │   │   ├── SOUL.md         # Persona definition
│   │   │   │   └── config.toml     # MCP + LLM config
│   │   │   ├── maria/              # French Language Expert
│   │   │   ├── katrin/             # German Language Expert
│   │   │   └── yuki/               # Japanese Language Expert
│   │   └── README.md
│   └── _template/                  # Template for new workspaces
├── scripts/
│   ├── setup-keycloak-users.sh     # Create Keycloak users from workspace.yaml
│   └── setup-workspace-api.sh      # Create Bowrain workspace + project + members
├── dashboard/                      # Activity dashboard (agents.dev.bowrain.cloud)
├── docker-compose.yaml             # Local dev stack
├── Makefile                        # GitOps commands
├── .env.example                    # Environment template
└── README.md
```

## Environment Variables

| Variable        | Required   | Default                 | Description                            |
| --------------- | ---------- | ----------------------- | -------------------------------------- |
| `BOWRAIN_URL`   | For remote | `http://localhost:8080` | Bowrain server URL                     |
| `JWT_SECRET`    | For local  | `dev-secret-...`        | JWT signing secret                     |
| `KC_URL`        | For users  | `http://localhost:8180` | Keycloak URL                           |
| `KC_ADMIN_PASS` | For users  | `admin`                 | Keycloak admin password                |
| `KEY_VAULT`     | Optional   | —                       | Azure Key Vault for prod token storage |

Set in `.env` (gitignored) or export before running make targets.

## Workspaces

| Workspace                            | Project    | Languages           | Agents | Status           |
| ------------------------------------ | ---------- | ------------------- | ------ | ---------------- |
| [excalidraw](workspaces/excalidraw/) | Excalidraw | fr-FR, de-DE, ja-JP | 4      | Active (v0.14.0) |

## Architecture

```
agentic-fleet (fleet repo, GitOps)
  ├── plan.yaml    → content hint, release tags, fork URL
  └── status.yaml  → current_release, phase

agentic-server (MCP endpoint)
  ├── walk_release     → clone fork, merge tag, strip targets, push source
  ├── get_fleet_summary → aggregate workspace state
  └── trigger_agent_session → dispatch agents to queues

Agent containers (Azure Container Apps Jobs / local docker-compose)
  ├── Pull memory from agent-memory repo
  ├── Run ZeroClaw with Bowrain MCP (27 tools) + Agentic MCP (9 tools)
  ├── File issues to agent-feedback when things break
  └── Push memory back to agent-memory repo

Coordinator (scheduled every 30min)
  ├── Reads fleet state from agentic-fleet
  ├── Advances releases via walk_release
  ├── Dispatches agents based on pending work
  └── Commits observations to fleet repo
```
