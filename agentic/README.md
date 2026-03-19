# Bowrain Agentic Testing System

Workspace-centric agentic testing for the Bowrain localization platform. Each
open-source project gets its own workspace with context-specific agent personas
that interact with Bowrain through its real interfaces -- CLI, REST API, and MCP
tools.

See [design docs](../platform/docs/agentic-testing/) for the full architecture
and [issue #54](https://github.com/neokapi/neokapi/issues/54) for the roadmap.

## Quick Start

1. Copy the environment template and fill in your keys:

   ```bash
   cp .env.example .env
   # Edit .env with your Azure AI keys and per-workspace agent tokens
   ```

2. Start the full stack (platform + all workspace agents):

   ```bash
   make up
   ```

3. Or start a single workspace:

   ```bash
   make up-excalidraw
   ```

4. Watch agent logs:

   ```bash
   make logs-excalidraw
   ```

5. Browse captured emails at http://localhost:8025 (Mailpit web UI).

6. Stop everything:

   ```bash
   make down
   ```

## Workspace Structure

Each workspace is a self-contained localization experiment for an open-source
project. Agents within a workspace have context-specific personas -- "L10N
Engineer" rather than "Developer", "French Language Expert" rather than
"Translator".

```
agentic/
├── workspaces/
│   ├── excalidraw/                 # First experiment (active)
│   │   ├── workspace.yaml          # Project config (upstream, languages, agents)
│   │   ├── agents/
│   │   │   ├── alex/               # L10N Engineer
│   │   │   ├── sophie/             # French Language Expert
│   │   │   ├── thomas/             # German Language Expert
│   │   │   └── mei/                # Reviewer
│   │   └── README.md
│   └── _template/                  # Template for new workspaces
├── docker-compose.yaml             # Full stack (platform + all workspaces)
├── email-mcp/                      # Standalone email MCP server
├── release-walker/                 # Upstream release detector
├── dashboard/                      # Activity visualization
├── entrypoint-with-memory.sh       # Git-backed memory for Azure jobs
├── scripts/
│   ├── setup-keycloak-users.sh     # Per-workspace Keycloak user creation
│   └── setup-workspace.sh          # Create Bowrain workspace + invite agents
├── Makefile                        # Per-workspace convenience targets
├── .env.example                    # Environment variable template
└── README.md
```

## Workspaces

### Active

| Workspace | Project | Languages | Agents | Status |
|-----------|---------|-----------|--------|--------|
| [excalidraw](workspaces/excalidraw/) | Excalidraw (collaborative whiteboard) | fr-FR, de-DE | 4 | Active |

### Planned

Future workspaces can be created from `workspaces/_template/`. Copy the template,
fill in project details, and add agents.

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make up` | Start all services (platform + all workspaces) |
| `make down` | Stop all services |
| `make logs` | Follow all logs |
| `make status` | Show running containers |
| `make up-excalidraw` | Start Excalidraw workspace agents only |
| `make logs-excalidraw` | Follow Excalidraw agent logs |
| `make shell-excalidraw-alex` | Shell into Alex's container |

## Creating a New Workspace

1. Copy the template:
   ```bash
   cp -r workspaces/_template workspaces/my-project
   ```

2. Rename template files (remove `.template` suffix) and fill in project details.

3. Add agent directories with SOUL.md, config.toml, and HEARTBEAT.md.

4. Add Docker Compose services for each agent in `docker-compose.yaml`.

5. Add workspace-specific tokens to `.env`.

6. Create Keycloak users and Bowrain workspace:
   ```bash
   ./scripts/setup-keycloak-users.sh
   ./scripts/setup-workspace.sh workspaces/my-project
   ```

## Agent Memory

Agent memory is stored in a separate git repository (`neokapi/agent-memory`)
that mirrors the workspace structure:

```
agent-memory/
├── excalidraw/
│   ├── alex/memory/
│   ├── sophie/memory/
│   ├── thomas/memory/
│   └── mei/memory/
```

## Dashboard

The activity dashboard (`dashboard/`) currently shows a unified view of all
agent activity. A workspace filter is planned for a future PR.
