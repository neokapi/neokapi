---
title: GitHub / GitLab
sidebar_position: 4
description: The forge connector turns a repository into a connected project with no CI configuration. A push webhook triggers the server, and results arrive as one open pull/merge request.
keywords: [github, gitlab, forge, connector, pull request, merge request, webhook, github app]
---

# GitHub / GitLab connector

The `forge` connector is a code connector. It watches a repository branch, and
when content changes it catches the repository up on the server and delivers the
results as **one open pull/merge request** that every later delivery updates in
place, never a direct push to the tracked branch.

This is the route for a team that wants a repository governed without anyone
running a command. There is no pipeline in the repository, no CLI on anyone's
machine, and no scheduled job. The alternative for a repository is the
[kapi connector](/server/connectors/kapi), which puts the same work under a
developer's own hand. Both can run side by side against one project.

:::note

The forge connector is not offered in the web app's add-connector dialog for
static-token setups. Create it through the workspace connectors API
(**manage connectors** permission), or install the server's GitHub App and bind
repositories from a project's Connectors page (below).

:::

## What it syncs

Content files matching the connector's glob `patterns` on the tracked branch.
Target files land at the conventional per-locale path: a path segment or
filename stem equal to the source language becomes the target language
(`locales/en/app.json → locales/fr/app.json`); other layouts get the language
suffixed to the stem.

When a run parks work for a person, the pull/merge request says so; approvals
travel through the project's [review session](/server/review) and arrive in a
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

Credentials are sealed at rest like every connector secret. Pushes to branches
other than the tracked one, including the delivery branch itself, are ignored,
so the connector's own deliveries never re-trigger a run.

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
```

Installing the app on a repository is then the only per-repo step. GitHub
redirects to the server's setup page (set the app's **Setup URL** to
`$SERVER/github/setup`), which lists the installation's repositories and binds
each to a project with one selection: no curl, no tokens. The setup page is a
complete on-ramp: a visitor who arrives without a Bowrain account signs in or
creates one (the installation is preserved through login), gets a workspace if
they have none, and can create a project from the repository inline. The same
binding is available from a project's Connectors page ("GitHub"), and the
workspace connectors API remains the automation path.

Static tokens keep working alongside app mode. GitLab always uses a project
access token.

See [The Bowrain GitHub App](/cli/ci/github-app) for the end-to-end walkthrough.

## Related

- [Connectors](/server/connectors): the full connector row
- [kapi connector](/server/connectors/kapi): the same repository, driven by a developer
- [Keeping content caught up](/the-loop): what a delivery contains
