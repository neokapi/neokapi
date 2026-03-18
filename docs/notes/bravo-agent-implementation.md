---
sidebar_position: 21
title: "Bravo Agent Implementation"
---
# Bravo Agent Implementation

This note provides implementation details for [AD-028](/docs/ad/028-bravo-agent).

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

## Azure AI Foundry Agent Configuration

### Agent definition

```json
{
  "name": "bravo",
  "model": "gpt-4o",
  "instructions": "<<system prompt from AD-028>>",
  "tools": [
    {
      "type": "mcp",
      "mcp": {
        "server_label": "bowrain",
        "server_url": "https://app.bowrain.com/mcp/",
        "require_approval": "never",
        "allowed_tools": []
      }
    }
  ],
  "tool_resources": {},
  "temperature": 0.3,
  "top_p": 0.9
}
```

### MCP connection authentication

Azure AI Foundry passes the scoped bearer token (`bwt_bravo_*`) as the Authorization header to bowrain's MCP endpoint. Bowrain's MCP auth middleware validates the token and extracts user identity.

```
Authorization: Bearer bwt_bravo_a1b2c3d4e5f6...
```

The token resolves to:
- `user_id` — the delegating user
- `workspace_id` — the workspace scope
- `conversation_id` — the originating conversation (for audit)
- `role` — inherited from the user's workspace membership

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
