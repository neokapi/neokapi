---
id: 032-permissions-and-access-control
sidebar_position: 32
title: "AD-032: Permissions and Access Control"
---
# AD-032: Permissions and Access Control

## Context

Bowrain has evolved from simple workspace-level RBAC (owner/admin/member/viewer)
to a platform with multiple access patterns: human users editing translations,
API tokens for CI/CD integration, AI agents operating autonomously, and the
@bravo assistant acting on behalf of users with varying levels of trust. Each
pattern requires different permission scoping:

- **Human users** need project-level roles with language restrictions
  (a French translator should not edit German translations)
- **API tokens** need GitHub-style scoped access (read-only, translate-only,
  project-scoped)
- **Persona agents** (test fleet) need persistent identities with project
  membership, same as human users
- **@bravo** needs just-in-time, session-scoped permissions that the user
  grants per conversation — and its three interaction modes (Ask, Co-worker,
  Voice) each imply different permission ceilings

The system must enforce a single invariant: **every layer can only restrict
permissions, never expand them.** A token cannot grant more than the user has.
A session grant cannot exceed the token's scope. @bravo in Ask mode cannot
perform mutations even if the user is a project admin.

## Decision

### Capability Envelope Model

Effective permissions are the intersection of three layers:

```
Effective = base_permissions ∩ token_scopes ∩ session_grants
```

Each layer narrows. No layer can escalate beyond the one above it.

```
┌─────────────────────────────────────────────────────┐
│  Layer 1: Base Permissions                          │
│  (workspace role + project membership + languages)  │
│                                                     │
│  ┌───────────────────────────────────────────────┐  │
│  │  Layer 2: Token Scopes                        │  │
│  │  (API token or agent token restrictions)      │  │
│  │                                               │  │
│  │  ┌─────────────────────────────────────────┐  │  │
│  │  │  Layer 3: Session Grants                │  │  │
│  │  │  (@bravo mode, MCP session, JIT scope)  │  │  │
│  │  └─────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

### Layer 1: Base Permissions

Implemented. See the `project_members` and `role_templates` tables. Each user
has a workspace role (owner/admin/member/viewer) and optionally an explicit
project membership with a role template and language scope.

**Permission primitives** (bitmask, 15 bits):

| Permission | Bit | Description |
|---|---|---|
| `view_content` | 0 | View source and target content |
| `edit_source` | 1 | Edit source text |
| `translate` | 2 | Add/edit translations (language-scoped) |
| `review` | 3 | Approve/reject translations (language-scoped) |
| `manage_terms` | 4 | Edit terminology |
| `manage_tm` | 5 | Edit translation memory |
| `run_flows` | 6 | Execute processing flows |
| `manage_files` | 7 | Upload/delete files |
| `manage_streams` | 8 | Create/merge/delete streams |
| `manage_connectors` | 9 | Configure connectors |
| `manage_automation` | 10 | Create/edit automation rules |
| `manage_members` | 11 | Add/remove project members |
| `manage_project` | 12 | Edit project settings, archive |
| `manage_brand` | 13 | Edit brand voice profiles |
| `manage_assets` | 14 | Upload/delete media assets |

**Role templates** are workspace-scoped and configurable. Five built-in
templates are seeded on workspace creation: project-admin (all), developer,
translator, reviewer, observer. Admins can rename, customize, or add custom
templates.

**Resolution**: `ProjectAccessMiddleware` resolves permissions per-request via
a single JOIN query. When no explicit project membership exists, fallback
permissions are derived from the workspace role (backward compatibility).

### Layer 2: Token Scopes

API tokens (`bwt_*`) and agent tokens (`bwt_bravo_*`) carry scope restrictions
that narrow the user's base permissions. The `api_tokens.scopes` column
(currently `'["*"]'`) is extended with a structured scope vocabulary.

#### Scope Format

```
scope       = "*"                        // full access (current default)
            | action                     // workspace-wide action
            | action ":" constraint      // constrained action
            | "project:" id ":" action   // project-scoped action
            | "project:" id ":" action ":" constraint

action      = "read" | "translate" | "review" | "manage" | "admin"
constraint  = locale ("," locale)*       // language restriction
```

#### Scope-to-Permission Mapping

| Scope | Grants |
|---|---|
| `*` | All permissions the user has (no restriction) |
| `read` | `view_content` only |
| `translate` | `view_content`, `translate` |
| `translate:fr,de` | `view_content`, `translate` for fr and de only |
| `review` | `view_content`, `translate`, `review` |
| `manage` | All non-destructive permissions |
| `admin` | All permissions |
| `project:proj-123:translate:fr` | `translate` for fr on project proj-123 only |

#### Middleware

`ScopeRestrictionMiddleware` runs after `ProjectAccessMiddleware`. It parses
the token's scopes and intersects them with the resolved `project_permissions`
on the echo context:

```go
func ScopeRestrictionMiddleware() echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            // Only applies when an API token is used (api_token_id on context)
            tokenID, _ := c.Get("api_token_id").(string)
            if tokenID == "" {
                return next(c) // not an API token request
            }
            // Parse scopes, intersect with project_permissions and
            // project_languages, update context
            ...
        }
    }
}
```

### Layer 3: Session Grants

Session grants provide just-in-time, ephemeral permission scoping for @bravo
conversations and MCP tool sessions. They are the innermost restriction layer
— the user's permissions and token scopes set the ceiling, and the session
grant narrows further based on the interaction mode and explicit user consent.

#### Data Model

```go
type SessionGrant struct {
    SessionID   string     // conversation ID or MCP session ID
    UserID      string     // who granted
    Permissions Permission // bitmask subset of user's base permissions
    Languages   []string   // language constraint (empty = all allowed)
    ProjectIDs  []string   // project constraint (empty = all accessible)
    Tools       []string   // MCP tool allowlist (empty = all per config)
    Mode        AgentMode  // ask, coworker, voice
    ExpiresAt   time.Time  // auto-expire
}

type AgentMode string

const (
    AgentModeAsk      AgentMode = "ask"       // read-only, advisory
    AgentModeCoworker AgentMode = "coworker"  // full tool access
    AgentModeVoice    AgentMode = "voice"     // brand voice scoped
)
```

#### Storage

Session grants are ephemeral — stored in the `SessionStateStore` (in-memory
or Redis), not in the database. They expire when the conversation ends or
after a configurable timeout. The existing `SessionStateStore` interface
(used for OIDC handshake state) is the natural home.

#### @bravo Mode Permissions

Each @bravo mode defines a permission ceiling that intersects with the user's
base permissions:

| Mode | Permission Ceiling | Description |
|---|---|---|
| **Ask** | `view_content` | Read-only. Can query projects, search TM/terms, explain content. Cannot modify anything. |
| **Co-worker** | User's full permissions | Full access to all tools the user has permission for. Destructive operations require approval per `AgentConfig.RequireApproval`. |
| **Voice** | `view_content`, `manage_brand`, `review` | Brand voice focused. Can check voice compliance, review translations, manage brand profiles. Cannot modify content directly. |

#### Step-Up Prompting

When @bravo is in a restricted mode and the user requests an action that
requires higher permissions, @bravo does not silently fail. Instead, it
explains the restriction and offers to step up:

```
User: "Translate this file to French"

@bravo (Ask mode): "I'm in Ask mode right now, which means I can answer
questions but can't modify content. To translate files, switch to Co-worker
mode using the mode selector above. Would you like me to explain the
translation workflow instead?"
```

If the user switches modes, the session grant is updated in-place — the
conversation continues with the new permissions without losing context.

The step-up flow:

1. @bravo detects the requested action requires a permission it doesn't have
2. It checks whether the user's base permissions include the needed permission
3. If yes: suggests switching to a mode that allows it
4. If no: explains the user doesn't have this permission (e.g., "You're an
   observer on this project and can't translate")
5. Mode switches are instant — no re-authentication, just a session grant
   update

#### How Session Grants Flow Through the System

```
User opens @bravo in Ask mode
  → BravoContext creates SessionGrant(mode=ask, permissions=view_content)
  → Stored in SessionStateStore keyed by conversation ID

User sends "What languages does this project support?"
  → AgentService creates scoped token with session grant permissions
  → ZeroClaw container receives token
  → MCP tool call: get_project → bowrain MCP server
  → SessionGrantMiddleware intersects token permissions with session grant
  → view_content is allowed → tool succeeds → response streamed

User sends "Translate landing.html to French"
  → AgentService checks session grant: translate not in ask mode ceiling
  → Returns step-up prompt (no tool call made)

User switches to Co-worker mode
  → BravoContext updates SessionGrant(mode=coworker, permissions=user_full)
  → SessionStateStore updated
  → @bravo: "Got it, I'm now in Co-worker mode. Let me translate that file..."
  → MCP tool call: run_flow(pseudo-translate) → allowed
```

#### Middleware

`SessionGrantMiddleware` runs after `ScopeRestrictionMiddleware` (if present).
It applies only when the request context has a session grant (identified by
`bravo_session_id` on the echo context):

```go
func SessionGrantMiddleware(stateStore SessionStateStore) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            sessionID, _ := c.Get("bravo_session_id").(string)
            if sessionID == "" {
                return next(c)
            }
            grant, err := stateStore.GetSessionGrant(sessionID)
            if err != nil || grant == nil {
                return next(c)
            }
            // Intersect project_permissions with grant.Permissions
            // Intersect project_languages with grant.Languages
            // Update context
            ...
        }
    }
}
```

### Full Middleware Chain

```
Request
  → AuthMiddleware          (identify: JWT / bwt_ / session cookie)
  → WorkspaceAccessMiddleware  (workspace membership → workspace_role)
  → ProjectAccessMiddleware    (project membership → project_permissions)
  → ScopeRestrictionMiddleware (API token scopes → narrow permissions)
  → SessionGrantMiddleware     (@bravo session → narrow further)
  → Handler                    (requirePermission checks effective perms)
```

`requirePermission` and `requireLanguagePermission` don't change — they
always check what's on the echo context. The new middlewares just narrow
`project_permissions` before the handler sees them.

### Persona Agents (Test Fleet)

Persona agents are users. They authenticate via pre-generated JWTs and have
project memberships with role templates and language scopes — identical to
human users. No special handling needed. The existing `ensureUserExists`
middleware auto-creates user records for agent JWTs.

### Identity Attribution

All access patterns preserve actor identity for audit:

| Access Pattern | Actor Format | Example |
|---|---|---|
| Human user | `user_id` | `"u_abc123"` |
| API token | `user_id` (token owner) | `"u_abc123"` |
| @bravo | `bravo:<user_id>` | `"bravo:u_abc123"` |
| Persona agent | `user_id` (agent user) | `"u_agent_fr"` |

### API Token Creation UI

When creating an API token, the user selects scopes from a checklist:

```
Token Scopes
  ● Full access (all permissions)
  ○ Custom:
    ☑ Read content
    ☐ Translate      Languages: [fr] [de] [+]
    ☐ Review
    ☐ Manage files
    ☐ Run flows
    ☐ Manage (all non-destructive)

  Project restriction:
    ○ All projects
    ○ Specific projects: [Landing Page ▾] [+]
```

The resulting scopes array is stored in `api_tokens.scopes`:
```json
["translate:fr,de", "read"]
```

## Consequences

- Permission checks remain O(1) bitmask operations regardless of which
  layers are active — each middleware narrows the bitmask, handlers check
  the result.
- Token scopes are backward-compatible: existing `["*"]` tokens continue
  to work unchanged.
- @bravo modes integrate naturally: the mode sets the session grant ceiling,
  step-up prompting guides users to the right mode without breaking flow.
- Persona agents need no special treatment — they're users with project
  memberships.
- The `SessionStateStore` (already used for OIDC state) gains a new entry
  type but no new infrastructure.
- Adding new permissions requires only appending to the Go iota sequence
  and updating the UI checkbox list — existing bitmask values are stable.
