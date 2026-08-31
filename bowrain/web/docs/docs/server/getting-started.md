---
sidebar_position: 2
title: First login and first project
sidebar_label: First login and first project
description: A step-by-step guide to your first login, creating a workspace, establishing the context, and bringing a first project's content in from the browser.
---

# First login and first project

This guide walks you through first login, workspace creation, establishing the
context your content is written in, and a first project. The steps below use
the browser; the [desktop app](/server/desktop-app) follows the same flow after
its own [first sign-in](/server/desktop-app#first-sign-in).

## Prerequisites

You need access to a Bowrain server: a hosted workspace at
[app.bowrain.cloud](https://app.bowrain.cloud) or one your team runs. Open
your server URL in a browser to begin. (Running your own server? See
[For developers: self-hosting](/server/installation).)

## Logging in

The web UI redirects you to the identity provider for authentication. On the
hosted service that is a managed sign-in; a self-hosted server uses whatever
OIDC provider it is configured with, so you may see a username and password
form, self-registration, or social sign-in.

After authenticating, you are redirected back to the Bowrain web UI with an
active session.

## Personal workspace

After your first login, a **personal workspace** is created for you. It is
named after your display name and is ready to use immediately.

## Creating a team workspace

To collaborate with others, create an additional workspace:

1. Click the **+** button in the workspace rail (left edge of the screen)
2. Enter a **Name** (for example "My Team"); the slug is derived from it
3. Adjust the **Slug** if needed (URL-safe identifier)
4. Click **Create**

You are added as the workspace **owner** and switched to the new workspace.

## Establishing the context

Before any content arrives, give the workspace something to govern it with.
Open **Context** in the sidebar.

1. **Scan what you already publish.** A [context scan](/server/context-scan)
   reads pasted copy, public pages, uploaded documents or a public repository,
   and proposes the axes your content varies along, a voice profile draft, and
   term candidates, each with the evidence behind it. Nothing it proposes is
   enforced until you approve it.
2. **Approve the axes.** An approved axis such as `brand` or `mode` becomes a
   coordinate the workspace's content can sit at. For a project connected from
   a checkout, the approval is delivered by `kapi pull` as an edit to
   `defaults.coordinates` in `kapi.yaml`, for review in git.
3. **Adopt the voice and the terms.** Save the edited voice profile at the
   point it governs, and promote the term candidates you want into concepts.
   Most workspaces call their default profile *brand*; a workspace with several
   audiences carries several profiles.

A thin profile is enough to start. The rules you actually need surface as
people correct real output, and every correction feeds the
[learning loop](/server/context-voice).

## Creating a project

1. From the project list, click **New Project**
2. Enter the **Project name**
3. Select the **Source language** (for example English)
4. Optionally select **Target languages**. A project with none is source-only:
   runs read it, check it against the context, and write nothing back. Add a
   language later when you ship in one; it is one more coordinate, not a
   second project.
5. Click **Create**

The project opens in the project view.

## Bringing content in

Content reaches a project through a [connector](/server/connectors), or by
upload:

1. **Drag and drop** files onto the upload zone in the project view, or click
   **Add Files** to browse. The server detects the format; the
   [format reference](https://neokapi.github.io/formats) lists what it reads.
2. Or open the project's **Connectors** view and connect the system the content
   already lives in, so source flows in and published results flow back.

Files appear in the file list with a format icon, block count, word count and
the collection they sit in.

## Drafting and checking

Content that arrived is caught up by a [run](/the-loop): start one from the
project's **Runs** view with **Run now**, or let the project's push policy
start one. A run checks the source against the context, drafts any target
languages with AI steered by the voice and the terms in force at each point,
checks what it produced, and parks what a person has to decide.

To work on one file, open it from the project view. The
[editor](/server/translation-editor) shows the source, the target if there is
one, and beside each block the checks, the memory matches and the terms in
force. Press **Enter** on a block to edit it, **Enter** again to save and
advance, **Escape** to cancel. **AI draft** in the toolbar drafts a file's
targets on demand; **Pseudo** generates pseudo-translations for layout testing.

## Reviewing

What a run parks lands in the project's [review session](/server/review).
Approve, reject or edit each block, or approve everything that passes the
checks and the voice bar in one action. Approving the last pending block starts
the run that delivers. The per-locale [ship state](/server/review#ship-states)
on the dashboard tells you what can go out today.

## Publishing and exporting

Click **Export** in the editor toolbar to download a file in its original
format with its translations applied, or **Publish** on the project's
connector to write approved results back to the system they came from.

## Inviting team members

Invite colleagues to your workspace:

1. Go to **Settings** in the sidebar
2. Open the **Invitations** section
3. Enter the email address of the person to invite
4. Select a role:
   - **Admin**: manage projects and members
   - **Member**: contribute and review content
   - **Viewer**: read-only access
5. Click **Invite**

This creates an invite link. Copy it and share it directly, or, if email is
configured, the invite is also sent automatically. The invited person signs in
or registers and is added to the workspace with the assigned role. Members are
never metered, so invite everyone who might notice what a rule missed.

Pending invitations are listed in Settings, where you can see their usage and
revoke the ones no longer needed. Per-project roles, bound to a language or a
point, are covered in [Members and roles](/server/members-and-roles).

## CLI connection

Connect kapi to your server for command-line workflows:

```bash
kapi auth login    # the hosted service is the default; --server for self-hosted
```

This starts a [device authorization flow](https://www.rfc-editor.org/rfc/rfc8628):

1. The CLI displays a URL and a one-time code
2. Open the URL in your browser
3. Enter the code to authorize the CLI
4. The CLI receives a token and stores it in the operating-system keychain

After login, CLI commands authenticate with the server automatically.

### Claiming anonymous projects

If you started with `kapi init` locally (without a server connection), you
can claim that project into your server workspace:

```bash
kapi auth claim
```

This transfers the anonymous local project into your personal workspace on
the server, preserving all files and translations.

## What's next

- [Context](/server/context): the hub, profiles per point, and governance
- [Review](/server/review): the session, bulk approval, and ship states
- [The editor](/server/translation-editor): Visual and Table views, toolbar, keyboard shortcuts, context panel
- [Walkthroughs](/server/walkthroughs): step-by-step workflows
