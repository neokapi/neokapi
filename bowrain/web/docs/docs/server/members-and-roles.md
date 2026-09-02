---
title: Members and roles
description: Administering a Bowrain workspace, from inviting members and the roles they hold to project role templates, grants bound to a point in the context space, custodians, API tokens, and the governance controls for teams that need stricter access separation.
sidebar_position: 10
---

# Members and roles

A workspace groups the people who work on its projects. This page covers the
day-to-day administration: inviting members, the roles they hold, the finer
permission bundles that apply per project, how a grant is bound to a point in
the context space, who counts as a custodian, API tokens for programmatic
access, and the governance controls for teams that need stricter separation.

Member and role administration is available to workspace owners and admins.

## Workspace roles

Every workspace member holds one workspace-level role, which sets what they can
do across the whole workspace.

| Role | What it can do |
| --- | --- |
| **Owner** | Full control, including billing and deleting the workspace. Each workspace has one owner, its creator. |
| **Admin** | Manage members, settings, roles, tokens, and all projects. Cannot manage billing or delete the workspace. |
| **Member** | Create and edit projects, and push and pull content. |
| **Viewer** | Read-only access to projects and their content. |

These roles are also summarized on the [Workspaces](/server/workspaces) page.
For fine-grained control within a single project, use
[role templates](#project-role-templates) rather than the workspace role.

Members are never metered: a plan caps custodian seats, markets and brands, and
nothing else. Invite everyone who might notice what a rule missed.

## Inviting members

Go to **Workspace Settings > Members** and choose **Invite**. Enter the email
address and pick the role to grant: Admin, Member, or Viewer. (Ownership is
held by the workspace creator and is not granted through an invitation.)

An invitation is delivered by email and also carries a shareable join link. The
recipient creates an account if they do not already have one, then joins the
workspace with the role you chose. Each invitation has an expiry date, and a
pending invitation can be revoked from the same page at any time before it is
used.

## Project role templates

A workspace role is broad. Within a project, access is governed by **role
templates**, named bundles of permissions that decide exactly what a member may
do on that project.

Each new workspace is seeded with built-in templates:

| Template | Intended for |
| --- | --- |
| **Project Admin** | Full control over the project. |
| **Developer** | Manage files, run flows, manage connectors and automation, and contribute translations. |
| **Translator** | Translate content, scoped to specific languages. |
| **Reviewer** | Translate, review, and approve translations, scoped to specific languages or points. |
| **Observer** | Read-only access to project content. |

Manage templates from **Workspace Settings > Roles**. You can adjust the
permissions of a built-in template, and create custom templates of your own;
built-in templates cannot be deleted, but custom ones can.

A template is assembled from individual permissions grouped by area:

- **Content**: view content, edit source, translate, review. `translate` writes
  and rejects a translation; approving one is `review`, so a template without it
  can contribute and send work back but never sign it off.
- **Knowledge**: manage terms (`manage_terms`), content memory
  (`manage_memory`), voice profiles (`manage_voice`), and assets.
- **Operations**: run flows, manage files, manage streams, manage connectors,
  manage automation.
- **Administration**: manage members, manage project.

## Project-level members

Adding someone to a project draws on both mechanisms above. From a project's
members surface you assign a workspace member a **role template**, and
optionally bound it:

- **By language.** For the language-scoped templates such as Translator and
  Reviewer, a set of **languages** they are limited to. A reviewer scoped to
  `fr` sees and acts on French work only; leaving the list empty grants all
  languages.
- **By point.** The `review`, `manage_voice` and `manage_terms` permissions
  are bound to a point in the [context space](/getting-started/the-context-graph):
  a region named by axis values such as `brand=acme` or
  `brand=acme,channel=support`. A person bounded that way reviews, or authors
  voice and terms, only for content that sits at or under that point, and the
  binding is carried on their membership.

This is how a single project can host translators for different languages, a
reviewer per market, a custodian per brand, and a project admin, each with
exactly the access their job needs. A language is one more axis, so bounding by
language and bounding by point are the same mechanism.

## Custodians and points

A **custodian** is a member who may author what governs content, the voice and
the terms, over a bounded region. The role is derived from a grant rather than
declared: giving someone `manage_voice` or `manage_terms` at a point makes them
the custodian of that point. Custodians own rules and escalations; a review
queue is a contributor's beat, and contributors are never metered.

- Custodian seats are the one thing a plan meters about people. A grant that
  would exceed the plan's allowance is refused when it is made, with the count
  and the limit.
- The workspace owner keeps an implicit, unbillable custodianship at the root
  point, so an approval is always possible.
- Points nobody holds are reported on their profile card in the
  [Context hub](/server/context#profiles), as the operator's queue and the
  buyer's exposure. An uncovered point never blocks a run or a build.
- When a plan carries no custodian seats, existing custodial authority is
  suspended, not deleted, and returns with the next plan.

See [Billing and credits](/server/billing-and-credits#custodians) for the plan
allowances.

## API tokens

For programmatic access, such as CI pipelines, scripts, and integrations, a
workspace owner or admin can issue API tokens from **Workspace Settings > API
tokens**. API access is available on every plan; tokens are not metered.

When creating a token you set:

- **Name**: a label to identify where the token is used.
- **Expiration**: 7, 30, 60, or 90 days, a custom date, or no expiration.
- **Scope**: either full access, or restricted to a single action (read,
  translate, review, or manage), with translate and review optionally limited to
  specific BCP-47 languages.

The token value is shown once, at creation, and cannot be retrieved again, so
copy it then. A token can be deleted at any time, which immediately revokes
access for anything using it. From the command line, `kapi auth token create`,
`list` and `delete` do the same (see [`kapi auth`](/cli/commands/auth)).

## Governance controls

Workspaces that need stricter access separation have a set of governance
controls under **Workspace Settings > Governance** (owner and admin only). In
brief:

- **Separation of duties**: whether a person may approve their own work. Set
  it to off, warn (record a warning but allow it), or block (refuse an approval
  of content the approver wrote). It covers approving a translation block by
  block, over a selection, and through **Approve everything passing**, where a
  refused target is left pending and counted. A draft a run produced has no
  human author, so this rule never stops the one person in a workspace from
  approving it.
- **Teams**: group members so they can be granted project roles in bulk.
- **Deny rules**: negative permissions that always override grants. A rule
  targets a user, a workspace role, or a team, and removes specific permissions
  from them.
- **Workspace role overrides**: tune the default permissions of a workspace
  role when the built-in defaults do not fit.

## Next steps

- [Workspaces](/server/workspaces): the workspace concept and settings.
- [Review](/server/review): the session a reviewer works in, and what a
  correction made there becomes.
- [Billing and credits](/server/billing-and-credits): plans, custodian seats,
  and AI credits.
