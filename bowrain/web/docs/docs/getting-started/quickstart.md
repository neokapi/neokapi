---
sidebar_position: 4
title: Quick start
slug: /quickstart
description: Get content into a Bowrain workspace, from the systems it already lives in, from a repository with no pipeline, or from a developer's checkout.
keywords: [quick start, bowrain, connectors, workspace, context scan, review, publish, kapi]
---

# Quick start

Content reaches a workspace through a [connector](/server/connectors). Every
route leads to the same profiles, vocabulary, content memory and review
session. Pick the route that matches where your source lives.

| Your source lives in | Start here | Anything to install? |
| --- | --- | --- |
| A content platform, a design tool, or documents | [From your content](#from-your-content) | No |
| A repository without a CI pipeline | [From a repository](#from-a-repository) | No |
| A repository a developer works in every day | [From a checkout](#from-a-checkout) | The kapi CLI |

## From your content

You write and publish content, and you want it to fit its audience, possibly
in more languages too. There is nothing to install. Open
[app.bowrain.cloud](https://app.bowrain.cloud) (or your own server) and sign
in. Bowrain creates a personal workspace on your first login.

1. **Establish the context.** Run a [context scan](/server/context-scan) over
   content you already trust. It proposes the axes your content varies along, a
   voice profile and term candidates for you to review and approve. The
   workspace adds terms and content memory as you work.
2. **Create a project** and bring content in. Upload files directly in the web
   app, or connect the systems your content lives in (WordPress, HubSpot,
   Figma) so source flows in and published results flow back. See
   [First login and first project](/server/getting-started) and
   [Connectors](/server/connectors).
3. **Draft and check.** A [run](/the-loop) checks the source against the
   context. It drafts any target languages with the voice profile, terms and
   memory that apply at each point, and the checks flag what drifts.
4. **Review and publish.** Step through the [review session](/server/review),
   approve what is right (corrections feed the
   [learning loop](/server/context-voice)), then export, or publish back
   through the connector.

The end-to-end path for this route is
[Publish on brand](/server/publish-on-brand).

## From a repository

Your source files live in a GitHub or GitLab repository, and you don't want to
add a pipeline to it. Connect the repository server-side. A push webhook starts
a run, and the results arrive as **one open pull/merge request** that each later
delivery updates in place.

You install nothing and check nothing out. The server holds the credentials
and does the work. With a GitHub App registered for your server, the only
per-repository step is to install the app on the repository and bind it to a
project.

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

`kapi up` pushes what drifted and runs on the server with the organization's
keys and shared memory. It streams progress into the terminal and pulls the
results down. Items a machine cannot decide wait in the team's
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
