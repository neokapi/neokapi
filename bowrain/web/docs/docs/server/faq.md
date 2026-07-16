---
title: FAQ
sidebar_position: 22
description: Short, honest answers — kapi versus Bowrain, self-hosting, credits and bring-your-own keys, your data if you leave, supported formats, model training, review statuses, and MT providers.
---

# Frequently asked questions

## When do I need Bowrain, and when is kapi enough?

kapi is the open-source, single-user toolchain: it reads, translates, and ships
content in many formats, runs checks, and uses translation memory — all locally,
from files you own, with no server or account. Bowrain is the server platform that
hosts brand voice, terminology, and translation memory once — across every
project, surface, and teammate — adds real-time collaboration, connectors,
automation, and versioned history, and turns your corrections into enforced
checks.

The relationship is the same as git and GitHub: you start in kapi and connect a
project to Bowrain when the work should outlive a single run — several projects
or surfaces to keep consistent, corrections that should compound, or a team.
Bowrain is as much for a solo developer with many surfaces as it is for a team. The full comparison
is in [Kapi vs Bowrain](/getting-started/kapi-vs-bowrain).

## Can I self-host Bowrain?

Yes. Bowrain is available under the AGPL-3.0 license, or under a commercial
license, and runs as a set of Docker services (server, worker, PostgreSQL, a job
queue, and the web UI) behind your own OIDC provider. A self-hosted deployment
runs without the billing pipeline: no credit limits, no plan gates. See
[Self-hosting](/server/self-hosting).

## Do bring-your-own-key runs consume credits?

No. When a workspace configures its own AI provider key, translation and check
operations run against that provider account, and those runs are not metered
against Bowrain credits. (Usage is still counted internally, for abuse
protection.) Credits are only spent when an operation uses the shared platform
provider. See [Security and privacy](/server/security-and-privacy#bring-your-own-ai-keys).

## What happens to my data if I leave?

You take it with you. Content round-trips back to the source formats you imported,
and your linguistic assets export to open interchange formats through the kapi
CLI: `kapi tm export` writes TMX, and `kapi termbase export` writes TBX, CSV, or
JSON. A self-hosted deployment additionally keeps everything in a PostgreSQL
database you can back up directly. There is no proprietary lock-in format holding
your content hostage.

## Which formats are supported?

Bowrain reads and writes the same formats the neokapi engine supports:
localization formats, document formats, data formats, subtitle formats, and office
formats. Rather than repeat a list that changes as formats are added, see the
generated [format reference](https://neokapi.github.io/web/neokapi/docs/features/formats)
for the current, complete set.

## Does Bowrain train models on my content?

No. Bowrain has no model-training pipeline. Your content is sent to an AI provider
only to carry out an operation you initiated — a translation or a quality check —
and, with a bring-your-own key, that request goes to your own provider account.
The platform's analytics measure product usage, not content (see
[Security and privacy](/server/security-and-privacy#analytics)).

## How do review statuses work?

Review status is tracked per block and **per locale**, so reviewing the French
target of a block never changes its German target. A block moves through four
states — **Not Started**, **Draft**, **Translated**, and **Reviewed** — and you
can filter a file by state to work through one stage at a time. In the Review
surface, approving a block marks it Reviewed; rejecting it sends it back to Draft.
See [Review](/server/review).

## Can I use my own machine-translation provider?

Bowrain translates with AI (large language model) providers, not dedicated
machine-translation engines. You can bring your own AI provider key — Anthropic,
OpenAI, Azure OpenAI, Google Gemini, or a local Ollama model — and translation
runs against it. There is no separate machine-translation provider integration to
configure: dedicated MT providers are not part of the product.

## I have a question that is not here

Contact **hello@bowrain.cloud**.
