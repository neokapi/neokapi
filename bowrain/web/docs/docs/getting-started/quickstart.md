---
sidebar_position: 4
title: Quick start
slug: /quickstart
description: Get content into a Bowrain workspace, from the systems it already lives in, from a repository with no pipeline, or from a developer's checkout.
keywords: [quick start, bowrain, connectors, workspace, context scan, review, publish, kapi]
---

# Quick start

Content reaches a workspace through a [connector](/server/connectors), and every
route meets in the same place: the same profiles, the same vocabulary, the same
content memory, the same review session. Pick the route that matches where
your source lives.

| Your source lives in | Start here | Anything to install? |
| --- | --- | --- |
| A content platform, a design tool, or documents | [From your content](#from-your-content) | No |
| A repository, and nobody wants a pipeline | [From a repository](#from-a-repository) | No |
| A repository a developer works in every day | [From a checkout](#from-a-checkout) | The kapi CLI |

## From your content

You write and publish content; you want it on profile for the audience it was
written for, and optionally in more languages. Nothing to install: open
[app.bowrain.cloud](https://app.bowrain.cloud) (or your own server) and sign
in. A personal workspace is created on first login.

1. **Establish the context.** Run a [context scan](/server/context-scan) over
   content you already trust. It proposes the axes your content varies along, a
   voice profile and term candidates for you to review and approve. Vocabulary
   and content memory grow in the workspace from there.
2. **Create a project** and bring content in. Upload files directly in the web
   app, or connect the systems your content lives in (WordPress, HubSpot,
   Figma) so source flows in and published results flow back. See
   [First login and first project](/server/getting-started) and
   [Connectors](/server/connectors).
3. **Draft and check.** A [run](/the-loop) checks the source against the
   context and drafts any target languages against the voice profile,
   vocabulary, and memory in force at each point; checks flag what drifts.
4. **Review and publish.** Step through the [review session](/server/review),
   approve what is right (corrections feed the
   [learning loop](/server/context-voice)), then export, or publish back
   through the connector.

The end-to-end path for this route is
[Publish on brand](/server/publish-on-brand).

## From a repository

Your source files live in a GitHub or GitLab repository, and you would rather
not add a pipeline to it. Connect the repository server-side: a push webhook
triggers a run, and the results arrive as **one open pull/merge request** that
every later delivery updates in place.

Nothing is installed and nothing is checked out; the server holds the
credentials and does the work. With a GitHub App registered for your server,
installing the app on a repository and binding it to a project is the only
per-repository step.

See the [GitHub / GitLab connector](/server/connectors/github) for setup, and
[The Bowrain GitHub App](/cli/ci/github-app) for the walkthrough.

## From a checkout

A developer edits the source files every day and wants the results in the
working tree they already have open. This is the [kapi
connector](/server/connectors/kapi): the kapi CLI with the bowrain plugin
installed:

```bash
brew install neokapi/tap/bowrain-cli
```

Connect the repository to a workspace, then catch it up:

```bash
kapi init        # write kapi.yaml + .kapi/, sign in, connect to a workspace
kapi up          # catch the project up; runs on the server
kapi status      # per-scope coverage, ship standing, server standing, and venue
```

`kapi up` pushes what drifted, runs on the server with the organization's keys
and shared memory, streams progress into the terminal, and pulls the results
down. What a machine cannot decide parks into the team's
[review session](/server/review).

The full command set, flows, and the CI contract are in the
[CLI section](/cli/overview); [Installation](/installation) covers the other
platforms.

## Next steps

- [Connectors](/server/connectors): every route into a workspace, side by side
- [Keeping content caught up](/the-loop): what a run does and who starts it
- [Publish on brand](/server/publish-on-brand): the content route, end to end
- [The loop in CI](/cli/ci/overview): pipelines and the ship gate
- [Walkthrough](/walkthroughs/bowrain-getting-started): the developer route on video
