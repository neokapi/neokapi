---
title: GitLab CI
sidebar_label: GitLab CI
---

# GitLab CI

The [`kapi-components`](https://gitlab.com/neokapi/kapi-components) in the
GitLab CI/CD Catalog run the kapi loop from a `.gitlab-ci.yml` include: `up`
catches the project up and reports what changed, and opens a merge request
when you set `deliver: mr`; `check` gates merge requests on the project's ship
gates. Both run on the `ghcr.io/neokapi/kapi` image with the server-sync
plugin preinstalled, so a job needs no install step.

## CI/CD variables

Set these under **Settings → CI/CD → Variables**:

| Variable | Purpose |
|---|---|
| `BOWRAIN_AUTH_TOKEN` | Server auth token, minted with `kapi auth token create`; mark it **masked** |
| `GITLAB_API_TOKEN` | Project access token with **api** scope; pushes the branch and opens the MR under `deliver: mr`, and posts and updates the MR notes |
| `BOWRAIN_SERVER_URL` | **Self-hosted only.** The hosted service (`https://app.bowrain.cloud`) is the built-in default, and project commands read the server from the checked-out recipe's `bowrain:` block |

On a connected project (a recipe with a `bowrain:` block), `kapi up` runs the
loop on the **server**: the organization's AI keys, shared content memory, and
terms live there, so the job carries no AI keys of its own. Only a
local-venue run, a project with no `bowrain:` block, or `kapi up --local`,
needs a provider key such as `ANTHROPIC_API_KEY` in the job. See
[CI authentication](/cli/ci/overview#authenticating-a-runner).

## Catch up and deliver a merge request

```yaml title=".gitlab-ci.yml"
stages: [build, test, kapi, deploy]

include:
  - component: gitlab.com/neokapi/kapi-components/up@0.2.0
    inputs:
      stage: kapi
      deliver: mr
```

By default the `kapi_up` job runs on scheduled and web-triggered pipelines
and on pushes to the default branch. It runs `kapi up` and reports what the
run changed in the `kapi-changed-files.txt` artifact and the
`KAPI_HAS_CHANGES` variable, for a delivery job of your own. With
`deliver: mr`, when the run produced changes, the job pushes a branch and
opens a merge request through the API (`GITLAB_API_TOKEN`) carrying the run
report: outcome, passes, parked locales. `deliver: commit` pushes a commit to
the pipeline's branch instead. A run that **parks** still delivers what it
caught up; the parked locales are the review queue, not a failure (set
`fail_on_parked: true` to hard-fail instead).

The `stage` input names the stage the job runs in, and the pipeline's
`stages:` list must declare it: GitLab refuses to create a pipeline in which a
job names an undeclared stage.

The job writes its report files (`kapi.env`, `kapi-up.ndjson` and
`kapi-changed-files.txt`) into the checkout. List them in the repository's
`.gitignore`, or the job counts them as changes and a delivery commits them.

The job publishes its result as dotenv variables for downstream jobs:

| Variable | Value |
|---|---|
| `KAPI_OUTCOME` | `converged` or `parked` |
| `KAPI_PASSES` | Reconciliation passes the run took |
| `KAPI_PARKED_LOCALES` | Comma-separated locales still short of their gate |
| `KAPI_HAS_CHANGES` | `true` when the run left changes in the working tree, `false` otherwise |

```yaml title=".gitlab-ci.yml"
report:
  stage: .post
  script:
    - 'echo "kapi up: $KAPI_OUTCOME after $KAPI_PASSES pass(es); parked: ${KAPI_PARKED_LOCALES:-none}"'
```

## Gate merge requests

```yaml title=".gitlab-ci.yml"
include:
  - component: gitlab.com/neokapi/kapi-components/check@0.2.0
```

The `kapi_check` job runs on merge-request pipelines and on pushes to the
default branch, runs `kapi check --ship`, and fails on exit `3`: a voice,
terms, rule-based check, or coverage gate is unmet. It posts one threaded MR
note with the failing gates and findings, updated in place on re-runs, and
keeps the full `--json` report as a job artifact. Exit `4`, a gate that did
not run, also fails the job, with no MR note; the report artifact names the
cause in `did_not_run_cause`. The gate is the explicit, opt-in enforcement
point, and no other job fails on target-language drift. To leave pushes to the
default branch ungated, set the `rules` input to merge-request pipelines alone.

## Report the cost of a change on its merge request

With `plan: true` the job dry-runs the loop (pending units, memory reuse, and
a token estimate; no writes, no provider calls) and posts the result as one
threaded MR note that re-runs update in place. Declare the `kapi` stage as in
the catch-up example:

```yaml title=".gitlab-ci.yml"
include:
  - component: gitlab.com/neokapi/kapi-components/up@0.2.0
    inputs:
      stage: kapi
      plan: true
      rules:
        - if: $CI_PIPELINE_SOURCE == "merge_request_event"
```

## Related

- [The loop in CI](/cli/ci/overview): the surfaces map, the exit-code contract, and CI authentication
- [The Bowrain GitHub App](/cli/ci/github-app): forge delivery with no pipeline, GitLab included
- [`kapi up`](/cli/commands/up): flags, venue resolution, the run stream
- [Gate governed terms in CI](/cli/use-cases/brand-terminology-ci): governed terms as a merge gate
