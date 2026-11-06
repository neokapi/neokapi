---
title: The Bowrain GitHub App
sidebar_label: The Bowrain GitHub App
---

import { SwimlaneDiagram } from "@neokapi/docs-shared";

# The Bowrain GitHub App

A repository can stay caught up with no CI configuration. The `forge`
connector links a GitHub or GitLab repository to a workspace server-side, and
the results arrive as **one open pull/merge request** that every later
delivery updates in place.

<SwimlaneDiagram
  actors={[
    { label: "Repository", sub: "tracked branch", role: "io" },
    { label: "Bowrain server", sub: "on-push policy", role: "translate" },
    { label: "Reviewer", sub: "review session", role: "qa" },
  ]}
  messages={[
    { from: 0, to: 1, label: "push webhook", detail: "verified secret" },
    { from: 1, to: 1, label: "read changed source, run", detail: "holds what needs a person" },
    { from: 1, to: 2, label: "work waiting for review" },
    { from: 1, to: 0, label: "one PR/MR, updated in place", detail: "you review and merge" },
  ]}
  caption="A push reaches the server through a webhook, the server runs the project, and the results come back as one pull request."
/>

On GitHub, the Bowrain GitHub App removes the remaining per-repository steps:
there is no token and no webhook secret to manage. GitLab always uses a
project access token.

## When to choose it

- Nobody owns the repository's CI, as in docs, marketing and content
  repositories with no pipeline to extend.
- You want no configuration in the repository: no workflow file, no component
  include and no CI secrets to rotate.

Choose [GitHub Actions](/cli/use-cases/github-actions) or
[GitLab CI](/cli/ci/gitlab) instead when you want to shape the job yourself:
custom triggers, a merge gate (`kapi check --ship`), or plan comments on pull
requests. You can combine them: forge delivery produces the results while a
small CI job gates.

## How delivery behaves

- Each project keeps **one open PR/MR**. A new run recreates the delivery
  branch (default `bowrain/translations`) from the tracked branch's tip,
  force-pushes it, and updates the open request in place, so stale requests
  never pile up.
- The connector **never pushes to the tracked branch**. The delivery branch
  must differ from it, and every delivery is a request you review and merge.
- When the produced output equals the tracked branch, nothing is delivered.
- The connector ignores pushes to any other branch, including the delivery
  branch, so its own deliveries never start another run.
- When a run leaves work waiting for review, the request says so. Approvals
  made in the project's [review session](/server/review) arrive in a later
  delivery.

## Connect a repository

The connector is created through the workspace connectors API
(`manage-connectors` permission); the full field reference lives at
[the GitHub / GitLab connector](/server/connectors/github).
A minimal GitHub connector in token mode:

```json
{
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
}
```

`repo` must be an `https` URL. The forge kind is inferred from the host
(`github` for github.com, `gitlab` otherwise, self-managed GitLab included)
and can be set explicitly with `forge`. Optional fields (`delivery_branch`,
`pr_title`, `pr_labels`, `git_user_name`, `git_user_email`) are documented on
the connectors page.

## Webhook setup and verification

Add a **push** webhook in the repository settings pointing at:

```
POST $SERVER/api/webhooks/forge/<connector-id>
```

with the connector's `webhook_secret`. GitHub signs each payload with an HMAC
(`X-Hub-Signature-256`); GitLab sends the secret token (`X-Gitlab-Token`).
The server verifies the secret before acting, and only a verified push to the
**tracked** branch starts a run.

## Token mode and app mode

- **Token mode** (default): the connector carries a forge API token with
  pull/merge-request write access: a GitHub fine-grained PAT with contents
  and pull-requests write, or a GitLab project access token with api scope.
  Credentials are sealed at rest like every connector secret.
- **App mode** (`"auth": "app"`, GitHub only): the connector carries no
  token and no webhook secret. GitHub routes every installed repository's
  pushes to one app endpoint, `POST $SERVER/api/webhooks/github-app`, and the
  server mints a short-lived per-installation token for each delivery.
  Installing the app on a repository is the only per-repo step. Token mode
  keeps working alongside app mode; GitLab always uses a project access
  token.

  One app serves every workspace, so the server records which workspace each
  installation belongs to and serves the setup endpoints only to that
  workspace. The server makes the record when the install starts from
  Bowrain, because the install link carries signed state that GitHub returns
  on the redirect. It drops the record as soon as GitHub reports the app
  uninstalled. An installation started elsewhere, such as GitHub's own app page
  or the Marketplace, arrives without a workspace. Open **Connect GitHub** from
  the workspace to attach it.

## Self-hosting: register the app

On Bowrain Cloud the app is already registered. A self-hosted server needs
its own GitHub App, registered once (GitHub → Settings → Developer settings →
GitHub Apps → New):

- **Webhook URL** `$SERVER/api/webhooks/github-app`, with a webhook secret,
  subscribed to **push**, **installation** and **installation repositories**.
  The last two keep the installation records current. Without
  `installation`, the server misses an uninstall, and the workspace keeps a
  claim on an installation the app has lost.
- **Setup URL** `$SERVER/github/setup`, with *Redirect on update* enabled, so
  GitHub returns the installation (and the signed state that attributes it)
  after an install or a repository-access change.
- **Repository permissions**: Contents read and write, Pull requests read and
  write, Metadata read.

Download the app's private key and configure the server:

```bash
GITHUB_APP_ID=<app id>
GITHUB_APP_PRIVATE_KEY_FILE=/etc/bowrain/github-app.pem  # or GITHUB_APP_PRIVATE_KEY with the PEM text
GITHUB_APP_WEBHOOK_SECRET=<webhook secret>
GITHUB_API_URL=https://<host>/api/v3  # GitHub Enterprise Server only; omit for github.com
```

The server and the worker read the same variables; set them on both.

## Related

- [The loop in CI](/cli/ci/overview): the surfaces map and when to prefer a pipeline
- [GitHub / GitLab connector](/server/connectors/github): the full forge connector configuration reference
- [Automation](/server/automation): the on-push convergence policy and server-side rules
- [Keeping content caught up](/the-loop): produce, promote, release
