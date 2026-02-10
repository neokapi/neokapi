---
title: Server Walkthrough
sidebar_position: 11
---

# Server Walkthrough

This guide walks through the gokapi-server web interface, from first login
to creating your first project.

## Starting the Server

After setting up with Docker Compose (see [Self-Hosting](./self-hosting.md)):

```bash
docker compose up -d
```

Open `http://localhost:8080` in your browser.

## Logging In

The web UI redirects you to your configured identity provider (Dex) for
authentication. Depending on your Dex configuration, you will see options
like:

- **GitHub** — sign in with your GitHub account
- **Google** — sign in with your Google workspace account
- **LDAP** — sign in with your corporate credentials
- **Mock** (development) — enter any email to authenticate

After authenticating, you are redirected back to the gokapi web UI with
an active session.

## Creating a Workspace

After your first login, create a workspace to organize your translation
projects:

1. Navigate to the **Workspaces** section
2. Click **Create Workspace**
3. Enter a name (e.g., "My Team") and slug (e.g., "my-team")
4. Click **Create**

You are automatically added as the workspace **owner**.

## Creating a Project

Within a workspace, create your first translation project:

1. Select your workspace
2. Click **New Project**
3. Enter the project name, source locale (e.g., `en`), and target
   locales (e.g., `fr`, `de`, `ja`)
4. Click **Create**

## Inviting Team Members

Add colleagues to your workspace:

1. Go to **Workspace Settings** > **Members**
2. Enter the email address of the person to invite
3. Select a role:
   - **Owner** — full control including workspace deletion
   - **Admin** — manage projects and members
   - **Editor** — translate and review content
   - **Viewer** — read-only access
4. Click **Add Member**

The invited user will see the workspace after logging in with the
same email address.

## CLI Connection

Connect the kapi CLI to your server for command-line workflows:

```bash
kapi auth login --server http://localhost:8080
```

This starts a [device authorization flow](https://www.rfc-editor.org/rfc/rfc8628):

1. The CLI displays a URL and a one-time code
2. Open the URL in your browser
3. Enter the code to authorize the CLI
4. The CLI receives a token and stores it locally

After login, CLI commands automatically authenticate with the server:

```bash
# List workspaces
curl -s -H "Authorization: Bearer $(cat ~/.config/gokapi/auth.json | jq -r .access_token)" \
  http://localhost:8080/api/v1/workspaces | jq
```

## What's Next

- [Self-Hosting](./self-hosting.md) — production deployment with TLS and backups
- [Workspaces](./workspaces.md) — workspace concepts and API reference
- [Automation](./automation.md) — CI/CD integration
