---
id: 028-bravo-agent
sidebar_position: 28
title: "AD-028: @bravo Agent"
---
# AD-028: @bravo Agent

## Context

Bowrain is a collaborative localization platform where users manage translation projects, run flows, maintain terminology, and synchronize content through connectors. Many localization tasks are repetitive, multi-step, or require scripting — batch-translating files, running QA across streams, reformatting terminology imports, auditing brand voice compliance across projects. These tasks are well-suited for AI automation but currently require manual orchestration through the UI or CLI.

The platform already has strong foundations for agent integration:

- **MCP server** (`platform/server/mcp/`) — Streamable HTTP transport with OAuth 2.1 bearer token validation, currently exposing brand voice tools, resources, and prompts
- **Event bus** (`platform/core/event/`) — typed events with causation tracking, automation engine that evaluates rules on events
- **Tool and flow system** (`core/tool/`, `core/flow/`) — channel-based streaming pipeline with FlowExecutor, tool registry with 15+ built-in tools
- **AI infrastructure** (`core/ai/`) — LLMProvider interface with Anthropic, OpenAI, Azure OpenAI, Ollama backends
- **Workspace access control** — role-based (owner/admin/member/viewer) with API token and JWT authentication
- **Real-time infrastructure** — WebSocket hubs for collaboration and notifications, SSE capability via Echo
- **Activity and notification system** ([AD-027](./027-activities-tasks-notifications.md)) — activity feeds, task assignments, multi-channel notifications

What's missing is an **AI agent** that can operate within this infrastructure on behalf of a user — understanding context, calling platform tools, executing scripts, and streaming progress back to the user in real time.

## Decision

### @bravo: An AI Agent for Bowrain

**@bravo** is a workspace-scoped AI agent that acts on behalf of a user with delegated permissions. It runs as a lightweight [ZeroClaw](https://github.com/zeroclaw-labs/zeroclaw) agent in a Docker container within the same container app cluster as bowrain, connected to bowrain's expanded MCP server for tool access and to Azure OpenAI for model inference. This keeps the agent runtime self-hosted and vendor-agnostic — Azure is used only for LLM models, not for agent orchestration. Users interact with @bravo through a collapsible side panel in the web app.

### Design Principles

1. **Same access, explicit delegation** — @bravo inherits the invoking user's workspace role and permissions. No elevation. Every action is attributable to the user via `actor: "bravo:<user_id>"`.
2. **Configurable per workspace** — workspace admins control which MCP tools @bravo can access, which require human approval, and whether code execution is enabled. Start safe, expand deliberately.
3. **Transparent execution** — every tool call, script execution, and decision is streamed to the user in real time. The user sees what @bravo is doing and can intervene.
4. **Human-in-the-loop for destructive operations** — workspace admins define which tools require explicit approval before execution (e.g., deletions, connector pushes).
5. **Auditable** — all agent actions emit events to the event bus, appear in activity feeds, and are logged with full input/output for compliance.

---

## Architecture

### System Overview

```
┌────────────────────────────────────────────────────────────────┐
│  React Frontend                                                │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  @bravo Side Panel (assistant-ui)                        │  │
│  │  • Streaming message thread                              │  │
│  │  • Tool call cards with expand/collapse                  │  │
│  │  • Code execution display (input + stdout/stderr)        │  │
│  │  • Approval cards for gated operations                   │  │
│  └──────────────────────┬───────────────────────────────────┘  │
└─────────────────────────┼──────────────────────────────────────┘
                          │ SSE stream + REST
                          ▼
┌────────────────────────────────────────────────────────────────┐
│  Bowrain Server                                                │
│                                                                │
│  ┌──────────────┐  ┌───────────────┐  ┌─────────────────────┐ │
│  │  Agent API   │  │ Agent Service │  │ Sandbox Executor    │ │
│  │  /bravo/*    │  │ (orchestrate, │  │ (container-based    │ │
│  │  SSE stream  │  │  lifecycle,   │  │  script execution,  │ │
│  │  REST CRUD   │  │  tool policy) │  │  resource-limited)  │ │
│  └──────┬───────┘  └──────┬────────┘  └─────────────────────┘ │
│         │                 │                                    │
│  ┌──────┴─────────────────┴──────────────────────────────────┐ │
│  │  Expanded MCP Server (/mcp/*)                             │ │
│  │  content · flow · tm · termbase · connector · sandbox     │ │
│  │  + existing brand voice tools                             │ │
│  └──────────────────────┬────────────────────────────────────┘ │
└─────────────────────────┼────────────────────────────────────────┘
                          │ MCP Streamable HTTP
                          ▼
┌────────────────────────────────────────────────────────────────┐
│  Container App Cluster                                         │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  ZeroClaw Agent Containers (one per active conversation)  │  │
│  │  • ~5MB RAM, sub-10ms cold start, single Rust binary     │  │
│  │  • MCP client → bowrain MCP server (tools)               │  │
│  │  • Azure OpenAI provider (model inference only)           │  │
│  │  • SQLite memory per conversation (vector + FTS5)         │  │
│  │  • Gateway mode: HTTP API for bowrain ↔ agent comms       │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                │
│            │ Azure OpenAI API (model inference only)            │
│            ▼                                                   │
│  ┌──────────────────┐                                          │
│  │  Azure OpenAI    │  No agent lock-in — only model access    │
│  │  (GPT-4o, etc.)  │  Swappable to Anthropic, Ollama, etc.   │
│  └──────────────────┘                                          │
└────────────────────────────────────────────────────────────────┘
```

### Why ZeroClaw

[ZeroClaw](https://github.com/zeroclaw-labs/zeroclaw) is an ultra-lightweight Rust-based AI agent runtime (MIT/Apache 2.0). It compiles to a ~3.4MB binary, uses &lt;5MB RAM, and cold-starts in under 10ms. Key properties that make it ideal for @bravo:

- **MCP client built-in** — connects to external MCP servers at startup and exposes their tools to the agent loop as native tools via `McpToolWrapper`. Supports both Stdio and Streamable HTTP transports.
- **Multi-provider** — supports Azure OpenAI, OpenAI, Anthropic, Google Gemini, Ollama, OpenRouter. No vendor lock-in; swap providers via config.
- **Three runtime modes** — `agent` (CLI), `gateway` (HTTP API), `daemon` (gateway + channels + scheduler). We use `gateway` mode for HTTP communication with bowrain.
- **SQLite memory** — built-in hybrid search (70% vector cosine + 30% FTS5 BM25) for conversation context. Each container gets its own SQLite file.
- **Trait-based architecture** — every subsystem (providers, channels, memory, tools) is a Rust trait; components are swappable through configuration.
- **Container-friendly** — single static binary, minimal dependencies, designed for edge/cloud deployment.

### Backend

#### New Packages

| Package | Purpose |
|---------|---------|
| `platform/core/agent/` | Domain types: `Conversation`, `Message`, `ToolCall`, `AgentConfig`, `AgentStore` interface |
| `platform/agent/` | AgentStore implementations (PostgreSQL, SQLite), migrations |
| `platform/service/agent.go` | AgentService: orchestrates agent lifecycle, manages sessions, enforces policy |
| `platform/service/agent_pool.go` | ZeroClaw container pool: spawn, health-check, recycle agent containers |
| `platform/server/agent_handler.go` | HTTP handlers for `/bravo/*` endpoints |
| `platform/server/mcp/tools_content.go` | MCP tools: list/get/create/update projects, blocks, streams, versions |
| `platform/server/mcp/tools_flow.go` | MCP tools: list/run flows, check status |
| `platform/server/mcp/tools_tm.go` | MCP tools: TM search, import |
| `platform/server/mcp/tools_termbase.go` | MCP tools: term search, add |
| `platform/server/mcp/tools_connector.go` | MCP tools: pull, push, status |
| `platform/server/mcp/tools_sandbox.go` | MCP tool: execute_script |
| `platform/server/mcp/tool_policy.go` | Per-workspace tool access policy enforcement |
| `platform/sandbox/` | Container-based code execution: executor, isolation, resource limits |

#### Data Model

```go
// Conversation — a chat session between a user and @bravo
type Conversation struct {
    ID          string    // UUID
    WorkspaceID string    // workspace scope
    UserID      string    // invoking user
    ProjectID   string    // optional project context
    Title       string    // auto-generated or user-set
    Status      string    // "active" | "completed" | "failed"
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// Message — a single turn in a conversation
type Message struct {
    ID             string     // UUID
    ConversationID string
    Role           string     // "user" | "assistant" | "system" | "tool"
    Content        string     // text content (markdown)
    ToolCalls      []ToolCall // tools invoked in this message
    CreatedAt      time.Time
}

// ToolCall — an MCP tool invocation by @bravo
type ToolCall struct {
    ID        string          // UUID
    MessageID string
    ToolName  string          // MCP tool name
    Input     json.RawMessage // tool input parameters
    Output    json.RawMessage // tool result (populated on completion)
    Status    string          // "pending" | "running" | "completed" | "failed" | "needs_approval" | "denied"
    Duration  time.Duration
    Error     string          // error message if failed
}

// AgentConfig — per-workspace @bravo configuration
type AgentConfig struct {
    WorkspaceID     string   // one config per workspace
    Enabled         bool     // master switch
    AllowedTools    []string // whitelist (empty = all available)
    DeniedTools     []string // blacklist (overrides allowed)
    RequireApproval []string // tools that pause for human approval
    CodeExecEnabled bool     // sandbox code execution toggle
    MaxConcurrent   int      // max active conversations per workspace
}
```

#### AgentStore Interface

```go
type AgentStore interface {
    // Conversations
    CreateConversation(ctx context.Context, conv *Conversation) error
    GetConversation(ctx context.Context, id string) (*Conversation, error)
    ListConversations(ctx context.Context, workspaceID, userID string, limit, offset int) ([]*Conversation, int, error)
    UpdateConversation(ctx context.Context, conv *Conversation) error
    DeleteConversation(ctx context.Context, id string) error

    // Messages
    AddMessage(ctx context.Context, msg *Message) error
    ListMessages(ctx context.Context, conversationID string, limit, offset int) ([]*Message, error)

    // Tool calls
    AddToolCall(ctx context.Context, tc *ToolCall) error
    UpdateToolCall(ctx context.Context, tc *ToolCall) error

    // Config
    GetAgentConfig(ctx context.Context, workspaceID string) (*AgentConfig, error)
    SaveAgentConfig(ctx context.Context, cfg *AgentConfig) error

    Close() error
}
```

#### API Endpoints

All under `/api/v1/workspaces/:ws/bravo/`, protected by auth + workspace middleware:

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/conversations` | Create conversation (optional `project_id` context) |
| `GET` | `/conversations` | List conversations (paginated, filtered by user) |
| `GET` | `/conversations/:id` | Get conversation with recent messages |
| `DELETE` | `/conversations/:id` | Delete conversation |
| `POST` | `/conversations/:id/messages` | Send message → SSE stream of agent response |
| `GET` | `/conversations/:id/messages` | List messages (paginated) |
| `POST` | `/conversations/:id/tool-calls/:tcid/approve` | Approve gated tool call |
| `POST` | `/conversations/:id/tool-calls/:tcid/deny` | Deny gated tool call |
| `POST` | `/conversations/:id/cancel` | Cancel running agent |
| `GET` | `/config` | Get workspace agent config |
| `PUT` | `/config` | Update config (admin/owner only) |
| `GET` | `/tools` | List available tools (respects policy) |

#### SSE Stream Protocol

`POST /conversations/:id/messages` returns `Content-Type: text/event-stream`:

```
event: message_start
data: {"id":"msg_1","role":"assistant"}

event: content_delta
data: {"delta":"Let me translate those files..."}

event: tool_call_start
data: {"id":"tc_1","tool":"run_flow","input":{"flow":"pseudo-translate","project_id":"..."}}

event: tool_call_end
data: {"id":"tc_1","status":"completed","output":{"blocks_processed":45},"duration_ms":3200}

event: needs_approval
data: {"id":"tc_2","tool":"connector_push","input":{"connector":"github","project_id":"..."}}

event: content_delta
data: {"delta":"Translation complete. 45 blocks processed across 3 files."}

event: message_end
data: {"id":"msg_1"}
```

#### Identity Delegation

When @bravo executes MCP tools on behalf of a user:

1. AgentService creates a **scoped agent token** (`bwt_bravo_<random>`) tied to the user's ID, workspace, and conversation ID
2. Token is injected into the ZeroClaw container's MCP config as the bearer credential
3. Bowrain's MCP auth middleware resolves the token → extracts `user_id` and `workspace_role`
4. All tool calls execute under the user's permissions — no escalation possible
5. Token is short-lived (conversation duration + grace period) and revoked on conversation/container end
6. Events emitted use `actor: "bravo:<user_id>"` to distinguish agent actions from direct user actions

#### Tool Policy Enforcement

```go
type PolicyDecision string

const (
    PolicyAllow   PolicyDecision = "allow"
    PolicyDeny    PolicyDecision = "deny"
    PolicyApprove PolicyDecision = "approve" // needs human approval
)

type ToolPolicy struct {
    config *AgentConfig
}

func (p *ToolPolicy) Check(toolName string) PolicyDecision
```

Evaluated in the agent loop before each MCP tool execution. On `approve`, the SSE stream emits `needs_approval` and the loop pauses until the user responds via the approve/deny endpoint.

#### Agent Service Orchestration

```go
type AgentService struct {
    store       agent.AgentStore
    authStore   auth.AuthStore
    mcpServer   *mcp.MCPServer
    pool        *AgentPool           // manages ZeroClaw containers
    eventBus    event.EventBus
    sandbox     *sandbox.Executor
    policy      *ToolPolicy
}

// SendMessage orchestrates the full agent loop:
// 1. Persist user message
// 2. Create scoped agent token for MCP delegation
// 3. Acquire or spawn a ZeroClaw container from the pool
// 4. POST message to ZeroClaw gateway HTTP API
// 5. Stream response chunks from ZeroClaw → SSE to client
// 6. ZeroClaw calls bowrain MCP tools autonomously (with scoped token)
// 7. Tool policy is enforced at the MCP server level
// 8. On needs_approval: ZeroClaw pauses, bowrain notifies client, waits for user
// 9. Persist assistant message + tool calls on completion
// 10. Emit events to event bus
func (s *AgentService) SendMessage(ctx context.Context, convID, userID, content string, stream SSEWriter) error
```

#### ZeroClaw Container Pool

```go
// platform/service/agent_pool.go

type AgentPool struct {
    containerRuntime ContainerRuntime  // Docker/containerd/ACA API
    mcpEndpoint      string            // bowrain's MCP server URL
    bravoImage       string            // e.g. "ghcr.io/neokapi/bravo-agent:latest"
    maxPerWorkspace  int               // from AgentConfig.MaxConcurrent
}

type ContainerRuntime interface {
    Spawn(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error)
    Stop(ctx context.Context, containerID string) error
    Health(ctx context.Context, containerID string) (bool, error)
}

type AgentContainer struct {
    ID           string // container ID
    GatewayURL   string // e.g. "http://10.0.1.42:42617"
    ConversationID string
    WorkspaceID  string
    CreatedAt    time.Time
}

// Spawn creates a ZeroClaw container with:
// - config.toml injected via volume/env with MCP server URL + scoped bearer token
// - Azure OpenAI credentials from workspace config
// - System prompt with workspace/user context
// - Gateway mode listening on internal cluster network
// Container is ephemeral — recycled when conversation ends or idles
```

### Expanded MCP Tools

| Category | Tool | Description |
|----------|------|-------------|
| **Content** | `list_projects` | List workspace projects with filters |
| | `get_project` | Get project details, languages, stats |
| | `create_project` | Create a new project |
| | `update_project` | Update project settings |
| | `list_blocks` | Search/filter blocks (by locale, status, collection) |
| | `get_block` | Get block with source + all target translations |
| | `update_block` | Update a block's target translation |
| | `create_version` | Create a named version snapshot |
| | `list_streams` | List project streams |
| | `diff_streams` | Compare two streams |
| | `merge_stream` | Merge stream into main |
| **Flow** | `list_flows` | List available flows and presets |
| | `run_flow` | Execute a flow on project/files |
| | `get_flow_status` | Check flow execution status |
| **TM** | `tm_search` | Search translation memory |
| | `tm_import` | Import TM entries from file data |
| **Termbase** | `term_search` | Search terminology |
| | `term_add` | Add term entries |
| **Connector** | `connector_pull` | Pull content from external source |
| | `connector_push` | Push content to external target |
| | `connector_status` | Check connector sync status |
| **Sandbox** | `execute_script` | Run code in sandboxed container |
| **Brand** | *(existing)* | `check_vocabulary`, `list_profiles`, `get_voice_guide` |

### Code Execution Sandbox

```go
// platform/sandbox/executor.go

type Executor struct {
    workDir     string        // base temp directory
    timeout     time.Duration // default 60s, max 5m
    memoryLimit int64         // default 256MB
    cpuLimit    float64       // default 1.0 core
}

type ExecRequest struct {
    Language string            // "python" | "bash" | "node"
    Code     string            // script source
    Files    map[string][]byte // input files mounted into sandbox
    Env      map[string]string // environment variables
}

type ExecResult struct {
    Stdout   string            // captured stdout
    Stderr   string            // captured stderr
    ExitCode int
    Files    map[string][]byte // output files produced
    Duration time.Duration
}
```

- Runs in ephemeral containers (Docker/nerdctl) with seccomp profiles
- No network access by default
- Read-only root filesystem; writable `/workspace` mount for I/O
- Pre-built images include common localization libraries (Python: polib, babel, json, csv, lxml; Node: i18next, icu)
- Resource limits enforced by container runtime
- Admin-togglable per workspace via `AgentConfig.CodeExecEnabled`

### ZeroClaw Agent Runtime

#### Container Configuration

Each @bravo container runs ZeroClaw in **gateway mode** with a dynamically generated `config.toml`:

```toml
# Generated per-conversation by AgentPool

[general]
name = "bravo"
system_prompt = """
You are @bravo, an AI assistant for the Bowrain localization platform.
You help users manage translation projects, run localization workflows,
check quality, manage terminology, and automate complex tasks through
code and scripts.

You are operating in workspace "{{workspace_name}}" on behalf of
"{{user_name}}" (role: {{user_role}}).

Capabilities:
- Manage projects, streams, collections, and content blocks
- Run translation, QA, and brand voice flows
- Search and manage translation memory and terminology
- Pull/push content through connectors
- Write and execute Python/Bash/Node scripts for bulk operations
- Check and enforce brand voice compliance

Guidelines:
- Explain what you plan to do before executing tools
- For destructive operations (deletions, overwrites, pushes), always
  confirm with the user first
- When writing scripts, explain the code before running it
- Report results with clear counts and summaries
- If a task is ambiguous, ask for clarification rather than guessing
- Prefer using existing flows and tools over custom scripts when possible
"""

[model]
provider = "azure-openai"                    # swappable to "anthropic", "ollama", etc.
model = "gpt-4o"
api_base = "https://my-instance.openai.azure.com/"
api_key = "{{from_workspace_config}}"        # injected at container spawn
api_version = "2025-12-01-preview"

[gateway]
host = "0.0.0.0"
port = 42617

[memory]
enabled = true
provider = "sqlite"                          # per-conversation SQLite file

[mcp.bowrain]
transport = "http"
url = "https://app.bowrain.com/mcp/"         # or internal cluster URL
headers = { Authorization = "Bearer {{scoped_agent_token}}" }
tool_timeout_secs = 120
```

ZeroClaw's `McpRegistry` discovers all available tools from bowrain's MCP server at startup and registers them as native tools in the agent loop via `McpToolWrapper`. The agent sees `list_projects`, `run_flow`, `execute_script`, etc. as first-class tools.

#### Why Gateway Mode

ZeroClaw's gateway mode exposes an HTTP API (Axum-based) that bowrain uses to send messages and stream responses:

```
Bowrain AgentService → POST /message → ZeroClaw gateway → agent loop
                                                            ├── LLM call (Azure OpenAI)
                                                            ├── MCP tool call (→ bowrain MCP server)
                                                            └── stream response chunks → Bowrain → SSE → React
```

#### Provider Flexibility (No Vendor Lock-in)

The `[model]` section is the **only** Azure dependency. Swapping to another provider requires changing 3 config lines — no code changes:

```toml
# Anthropic
[model]
provider = "anthropic"
model = "claude-sonnet-4-20250514"
api_key = "sk-ant-..."

# Self-hosted Ollama
[model]
provider = "ollama"
model = "llama3:70b"
api_base = "http://ollama.internal:11434"
```

#### Container Lifecycle

1. **Spawn** — on first message in a conversation, AgentPool creates a container with injected `config.toml`, scoped token, and model credentials
2. **Warm** — container stays alive for the conversation duration + idle timeout (default 5m)
3. **Reuse** — subsequent messages in the same conversation hit the same container (ZeroClaw retains conversation context in SQLite memory)
4. **Recycle** — on idle timeout or conversation end, container is stopped and removed; SQLite memory can be archived for conversation history
5. **Scale** — multiple containers run concurrently across the cluster, one per active conversation, capped by `AgentConfig.MaxConcurrent`

#### @bravo Docker Image

```dockerfile
FROM ghcr.io/zeroclaw-labs/zeroclaw:latest AS runtime
# ZeroClaw binary is ~3.4MB, base image ~16MB total

WORKDIR /bravo
COPY config.toml.template /bravo/config.toml.template

# Config is injected at runtime via volume mount or env substitution
EXPOSE 42617
CMD ["zeroclaw", "gateway"]
```

Built as `ghcr.io/neokapi/bravo-agent:latest`. The image is tiny (~16MB) and cold-starts in &lt;10ms.

### Frontend

#### Library: assistant-ui

Chosen because it uses the **same UI stack** as bowrain (shadcn/ui + Radix + Tailwind CSS), provides composable primitives rather than monolithic components, is backend-agnostic (works with our custom SSE streaming), has excellent tool call visualization with status-based rendering, and is MIT licensed with 200K+ monthly downloads.

#### Component Structure

```
packages/ui/src/
├── components/bravo/
│   ├── BravoPanel.tsx              # Collapsible right-side sheet
│   ├── BravoPanelTrigger.tsx       # TopBar button to toggle panel
│   ├── BravoThread.tsx             # Message thread (assistant-ui Thread)
│   ├── BravoComposer.tsx           # Message input with file attach
│   ├── BravoToolCall.tsx           # Tool call card (expandable)
│   ├── BravoCodeBlock.tsx          # Code + execution result display
│   ├── BravoApprovalCard.tsx       # Human-in-the-loop approve/deny
│   └── BravoConversationList.tsx   # Conversation history list
├── hooks/
│   └── useBravo.ts                 # SSE stream management, state, API calls
├── context/
│   └── BravoContext.tsx            # Panel state, active conversation
└── types/
    └── bravo.ts                    # TS types matching Go data model
```

#### Panel Design

A 480px collapsible right-side sheet (non-modal, doesn't block main content):

```
┌──────────────────────────────────────────────────┬──────────────────┐
│  Main bowrain content area                       │ @bravo          │
│                                                  │                  │
│  (projects, editor, streams, etc.)               │ ┌──────────────┐│
│                                                  │ │ Message      ││
│                                                  │ │ thread       ││
│                                                  │ │              ││
│                                                  │ │ 🔧 run_flow ││
│                                                  │ │   ✓ Done     ││
│                                                  │ │              ││
│                                                  │ │ Translated   ││
│                                                  │ │ 45 blocks... ││
│                                                  │ ├──────────────┤│
│                                                  │ │ [Message...] ││
│                                                  │ └──────────────┘│
└──────────────────────────────────────────────────┴──────────────────┘
```

The panel is available on every page. Context-aware: when opened from a project page, it pre-populates the project context.

#### Tool Call Visualization

Collapsed:
```
┌───────────────────────────────────────┐
│ 🔧 run_flow                      ✓  │
│ Ran pseudo-translate on 3 files      │
│ ▸ Show details                       │
└───────────────────────────────────────┘
```

Expanded:
```
┌───────────────────────────────────────┐
│ 🔧 run_flow                      ✓  │
│                                       │
│ Input:                                │
│   flow: "pseudo-translate"            │
│   project_id: "proj_abc"             │
│   target_locale: "fr-FR"             │
│                                       │
│ Output:                               │
│   blocks_processed: 45               │
│   duration: 3.2s                     │
│                                       │
│ ▾ Hide details                       │
└───────────────────────────────────────┘
```

Approval (paused):
```
┌───────────────────────────────────────┐
│ ⚠ connector_push                     │
│ Push 45 updated blocks to GitHub     │
│ connector "my-app-repo"?             │
│                                       │
│      [Approve]    [Deny]             │
└───────────────────────────────────────┘
```

### Event Integration

New event types:

| Event Type | Emitted When |
|-----------|--------------|
| `agent.conversation.created` | User starts a new @bravo conversation |
| `agent.message.sent` | @bravo sends a response |
| `agent.tool.executed` | @bravo completes a tool call |
| `agent.tool.approved` | User approves a gated tool call |
| `agent.tool.denied` | User denies a gated tool call |
| `agent.code.executed` | @bravo runs a script in the sandbox |

All events carry `actor: "bravo:<user_id>"` and integrate with:
- **ActivityRecorder** — "@bravo translated 45 blocks in project X" appears in feeds
- **NotificationDispatcher** — notify user when long-running agent tasks complete
- **AutomationEngine** — agent actions can trigger automation rules (e.g., agent translation → auto QA)
- **Audit log** — full input/output recorded for compliance

### Security

| Concern | Approach |
|---------|----------|
| Permission enforcement | Agent inherits user's workspace role; all MCP calls go through existing auth middleware |
| Tool access control | `AgentConfig` whitelist/blacklist per workspace; admin-managed |
| Human-in-the-loop | `RequireApproval` list gates destructive operations; agent loop pauses until user responds |
| Code execution isolation | Ephemeral containers: no network, resource limits, read-only root, allowlisted languages |
| Token scoping | Short-lived `bwt_bravo_*` tokens tied to conversation, revoked on end |
| Audit trail | All agent actions emit events with full I/O; activity feed shows @bravo actions distinctly |
| Rate limiting | `MaxConcurrent` caps active conversations per workspace |

---

## Implementation Phases

### Phase 1: Foundation
- Data model + AgentStore (PostgreSQL + SQLite)
- Agent API endpoints with SSE streaming
- Expand MCP with content + flow tools
- Tool policy system
- Basic React side panel with assistant-ui
- Mock agent backend for end-to-end testing

### Phase 2: ZeroClaw Integration + Core Tools
- ZeroClaw container pool (spawn, health-check, recycle)
- @bravo Docker image + config.toml templating
- Identity delegation (scoped tokens)
- Azure OpenAI model connection
- TM, termbase, connector MCP tools
- Tool call visualization in UI
- Conversation history

### Phase 3: Code Execution
- Sandbox executor (container-based)
- `execute_script` MCP tool
- Code block rendering in UI
- Execution policies and resource limits

### Phase 4: Polish + Enterprise
- Human-in-the-loop approval flow (end to end)
- Admin configuration UI
- Agent activity in workspace feeds
- Usage metrics and cost tracking
- Rate limiting and quotas

---

## Alternatives Considered

### Frontend library

| Option | Verdict |
|--------|---------|
| **assistant-ui** | **Chosen.** Same stack (shadcn/Radix/Tailwind), composable, backend-agnostic, excellent tool call rendering |
| CopilotKit | More opinionated, AG-UI protocol adds complexity, less aligned with existing UI stack |
| Vercel AI SDK | Next.js-centric hooks; bowrain uses Vite + TanStack Router, not Next.js |
| Custom from scratch | Unnecessary; assistant-ui provides the primitives while allowing full customization |

### Agent runtime

| Option | Verdict |
|--------|---------|
| **ZeroClaw containers + Azure OpenAI** | **Chosen.** Self-hosted, no vendor lock-in, ~5MB RAM per agent, native MCP client, provider-swappable via config. Azure used only for models. |
| Azure AI Foundry Agent Service | Full managed service but hard vendor lock-in to Azure agent orchestration, data leaves your infrastructure |
| Direct LLM API + custom Go agent loop | Full control but must implement tool loop, memory, streaming from scratch |
| LangChain/LangGraph | Python-centric, heavy dependency, doesn't leverage existing Go MCP server |

### Code execution

| Option | Verdict |
|--------|---------|
| **Server-side containers** | **Chosen.** Full control, bowrain owns the sandbox, workspace-scoped, no data leaves the infrastructure |
| Azure Code Interpreter | Data goes to Azure sandbox, less control over available libraries, harder to provide workspace context files |
| MCP tool composition only | Too limiting; users expect script execution for bulk/custom operations |
