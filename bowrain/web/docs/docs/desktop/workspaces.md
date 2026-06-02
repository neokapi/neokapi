---
sidebar_position: 3
title: Workspaces
---

# Workspaces in the desktop app

The desktop is a [working copy of the server](/desktop/overview), so its
workspaces are **the server's workspaces**. You sign in once and the desktop
shows the same workspaces, projects, and resources you see in the
[web app](/server/web-overview). A workspace's concepts — roles, members, and
permissions — are defined server-side; see [Workspaces](/server/workspaces) for
the reference.

## Sidebar layout

Bowrain uses a Slack-style sidebar with two panels.

### Workspace rail

The narrow icon rail on the far left shows your workspaces:

- Each workspace appears as a colored icon with its first letter (or custom logo).
- The active workspace has a pill-shaped highlight.
- Click a workspace icon to switch to it.
- Your **avatar** at the bottom opens the account menu.

### Navigation panel

The wider panel shows navigation for the active workspace:

- **Translate** — project list and translation editor (the main view)
- **Termbase** — terminology management
- **Memory** — translation memory explorer
- **Flows** — flow editor
- **Connectors** — content source management
- **Settings** — workspace and app configuration

Below the navigation items, a collapsible project list provides quick project
switching.

## Connecting to a server

The desktop is meaningful once connected to a `bowrain-server`:

1. Open **Settings** in the navigation panel.
2. Enter the server URL (e.g. `https://bowrain.example.com`).
3. Click **Connect** — this opens the login flow in your browser.
4. After authorization, your server workspaces appear in the workspace rail.

Until you connect, the rail shows a single placeholder **Personal** workspace.
The desktop does not author local-file projects there — creating and configuring
projects is [kapi's job](/getting-started/kapi-vs-bowrain). Connect to a server
to open real projects.

## Account menu

Click your avatar at the bottom of the workspace rail to open the account menu:

- **Email** — your login email.
- **Sign out** — disconnect from the server and remove stored credentials.

## Members and roles

Workspace membership is managed server-side. In a connected workspace, open
**Settings** to invite members by email with a role, change roles, or remove
members. See [Workspaces](/server/workspaces) for the role and permission model.

## Workspace-scoped resources

All resources are scoped to the active workspace and held server-side:

| Resource    | Scope                          |
| ----------- | ------------------------------ |
| Projects    | Per workspace                  |
| Connectors  | Per workspace                  |
| Flows       | Per workspace                  |
| TM entries  | Per project (within workspace) |
| Terminology | Per project (within workspace) |

Switching workspaces shows a different set of projects and resources. The
desktop holds only a local cache of the active workspace's content for fast,
offline-capable access — never a source of truth.
