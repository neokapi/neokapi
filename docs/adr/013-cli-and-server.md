---
id: 013-cli-and-server
sidebar_position: 13
title: "ADR-013: Kapi CLI and Server"
---
# ADR-013: Kapi CLI and gokapi-server

## Context

gokapi exposes its functionality through two application entry points: the
**kapi CLI** for developers, CI/CD pipelines, and power users, and the
**gokapi-server** for remote access, Bowrain connectivity, and API integrations.

The CLI command structure must reflect the connector-first architecture described
in [ADR-005](005-connector-system.md). Top-level commands like `kapi connect`,
`kapi pull`, and `kapi push` sit alongside traditional file-oriented commands
like `kapi convert`. The server needs both REST (for external integrations and
webhooks) and gRPC (for Bowrain-to-server streaming communication as described
in [ADR-012](012-bowrain.md)).

## Decision

### Kapi CLI: Cobra Command Structure

The CLI uses [Cobra](https://github.com/spf13/cobra) for hierarchical
subcommands. The root command is defined in `cmd/kapi/root.go`; each subcommand
lives in its own file. The command taxonomy reflects the platform architecture:

```
kapi
├── connect          # Connector management
│   ├── add          # Configure a new connector
│   ├── list         # List configured connectors
│   ├── remove       # Remove a connector
│   └── status       # Show connector sync status
├── pull             # Pull content from a connector into the store
├── push             # Push translations back to a connector
├── convert          # Direct file conversion (shortcut using FileConnector)
│   ├── extract      # Extract translatable content to XLIFF/KAZ
│   └── merge        # Merge translations back into source format
├── translate        # Run translation flow
├── flow             # Flow management
│   ├── run          # Execute a named flow
│   ├── list         # List available flows
│   └── create       # Create a new flow definition
├── store            # Content Store operations
│   ├── version      # Create/list/diff versions
│   ├── export       # Export project as KAZ
│   └── import       # Import KAZ into store
├── tm               # Translation Memory
│   ├── import       # Import TMX
│   ├── export       # Export TMX
│   ├── lookup       # Query TM
│   └── stats        # TM statistics
├── termbase         # Terminology
│   ├── import       # Import CSV/TBX/JSON
│   ├── export       # Export
│   ├── lookup       # Query terms
│   ├── search       # Search concepts
│   ├── stream       # Manage terminology streams
│   └── stats        # Statistics
├── formats          # Format listing
│   └── list         # List available formats (built-in + plugin)
├── tools            # Tool listing
│   └── list         # List available tools
├── plugins          # Plugin management
│   ├── install      # Install a plugin
│   ├── list         # List installed plugins
│   ├── update       # Update plugins
│   ├── search       # Search plugin registry
│   └── audit        # Audit plugin usage across projects
└── serve            # Start gokapi-server
```

### Connector-First Commands

The primary workflow commands (`pull`, `push`, `connect`) are top-level,
reflecting the platform's connector-first architecture
([ADR-005](005-connector-system.md)). Traditional file operations (`convert`,
`extract`, `merge`) remain available as shortcuts that use the built-in
FileConnector internally.

```bash
# Connector workflow
kapi connect add contentful --api-key $KEY --space-id $SPACE
kapi pull contentful --project my-website
kapi translate --project my-website --target fr,de
kapi push contentful --project my-website

# File workflow (uses FileConnector internally)
kapi convert -i page.html -o page.xliff
kapi translate -i content.xliff --target fr --provider anthropic
kapi convert merge -i page.xliff -o page-fr.html --target fr
```

### Tool and Flow Execution

Tools are registered in the tool registry ([ADR-006](006-tool-system.md)) and
can be invoked individually or composed into flows. The CLI resolves tools by
name, including plugin-provided tools ([ADR-007](007-plugin-system.md)):

```bash
# Run a single tool
kapi tools run pseudo-translate -i content.xliff -o pseudo.xliff

# Execute a named flow
kapi flow run ai-translate-qa --project my-website --target fr,de,ja
```

### gokapi-server

The server binary (`cmd/gokapi-server/`) provides remote access via two
protocols, started by `kapi serve` or directly as `gokapi-server`.

**REST API** (Echo v4) serves external integrations, webhooks, and simple HTTP
clients:

```
POST   /api/v1/projects
GET    /api/v1/projects/:id
POST   /api/v1/projects/:id/pull
POST   /api/v1/projects/:id/push
GET    /api/v1/projects/:id/blocks
PUT    /api/v1/projects/:id/blocks/:hash/translation
POST   /api/v1/flows/run
GET    /api/v1/connectors
GET    /api/v1/formats
GET    /api/v1/health
```

**gRPC** serves Bowrain-to-server communication with streaming support:

```protobuf
service GokapiService {
    rpc CreateProject(CreateProjectRequest) returns (Project);
    rpc StreamBlocks(StreamBlocksRequest) returns (stream Block);
    rpc UpdateTranslation(UpdateTranslationRequest) returns (Block);
    rpc ExecuteFlow(ExecuteFlowRequest) returns (stream FlowProgress);
    rpc Subscribe(SubscribeRequest) returns (stream Event);
}
```

gRPC streaming enables real-time flow progress, block updates, and event
subscriptions for the Bowrain desktop app ([ADR-012](012-bowrain.md)).
The REST API and gRPC server share the same underlying service layer; protocol
handlers are thin adapters over the core domain logic.

### CI/CD Integration

The CLI is designed for non-interactive use in CI/CD pipelines. All commands
accept flags for machine-readable output (`--json`), conditional execution
(`--if-changed`), and quality gates (`--gate`):

```yaml
# GitHub Actions example
- name: Pull content
  run: kapi pull contentful --project $PROJECT --if-changed

- name: Translate
  run: kapi flow run ai-translate-qa --project $PROJECT --target fr,de,ja
  env:
    GOKAPI_AI_PROVIDER: anthropic
    GOKAPI_AI_API_KEY: ${{ secrets.ANTHROPIC_KEY }}

- name: Push if quality gate passes
  run: kapi push contentful --project $PROJECT --gate terminology-compliance
```

Plugin dependencies declared in project config ensure reproducible builds:

```yaml
plugins:
  required:
    - name: gokapi-format-docx
      version: ">=1.2.0"
```

## Alternatives Considered

- **REST only**: Simpler but no streaming for real-time progress and events.
  Bowrain would need polling, degrading the editing experience.
- **gRPC only**: Efficient and type-safe but harder for simple integrations,
  webhooks, and `curl`-based debugging.
- **GraphQL**: Flexible queries but over-engineered for this use case where the
  API surface is well-defined and resource-oriented.
- **File-first CLI taxonomy** (traditional `extract`/`merge` at top level):
  Doesn't reflect the connector-first platform vision. Connectors are the
  primary integration mechanism; file operations are the special case.

## Consequences

- Connector-first CLI commands (`connect`, `pull`, `push`) make the platform
  workflow discoverable at the top level.
- Traditional file commands (`convert`, `extract`, `merge`) remain available for
  backward compatibility and simple one-off operations.
- REST + gRPC dual protocol serves both external integrations and Bowrain's
  real-time requirements.
- gRPC streaming enables live flow progress and event subscriptions in Bowrain
  ([ADR-012](012-bowrain.md)).
- CI/CD integration via environment variables, `--json` output, conditional
  execution flags, and declarative plugin dependencies.
- `kapi serve` bridges the CLI and server: a single binary distribution includes
  both modes of operation.
