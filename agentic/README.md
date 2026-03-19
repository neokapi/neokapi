# Bowrain Agentic Testing System

Agentic testing for the Bowrain localization platform. Agent personas (developer,
brand manager, translators, PM, QA) run as independent ZeroClaw instances in
Docker containers, interacting with Bowrain through its real interfaces — CLI,
REST API, and MCP tools.

See [design docs](../platform/docs/agentic-testing/) for the full architecture
and [issue #54](https://github.com/neokapi/neokapi/issues/54) for the roadmap.

## Quick Start

1. Copy the environment template and fill in your keys:

   ```bash
   cp .env.example .env
   # Edit .env with your Azure AI keys and agent tokens
   ```

2. Start the stack:

   ```bash
   make up
   ```

3. Watch agent logs:

   ```bash
   make logs-alex
   ```

4. Browse captured emails at http://localhost:8025 (Mailpit web UI).

5. Stop everything:

   ```bash
   make down
   ```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make up` | Start all services |
| `make down` | Stop all services |
| `make logs` | Follow all logs |
| `make logs-alex` | Follow Developer agent logs |
| `make status` | Show running containers |
| `make shell-alex` | Shell into Developer agent container |

## Structure

```
agentic/
├── agents/                  # Agent workspace definitions
│   ├── shared/AGENTS.md     # Team roster
│   └── alex-developer/      # Developer agent (Alex Chen)
├── config/projects/         # Project configurations
├── docker-compose.yaml      # Full local stack
├── email-mcp/               # Standalone email MCP server (Phase 1)
├── entrypoint-with-memory.sh # Git-backed memory for Azure jobs
├── .env.example             # Environment variable template
└── Makefile                 # Convenience targets
```
