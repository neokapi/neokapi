# Bowrain Agentic Testing System

Workspace-centric agentic testing for the Bowrain localization platform. Each
open-source project gets its own workspace with context-specific agent personas
that interact with Bowrain through its real interfaces — CLI, REST API, and MCP
tools.

See [issue #54](https://github.com/neokapi/neokapi/issues/54) for the roadmap.

## GitOps Workflow

The `agentic/` directory is the source of truth. All workspace configuration
lives in git. The Makefile provides the commands to make it real.

### Adding a new project

1. **Configure** — copy the template and fill in project details:

   ```bash
   cp -r workspaces/_template workspaces/my-project
   # Edit workspaces/my-project/workspace.yaml
   # Create agents with SOUL.md + config.toml in agents/<name>/
   ```

2. **Onboard** — create users, workspace, and populate content:

   ```bash
   make onboard W=my-project
   ```

   This runs three steps in sequence:
   - `make users` — creates Keycloak users for each agent
   - `make workspace` — creates the Bowrain workspace, project, and adds agent members
   - `make content` — walks upstream releases into the workspace

3. **Deploy** — trigger the persona-agent container build:

   ```bash
   make deploy
   ```

Each step is idempotent and can be run independently.

### Day-to-day operations

```bash
make list                    # List all configured workspaces
make status W=excalidraw     # Show agents, languages, coverage
make content W=excalidraw    # Re-run release walker (picks up new releases)
make deploy                  # Rebuild + deploy agent containers
```

### Local development

```bash
cp .env.example .env         # Fill in keys
make up                      # Start all services (docker compose)
make up-excalidraw           # Start only Excalidraw agents
make logs-excalidraw         # Follow Excalidraw agent logs
make down                    # Stop everything
```

## Directory Structure

```
agentic/
├── workspaces/
│   ├── excalidraw/                 # Active workspace
│   │   ├── workspace.yaml          # Project config (source of truth)
│   │   ├── agents/
│   │   │   ├── alex/               # L10N Engineer
│   │   │   │   ├── SOUL.md         # Persona definition
│   │   │   │   ├── config.toml     # MCP + LLM config
│   │   │   │   └── HEARTBEAT.md    # Session log
│   │   │   ├── sophie/             # French Language Expert
│   │   │   ├── thomas/             # German Language Expert
│   │   │   └── mei/                # Reviewer
│   │   └── README.md
│   └── _template/                  # Template for new workspaces
├── scripts/
│   ├── setup-keycloak-users.sh     # Create Keycloak users from workspace.yaml
│   ├── setup-workspace-api.sh      # Create Bowrain workspace + project + members
│   └── walk-releases.sh            # Walk upstream releases into Bowrain
├── dashboard/                      # Activity dashboard (agents.dev.bowrain.cloud)
├── docker-compose.yaml             # Local dev stack
├── Makefile                        # GitOps commands
├── .env.example                    # Environment template
└── README.md
```

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `BOWRAIN_URL` | For remote | `http://localhost:8080` | Bowrain server URL |
| `JWT_SECRET` | For local | `dev-secret-...` | JWT signing secret |
| `KC_URL` | For users | `http://localhost:8180` | Keycloak URL |
| `KC_ADMIN_PASS` | For users | `admin` | Keycloak admin password |
| `KEY_VAULT` | Optional | — | Azure Key Vault for prod token storage |

Set in `.env` (gitignored) or export before running make targets.

## Workspaces

| Workspace | Project | Languages | Agents | Status |
|---|---|---|---|---|
| [excalidraw](workspaces/excalidraw/) | Excalidraw | fr-FR, de-DE | 4 | Active |

## Architecture

```
workspace.yaml (git)
  → make users     → Keycloak users (no password, token-only)
  → make workspace → Bowrain workspace + project + members
  → make content   → Source blocks + community translations

Agent containers (Azure Container Apps Jobs)
  → Read SOUL.md + config.toml from fleet repo
  → Exchange API token for JWT
  → Run ZeroClaw with Bowrain MCP (27 tools) + Agentic MCP (9 tools)
  → Publish execution events to Redis → PostgreSQL
  → Push memory changes back to fleet repo

Coordinator (observer, not operator)
  → Reads fleet state + execution history
  → Files GitHub issues with observations/suggestions
  → Never dispatches agents or modifies infrastructure
```
