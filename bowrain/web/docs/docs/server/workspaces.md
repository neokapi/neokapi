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

### Personal workspace

In the Bowrain desktop app, a "Personal" workspace is created automatically for local, single-user work. No server connection is needed for the personal workspace.

When you connect to a Bowrain server, the workspaces you have been invited to appear alongside your personal workspace in the workspace rail.

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
