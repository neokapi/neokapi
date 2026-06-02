---
id: 016-bravo-agent
sidebar_position: 16
title: "AD-016: Bravo Agent"
---

# AD-016: Bravo Agent

## Summary

`@bravo` is a workspace-scoped AI agent that acts on behalf of an
authenticated user with delegated permissions. The runtime is a pool
of ZeroClaw containers in gateway mode; each conversation gets its own
container. Bravo reaches bowrain capabilities through the server's
MCP cloud endpoint, using a short-lived scoped token so every tool
call executes as the invoking user. Three operating modes — Ask,
Co-worker, and Voice — map to progressively broader write authority.

## Context

Many localization workflows are repetitive (batch-translating, running
QA across streams, reformatting terminology imports, auditing brand
voice) or require scripting around platform operations. Users benefit
from an assistant that understands their workspace, calls platform
tools, executes short scripts when needed, and streams progress back
in real time. The agent has to inherit existing permissions rather
than elevate them, and every action it takes must be auditable.

## Decision

### Runtime

@bravo runs as a fleet of ZeroClaw containers orchestrated by the
server. ZeroClaw is a Rust-based agent framework that compiles to a
~3.4 MB static binary, uses under 5 MB of RAM, cold-starts in under
10 ms, ships a built-in MCP client, and supports multiple LLM
providers through configuration. Each conversation is assigned a
container from the pool; the container stays warm for the conversation
duration plus an idle timeout, then recycles.

Containers run in **gateway mode** — they expose an HTTP API that the
server uses to send messages and stream responses. This keeps agent
orchestration out of the server process and keeps the runtime
self-hosted.

### Model inference

The container's `[model]` section in its ZeroClaw config points at an
LLM provider. The default is Azure OpenAI (with workspace-provided
keys); `provider = "anthropic"`, `provider = "ollama"`, or any other
supported backend swaps in with three config lines. The Azure dependency
is scoped to model inference only — agent orchestration stays on
bowrain infrastructure.

### Identity delegation

When a user sends a message, the `AgentService`:

1. Creates a **scoped agent token** (`bwt_bravo_<random>`) tied to the
   user ID, workspace, and conversation ID. The token is short-lived
   (conversation duration plus a grace period) and revoked on
   conversation end.
2. Injects the token into the container's MCP configuration as a
   bearer credential.
3. Spawns or selects a warm container, POSTs the user message to its
   gateway endpoint.
4. Streams response chunks back to the client via SSE.

Every MCP tool call resolves the token to the user's identity via the
existing auth middleware, so all actions run under the user's
workspace role. Events emitted by the agent use `actor: "bravo:<user_id>"`
so the activity feed, audit log, and notification dispatcher attribute
them correctly.

**Real agent output requires SSE streaming plus a configured container
pool (or a worker queue).** `AgentService.SendMessageStream` is the path
that reaches a real ZeroClaw container — directly when a container pool is
attached (`SetPool`), or via Service Bus and a Redis SSE relay when a
queue is configured (`SetQueue`). With neither configured, it falls back
to a local placeholder stream. The synchronous JSON `SendMessage` path
(used by non-streaming clients) does not invoke the runtime at all: it
persists the message and returns a canned echo reply that points the
caller at SSE streaming. So a deployment that has only wired the
synchronous path, or has no pool/queue, gets the placeholder — not model
output.

### Modes

| Mode           | Authority                                                                    | Typical use                                    |
| -------------- | ---------------------------------------------------------------------------- | ---------------------------------------------- |
| **Ask**        | Read-only — list, search, inspect. Writes denied.                            | Exploration, reporting, coaching               |
| **Co-worker**  | Read + write. Destructive operations pause for approval.                     | Everyday work: running flows, editing blocks   |
| **Voice**      | Read + write scoped to workspace automation authorship.                      | Designing automation rules, brand voice profiles |

Modes are selected per conversation and gate the tool policy described
below.

### Tool access via MCP

Bowrain-server exposes an MCP cloud endpoint at `/mcp/` using the
Streamable HTTP transport. Bravo's container connects on startup and
registers the available tools as native tools in the agent loop.

| Category      | Tools                                                                                     |
| ------------- | ----------------------------------------------------------------------------------------- |
| **Content**   | `list_projects`, `get_project`, `create_project`, `update_project`, `list_blocks`, `get_block`, `update_block`, `create_version`, `list_streams`, `diff_streams`, `merge_stream` |
| **Flow**      | `list_flows`, `run_flow`, `get_flow_status`                                               |
| **TM**        | `tm_search`, `tm_import`                                                                  |
| **Termbase**  | `term_search`, `term_add`                                                                 |
| **Connector** | `connector_pull`, `connector_push`, `connector_status`                                    |
| **Sandbox**   | `execute_script`                                                                          |
| **Brand**     | `check_vocabulary`, `list_profiles`, `get_voice_guide`                                    |
| **Tasks**     | `list_my_tasks`, `claim_task`, `complete_task`                                            |

Tool discovery is dynamic — new tools added to the MCP server become
available to Bravo without code changes.

### Session-scoped permission grants

Each conversation carries a `PermissionGrant` set that defaults to the
mode's authority. The user can elevate a single-session grant during
the conversation — "allow me to push to GitHub during this task" —
which records an explicit grant with a short TTL. On conversation end,
all session-scoped grants revoke. See
[AD-003](003-permissions.md) for the permission model
itself.

### Approval flow

Destructive operations prompt a step-up confirmation. The
`AgentConfig.RequireApproval` list enumerates tools that pause the
agent loop pending user action:

```
event: needs_approval
data: {"id":"tc_2","tool":"connector_push","input":{"connector":"github","project_id":"..."}}
```

The UI renders an approval card. When the user clicks Approve or Deny,
`POST /bravo/conversations/:cid/tool-calls/:tcid/approve` (or
`/deny`) resumes or aborts the loop. Approve is one-shot — the
approval applies only to the specific tool call, not subsequent calls
to the same tool.

### Event integration

Bravo emits event-bus events that integrate with the rest of bowrain:

| Event                        | When                                       |
| ---------------------------- | ------------------------------------------ |
| `agent.conversation.created` | User starts a new conversation             |
| `agent.message.sent`         | Bravo sends a response                    |
| `agent.tool.executed`        | Bravo completes a tool call               |
| `agent.tool.approved`        | User approves a gated tool call           |
| `agent.tool.denied`          | User denies a gated tool call             |
| `agent.code.executed`        | Bravo runs a script in the sandbox        |

These events flow into the ActivityRecorder ("@bravo translated 45
blocks in Website"), the NotificationDispatcher (long-running agent
tasks complete), the AutomationEngine (rules can react to agent
actions), and the audit log (full I/O persisted for compliance).

The automation engine's `run_bravo` action
([AD-013](013-automation-engine.md)) can trigger Bravo from rules —
"when `quality.gate.failed` fires on `brand-compliance`, ask @bravo to
suggest fixes".

### Sandbox

The `execute_script` tool runs code in isolated ephemeral containers:

```go
type ExecRequest struct {
    Language string            // "python" | "bash" | "node"
    Code     string
    Files    map[string][]byte
    Env      map[string]string
}

type ExecResult struct {
    Stdout   string
    Stderr   string
    ExitCode int
    Files    map[string][]byte
    Duration time.Duration
}
```

Sandbox properties:

- Ephemeral Docker containers with seccomp profiles.
- No network access by default.
- Read-only root filesystem; writable `/workspace` mount.
- Pre-built images include localization libraries (Python: `polib`,
  `babel`, `lxml`; Node: `i18next`, `icu`).
- Defaults: 60 s timeout (max 5 min), 256 MB memory, 1.0 core CPU.
- Admin-togglable per workspace via `AgentConfig.CodeExecEnabled`.

### Per-workspace configuration

```go
type AgentConfig struct {
    WorkspaceID     string
    Enabled         bool
    AllowedTools    []string   // whitelist (empty = all)
    DeniedTools     []string   // blacklist (overrides allowed)
    RequireApproval []string   // tools that pause for approval
    CodeExecEnabled bool
    MaxConcurrent   int
}
```

Admins manage the config through the web UI. Changes apply to future
conversations; active conversations finish under their original
policy.

### API

```
POST   /:ws/bravo/conversations
GET    /:ws/bravo/conversations
GET    /:ws/bravo/conversations/:cid
DELETE /:ws/bravo/conversations/:cid
POST   /:ws/bravo/conversations/:cid/messages          # returns SSE
GET    /:ws/bravo/conversations/:cid/messages
POST   /:ws/bravo/conversations/:cid/tool-calls/:tcid/approve
POST   /:ws/bravo/conversations/:cid/tool-calls/:tcid/deny
POST   /:ws/bravo/conversations/:cid/cancel
PATCH  /:ws/bravo/conversations/:cid/mode
GET    /:ws/bravo/config
PUT    /:ws/bravo/config
GET    /:ws/bravo/tools
GET    /:ws/bravo/usage                                 # tokens + container time
```

### Container pool

```go
type AgentPool struct {
    containerRuntime ContainerRuntime
    mcpEndpoint      string
    bravoImage       string
    maxPerWorkspace  int
}

type ContainerRuntime interface {
    Spawn(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error)
    Stop(ctx context.Context, containerID string) error
    Health(ctx context.Context, containerID string) (bool, error)
}
```

A spawned container receives a generated `config.toml` via volume or
environment substitution with:

- MCP endpoint URL and scoped bearer token.
- Model provider credentials from the workspace configuration.
- System prompt carrying workspace and user context (name, role,
  project identifier if the conversation is project-scoped).
- Gateway listening on the internal cluster network.

Lifecycle: spawn on first message → reuse for subsequent messages in
the same conversation (ZeroClaw retains context in its per-container
SQLite memory) → recycle after idle timeout or conversation end.
`MaxConcurrent` caps the number of simultaneous conversations per
workspace.

### Frontend

The web and desktop apps render Bravo as a 480 px collapsible
right-side sheet backed by the `assistant-ui` library. Components live
under `packages/ui/src/components/bravo/` and share the app's shadcn
primitives. Tool call visualization is status-aware:

- Pending → spinner card.
- Completed → collapsed card with summary; expandable to show input
  and output JSON.
- Needs approval → approve/deny buttons inline.

## Consequences

- Bravo inherits the user's authority. There is no path to privilege
  escalation — the token is user-scoped, short-lived, and audited.
- Every tool call is attributable. Activity feeds, audit logs, and
  notifications distinguish agent actions from direct user actions.
- Runtime is self-hosted and vendor-neutral. Switching model providers
  does not touch orchestration code.
- Code execution is first-class but gated: admins control availability
  per workspace, and destructive operations always require a human
  confirmation.
- Session-scoped permission grants give the agent enough latitude for
  one task without compromising workspace posture.
- Each container is disposable, so one misbehaving conversation cannot
  affect others.

## Related

- [AD-013: Automation Engine](013-automation-engine.md) — `run_bravo` action
- [AD-014: Translator Workflow](014-translator-workflow.md) — task MCP tools, activity events
- [AD-015: Server-Side AI Operations](015-server-ai-operations.md) — AI quota accounting
- [AD-003: Identity and Permissions](003-permissions.md) — token scopes, session grants
- [AD-018: Billing and Plans](018-billing-and-plans.md) — @bravo quota and plan gating
- [Bravo Agent Implementation](/notes/bravo-agent-implementation) — schemas and container details
