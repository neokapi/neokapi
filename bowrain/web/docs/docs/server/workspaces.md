---
title: Workspaces
sidebar_position: 9
---

# Workspaces

A workspace is the top-level organizational unit in Bowrain. It groups projects, members, terminology, and translation memory together — similar to a GitHub organization or Slack workspace. Every project belongs to exactly one workspace.

## Concepts

### Workspace contents

A workspace holds:

- **Projects** — translation projects with content, translation memory, and terminology
- **Members** — users with role-based access
- **Connectors** — configured content sources shared across projects
- **Settings** — workspace-level configuration

### Roles

Each workspace member has a role that determines their permissions:

| Role       | Description             | Permissions                                    |
| ---------- | ----------------------- | ---------------------------------------------- |
| **Owner**  | Workspace creator       | Full control, delete workspace, manage billing |
| **Admin**  | Workspace administrator | Manage members, settings, all projects         |
| **Member** | Regular team member     | Create and edit projects, push and pull content |
| **Viewer** | Read-only access        | View projects and translations                 |

### Before you connect

Until the desktop app is connected to a Bowrain server, its workspace rail shows
a single placeholder "Personal" workspace. It is not a place to author
local-file projects — that is [kapi's job](/getting-started/kapi-vs-bowrain).
Once you connect, the workspaces you have been invited to appear in the rail and
the desktop opens their server-hosted projects.

## Managing workspaces

### Switching workspaces

The workspace rail on the left side of the web and desktop apps shows all workspaces you belong to. Click a workspace to switch to it.

### Creating a workspace

Click the "+" button at the bottom of the workspace rail, or go to **Settings > Create Workspace**.

### Inviting members

Go to **Workspace Settings > Members > Invite**. Enter the email address and choose a role. The invitation is sent by email; the recipient creates an account if they do not already have one.

### Workspace settings

Access workspace settings from the navigation panel in the web or desktop app. You can update the workspace name, slug, and description, and manage member roles.

## Next Steps

- [Installation](/server/installation)
- [Self-Hosting](/server/self-hosting)
- [Connectors](/server/connectors)
- [Automation](/server/automation)
