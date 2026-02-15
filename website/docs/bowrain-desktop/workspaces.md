---
sidebar_position: 3
title: Workspaces
---

# Workspaces in Bowrain

Bowrain uses a Slack-inspired sidebar for workspace navigation, giving you quick
access to all your translation workspaces, projects, and resources.

## Sidebar Layout

The Bowrain sidebar has two panels:

### Workspace Rail

The narrow icon rail on the far left shows your workspaces:

- Each workspace is shown as a colored icon with its first letter (or custom logo)
- The active workspace has a pill-shaped highlight
- Click a workspace icon to switch to it
- The **+** button at the bottom creates a new workspace
- Your **avatar** at the bottom opens the account menu

### Navigation Panel

The wider panel to the right of the rail shows navigation for the active workspace:

- **Translate** — Project list and translation editor (the main view)
- **Termbase** — Terminology management
- **Memory** — Translation memory explorer
- **Flows** — Pipeline editor
- **Connectors** — Content source management
- **Settings** — Workspace and app configuration

Below the navigation items, a collapsible project list provides quick project
switching without going back to the full project view.

## Personal Workspace

When running Bowrain standalone (not connected to a server), a "Personal"
workspace is created automatically. This is your default workspace for local
work with `.kaz` files and local projects.

## Connecting to a Server

To access shared workspaces on a `bowrain-server`:

1. Open **Settings** in the navigation panel
2. Enter the server URL (e.g., `https://gokapi.example.com`)
3. Click **Connect** — this opens the login flow in your browser
4. After authorization, your server workspaces appear in the workspace rail

You can work with both local (Personal) and server workspaces simultaneously.
The workspace rail shows all available workspaces regardless of where they live.

## Account Menu

Click your avatar at the bottom of the workspace rail to open the account menu:

- **Email** — Your login email (for server-connected workspaces)
- **Sign Out** — Disconnect from the server and remove stored credentials

## Creating a Workspace

1. Click the **+** button in the workspace rail
2. Enter a name for the workspace (e.g., "Acme Translations")
3. The workspace is created and becomes active

On a connected server, other team members can be invited to the workspace through
the Settings panel.

## Managing Members

In a server-connected workspace, navigate to **Settings** to manage members:

- **Invite members** by email with a selected role
- **Change roles** (owner, admin, member, viewer)
- **Remove members** from the workspace

See [Workspaces](/docs/bowrain-server/workspaces) in the User Guide for details
on roles and permissions.

## Workspace-Scoped Resources

All resources are scoped to the active workspace:

| Resource | Scope |
|----------|-------|
| Projects | Per workspace |
| Connectors | Per workspace |
| Flows | Per workspace |
| TM entries | Per project (within workspace) |
| Terminology | Per project (within workspace) |

Switching workspaces shows a completely different set of projects and resources.
