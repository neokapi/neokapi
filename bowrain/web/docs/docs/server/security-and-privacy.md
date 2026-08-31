---
title: Security and privacy
sidebar_position: 20
description: How Bowrain handles authentication, access control, the audit log, data residency, analytics, and your content. The facts a team lead needs before signing up.
---

# Security and privacy

This page describes how Bowrain handles identity, access, auditing, and your
content. It is written for the person deciding whether to bring a team onto the
platform. Every statement here reflects how the shipped software behaves; where a
detail is an operational fact about hosted Bowrain Cloud rather than a property of
the code, it is marked as such.

## Authentication

Bowrain authenticates through OpenID Connect (OIDC). After a successful sign-in,
the server issues a short-lived JSON Web Token (JWT) for the session and a
longer-lived refresh token. Refresh tokens rotate on every use, and the server
stores only a SHA-256 hash of each one, never the token itself. Reuse of an
already-rotated refresh token invalidates the entire session family, so a stolen
refresh token is single-use and detectable.

**Hosted Bowrain Cloud.** Sign-in is handled by a managed, hosted identity
service. You do not run, configure, or patch an identity provider yourself.

**Self-hosted deployments** bring their own OIDC provider. The server is
provider-neutral: it uses standard OIDC discovery to resolve the authorization
and token endpoints from the issuer, so any OIDC-compliant provider works, such
as Keycloak, Auth0, Okta, Google, Azure AD, or Dex. You point the server at
your issuer with `BOWRAIN_OIDC_ISSUER_URL` and supply the client credentials. See
[Self-hosting](/server/self-hosting) for the full configuration surface.

Tokens are held differently per client, so a token that leaks from one surface
does not compromise the others:

| Client       | Where the session token lives                                             |
| ------------ | ------------------------------------------------------------------------- |
| Web app      | HttpOnly, Secure, SameSite cookies; JavaScript in the page cannot read them |
| Desktop app  | The operating-system keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service) |
| CLI (kapi)   | The same operating-system keychain; only non-secret metadata is written to disk, scoped by file permissions |

Clients communicate with the server over HTTPS.

Operator and administrator identities are kept separate from customer
identities: the admin control plane authenticates against its own identity realm,
distinct from the one your team signs in with.

## Access control

Access is organized around workspaces. Every project belongs to exactly one
workspace, and every API route is workspace-scoped, so tenancy is enforced at the
transport layer rather than left to individual handlers.

### Roles

Each workspace member holds a role that sets a ceiling on what they can do:

| Role   | Scope                                                              |
| ------ | ----------------------------------------------------------------- |
| Owner  | Full control, including deleting the workspace and managing billing |
| Admin  | Manage members, settings, connectors, and all projects            |
| Member | Create and edit content, run flows, push and pull                 |
| Viewer | Read-only access                                                  |

A project membership can narrow a member's workspace role further, by role
template, by language, and by a point in the context space, so a reviewer can
be granted rights to French only, or to one brand's support channel only, on
one project, without gaining them workspace-wide. See
[Members and roles](/server/members-and-roles).

### Administrative governance

Beyond the base roles, a workspace owner or admin can shape permissions with
three mechanisms, all managed per workspace and all themselves recorded in the
audit log:

- **Groups**: named teams. A group grants a project role (optionally scoped to a
  language) to all of its members at once, so team access does not require
  per-person, per-project rows.
- **Deny rules**: subtractive rules that remove a set of permissions from a
  user, a role, or a group. A deny always wins: it is applied after grants are
  resolved, so it cannot be overridden by a later grant. This is how you express
  "this person keeps everything except the ability to publish" once, rather than
  by editing every role they hold.
- **Role overrides**: retune the default permissions of a built-in workspace
  role for one workspace, for example making the `member` role read-only.

### Content-status gating and separation of duties

Edits are also gated by the workflow status of the content itself. A block that is
in review requires the review permission to change; a published block requires the
project-management permission to reopen. Publishing is treated as an approval
step, and Bowrain can enforce **separation of duties** so that the person who
wrote a block is not the person who approves it. The workspace chooses the
mode: off, warn (the self-approval is recorded but allowed), or block (the
self-approval is rejected). Drafts a run produced carry machine authorship, so
the rule never stops a solo reviewer from approving them.

## The audit log

Every event on the platform is persisted to an append-only, hash-chained audit
log. Each entry records the actor, the event type, the resource, the effect, the
request context, and before/after snapshots. Entries are partitioned into chains
(per workspace, per project, or system-wide); each entry stores the SHA-256 of its
predecessor's hash combined with a canonical serialization of its own fields, so
the chain is tamper-evident.

The log is append-only at the database: a trigger rejects every update and delete
against the audit table, with the sole sanctioned exception of the scheduled
retention pruner. Any client holding the audit-read permission can verify chain
integrity through a dedicated endpoint (`GET /:workspace/audit-log/verify`), which
walks the chain, recomputes each hash, and reports the first broken link if one
exists. The log can also be forwarded to an external SIEM system.

Content changes are recoverable without destroying history: a rollback restores a
prior value as a new edit, so the record of both the mistake and its correction is
preserved and the restore can itself be undone.

## Data residency

Hosted Bowrain Cloud runs in Amazon Web Services, in the `eu-north-1` (Stockholm)
region. This is an operational fact about the managed deployment. Self-hosted
deployments run wherever you run them.

## Analytics

The web app and the server can send product-analytics events to PostHog. The
default ingestion host is PostHog's EU host (`eu.i.posthog.com`), and analytics
are only active when an analytics key is configured; a deployment with no key
configured sends no events. Server-side events cover product usage and
conversion: workspace, project, and membership lifecycle; flow runs; content
sync and connector publishes; review decisions; and the subscription funnel.
Events carry identifiers, outcomes, and bucketed counts, never your content,
file paths, or source text, and billing and access decisions themselves run
entirely on the server, outside analytics. A self-hosted deployment that does
not configure a key collects no product analytics.

The documentation sites and the landing page use the same service in a
cookieless mode: analytics state is held in memory only, nothing is written to
cookies or local storage, the browser's Do Not Track setting is respected, and
events go to the EU ingestion host.

## Your content: ownership and export

Bowrain stores your content, content memory, and terms so a team can share
them; it does not lock them in. Content is held in the platform's content store
and round-trips back to the source formats you brought it in as. Content memory
and terms export to open, standard interchange formats through the kapi CLI:

- `kapi memory export` writes **TMX 1.4**, the standard interchange format for
  content memory.
- `kapi terms export` writes **TBX**, **CSV**, or **JSON**.

The web app's terms explorer also exports the terms store as JSON. Combined with
the non-destructive history above, this means your assets are portable at any
time. If you leave, you take your content and your memory with you.

## Bring-your-own AI keys

A workspace can configure its own AI provider key. When it does, drafting and
check operations run against the customer's own provider account, and that
traffic does not pass through a shared platform key. Runs on a bring-your-own
key are not metered against Bowrain credits (usage is still counted internally
for abuse protection). See the [FAQ](/server/faq) for how this interacts with
credits.

## Compliance and questions

Bowrain does not currently hold formal third-party certifications such as SOC 2 or
ISO 27001, and this page does not claim any. For security, compliance, or data-
processing questions, including a current description of the managed deployment's
controls, contact **hello@bowrain.cloud**.
