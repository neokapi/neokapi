# Pushes and pull requests made by workflows

Several workflows commit to this repository. Main requires the `CI passed`
check, so a commit a workflow pushes has to get CI like any other. Which token
makes the push decides whether it does.

## What GitHub does with each token

From GitHub's documentation ("Triggering a workflow from a workflow", and
"GITHUB_TOKEN"):

- A push made with `GITHUB_TOKEN` starts no `push` workflow runs.
- When `GITHUB_TOKEN` opens or updates a pull request, GitHub creates the
  pull request's `pull_request` runs (`opened`, `synchronize`, `reopened`) in
  an approval-required state. They show as `action_required` and run once a
  person with write access chooses **Approve workflows to run**. Other
  activity types (`labeled`, `edited`, `closed`) create no runs.
- `workflow_dispatch` and `repository_dispatch` always create runs.
- A push or pull request made with a GitHub App installation token, or a
  personal access token, starts the workflows as a person's push does.

## The workflows that push here

| Workflow | What it pushes | What happens to CI |
| --- | --- | --- |
| `release-docs.yml` (update) | force-pushes `bot/release-downloads-<track>` and opens its pull request | runs wait for approval |
| `dogfood-sync.yml` | `bot/dogfood-sync` through `scripts/auto-pr.sh` | runs wait for approval |
| `evals-refresh.yml` (model-drift, eval-publish) | `bot/evals-model-prices`, `bot/evals-dashboards` through `scripts/auto-pr.sh` | runs wait for approval |
| the catalog workflow (job `catalogs`) | a catalog commit to a same-repo pull request's branch, from its auto-fix script | the bot commit's runs wait for approval |
| `dependabot-go-tidy.yml`, `dependabot-pnpm-relock.yml` | a tidy or relock commit to the Dependabot branch | the bot commit's runs wait for approval |
| `release-docs.yml` (stable) | fast-forwards `docs/stable` | no CI expected; the job dispatches the docs deploys itself |

`pages-deploy.yml`, `release.yml`, `release-bowrain.yml` and
`_publish-plugin-formula.yml` push to other repositories (the Pages
repository and `homebrew-tap`), with their own credentials.

## The fix: a bot GitHub App

Each pushing job mints an installation token for a bot App
(`actions/create-github-app-token`) and uses it for the checkout, the push
and `gh pr create`. CI then runs on what the bot pushes, with no approval and
no change to the ruleset. The job holds the token only for its own run.

Until the App exists, the jobs fall back to `GITHUB_TOKEN`: the step that
mints the token runs only when `BOT_APP_PRIVATE_KEY` is set. The pull request
body and the catalog auto-fix's warning then say to approve the held runs, and the Bot PR
Watch workflow (`scripts/stale-bot-prs.py`) reports a bot pull request open
more than three days, or runs held for approval more than a day.

Two other routes were considered and not taken:

- **Dispatching CI on the bot branch.** A `workflow_dispatch` run creates
  runs, but `ci.yml`'s path filter runs only on `pull_request`, so on a
  dispatch the path-gated jobs skip and `CI passed` would pass having checked
  almost nothing.
- **Publishing the required check from the bot workflow.** A check the bot
  writes through the checks API has the same name as the real one, so `CI
  passed` would no longer mean that CI ran.

## What the founder creates

1. A GitHub App owned by the `neokapi` organization (Organization settings →
   Developer settings → GitHub Apps → New GitHub App). No webhook. Repository
   permissions: Contents read and write, Pull requests read and write
   (Metadata read is added automatically). Install it on `neokapi/neokapi`
   only.
2. A private key for it (the App's page → Private keys → Generate).
3. Two repository secrets, under Settings → Secrets and variables → Actions:
   `BOT_APP_CLIENT_ID` (the App's Client ID) and `BOT_APP_PRIVATE_KEY` (the
   whole PEM file).
4. The same two secrets under Settings → Secrets and variables → Dependabot.
   A workflow that a Dependabot pull request starts sees only Dependabot
   secrets.

The App is not a bypass actor on any ruleset. Its pull requests need `CI
passed` like every other.
