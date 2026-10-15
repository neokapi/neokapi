---
sidebar_position: 6
title: How Bowrain and kapi fit together
sidebar_label: Bowrain and kapi
description: kapi processes local project files. Bowrain shares context across projects and adds collaborative review, connectors and server automation.
keywords: [kapi, bowrain, context graph, reach, connectors, governed review, open source]
---

# How Bowrain and kapi fit together

Every project carries a [content context](/getting-started/the-context-graph):
the coordinates that fix voice, terms and rules for a particular audience,
surface, market and moment. Both products resolve that context and apply it to
real files. They differ in how far it reaches.

> **kapi holds the context graph for one project. Bowrain holds the same graph
> across projects.**

## Reach, not capability

[kapi](https://neokapi.github.io/) is an Apache-2.0 content toolchain. It reads and
writes local files, resolves project context, runs checks, drafts text and
translates. It works without a Bowrain server or account.

Bowrain runs the same content engine for a shared workspace. Projects can reuse
voice profiles, terms and content memory. Bowrain also provides:

- **Collaborative review.** Members can review changes, record approvals and
  inspect the audit trail. See [Review](/server/review),
  [Voice and corrections](/server/context-voice) and
  [Members and roles](/server/members-and-roles).
- **Cross-project context.** Workspace terms, profiles and approved wording
  can apply to content from several projects. See the
  [Context hub](/server/context).

## From one machine to a workspace

Kapi Desktop shows how far a project's context reaches, as three rungs in a
project's **Share** section. The context is the same on every rung: rules,
terms, voice profiles, content memory, and what assistants noticed. What
changes is who reads it and where it is decided.

- **On this Mac.** kapi keeps the context in a workspace on your machine. Every
  clone and worktree of the project there reads it, and nobody else does.
- **Shared with your team.** The recipe names a place: a git ref in the
  project's own repository, a folder, or an S3 bucket. `kapi context sync`
  takes in what others shared and shares what your machine recorded, so every
  machine that syncs reads the same context. Each person still decides on
  their own machine.
- **Managed on Bowrain.** A Bowrain workspace holds every project's context in
  one place, on the server. A project reaches it through the `bowrain:` block
  in its recipe and the kapi-bowrain plugin, and Kapi Desktop then names the
  server and the project there.

A shared place gives every machine the same context. A workspace gives the
team one place to decide it, and adds what a shared place has no means to
hold:

- **Roles.** Who may review, approve and change the context is set per member.
  See [Members and roles](/server/members-and-roles).
- **Review on the record.** A named reviewer approves, and the decision stays
  in the audit trail. See [Review](/server/review).
- **Connectors.** Content from content platforms, design tools and
  repositories meets the same context as your checkouts. See
  [Connectors](/server/connectors).
- **Server automation.** Rules respond to events across the workspace, for
  content from any connector. See [Automation](/server/automation).
- **The loop on the server.** `kapi up` on a connected project runs the loop
  on the server, on the organization's keys and its shared content memory and
  terms. See [`kapi up`](/cli/commands/up).

## kapi in two roles

- **The engine underneath.** The same format handling, checks, and flow
  execution run inside Bowrain's server, which is why the platform behaves
  identically whether content arrived from a content platform or from a
  repository.
- **One connector into the platform.** With the bowrain plugin installed, kapi
  connects a developer's checkout to a workspace, the developer and CI route
  described in [the kapi connector](/server/connectors/kapi). It sits alongside
  the content-platform, design, and repository connectors rather than in front
  of them.

## Where each one writes

**kapi owns the local files and the project configuration.** The `kapi.yaml`
recipe, with its content collections, flows, plugins, languages, coordinates,
voice binding and `bowrain:` block, is authored and versioned in the repository
with everything else. Bowrain never writes it on its own: when a person approves
an axis a [context scan](/server/context-scan) proposed, the approval arrives as
a `kapi pull` that edits `defaults.coordinates` in your working tree, for you to
review and commit like any other change.

**The Bowrain desktop app caches server data for local use.**
The Bowrain desktop app is a working copy of the server: a content cache, an
offline edit queue, and memory and terms mirrors. It does not author local
files or source projects from a filesystem; sourcing from a filesystem or a git
checkout happens *server-side* through [connectors](/server/connectors), on the
host the server runs on.

## At a glance

| | **kapi** | **Bowrain** |
| --- | --- | --- |
| Reach of the graph | One project | Every project in the workspace |
| Shape | A CLI + desktop app you install | A server + web and desktop clients |
| Who decides | You, in a commit | A reviewer, on the record |
| Where context is stored | A local workspace, shared by project checkouts | On the server, with a desktop cache |
| How guidance is updated | Corrections and decisions are recorded in the workspace context store | Corrections aggregate into candidate rules a reviewer promotes |
| Content sources | Local files you own | Every [connector](/server/connectors): content platforms, design tools, repositories, checkouts |
| Automation | Local recipe rules | Server-side, event-driven |
| Cost | Free, open source | Hosted plans / self-host |

## Which one to reach for

Use kapi to process local project files from a terminal, the desktop app, CI or
an AI assistant over [MCP](/cli/mcp).

Use Bowrain when you need shared context across projects,
[connectors](/server/connectors) for external content systems,
[collaborative editing](/server/collaboration), or reviewed and audited decisions.

See the [introduction](/introduction) for what the platform does, and
[Connectors](/server/connectors) for every route into it.
