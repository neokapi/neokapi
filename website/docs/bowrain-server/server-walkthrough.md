---
title: Server Walkthrough
sidebar_position: 11
---

# Server Walkthrough

This guide walks through the bowrain-server web interface, from first login
to creating your first project. For comprehensive documentation of all
web application features, see the [Web Application](/docs/bowrain-web/overview) section.

## Starting the Server

After setting up with Docker Compose (see [Self-Hosting](./self-hosting.md)):

```bash
docker compose up -d
```

Open `http://localhost:8080` in your browser.

For local single-user mode, use `kapi serve` instead — no Docker or authentication required.

## Logging In

The web UI redirects you to your configured OIDC identity provider for
authentication. Depending on your provider configuration, you will see options
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

1. Click the **+** button in the workspace rail (left edge of the screen)
2. Enter a **Name** (e.g., "My Team") — the slug is auto-generated
3. Adjust the **Slug** if needed (URL-safe identifier)
4. Click **Create**

You are automatically added as the workspace **owner**.

## Creating a Project

Within a workspace, create your first translation project:

1. Navigate to the **Translate** view (the default)
2. Click **New Project** on the dashboard
3. Enter the project name, source locale (e.g., English), and target
   locales (e.g., French, German, Japanese)
4. Click **Create**

The project opens in the project view where you can upload files.

## Uploading Files

1. **Drag and drop** files onto the upload zone, or click **Add Files** to browse
2. The server auto-detects the file format (HTML, XML, JSON, YAML, PO, Markdown, XLIFF, and more)
3. Files appear with format icon, block count, and word count
4. Click a file name to open it in the translation editor

## Translation Editor

The editor displays source and target text side by side with tools for
translation. Key features:

- **Four layout modes** — grid, focus, split horizontal, split vertical
- **AI translation** — translate entire files using Anthropic, OpenAI, or Ollama
- **TM leverage** — match source blocks against translation memory
- **Context panel** — per-block TM matches and terminology suggestions
- **Keyboard navigation** — Enter to edit, Escape to cancel, arrow keys to navigate
- **Progress tracking** — color-coded status bar showing translation progress
- **File export** — download translated files in their original format

For full details, see [Translation Editor](/docs/bowrain-web/translation-editor).

## Translation Memory & Terminology

Access workspace-scoped linguistic resources from the sidebar:

- **Memory** — browse, search, add, edit, and delete TM entries. Filter by locale.
- **Termbase** — manage terminology concepts with lifecycle statuses, domains, and import/export.

Both resources are available per-block in the editor's context panel for in-context suggestions.

For details, see [Translation Memory](/docs/bowrain-web/translation-memory) and
[Terminology](/docs/bowrain-web/terminology).

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
curl -s -H "Authorization: Bearer $(cat ~/.config/kapi/auth.json | jq -r .access_token)" \
  http://localhost:8080/api/v1/workspaces | jq
```

## What's Next

- [Web Application](/docs/bowrain-web/overview) — full feature documentation with screenshots
- [Web App Walkthroughs](/docs/bowrain-web/walkthroughs) — step-by-step translation workflows
- [Self-Hosting](./self-hosting.md) — production deployment with TLS and backups
- [Workspaces](./workspaces.md) — workspace concepts and API reference
- [Automation](./automation.md) — CI/CD integration
