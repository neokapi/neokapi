---
sidebar_position: 4
title: Quick start
slug: /quickstart
---

# Quick start

There are two ways into Bowrain, and they meet in the same workspace. **Start
from your content** if the work lives in a CMS, a design tool, or documents —
no install, nothing local. **Start from a codebase** if the source files live
in a repository you work in. Either way, the workspace's brand voice,
terminology, and translation memory govern what gets produced, and the
[kapi loop](/the-loop) keeps it caught up.

## Start from your content

You write and publish content; you want it on brand, and optionally in more
languages. Nothing to install — open [bowrain.cloud](https://bowrain.cloud)
(or your own server) and sign in. A personal workspace is created on first
login.

1. **Create a project** and bring content in. Upload files directly in the web
   app — the editor works on the formats you already publish — or have your
   content systems connected server-side (WordPress, HubSpot, Figma) so source
   flows in and published results flow back. See
   [Getting started on the server](/server/getting-started) and
   [Connectors](/server/connectors).
2. **Establish the brand context.** Run a [brand scan](/server/brand-scan) over
   content you already trust — it drafts a voice profile and glossary
   candidates for you to review and adopt. Terminology and translation memory
   grow in the workspace from there.
3. **Translate and check.** In the [editor](/server/translation-editor), AI
   drafts against the workspace's voice profile, terminology, and memory;
   checks flag what drifts.
4. **Review and publish.** Step through the [Review surface](/server/review),
   approve what's right — corrections feed the
   [learning loop](/server/brand-voice) — then export, or publish back through
   the connector.

The end-to-end path for this door is
[Publish on brand](/server/publish-on-brand).

## Start from a codebase

Your source files live in a repository. Sync them with the
[kapi CLI](/installation) (with the bowrain plugin installed) and let the loop
run on the server.

### Initialize a project

Create a `.kapi` project — a `<dir-name>.kapi` recipe at the project root with
a sibling `.kapi/` state directory (like `.git` for content):

```bash
kapi init
```

The interactive wizard signs you in and connects the project to a workspace on
your server (or creates an anonymous project with a claim link, or a local-only
project). Or skip the wizard with flags:

```bash
kapi init --name my-app --source en --targets fr,de
```

### Bring it up to date

One verb runs the [kapi loop](/the-loop): reuse what the workspace has
translated before, AI-translate the rest, check everything produced, park what
needs a person:

```bash
kapi up          # runs on the server — org keys, shared memory and terminology
kapi up --plan   # dry run: pending work, TM leverage, and a token estimate
```

On a connected project `kapi up` prints its venue first (*server*), streams the
run's progress into your terminal, and pulls the results down. Parked units
land in the team's [review queue](/server/review).

### See where things stand

```bash
kapi status      # per-locale coverage, server standing, and the venue
kapi diff        # what changed locally since the last sync
```

### Move content without translating

`kapi push` and `kapi pull` are pure transport — like `git push` and
`git fetch`, they move project state and never produce translations:

```bash
kapi push        # send local changes to the server
kapi pull        # fetch teammates' and reviewers' work
```

### Run a specific flow

For a custom composition — one named flow, one pass, no gate loop — define a
flow in `.kapi/flows/` and run it:

```bash
kapi run my-flow
```

See [Flows](/cli/flows/overview).

### Key commands

| Command                 | Description                                     |
| ----------------------- | ----------------------------------------------- |
| `kapi init`             | Initialize a project                            |
| `kapi up`               | Run the loop — catch every language up          |
| `kapi status`           | Coverage, sync state, and venue                 |
| `kapi diff`             | What changed since the last sync                |
| `kapi push` / `kapi pull` | Pure transport to and from the server         |
| `kapi check --ship`     | The release gate (exit 3 when unmet)            |
| `kapi run <flow>`       | Execute a composed or custom flow               |
| `kapi pseudo-translate` | Local pseudo-translation for testing            |

## Next steps

- [The kapi loop on Bowrain](/the-loop) — how produce, promote, and release fit together
- [Publish on brand](/server/publish-on-brand) — the content door, end to end
- [The loop in CI](/cli/ci/overview) — GitHub Actions, GitLab CI, and the Bowrain GitHub App
- [Walkthrough](/walkthroughs/bowrain-getting-started) — the developer path on video
