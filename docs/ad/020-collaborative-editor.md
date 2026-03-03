---
id: 020-collaborative-editor
sidebar_position: 20
title: "AD-020: Collaborative Editor and Offline-First Desktop"
---
# AD-020: Collaborative Editor and Offline-First Desktop

## Context

The Bowrain desktop app ([AD-012](./012-bowrain.md)) started as a standalone file editor. As Bowrain Server evolved into a multi-user platform ([AD-015](./015-auth-and-workspaces.md)), the desktop app needed to connect to the server for real-time collaborative editing while remaining fully functional offline.

This creates a fundamental tension: the app must be useful without a network connection (airplane mode, poor connectivity, local-only workflows) while also supporting real-time collaboration when connected (presence awareness, instant block updates, team coordination).

## Decision

### Architecture: gRPC EditorService

The desktop app communicates with Bowrain Server via a dedicated gRPC `EditorService` with 24 RPCs organized into 7 categories:

| Category | RPCs | Purpose |
|----------|------|---------|
| **Auth & Workspace** | GetCurrentUser, ListWorkspaces | Identity and workspace discovery |
| **Projects** | ListEditorProjects, GetEditorProject | Read-only project browsing |
| **Blocks** | GetBlocks, UpdateBlockTarget, ReviewBlock | Translation editing |
| **Context** | LookupTMForBlock, LookupTermsForBlock | TM and terminology lookups |
| **TM CRUD** | GetTMEntries, GetTMCount, AddTMEntry, UpdateTMEntry, DeleteTMEntry | Translation memory management |
| **Terminology** | GetTerms, GetTermCount, AddConcept, UpdateConcept, DeleteConcept, ImportTermsCSV, ImportTermsJSON, ExportTermsJSON | Terminology management |
| **Real-time** | WatchProject (server-streaming), UpdatePresence | Collaboration |

gRPC was chosen over REST for the desktop client because:
- **Server-streaming** (`WatchProject`) delivers real-time block changes and presence events without polling
- **Binary protocol** reduces overhead for frequent small updates (block edits)
- **Strong typing** via protobuf generates both Go server code and Go client code

The gRPC port follows a discovery convention: **HTTP port + 1000** (e.g., `localhost:8080` → gRPC at `localhost:9080`), with TLS auto-detected from the URL scheme.

### Real-time Collaboration

Two mechanisms enable real-time awareness:

**WatchProject** — A server-streaming RPC that opens when a user navigates to a project. It delivers two event types:
- `BlockChangeEvent`: block created/updated/deleted, with the editor's name
- `PresenceChangeEvent`: user joined/moved to a different block/left

**UpdatePresence** — A fire-and-forget unary RPC called when a user moves focus to a different block. This updates the user's cursor position for other collaborators.

Together, these enable features like showing who is editing which block, highlighting recently changed blocks, and avoiding conflicting edits.

### Connection State Machine

The desktop app manages four connection states:

```
disconnected ──StartLogin──→ connecting ──ConnectToServer──→ connected
                                                                │
                                                          (connection lost)
                                                                │
                                                                ▼
                                                             offline
                                                                │
                                                      (reconnect loop)
                                                                │
                                                                ▼
                                                            connected
```

- **disconnected**: No server configured. Local-only mode.
- **connecting**: Login in progress (PKCE flow active).
- **connected**: Active gRPC connection. Server-first for all operations.
- **offline**: Was connected but lost connection. Uses local cache, queues mutations.

### Offline-First Architecture

When connected, the app follows a **server-first, cache-locally** pattern:

1. **Read operations**: Call server via gRPC. On success, cache the response in local SQLite. On failure, fall back to cached data and transition to offline mode.
2. **Write operations**: Call server. On failure, enqueue the mutation in the offline queue and transition to offline mode.

The offline queue is a SQLite table (`pending_changes`) with FIFO ordering:

```sql
CREATE TABLE pending_changes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    operation  TEXT NOT NULL,
    payload    TEXT NOT NULL DEFAULT '{}',
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);
```

Queued mutations include: `UpdateBlockTarget`, `ReviewBlock`, `AddTMEntry`, `UpdateTMEntry`, `AddConcept`, `UpdateConcept`.

### Reconnection and Queue Replay

When the connection is lost:
1. State transitions to `offline`
2. A reconnection goroutine starts with exponential backoff (2s → 60s max)
3. Each attempt calls `ConnectToServer()` with stored credentials
4. On successful reconnection:
   - State returns to `connected`
   - Offline queue replays in FIFO order (oldest first)
   - Replay stops on first failure to preserve ordering guarantees
   - Completed mutations are purged from the queue

### Desktop Authentication: PKCE + Keyring

Desktop auth uses OAuth 2.0 Authorization Code with PKCE (RFC 7636):

1. App generates a PKCE code verifier and challenge
2. Opens system browser to `{server}/api/v1/auth/desktop/login` with the challenge
3. User authenticates via Keycloak OIDC in the browser
4. Server redirects to `bowrain://auth/callback` with tokens
5. OS routes the URL protocol to the app's `HandleAuthURL` handler
6. App exchanges the callback for access and refresh tokens

**Token storage is split for security:**
- **Secrets** (access token, refresh token): Stored in OS keyring (macOS Keychain, Windows Credential Manager, Linux Secret Service)
- **Metadata** (server URL, expiry, user info): Stored in `<UserConfigDir>/bowrain-desktop/auth.json`

This ensures tokens are never written to plaintext files and benefit from OS-level encryption and access control.

### gRPC Authentication

Both unary and streaming RPCs use JWT authentication:
- Client sets `authorization: Bearer <token>` in gRPC metadata
- Server interceptors extract and validate JWT claims
- Claims are injected into the request context
- Handlers retrieve claims via `GRPCUserFromContext(ctx)`

The same JWT tokens used for REST API access work for gRPC, providing unified auth across both protocols.

## Alternatives Considered

- **REST + WebSocket for streaming**: Would work but requires two protocols. gRPC provides both request/response and streaming in one framework.
- **Operational Transform / CRDT**: Full conflict resolution for concurrent edits. Deferred — the current model uses last-writer-wins at the block level, which is sufficient for translation workflows where blocks are typically edited by one person at a time. Presence awareness reduces conflicts.
- **Token storage in config files**: Simpler but insecure. OS keyring integration adds a dependency per platform but protects credentials properly.
- **Polling for real-time updates**: Would work but creates unnecessary load and latency. Server-streaming gRPC is more efficient and delivers instant updates.

## Consequences

- The desktop app works fully offline with local SQLite cache and offline queue. Users can translate without a network and sync later.
- Real-time presence and block change streaming enable team collaboration without polling.
- PKCE + keyring provides secure auth without exposing tokens to filesystem-level attacks.
- The exponential backoff reconnection is transparent to the user — the app silently reconnects and replays queued changes.
- gRPC port discovery convention (HTTP + 1000) means no additional configuration for users — connect to the web URL and gRPC is auto-discovered.
- The 24-RPC EditorService covers all desktop editing needs: blocks, TM, terminology, presence. New features require new RPCs but no protocol changes.
