---
title: GitHub / GitLab
sidebar_position: 4
description: The forge connector turns a repository into a connected project with no CI configuration. A push webhook triggers the server, and results arrive as one open pull/merge request.
keywords: [github, gitlab, forge, connector, pull request, merge request, webhook, github app]
---

# GitHub / GitLab connector

The `forge` connector keeps a repository's content caught up with nobody
running a command. It watches a repository branch. When content changes, it
catches the repository up on the server and delivers the results as **one open
pull/merge request**, which every later delivery updates in place. The tracked
branch changes only when you merge.

The repository needs no pipeline, nobody installs a CLI, and there is no
scheduled job. To run the same work from a developer's machine, use the
[kapi connector](/server/connectors/kapi). Both can run side by side against
one project.

:::note

For a static-token setup, create the connector through the workspace
connectors API (**manage connectors** permission), because the web app's
add-connector dialog leaves it out. With the server's GitHub App installed,
you bind repositories from a project's Connectors page (below).

:::

## What it syncs

Content files matching the connector's glob `patterns` on the tracked branch.
Target files land at the conventional per-locale path: a path segment or
filename stem equal to the source language becomes the target language
(`locales/en/app.json → locales/fr/app.json`); other layouts get the language
suffixed to the stem.

When a run leaves work waiting for review, the pull/merge request says so.
Approvals made in the project's [review session](/server/review) arrive in a
later delivery.

## Setup with a token

Create the connector, then point a webhook at the server:

```bash
curl -X POST "$SERVER/api/v1/$WORKSPACE/connectors" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{
    "type": "forge",
    "name": "website",
    "config": {
      "repo": "https://github.com/acme/website.git",
      "branch": "main",
      "project_id": "<project-id>",
      "token": "<forge API token>",
      "webhook_secret": "<random secret>",
      "patterns": "src/locales/**/*.json"
    }
  }'
```

Then add a **push webhook** in the repository settings pointing at
`$SERVER/api/webhooks/forge/<connector-id>` with the same secret. GitHub signs
the payload (secret field), GitLab sends it as the secret token. The token needs
pull/merge-request write access: a GitHub fine-grained PAT with contents +
pull-requests write, or a GitLab project access token with api scope.

Optional config: `forge` (`github` | `gitlab`; default inferred from the repo
host, self-managed GitLab included), `delivery_branch` (default
`bowrain/translations`), `pr_title`, `pr_labels`.

Credentials are encrypted at rest like every connector secret. The connector
ignores pushes to other branches, including the delivery branch, so its own
deliveries never start another run.

## GitHub App mode (no tokens, no per-repo webhooks)

With a GitHub App registered for your server, connectors on GitHub need neither
a token nor a webhook secret. GitHub routes every installed repository's pushes
to one app endpoint, and the server mints short-lived per-installation tokens
for each delivery:

```json
{
  "type": "forge",
  "name": "website",
  "config": {
    "repo": "https://github.com/acme/website.git",
    "branch": "main",
    "project_id": "<project-id>",
    "auth": "app",
    "patterns": "src/locales/**/*.json"
  }
}
```

Register the app once (GitHub → Settings → Developer settings → GitHub Apps →
New) with **webhook URL** `$SERVER/api/webhooks/github-app`, a webhook secret,
the **push** event, and repository permissions **Contents: read & write**,
**Pull requests: read & write**, **Metadata: read**. Download the private key
and configure the server:

```bash
GITHUB_APP_ID=<app id>
GITHUB_APP_PRIVATE_KEY_FILE=/etc/bowrain/github-app.pem   # or GITHUB_APP_PRIVATE_KEY (PEM text)
GITHUB_APP_WEBHOOK_SECRET=<webhook secret>
GITHUB_API_URL=https://<host>/api/v3   # GitHub Enterprise Server only; omit for github.com
```

The only per-repository step is then to install the app on the repository.
GitHub redirects to the server's setup page (set the app's **Setup URL** to
`$SERVER/github/setup`). The page lists the installation's repositories, and
one selection binds each to a project, with no curl and no tokens.

A visitor who arrives at the setup page without a Bowrain account signs in or
creates one, and the installation is kept through login. They get a workspace
if they have none, and can create a project from the repository on the same
page. A project's Connectors page ("GitHub") offers the same binding, and the
workspace connectors API covers automation.

Static tokens keep working alongside app mode. GitLab always uses a project
access token.

See [The Bowrain GitHub App](/cli/ci/github-app) for the end-to-end walkthrough.

## Related

- [Connectors](/server/connectors): the full connector row
- [kapi connector](/server/connectors/kapi): the same repository, driven by a developer
- [Keeping content caught up](/the-loop): what a delivery contains
