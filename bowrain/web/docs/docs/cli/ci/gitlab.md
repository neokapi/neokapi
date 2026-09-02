---
title: GitLab CI
sidebar_label: GitLab CI
---

# GitLab CI

The [`kapi-components`](https://gitlab.com/neokapi/kapi-components) in the
GitLab CI/CD Catalog run the kapi loop from a `.gitlab-ci.yml` include: `up`
catches the project up and delivers a merge request, `check` gates merge
requests on the project's ship gates. Both run on the `ghcr.io/neokapi/kapi`
image with the server-sync plugin preinstalled, so a job needs no install
step.

## CI/CD variables

Set these under **Settings → CI/CD → Variables**:

| Variable | Purpose |
|---|---|
| `BOWRAIN_AUTH_TOKEN` | Server auth token, minted with `kapi auth token create`; mark it **masked** |
| `GITLAB_API_TOKEN` | Project access token with **api** scope; opens and updates the MR, posts MR notes |
| `BOWRAIN_SERVER_URL` | **Self-hosted only.** The hosted service (`https://app.bowrain.cloud`) is the built-in default, and project commands read the server from the checked-out recipe's `bowrain:` block |

On a connected project (a recipe with a `bowrain:` block), `kapi up` runs the
loop on the **server**: the organization's AI keys, shared content memory, and
terms live there, so the job carries no AI keys of its own. Only a
local-venue run, a project with no `bowrain:` block, or `kapi up --local`,
needs a provider key such as `ANTHROPIC_API_KEY` in the job. See
[CI authentication](/cli/ci/overview#authenticating-a-runner).

## Catch up and deliver a merge request

```yaml title=".gitlab-ci.yml"
include:
  - component: gitlab.com/neokapi/kapi-components/up@0.1.0
```

By default the `kapi_up` job runs on scheduled and web-triggered pipelines
and on pushes to the default branch. It runs `kapi up`, and when the run
produced changes it pushes a branch and opens a merge request through the API
(`GITLAB_API_TOKEN`) carrying the run report: outcome, passes, parked
locales. A run that **parks** still delivers what it caught up; the parked
locales are the review queue, not a failure (set `fail_on_parked: true` to
hard-fail instead).

The job publishes its result as dotenv variables for downstream jobs:

| Variable | Value |
|---|---|
| `KAPI_OUTCOME` | `converged` or `parked` |
| `KAPI_PASSES` | Reconciliation passes the run took |
| `KAPI_PARKED_LOCALES` | Comma-separated locales still short of their gate |

```yaml title=".gitlab-ci.yml"
report:
  stage: .post
  script:
    - 'echo "kapi up: $KAPI_OUTCOME after $KAPI_PASSES pass(es); parked: ${KAPI_PARKED_LOCALES:-none}"'
```

## Gate merge requests

```yaml title=".gitlab-ci.yml"
include:
  - component: gitlab.com/neokapi/kapi-components/check@0.1.0
```

The `kapi_check` job runs on merge-request pipelines, runs
`kapi check --ship`, and fails on exit `3`: a voice, terms, rule-based check, or
coverage gate is unmet. It posts one threaded MR note with the failing gates
and findings, updated in place on re-runs, and keeps the full `kapi.check/v1`
report as a job artifact. Ordinary pushes never fail on target-language
drift; the gate is the explicit, opt-in enforcement point.

## Report the cost of a change on its merge request

With `plan: true` the job dry-runs the loop (pending units, memory reuse, and
a token estimate; no writes, no provider calls) and posts the result as one
threaded MR note that re-runs update in place:

```yaml title=".gitlab-ci.yml"
include:
  - component: gitlab.com/neokapi/kapi-components/up@0.1.0
    inputs:
      plan: true
      deliver: "none"
      rules:
        - if: $CI_PIPELINE_SOURCE == "merge_request_event"
```

## Related

- [The loop in CI](/cli/ci/overview): the surfaces map, the exit-code contract, and CI authentication
- [The Bowrain GitHub App](/cli/ci/github-app): forge delivery with no pipeline, GitLab included
- [`kapi up`](/cli/commands/up): flags, venue resolution, the run stream
- [Gate governed terms in CI](/cli/use-cases/brand-terminology-ci): governed terms as a merge gate
