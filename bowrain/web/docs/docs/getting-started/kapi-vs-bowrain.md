---
sidebar_position: 6
title: How Bowrain and kapi fit together
sidebar_label: Bowrain and kapi
description: kapi holds the context graph for one project; Bowrain holds the same graph across projects. The difference is reach, not capability, and working locally is not a reduced mode.
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

[kapi](https://neokapi.github.io/) is the Apache-2.0 engine: it reads and writes
content formats, resolves the project's context, checks content against it,
drafts and adapts text, and translates, entirely from local files, with no
server and no account. A project governed only by kapi is fully governed.
Nothing about working locally is a reduced mode, and the platform is not a later
stage you graduate to.

Bowrain runs the same engine and the same graph at organization scope. One
profile, one set of concepts, one content memory serve every project that draws
on them, and they are current for everyone at once rather than at whoever last
ran a command. That is the whole of the difference: a graph that reaches one
checkout, or a graph that reaches all of them.

Two things follow from reach that a single checkout cannot produce on its own:

- **Governed review.** A correction is only a shared rule once someone with the
  authority to decide has approved it and the decision is recorded. That needs
  more than one person, a history, and an audit trail; see
  [Review](/server/review), [Voice and corrections](/server/context-voice) and
  [members and roles](/server/members-and-roles).
- **Cross-project context.** A term banned in one product is usually banned in
  the next one. Concepts, profiles and approved wording compound only when they
  outlive the project that discovered them; see the
  [Context hub](/server/context).

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

**Bowrain's local footprint is cache and speed only, never a source of truth.**
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
| Where the graph lives | Committed files in the repository | Hosted and versioned (the desktop holds a **cache** only) |
| How it learns | You correct; decisions are recorded in `.kapi/state` and the profile is edited in the tree | Corrections aggregate into candidate rules a reviewer promotes |
| Content sources | Local files you own | Every [connector](/server/connectors): content platforms, design tools, repositories, checkouts |
| Automation | Local recipe rules | Server-side, event-driven |
| Cost | Free, open source | Hosted plans / self-host |

## Which one to reach for

kapi alone is the whole tool when one person works from one repository they own,
drafts and checks and translates its files from a terminal, a desktop app, CI,
or an AI assistant over [MCP](/cli/mcp).

Reach for Bowrain when the graph has to travel further: content lives in systems
beyond one checkout and should sync through
[connectors](/server/connectors); several projects should draw on one profile,
one vocabulary, and one content memory; several people work on the same content
and need to [see each other and edit together](/server/collaboration); or
decisions need history, approval and audit.

See the [introduction](/introduction) for what the platform does, and
[Connectors](/server/connectors) for every route into it.
