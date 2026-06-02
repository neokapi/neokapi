---
id: 020-governance-audit-rollback
sidebar_position: 20
title: "AD-020: Governance, audit, and rollback"
---

# AD-020: Governance, audit, and rollback

## Summary

Bowrain layers four governance concerns on top of the capability-envelope
permission model in [AD-003](003-permissions.md): negative permissions
(groups, deny rules, role overrides) that an administrator manages
per-workspace; attribute-based access control that gates content edits by a
block's workflow status and enforces four-eyes separation of duties at
publish; a tamper-evident, append-only audit log that hash-chains every event
and can be verified, retained on a schedule, and exported to an external SIEM;
and non-destructive rollback that restores prior content from history. Each of
these surfaces through workspace-scoped REST routes and is itself audited.

## Context

[AD-003](003-permissions.md) computes effective permissions by intersecting
base permissions, token scopes, and session grants — every layer can only
narrow what the one above allows. That model answers "what is this principal
allowed to do," but a regulated localization workspace also needs to answer:

- **How does an administrator take a capability away** from a specific user, a
  whole workspace role, or a team — independent of the role templates a user
  was granted?
- **When may content move between draft, review, and published**, and can the
  same person who translated a block also approve it?
- **What happened, who did it, and has the record been tampered with** — with
  an integrity guarantee strong enough for compliance review and an export path
  to a security information and event management (SIEM) system?
- **How is a mistaken change undone** without destroying the history that
  records both the mistake and the correction?

These are distinct from the permission envelope, so they live in their own
subsystem. The permission primitives they rely on — `audit_read` and
`rollback_changes` — are part of the bitmask documented in
[AD-003](003-permissions.md); this AD describes how they are enforced.

## Decision

### Negative permissions: groups, deny rules, role overrides

Three administrator-managed mechanisms shape base permissions before the token
and session layers narrow them further. All three are workspace-scoped and
restricted to the workspace owner or admin role.

**Groups** are named teams. A group has members (users) and role bindings that
grant a project role — with an optional language scope — to every member at
once. Group bindings feed the same project-permission resolution as direct
project memberships, so a translator added to the "French reviewers" group
inherits that group's project roles without per-project membership rows. Every
group sub-resource lookup verifies the group belongs to the request's
workspace, so a known group ID from another tenant cannot be addressed.

**Deny rules** subtract permissions. A rule names a subject — a user, a
workspace role, or a group — and a permission bitmask to remove, optionally
scoped to a single project (an empty project scope is workspace-wide).
`ResolveDenies` unions every rule that applies to a user (their own user rule,
their workspace-role rule, and any group-membership rule), and the resolved set
is subtracted from the user's permissions in `ProjectAccessMiddleware`
(`perms &^= denied`). **Denies always win:** a deny rule cannot be overcome by a
grant, because the subtraction runs after grants are resolved. This is the
escape hatch for "this user keeps everything except the ability to publish,"
expressed once rather than by editing every role they hold.

**Workspace role overrides** retune the default permissions of a built-in
workspace role (owner, admin, member, viewer) for one workspace. When a user
falls back to their workspace role for a project — because they have no explicit
project membership — the fallback honors the override
(`GetWorkspaceRoleOverride`) instead of the hardcoded role default, so a
workspace can, for example, make its `member` role read-only without touching
code.

The schema is the governance tables alongside the existing membership tables —
`groups` / `group_members` / `group_role_bindings`, `deny_rules`, and
`workspace_role_overrides` — all keyed by workspace and cascading on workspace
deletion.

### Attribute-based access control on content edits

Beyond "may this principal translate this locale," edits are gated by an
attribute of the content itself: the block's **workflow status**. Each block
carries a status — `draft`, `in_review`, or `published` — and an optional owner.
`requireEditableStatus` runs in addition to the base translate permission:

- **draft** — no extra requirement; normal permissions apply.
- **in_review** — requires the review permission for the locale, unless the
  acting user owns the block (an owner may keep working their own in-review
  content).
- **published** — editing requires the project-management permission;
  re-opening published content is privileged.

Status transitions go through `HandleSetBlockStatus`. Moving content to or from
review, or publishing, requires the review permission; un-publishing (moving a
published block back to draft or review) requires the project-management
permission. A structured reason can accompany a send-back.

**Four-eyes separation of duties.** Publishing a block is an approval step, and
the approver must not be the translator who authored the content.
`HandleSetBlockStatus` looks up the block's last attributed editor
(`GetLastEditor`) and calls `enforceSoD` before publishing. The behavior follows
the workspace separation-of-duties mode:

| Mode    | Behavior on self-approval                                     |
| ------- | ------------------------------------------------------------- |
| `off`   | No separation enforced — allowed.                             |
| `warn`  | A `sod.violation` event is recorded, but the action proceeds. |
| `block` | The action is rejected with a 403 and a `sod.violation` event. |

The mode defaults to `warn` when unset. `enforceSoD` is a reusable primitive —
it takes the acting user and the work's author — so other review and approval
handlers can adopt the same self-approval guard. ABAC and status workflow are
enforced on the PostgreSQL content store, which the server always uses.

### Tamper-evident audit log

Every event on the bus is persisted to an append-only, hash-chained `audit_log`
table by the `AuditLogger`, which subscribes to all events. A row records the
actor, source, event type, resource, effect, request context (request ID, IP,
user agent, causation ID), and before/after snapshots.

**Hash chaining.** Rows are partitioned into chains by a `chain_key` —
workspace-scoped events chain per workspace, project-only events chain per
project, and the rest share a `system` chain. Each row stores the SHA-256 of
its predecessor's hash concatenated with a canonical serialization of its own
auditable fields. A per-chain PostgreSQL advisory lock serializes appends so the
chain stays consistent under concurrency. The canonical payload truncates
timestamps to microseconds to match the stored column precision, so a row's hash
is reproducible from its stored data.

**Append-only enforcement.** A database trigger (`audit_log_append_only`)
rejects every `UPDATE` and `DELETE` on `audit_log`, raising an exception — the
single sanctioned exception is the retention pruner, which opts into deletion
for its own transaction via a session flag the trigger checks. Tampering with
the table by any other path is blocked at the database.

**Verification.** `VerifyChain` walks one chain in insertion order, recomputes
each row's hash, and confirms the `prev_hash` links are intact; a mismatch
reports the first broken row and the reason (a tampered row, or an insertion /
deletion / reorder). `VerifyAllChains` sweeps every chain. Because retention
prunes the oldest rows, verification anchors on the oldest retained row rather
than requiring a genesis link, so a pruned window still verifies.

**Retention.** `PruneOlderThan` deletes rows past a maximum age inside the
opt-in deletion transaction, and `AuditRetentionCleaner` runs it on an interval.
A non-positive maximum age disables pruning.

**SIEM export.** `SIEMExporter` subscribes to all events and forwards them to a
`SIEMSink` — `HTTPSink` posts each event as newline-delimited JSON to a webhook.
The exporter buffers on a channel and forwards from a single worker so it never
blocks the event bus; on overflow it drops and logs rather than stall the
system.

### Non-destructive rollback

Content can be restored to a prior state at three granularities, each requiring
the rollback permission and each recording the restore as a new edit (so the
rollback itself appears in history and can in turn be rolled back):

- **One block, one locale** (`HandleRollbackBlock`) — restore a target to a
  specific entry in that block's history. The full run sequence is restored when
  available (preserving inline markup), falling back to plain text. Rolling back
  a translation is language-scoped.
- **A batch** (`HandleRevertBatch`) — revert every target changed under one
  correlation ID (a single push, import, or AI-translate-file operation) to its
  pre-batch value. Targets the batch first created are blanked.
- **A point in time** (`HandleRestoreToPoint`) — restore an entire stream to the
  value it held at a past change-log cursor, named version, or timestamp.
  Targets unchanged since then are left alone; targets created after are blanked.

Every rollback emits a `rollback.performed` audit event recording the scope and
target.

### REST surface

Governance, audit, and rollback are addressed through workspace- and
project-scoped routes consistent with [AD-011](011-rest-api.md):

```
# Governance (workspace, admin/owner only)
GET    /:ws/groups                         POST /:ws/groups       DELETE /:ws/groups/:gid
GET    /:ws/groups/:gid/members            POST .../members       DELETE .../members/:uid
GET    /:ws/groups/:gid/bindings           POST .../bindings      DELETE .../bindings/:bid
GET    /:ws/deny-rules                     POST /:ws/deny-rules   DELETE /:ws/deny-rules/:rid
GET    /:ws/role-overrides                 PUT  /:ws/role-overrides/:role
GET    /:ws/sod                            PUT  /:ws/sod

# Audit
GET    /:ws/audit-log                       # workspace-wide, filterable
GET    /:ws/audit-log/verify                # chain integrity check
GET    /:ws/:id/audit-log                   # project-scoped

# Workflow status and rollback (project-scoped)
PUT    /:ws/:id/blocks/:ref/:bid/status     # draft / in_review / published
POST   /:ws/:id/blocks/:ref/:bid/rollback   # one block, one locale
POST   /:ws/:id/revert                      # a batch (by correlation id)
POST   /:ws/:id/restore                      # a stream to a point in time
```

Reading the audit log requires the audit-read permission; rollback routes
require the rollback permission. Governance routes require the workspace owner
or admin role. Every governance mutation — a group change, a deny rule, a role
override, a separation-of-duties mode change — is itself recorded in the audit
log.

## Consequences

- Administrators can subtract a capability from a user, role, or team without
  rewriting role templates, and denies cannot be overridden by a later grant.
- Group membership scales project access to teams; one binding grants a role to
  every member.
- Content cannot be published by its own author when separation of duties is set
  to block, and the warn mode records the conflict without halting work.
- The audit log is tamper-evident: appends are hash-chained, the table is
  append-only at the database, and any client with the audit-read permission can
  verify chain integrity. Retention and SIEM export keep the record bounded and
  forwardable without weakening that guarantee.
- Rollback is always non-destructive — restoring prior content writes a new
  edit, so the history that records the mistake and its correction is preserved
  and the restore can itself be undone.

## Related

- [AD-002: Authentication and Workspaces](002-authentication-and-workspaces.md) — workspace roles and memberships
- [AD-003: Permissions and Access Control](003-permissions.md) — the capability envelope and permission bitmask
- [AD-004: Content Store and Versioning](004-content-store.md) — block history backing rollback
- [AD-005: Streams](005-streams.md) — the stream scope a point-in-time restore operates on
- [AD-012: Distributed Event Bus](012-distributed-event-bus.md) — the event stream the audit log and SIEM exporter subscribe to
- [AD-013: Automation Engine](013-automation-engine.md) — quality gates and the status workflow they drive
- [AD-014: Translator Workflow](014-translator-workflow.md) — review activities that move blocks through the status workflow
