# Changelog

## Unreleased

### Multi-Registry Support
- **Multiple plugin registries**: Configure multiple plugin registries (e.g., official + company-internal) at global or project level
- **`kapi registry` command**: `add`, `remove`, `list` subcommands for managing global registries in `~/.config/kapi/kapi.yaml`
- **Project-level registries**: `registries` field in `.bowrain/config.yaml` overrides global registries for team portability
- **Multi-registry resolution**: Install/update tries registries in order (first match wins); search/list merges results from all registries
- **`--registry` flag**: Pin plugin operations to a specific named registry (orthogonal to existing `--channel` flag)
- **Backward compatible**: Existing `plugins.registry` single-URL config continues to work as a registry named "default"

## v0.8.0 — Platform Migration

### Content Store (Phase 1)
- **BlockIdentity**: Content-addressable hashing (SHA-256) for block deduplication and change detection
- **ContentRef**: Links blocks to external connector sources with sync tracking
- **DisplayHint**: UI rendering guidance (preview, context, max length, content type)
- **ContentStore interface**: Project CRUD, block storage with deduplication, version management
- **SQLite backend**: Full ContentStore implementation using the shared `internal/storage` layer with WAL mode
- **Version tracking**: Snapshot-based versioning with block-level diff between versions
- **Flow integration**: `WithStore()` option for connecting flows to the content store

### Connector System (Phase 2)
- **Connector interface**: Bidirectional `Pull`/`Push`/`List`/`Sync` with categories (File, Code, CMS, Design, Marketing)
- **ConnectorRegistry**: Factory-based registration pattern for connector types
- **FileConnector**: Reads/writes localization content from filesystem using format detection
- **GitConnector**: Clone/pull repos, discover resource files via glob patterns, commit translations
- **WordPressConnector**: REST API integration for WP posts/pages
- **FigmaConnector**: REST API integration for Figma text nodes with DisplayHints from bounding boxes
- **HubSpotConnector**: REST API integration for HubSpot CMS pages

### Server and CLI (Phase 3)
- **Service layer**: `ProjectService`, `ConnectorService`, `FlowService` shared between REST API and CLI
- **REST API expansion**: Full CRUD for projects, blocks, versions, connectors, pull/push operations
- **CLI connector commands**: `kapi connect add/list` for managing connectors
- **CLI store commands**: `kapi store version/versions/projects/export/import` for store management

### Event System and Automation (Phase 4)
- **EventBus**: In-process, channel-based pub/sub with per-subscriber goroutines
- **Event types**: Block, project, version, connector, flow, and quality events
- **EventEmittingStore**: Decorator that emits events on all ContentStore mutations
- **Automation engine**: Rule-based automation triggered by events with condition evaluation
- **Loop prevention**: Causation chain tracking with configurable max depth (default 5)
- **Quality gates**: Blocking and advisory gates with threshold-based evaluation
- **Webhook delivery**: HMAC-SHA256 signed webhooks with retry and exponential backoff

### gRPC Server (Phase 3.3)
- **Proto definitions**: `proto/v1/neokapi_service.proto` with streaming RPCs
- **NeokapiService**: Full gRPC server with project CRUD, block streaming, versions, connectors, flow execution, and event subscription
- **Concurrent servers**: REST and gRPC servers start concurrently with `--grpc-port` flag

### Bowrain Integration (Phase 5)
- **Store-backed projects**: ContentStore, ConnectorRegistry, and EventBus integrated into Bowrain backend
- **Connector management**: Wails-bound methods for listing, configuring, pulling, pushing, and syncing connectors
- **ConnectorPanel**: New React component for managing connectors with content browser and sync status
- **Navigation**: Connectors view added to Bowrain sidebar

### Documentation (Phase 6)
- **Developer docs**: Content Store, Connectors, Event System, Server API
- **User guide**: Content Store, Automation, CLI connect/store commands
- **Bowrain docs**: Project management and connectors guides
- **Sidebar updates**: New pages added to main documentation sidebar
