---
sidebar_position: 2
title: First login and translation
sidebar_label: First login and translation
---

# First login and translation

This guide walks you through first login, workspace creation, and translating
your first file in Bowrain. The steps below use the browser; the
[desktop app](/server/desktop-app) follows the same flow after its own
[first sign-in](/server/desktop-app#first-sign-in).

## Prerequisites

You need access to a Bowrain server — a hosted workspace at
[bowrain.cloud](https://bowrain.cloud) or one your team runs. Open your server
URL in a browser to begin. (Running your own server? See
[For developers → Self-hosting](/server/installation).)

## Logging In

The web UI redirects you to your configured identity provider for authentication. Depending on your provider configuration, you will see options like:

- **Username & password** — sign in with your account
- **Self-registration** — create a new account (if enabled in your identity provider)
- **Social login** — GitHub, Google, LDAP, or other configured identity providers

After authenticating, you are redirected back to the Bowrain web UI with an active session.

## Personal Workspace

After your first login, a **personal workspace** is automatically created for you. This workspace is named after your display name and is ready to use immediately — no manual workspace creation needed.

## Creating a Team Workspace

To collaborate with others, create an additional workspace:

1. Click the **+** button in the workspace rail (left edge of the screen)
2. Enter a **Name** (e.g., "My Team") — the slug is auto-generated
3. Adjust the **Slug** if needed (URL-safe identifier)
4. Click **Create**

You are automatically added as the workspace **owner** and switched to the new workspace.

## Creating a Project

1. From the **Translate** view (the default), click **New Project**
2. Enter the **Project name** (e.g., "Website Translation")
3. Select the **Source language** (e.g., English)
4. Select one or more **Target languages** (e.g., French, German, Japanese)
5. Click **Create**

The project opens in the project view.

## Uploading Files

1. **Drag and drop** files onto the upload zone in the project view, or click **Add Files** to browse
2. The server auto-detects the file format (HTML, XML, JSON, YAML, PO, Markdown, XLIFF, and more)
3. Files appear in the file list with format icon, block count, and word count

Supported formats include all formats registered in the neokapi format registry. See [Formats](https://neokapi.github.io/web/neokapi/docs/features/formats) for the complete list.

## Opening the Editor

Click any file name in the project view to open it in the translation editor. The editor loads all translatable blocks from the file and displays them in a source/target grid.

## Translating

### Manual Translation

1. Click a target cell or press **Enter** on the selected row to start editing
2. Type the translation
3. Press **Enter** to save and advance to the next block, or **Escape** to cancel

### AI Translation

1. Configure an AI provider in the backend (Anthropic, OpenAI, or Ollama)
2. Click **AI Translate** in the toolbar to translate the entire file
3. Review and edit the AI-generated translations as needed

### Machine Translation

1. Configure an MT provider (DeepL, Google, Microsoft, ModernMT, or MyMemory)
2. Select the provider from the toolbar dropdown
3. Click **AI Translate** to translate the file using the selected MT engine

### TM Leverage

1. If translation memory entries exist, click **TM Lookup** in the toolbar
2. The system matches source blocks against the TM and fills in matches
3. High-confidence matches are applied automatically; review others in the context panel

### Pseudo-Translation

Click **Pseudo** in the toolbar to generate pseudo-translations — useful for testing layout and character handling before starting real translation.

## Exporting

Click the **Export** button in the toolbar to download the translated file in its original format with all translations applied.

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

Connect kapi to your server for command-line workflows:

```bash
kapi auth login --server https://bowrain.cloud
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

- [Translation Editor](./translation-editor.mdx) — Visual and Table views, toolbar, keyboard shortcuts, context panel
- [Translation Memory](./translation-memory.mdx) — TM Explorer features
- [Terminology](./terminology.mdx) — term management and enforcement
- [Walkthroughs](./walkthroughs.md) — step-by-step workflows
