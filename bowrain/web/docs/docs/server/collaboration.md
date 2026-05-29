---
title: Real-time collaboration
sidebar_label: Real-time collaboration
description: A team translates together in Bowrain — see who is on which block, watch edits and reviews appear live, hand off review, and work offline on the desktop — with the web and desktop apps as equal real-time clients of the same server.
keywords: [collaboration, presence, real-time, co-editing, review, offline, web, desktop, bowrain]
---

# Real-time collaboration

Kapi is a single-user toolchain: one person, one checkout, files they own.
Bowrain is where a **team** works on the same content at the same time. The web
app and the desktop app are equal clients of one server, so a project is a
shared, live workspace rather than a file each person edits alone.

## Presence — see who is here

Open a project and you see your teammates: their names and avatars appear on the
blocks they are editing, and move with them as they navigate. You always know
who is on block 42 before you start typing there, and you watch a colleague work
through a file in real time. Presence is broadcast by the server to every client
watching the same project — whether they are in the browser or the desktop app.

## Live co-editing

Edits propagate as they happen. When a teammate translates a block, reviews one,
or updates a term, the change appears in your editor without a refresh, and the
progress bar moves as the team works. Concurrent edits to a shared project are
merged on the server, so two people working through the same file converge on one
consistent state instead of overwriting each other — the editing model is the
same in the browser and on the desktop.

## Review handoff

Translation and review are different jobs, and Bowrain routes them. Work is
assigned as **tasks** (translate, review, review terms, fix quality); a task
shows up in the assignee's activity feed, they claim it, do the work, and mark it
done, and the next person sees the handoff live. A reviewer marking a block
reviewed is a presence-and-progress event everyone watching the project sees,
not a silent state change discovered later. Corrections a reviewer makes feed the
[brand-voice correction loop](/server/brand-voice), so a fix made once becomes a
check that protects the whole project.

## Web and desktop, in sync

Both apps connect to the same server and the same project. Which you reach for is
a matter of where you work, not what you can do:

| | **Web** | **Desktop** |
| --- | --- | --- |
| Install | Nothing — open a URL | Native app (macOS / Linux / Windows) |
| Real-time presence & co-editing | Yes | Yes |
| Brand, terminology & TM context | Yes | Yes |
| Works offline | No (always online) | **Yes** — edit on a flight; changes queue and sync on reconnect |
| Local cache | Browser session | **Persistent** local store for fast, offline-capable work |
| Best for | Quick access, reviewers, occasional contributors | Daily translators, large files, unreliable connectivity |

Both are real-time clients of the same server, so presence, edits, reviews,
translation memory, and terminology are shared and consistent across them — a
reviewer in the browser and a translator in the desktop app see each other and
each other's work as it happens.

## Offline on the desktop

The desktop app keeps a local cache of the project, so you can keep translating
when the network drops — on a flight, in a tunnel, or during server maintenance.
Edits, reviews, and memory updates made offline queue locally and replay in order
the moment you reconnect, then rejoin the live session. You never lose work to a
dropped connection, and you do not have to think about sync — it happens on
reconnect.

## See it

- [Translation editor](/server/translation-editor) — the editing surface presence and live updates appear in
- [Workspaces & members](/server/workspaces) — invite teammates and assign roles
- [Brand voice & corrections](/server/brand-voice) — how a reviewer's correction becomes a shared check
