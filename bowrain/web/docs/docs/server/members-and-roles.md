---
title: Members and roles
description: Administering a Bowrain workspace — inviting members, workspace roles, project role templates, project-level membership, API tokens, and the governance controls for teams that need stricter access separation.
sidebar_position: 10
---

# Members and roles

A workspace groups the people who work on its projects. This page covers the
day-to-day administration: inviting members, the roles they hold, the finer
permission bundles that apply per project, API tokens for programmatic access,
and the governance controls for teams that need stricter separation.

Member and role administration is available to workspace owners and admins.

## Workspace roles

Every workspace member holds one workspace-level role, which sets what they can
do across the whole workspace.

| Role | What it can do |
| --- | --- |
| **Owner** | Full control, including billing and deleting the workspace. Each workspace has one owner — its creator. |
| **Admin** | Manage members, settings, roles, tokens, and all projects. Cannot manage billing or delete the workspace. |
| **Member** | Create and edit projects, and push and pull content. |
| **Viewer** | Read-only access to projects and translations. |

These roles are also summarized on the [Workspaces](/server/workspaces) page.
For fine-grained control within a single project, use
[role templates](#project-role-templates) rather than the workspace role.

## Inviting members

Go to **Workspace Settings > Members** and choose **Invite**. Enter the email
address and pick the role to grant — Admin, Member, or Viewer. (Ownership is
held by the workspace creator and is not granted through an invitation.)

An invitation is delivered by email and also carries a shareable join link. The
recipient creates an account if they do not already have one, then joins the
workspace with the role you chose. Each invitation has an expiry date, and a
pending invitation can be revoked from the same page at any time before it is
used.

Seats consumed by members count against the workspace's plan limit — one seat on
Free, three on Pro, unlimited on Team. See
[Billing and credits](/server/billing-and-credits#per-seat-pricing-on-team) for
how seats are priced.

## Project role templates

A workspace role is broad. Within a project, access is governed by **role
templates** — named bundles of permissions that decide exactly what a member may
do on that project.

Each new workspace is seeded with built-in templates:

| Template | Intended for |
| --- | --- |
| **Project Admin** | Full control over the project. |
| **Developer** | Manage files, run flows, manage connectors and automation, and contribute translations. |
| **Translator** | Translate content, scoped to specific languages. |
| **Reviewer** | Translate, review, and approve translations, scoped to specific languages. |
| **Observer** | Read-only access to project content. |

Manage templates from **Workspace Settings > Roles**. You can adjust the
permissions of a built-in template, and create custom templates of your own;
built-in templates cannot be deleted, but custom ones can.

A template is assembled from individual permissions grouped by area:

- **Content** — view content, edit source, translate, review.
- **Knowledge** — manage terminology, translation memory, brand voice, and assets.
- **Operations** — run flows, manage files, manage streams, manage connectors, manage automation.
- **Administration** — manage members, manage project.

## Project-level members

Adding someone to a project draws on both mechanisms above. From a project's
members surface you assign a workspace member a **role template**, and — for the
language-scoped templates such as Translator and Reviewer — an optional set of
**languages** they are limited to. A reviewer scoped to `fr-FR` sees and acts on
French work only; leaving the language list empty grants all languages.

This is how a single project can host translators for different languages, a
reviewer per locale, and a project admin, each with exactly the access their job
needs.

## API tokens

For programmatic access — CI pipelines, scripts, integrations — a workspace
owner or admin can issue API tokens from **Workspace Settings > API tokens**.
API access is available on Pro and above.

When creating a token you set:

- **Name** — a label to identify where the token is used.
- **Expiration** — 7, 30, 60, or 90 days, a custom date, or no expiration.
- **Scope** — either full access, or restricted to a single action (read,
  translate, review, or manage), with translate and review optionally limited to
  specific BCP-47 languages.

The token value is shown once, at creation, and cannot be retrieved again — copy
it then. A token can be deleted at any time, which immediately revokes access for
anything using it.

## Governance controls

Workspaces that need stricter access separation have a set of governance
controls under **Workspace Settings > Governance** (owner and admin only). These
go beyond everyday role assignment and have more depth than this page covers; in
brief:

- **Separation of duties** — whether a translator may approve their own work. Set
  it to off, warn (record a warning but allow it), or block (prevent anyone from
  reviewing or approving content they authored).
- **Teams** — group members so they can be granted project roles in bulk.
- **Deny rules** — negative permissions that always override grants. A rule
  targets a user, a workspace role, or a team, and removes specific permissions
  from them.
- **Workspace role overrides** — tune the default permissions of a workspace role
  when the built-in defaults do not fit.

## Next steps

- [Workspaces](/server/workspaces) — the workspace concept and settings.
- [Real-time collaboration](/server/collaboration) — how a team works on a
  project together, with tasks and review handoff.
- [Billing and credits](/server/billing-and-credits) — plans, seats, and AI
  credits.
