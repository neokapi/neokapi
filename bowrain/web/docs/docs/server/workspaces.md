---
title: Workspaces
sidebar_position: 9
---

# Workspaces

Workspaces are the top-level organizational unit in neokapi. They group projects,
members, and resources together — like a GitHub organization or Slack workspace.

## Concepts

### Workspace

A workspace is a container that holds:

- **Projects** — translation projects with content, TM, and terminology
- **Members** — users with role-based access
- **Connectors** — configured content sources shared across projects
- **Settings** — workspace-level configuration

Every project belongs to exactly one workspace. When you create a project, it
is created within the active workspace.

### Roles

Each workspace member has a role that determines their permissions:

| Role       | Description             | Permissions                                    |
| ---------- | ----------------------- | ---------------------------------------------- |
| **Owner**  | Workspace creator       | Full control, delete workspace, manage billing |
| **Admin**  | Workspace administrator | Manage members, settings, all projects         |
| **Member** | Regular team member     | Create/edit projects, pull/push content        |
| **Viewer** | Read-only access        | View projects and translations                 |

### Personal Workspace

In the Bowrain desktop app and `kapi serve`, a "Personal" workspace is created
automatically. This is your default workspace for local, single-user work. No
server connection is needed.

When you connect to a `bowrain-server`, you'll see all workspaces you've been
invited to alongside your personal workspace.

## Managing Workspaces

### Web UI and Bowrain

The workspace switcher in the left sidebar provides quick access:

1. **Switch workspace** — Click a workspace icon in the left rail
2. **Create workspace** — Click the "+" button at the bottom of the rail
3. **Workspace settings** — Select "Settings" in the navigation panel

### REST API

```bash
# List your workspaces
curl -H "Authorization: Bearer $TOKEN" \
  https://neokapi.example.com/api/v1/workspaces

# Create a workspace
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -d '{"name": "Acme Translations", "slug": "acme"}' \
  https://neokapi.example.com/api/v1/workspaces

# Add a member
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -d '{"user_id": "usr_abc", "role": "member"}' \
  https://neokapi.example.com/api/v1/workspaces/acme/members
```

## Workspace-Scoped Projects

All project operations are scoped to the active workspace:

```bash
# List projects in a workspace
curl -H "Authorization: Bearer $TOKEN" \
  https://neokapi.example.com/api/v1/workspaces/acme/projects

# Create a project in a workspace
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -d '{"name": "Website", "source_locale": "en", "target_locales": ["fr", "de"]}' \
  https://neokapi.example.com/api/v1/workspaces/acme/projects
```

In the Bowrain desktop app and web UI, the project list automatically filters
to the active workspace. Switching workspaces shows a different set of projects.

## Server Setup

Workspaces require a `bowrain-server` with authentication enabled. See the
[deployment guide](/developer/server) for setup instructions.

For local development and testing, use the provided Docker Compose configuration:

```bash
cd deploy
docker-compose up
```

This starts both Dex (OIDC provider) and bowrain-server pre-configured for
workspace and authentication testing.
