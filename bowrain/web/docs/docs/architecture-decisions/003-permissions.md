---
id: 003-permissions
sidebar_position: 3
title: "AD-003: Permissions and Access Control"
---

# AD-003: Permissions and Access Control

## Summary

Effective permissions in Bowrain are the intersection of three layers: base
permissions from workspace and project membership, token scopes that narrow
access for API tokens and agent tokens, and session grants that further scope
@bravo conversations and MCP sessions. Each layer can only restrict — never
expand — what the one above allows.

## Context

Bowrain serves multiple access patterns with very different trust
characteristics. Human translators edit content within language scopes. CI
pipelines authenticate as API tokens and need GitHub-style scoped access.
Persona agents in the test fleet authenticate as ordinary users with project
memberships. The @bravo AI assistant acts on behalf of a user with
just-in-time, session-scoped permissions that reflect the current interaction
mode.

A single principle governs all of it: **every layer can only narrow
permissions, never expand them.** A token cannot grant more than its user
has. A session grant cannot exceed the token's scope. @bravo in Ask mode
cannot mutate content even if the user is a project admin.

## Decision

### Capability Envelope Model

Effective permissions are computed as a bitmask intersection across three
layers:

```
Effective = base_permissions ∩ token_scopes ∩ session_grants
```

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

Each middleware in the request chain narrows the bitmask; handlers always
check what's on the request context.

### Permission Primitives (17-Bit Bitmask)

| Permission          | Bit | Description                                   |
| ------------------- | --- | --------------------------------------------- |
| `view_content`      | 0   | View source and target content                |
| `edit_source`       | 1   | Edit source text                              |
| `translate`         | 2   | Add/edit translations (language-scoped)       |
| `review`            | 3   | Approve/reject translations (language-scoped) |
| `manage_terms`      | 4   | Edit terminology                              |
| `manage_tm`         | 5   | Edit translation memory                       |
| `run_flows`         | 6   | Execute processing flows                      |
| `manage_files`      | 7   | Upload/delete files                           |
| `manage_streams`    | 8   | Create, merge, delete streams                 |
| `manage_connectors` | 9   | Configure connectors                          |
| `manage_automation` | 10  | Create/edit automation rules                  |
| `manage_members`    | 11  | Add/remove project members                    |
| `manage_project`    | 12  | Edit project settings, archive                |
| `manage_brand`      | 13  | Edit brand voice profiles                     |
| `manage_assets`     | 14  | Upload/delete media assets                    |
| `audit_read`        | 15  | Read the audit log                            |
| `rollback_changes`  | 16  | Roll back / restore content to a prior state  |

Adding a new permission appends to the iota sequence. Existing bitmask values
are stable.

### Layer 1: Base Permissions

Each user has a workspace role (see
[AD-002: Authentication and Workspaces](002-authentication-and-workspaces.md))
and optionally explicit project memberships with a role template and
language scope.

**Role templates** are workspace-scoped records in `role_templates`. Five
templates are seeded on workspace creation — project-admin (all),
developer, translator, reviewer, observer — and admins can customize or add
new templates.

```sql
CREATE TABLE role_templates (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    name         TEXT NOT NULL,
    permissions  BIGINT NOT NULL,     -- bitmask
    is_builtin   BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (workspace_id, name)
);

CREATE TABLE project_members (
    project_id       TEXT NOT NULL,
    user_id          TEXT NOT NULL,
    role_template_id TEXT NOT NULL REFERENCES role_templates(id),
    languages        TEXT NOT NULL DEFAULT '',  -- JSON array, empty = all
    PRIMARY KEY (project_id, user_id)
);
```

`ProjectAccessMiddleware` resolves permissions per request via a single JOIN
query. When no explicit project membership exists, fallback permissions are
derived from the workspace role so every workspace member has baseline
access.

### Layer 2: Token Scopes

API tokens (`bwt_*`) and agent tokens (`bwt_bravo_*`) carry scope
restrictions that narrow the user's base permissions. Scopes are stored as
a JSON array in `api_tokens.scopes`.

Scope grammar:

```
scope       = "*"                        // full access (user's full permissions)
            | action                     // workspace-wide action
            | action ":" constraint      // constrained action
            | "project:" id ":" action   // project-scoped action
            | "project:" id ":" action ":" constraint

action      = "read" | "translate" | "review" | "manage" | "admin"
constraint  = locale ("," locale)*       // language restriction
```

Scope-to-permission mapping:

| Scope                           | Grants                                         |
| ------------------------------- | ---------------------------------------------- |
| `*`                             | All permissions the user has (no restriction)  |
| `read`                          | `view_content` only                            |
| `translate`                     | `view_content`, `translate`                    |
| `translate:fr,de`               | `view_content`, `translate` for fr and de only |
| `review`                        | `view_content`, `translate`, `review`          |
| `manage`                        | All non-destructive permissions                |
| `admin`                         | All permissions                                |
| `project:proj-123:translate:fr` | `translate` for fr on project proj-123 only    |

`ScopeRestrictionMiddleware` runs after `ProjectAccessMiddleware`. It parses
the token's scopes and intersects them with the resolved project
permissions. When the request is not token-authenticated (e.g. a browser
session cookie), the middleware is a no-op.

### Layer 3: Session Grants

Session grants provide just-in-time, ephemeral permission scoping for
@bravo conversations and MCP tool sessions. The user's base permissions and
token scopes set the ceiling; the session grant narrows further based on
interaction mode and explicit user consent.

```go
type SessionGrant struct {
    SessionID   string     // conversation ID or MCP session ID
    UserID      string     // who granted
    Permissions Permission // bitmask subset of user's base permissions
    Languages   []string   // language constraint (empty = all allowed)
    ProjectIDs  []string   // project constraint (empty = all accessible)
    Tools       []string   // MCP tool allowlist (empty = all per config)
    Mode        AgentMode  // ask, coworker, voice
    ExpiresAt   time.Time
}

type AgentMode string

const (
    AgentModeAsk      AgentMode = "ask"
    AgentModeCoworker AgentMode = "coworker"
    AgentModeVoice    AgentMode = "voice"
)
```

Session grants are ephemeral — they live in the `SessionStateStore` (the
same in-memory or Redis store used for OIDC handshake state, see
[AD-002: Authentication and Workspaces](002-authentication-and-workspaces.md))
and expire when the conversation ends or after a configurable timeout.

### @bravo Modes

Each @bravo mode defines a permission ceiling that intersects with the
user's base permissions:

| Mode          | Permission Ceiling                       | Description                                                                                                |
| ------------- | ---------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| **Ask**       | `view_content`                           | Read-only. Can query projects, search TM/terms, explain content. Cannot modify anything.                   |
| **Co-worker** | User's full permissions                  | Full access to all tools the user has permission for. Destructive operations require approval per `AgentConfig.RequireApproval`. |
| **Voice**     | `view_content`, `manage_brand`, `review` | Brand voice focused. Can check voice compliance, review translations, manage brand profiles.               |

Voice is workspace-scoped: a Voice-mode session touches brand voice
profiles and review comments across all projects in the workspace, but
cannot translate or edit content directly.

### Step-Up Prompting

When @bravo is in a restricted mode and the user requests an action that
requires higher permissions, it does not silently fail. Instead it
explains the restriction and offers to switch modes:

```
User: "Translate this file to French"

@bravo (Ask mode): "I'm in Ask mode right now, which means I can answer
questions but can't modify content. To translate files, switch to Co-worker
mode using the mode selector above. Would you like me to explain the
translation workflow instead?"
```

If the user switches modes, the session grant is updated in place and the
conversation continues without losing context. The step-up flow:

1. @bravo detects the requested action requires a permission it lacks.
2. It checks whether the user's base permissions include that permission.
3. If yes, it suggests switching to a mode that allows the action.
4. If no, it explains the user doesn't have the permission (e.g., "You're
   an observer on this project and can't translate").
5. Mode switches are instant — no re-authentication, just a session grant
   update.

For destructive operations the server also emits a step-up prompt (e.g.
deleting a project, merging a stream, revoking a token), which the client
surfaces as an explicit confirm.

### Middleware Chain

```
Request
  → AuthMiddleware            (identify: JWT / bwt_ / session cookie)
  → WorkspaceAccessMiddleware  (workspace membership → workspace_role)
  → ProjectAccessMiddleware    (project membership → project_permissions)
  → ScopeRestrictionMiddleware (API token scopes → narrow permissions)
  → SessionGrantMiddleware     (@bravo session → narrow further)
  → PlanGuard                  (workspace subscription tier)
  → QuotaGuard                 (billing usage)
  → Handler                    (requirePermission checks effective perms)
```

`requirePermission` and `requireLanguagePermission` always read from the
echo context — the preceding middlewares narrow what they see. Plan and
quota guards run late so permission checks happen before subscription
checks.

### Subject Types and Attribution

All access patterns preserve actor identity for audit:

| Subject Type   | Actor Format            | Example            |
| -------------- | ----------------------- | ------------------ |
| Human user     | `user_id`               | `"u_abc123"`       |
| API token      | `user_id` (token owner) | `"u_abc123"`       |
| @bravo         | `bravo:<user_id>`       | `"bravo:u_abc123"` |
| Persona agent  | `user_id` (agent user)  | `"u_agent_fr"`     |

Every mutation records the actor in the audit log, so a
`bravo:u_abc123`-attributed edit is distinguishable from a direct
`u_abc123` edit.

Persona agents are users. They authenticate via pre-generated JWTs and
carry ordinary project memberships with role templates and language scopes
— identical treatment to human users. The `ensureUserExists` middleware
auto-creates user records for agent JWTs.

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

Tokens appear in the token list with a human-readable scope summary and a
copy-once secret that is never shown again.

## Consequences

- Permission checks remain O(1) bitmask operations regardless of which
  layers are active — each middleware narrows the bitmask, handlers check
  the result.
- Token scopes compose cleanly: a translator who creates an API token with
  `translate:fr` cannot exceed their own translate-only permission, even
  if they're later elevated to admin.
- @bravo modes integrate naturally: the mode sets the session grant
  ceiling, step-up prompting guides users to the right mode without
  breaking the conversation.
- Persona agents need no special treatment — they are users with project
  memberships.
- Adding a new permission requires appending to the iota sequence and
  updating the UI checklist; existing bitmask values are stable.
- Every mutation carries an attributable actor, including @bravo's
  `bravo:<user_id>` prefix that distinguishes agent-initiated changes from
  direct user edits.

## Related

- [AD-002: Authentication and Workspaces](002-authentication-and-workspaces.md)
- [AD-004: Content Store and Versioning](004-content-store.md)
