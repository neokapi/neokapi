---
title: Server Walkthrough
sidebar_position: 11
---

# Server Walkthrough

This guide walks through the bowrain-server web interface, from first login
to creating your first project. For comprehensive documentation of all
web application features, see the [Web Application](/docs/bowrain-web/overview) section.

## Starting the Server

After setting up with Docker Compose (see [Installation](./installation.md)):

```bash
docker compose up -d
```

Open `http://localhost:8080` in your browser.

For local single-user mode, use `kapi serve` instead — no Docker or authentication required.

## Logging In

The web UI redirects you to your configured OIDC identity provider for
authentication. With Keycloak (the recommended provider), you will see a
login form where you can:

- **Sign in** with your existing account
- **Register** a new account (if self-registration is enabled in Keycloak)
- **Use social login** — GitHub, Google, LDAP, or other identity providers configured in your Keycloak realm

After authenticating, you are redirected back to the Bowrain web UI with
an active session.

:::tip Development setup
In the Docker Compose development stack, Keycloak is pre-configured with
self-registration enabled. New users can create accounts directly. Email
verification is sent to Mailpit (accessible at `http://localhost:8025`).
:::

## Personal Workspace

When you log in for the first time, a **personal workspace** is automatically
created for you. This workspace is named after your display name and is
yours alone — no setup required.

You can start creating projects immediately in your personal workspace.

## Creating a Team Workspace

To collaborate with others, create an additional workspace:

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

Invite colleagues to your workspace using the invitation system:

1. Go to **Settings** in the sidebar
2. Scroll to the **Invitations** section
3. Enter the email address of the person to invite
4. Select a role:
   - **Admin** — manage projects and members
   - **Member** — translate and review content
   - **Viewer** — read-only access
5. Click **Invite**

This creates an invite link. You can:
- **Copy the link** and share it directly (via Slack, email, etc.)
- If SMTP is configured, the invite is also sent by email automatically

When the invited person clicks the link, they are directed to authenticate
with the identity provider. After signing in (or registering), they are
added to the workspace with the assigned role.

You can manage pending invitations in the Settings page — view active
invites, see usage counts, and revoke invites that are no longer needed.

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

After login, CLI commands automatically authenticate with the server.

### Claiming Anonymous Projects

If you started with `kapi init` locally (without a server connection), you
can claim that project into your server workspace:

```bash
kapi auth claim
```

This transfers the anonymous local project into your personal workspace on
the server, preserving all files and translations.

## What's Next

- [Web Application](/docs/bowrain-web/overview) — full feature documentation with screenshots
- [Web App Walkthroughs](/docs/bowrain-web/walkthroughs) — step-by-step translation workflows
- [Self-Hosting](./self-hosting.md) — production deployment with TLS and backups
- [Workspaces](./workspaces.md) — workspace concepts and API reference
- [Automation](./automation.md) — CI/CD integration
