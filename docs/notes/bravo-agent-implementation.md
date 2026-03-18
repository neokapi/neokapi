---
sidebar_position: 21
title: "Bravo Agent Implementation"
---
# Bravo Agent Implementation

This note provides implementation details for [AD-028](/docs/ad/028-bravo-agent). The agent runtime uses [ZeroClaw](https://github.com/zeroclaw-labs/zeroclaw), a lightweight Rust-based agent framework, running in Docker containers with Azure OpenAI for model inference (swappable to any provider).

## Database Schema

### PostgreSQL

```sql
-- Agent conversations
CREATE TABLE agent_conversations (
    id          TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id  TEXT REFERENCES projects(id) ON DELETE SET NULL,
    title       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'failed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_conv_workspace ON agent_conversations(workspace_id, user_id, created_at DESC);

-- Agent messages
CREATE TABLE agent_messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES agent_conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
    content         TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_msg_conv ON agent_messages(conversation_id, created_at);

-- Agent tool calls
CREATE TABLE agent_tool_calls (
    id          TEXT PRIMARY KEY,
    message_id  TEXT NOT NULL REFERENCES agent_messages(id) ON DELETE CASCADE,
    tool_name   TEXT NOT NULL,
    input       JSONB NOT NULL DEFAULT '{}',
    output      JSONB,
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'running', 'completed', 'failed', 'needs_approval', 'denied')),
    duration_ms INTEGER,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_tc_msg ON agent_tool_calls(message_id);
CREATE INDEX idx_agent_tc_status ON agent_tool_calls(status) WHERE status IN ('pending', 'running', 'needs_approval');

-- Agent config per workspace
CREATE TABLE agent_configs (
    workspace_id     TEXT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    enabled          BOOLEAN NOT NULL DEFAULT true,
    allowed_tools    JSONB NOT NULL DEFAULT '[]',
    denied_tools     JSONB NOT NULL DEFAULT '[]',
    require_approval JSONB NOT NULL DEFAULT '["connector_push", "connector_pull", "merge_stream"]',
    code_exec_enabled BOOLEAN NOT NULL DEFAULT false,
    max_concurrent   INTEGER NOT NULL DEFAULT 3,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Scoped agent tokens
CREATE TABLE agent_tokens (
    id              TEXT PRIMARY KEY,
    token_hash      TEXT NOT NULL UNIQUE,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id    TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL REFERENCES agent_conversations(id) ON DELETE CASCADE,
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_token_hash ON agent_tokens(token_hash) WHERE NOT revoked;
```

### SQLite (development)

Same schema adapted for SQLite types:
- `TIMESTAMPTZ` → `TEXT` (ISO 8601 strings)
- `JSONB` → `TEXT` (JSON stored as text)
- `BOOLEAN` → `INTEGER` (0/1)
- No `CHECK` constraints on JSON (validated in Go)

## MCP Tool Specifications

### Content Tools

#### `list_projects`

**Input:**
```json
{
  "workspace_id": "ws_abc",        // required
  "include_archived": false,        // optional, default false
  "limit": 20,                      // optional, default 20, max 100
  "offset": 0                       // optional
}
```

**Output:**
```json
{
  "projects": [
    {
      "id": "proj_abc",
      "name": "Mobile App",
      "source_locale": "en-US",
      "target_locales": ["fr-FR", "de-DE", "ja-JP"],
      "block_count": 1250,
      "status": "active",
      "updated_at": "2026-03-15T10:30:00Z"
    }
  ],
  "total": 5
}
```

#### `get_block`

**Input:**
```json
{
  "project_id": "proj_abc",
  "block_id": "blk_123",
  "stream": "main"                  // optional, default "main"
}
```

**Output:**
```json
{
  "id": "blk_123",
  "collection_id": "col_abc",
  "source": {
    "locale": "en-US",
    "text": "Welcome to our app"
  },
  "targets": {
    "fr-FR": {"text": "Bienvenue dans notre application", "status": "translated"},
    "de-DE": {"text": "", "status": "untranslated"}
  },
  "notes": [],
  "updated_at": "2026-03-15T10:30:00Z"
}
```

#### `update_block`

**Input:**
```json
{
  "project_id": "proj_abc",
  "block_id": "blk_123",
  "stream": "main",
  "locale": "fr-FR",
  "text": "Bienvenue dans notre application",
  "status": "translated"             // optional
}
```

**Output:**
```json
{
  "success": true,
  "block_id": "blk_123",
  "locale": "fr-FR"
}
```

#### `run_flow`

**Input:**
```json
{
  "project_id": "proj_abc",
  "flow": "pseudo-translate",        // flow name or preset
  "stream": "main",
  "target_locales": ["fr-FR"],       // optional, defaults to all project targets
  "params": {}                       // optional flow-specific params
}
```

**Output:**
```json
{
  "job_id": "job_xyz",
  "status": "completed",
  "blocks_processed": 45,
  "duration_ms": 3200,
  "errors": []
}
```

### Sandbox Tool

#### `execute_script`

**Input:**
```json
{
  "language": "python",              // "python" | "bash" | "node"
  "code": "import json\ndata = json.loads(open('/workspace/input.json').read())\nprint(len(data['keys']))",
  "files": {                         // optional, base64-encoded files mounted at /workspace/
    "input.json": "eyJrZXlzIjogWyJhIiwgImIiLCAiYyJdfQ=="
  },
  "timeout_seconds": 30              // optional, default 60, max 300
}
```

**Output:**
```json
{
  "exit_code": 0,
  "stdout": "3\n",
  "stderr": "",
  "duration_ms": 450,
  "output_files": {}                 // base64-encoded files written to /workspace/output/
}
```

## SSE Event Reference

| Event | Data Shape | When |
|-------|-----------|------|
| `message_start` | `{id, role}` | Agent begins a response message |
| `content_delta` | `{delta}` | Incremental text chunk |
| `tool_call_start` | `{id, tool, input}` | Agent invokes a tool |
| `tool_call_end` | `{id, status, output, duration_ms, error?}` | Tool execution completes |
| `needs_approval` | `{id, tool, input, description}` | Tool requires human approval |
| `approval_resolved` | `{id, decision}` | User approved or denied |
| `message_end` | `{id}` | Response message complete |
| `error` | `{code, message}` | Unrecoverable error |
| `done` | `{}` | Stream finished |

## Container Sandbox Configuration

### Dockerfile (Python sandbox)

```dockerfile
FROM python:3.12-slim

RUN pip install --no-cache-dir \
    polib==1.2.0 \
    babel==2.16.0 \
    lxml==5.3.0 \
    pyyaml==6.0.2 \
    xlsxwriter==3.2.0 \
    openpyxl==3.1.5

RUN useradd -m -s /bin/false sandbox
USER sandbox
WORKDIR /workspace

# No network, read-only root
# Writable: /workspace only
```

### Container runtime flags

```bash
docker run \
  --rm \
  --network none \
  --read-only \
  --tmpfs /tmp:size=64m \
  --mount type=bind,source=$HOST_WORKSPACE,target=/workspace \
  --memory 256m \
  --cpus 1.0 \
  --pids-limit 64 \
  --security-opt no-new-privileges \
  --security-opt seccomp=sandbox-profile.json \
  --timeout $TIMEOUT \
  bravo-sandbox-python \
  python /workspace/script.py
```

## ZeroClaw Agent Configuration

### Container pool management

```go
// platform/service/agent_pool.go

type AgentPool struct {
    runtime     ContainerRuntime      // Docker API / Azure Container Apps API
    bravoImage  string                // "ghcr.io/neokapi/bravo-agent:latest"
    mcpEndpoint string                // internal cluster URL to bowrain MCP
    containers  map[string]*AgentContainer // conversationID → container
    mu          sync.RWMutex
}

type ContainerRuntime interface {
    Spawn(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error)
    Stop(ctx context.Context, containerID string) error
    Health(ctx context.Context, containerID string) (bool, error)
    Logs(ctx context.Context, containerID string) (io.ReadCloser, error)
}

type ContainerConfig struct {
    Image       string
    ConfigTOML  string            // rendered config.toml content
    Memory      string            // "64m" (ZeroClaw needs ~5MB, generous limit)
    CPU         string            // "0.25"
    Network     string            // internal cluster network
    Labels      map[string]string // workspace_id, conversation_id, user_id
    IdleTimeout time.Duration     // auto-stop after idle
}

type AgentContainer struct {
    ID             string
    GatewayURL     string    // "http://10.0.1.42:42617"
    ConversationID string
    WorkspaceID    string
    UserID         string
    CreatedAt      time.Time
    LastActiveAt   time.Time
}
```

### Config template rendering

AgentPool renders `config.toml` per conversation by injecting:

| Variable | Source |
|----------|--------|
| `{{workspace_name}}` | From AuthStore workspace lookup |
| `{{user_name}}` | From AuthStore user lookup |
| `{{user_role}}` | From workspace membership |
| `{{from_workspace_config}}` | Azure OpenAI key from workspace credentials store |
| `{{scoped_agent_token}}` | Freshly minted `bwt_bravo_*` token |

### Container lifecycle API

```
AgentService.SendMessage()
  ├── pool.Acquire(conversationID)
  │     ├── if exists + healthy → return existing container
  │     └── if not → spawn new container
  │           ├── render config.toml from template
  │           ├── create scoped agent token
  │           ├── containerRuntime.Spawn(config)
  │           └── wait for health check (GET /health on gateway port)
  │
  ├── POST container.GatewayURL/message (stream response)
  │     ZeroClaw internally:
  │       ├── LLM call → Azure OpenAI
  │       ├── Tool calls → bowrain MCP server (via scoped token)
  │       └── Returns streamed response chunks
  │
  └── Stream chunks → SSE to React frontend
```

### Idle reaper

A background goroutine in AgentPool checks containers every 60s:
- Containers idle > `IdleTimeout` (default 5m) are stopped
- Conversations marked "completed" or "failed" have containers removed
- On container stop, SQLite memory file is optionally archived to object storage for conversation history

### @bravo Docker image

```dockerfile
FROM ghcr.io/zeroclaw-labs/zeroclaw:latest
WORKDIR /bravo
EXPOSE 42617
CMD ["zeroclaw", "gateway"]
```

Config is injected at runtime via volume mount:
```bash
docker run -d \
  --name bravo-${CONVERSATION_ID} \
  --memory 64m \
  --cpus 0.25 \
  --network bowrain-internal \
  -v /tmp/bravo-${CONVERSATION_ID}/config.toml:/bravo/config.toml:ro \
  -v /tmp/bravo-${CONVERSATION_ID}/memory.db:/bravo/memory.db \
  ghcr.io/neokapi/bravo-agent:latest
```

### MCP connection authentication

ZeroClaw passes the scoped bearer token (`bwt_bravo_*`) as configured in `[mcp.bowrain].headers` to bowrain's MCP endpoint. Bowrain's MCP auth middleware validates the token and extracts user identity.

```
Authorization: Bearer bwt_bravo_a1b2c3d4e5f6...
```

The token resolves to:
- `user_id` — the delegating user
- `workspace_id` — the workspace scope
- `conversation_id` — the originating conversation (for audit)
- `role` — inherited from the user's workspace membership

### Provider configuration examples

Azure OpenAI (default):
```toml
[model]
provider = "azure-openai"
model = "gpt-4o"
api_base = "https://my-instance.openai.azure.com/"
api_key = "{{from_workspace_config}}"
api_version = "2025-12-01-preview"
```

Swap to Anthropic (no code changes):
```toml
[model]
provider = "anthropic"
model = "claude-sonnet-4-20250514"
api_key = "{{from_workspace_config}}"
```

Swap to self-hosted Ollama (no code changes):
```toml
[model]
provider = "ollama"
model = "llama3:70b"
api_base = "http://ollama.internal:11434"
```

### Resource footprint

| Metric | Value |
|--------|-------|
| Image size | ~16MB |
| Cold start | &lt;10ms |
| Memory per agent | ~5MB (64MB limit) |
| CPU per agent | 0.25 cores |
| Agents per 1GB RAM | ~200 (theoretical) |
| Max concurrent per workspace | Configurable (default 3) |

## Frontend Integration Details

### assistant-ui Runtime Adapter

assistant-ui uses a runtime adapter pattern. For @bravo, we implement a custom `BravoRuntime` that bridges our SSE API:

```typescript
// packages/ui/src/hooks/useBravoRuntime.ts

import { ExternalStoreRuntime } from "@assistant-ui/react";

export function useBravoRuntime(workspaceSlug: string) {
  const store = useBravoStore(workspaceSlug);

  return new ExternalStoreRuntime({
    messages: store.messages,
    isRunning: store.streaming,
    onNew: async (message) => {
      store.sendMessage(message.content[0].text);
    },
    onCancel: () => {
      store.cancel();
    },
    convertMessage: (msg) => ({
      role: msg.role,
      content: msg.role === "assistant"
        ? [
            { type: "text", text: msg.content },
            ...msg.toolCalls.map(tc => ({
              type: "tool-call" as const,
              toolCallId: tc.id,
              toolName: tc.toolName,
              args: tc.input,
              result: tc.output,
            })),
          ]
        : [{ type: "text", text: msg.content }],
    }),
  });
}
```

### Zustand Store

```typescript
// packages/ui/src/hooks/useBravoStore.ts

interface BravoState {
  // Panel state
  isOpen: boolean;
  setOpen: (open: boolean) => void;

  // Conversations
  conversations: Conversation[];
  activeConversationId: string | null;
  loadConversations: () => Promise<void>;
  createConversation: (projectId?: string) => Promise<Conversation>;
  setActiveConversation: (id: string) => void;

  // Messages & streaming
  messages: Message[];
  streaming: boolean;
  pendingApproval: ToolCall | null;
  sendMessage: (content: string) => Promise<void>;
  approve: (toolCallId: string) => Promise<void>;
  deny: (toolCallId: string) => Promise<void>;
  cancel: () => void;
}
```

### Route Integration

The BravoPanel mounts in the workspace layout, available on all workspace pages:

```typescript
// bowrain/apps/web/src/routes/_workspace.tsx (layout route)

export default function WorkspaceLayout() {
  return (
    <BravoProvider workspaceSlug={workspace.slug}>
      <div className="flex h-screen">
        <Sidebar />
        <main className="flex-1">
          <TopBar>
            <BravoPanelTrigger />
          </TopBar>
          <Outlet />
        </main>
        <BravoPanel />
      </div>
    </BravoProvider>
  );
}
```

## Default Tool Policy

Initial `require_approval` defaults for new workspaces:

```json
[
  "connector_push",
  "connector_pull",
  "merge_stream",
  "create_project",
  "execute_script"
]
```

These can be modified by workspace admins via `PUT /bravo/config`.

Default `denied_tools` is empty (all tools available). Admins can restrict access by either:
- Setting `allowed_tools` to a whitelist (only these tools are available)
- Setting `denied_tools` to a blacklist (these tools are blocked, rest are available)

If both are set, `denied_tools` takes precedence over `allowed_tools`.
