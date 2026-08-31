---
title: FAQ
sidebar_position: 22
description: Short, honest answers. When a platform is warranted, self-hosting, credits and bring-your-own keys, your data if you leave, supported formats, model training, review statuses, and providers.
---

# Frequently asked questions

## When do I need Bowrain, and when is kapi enough?

kapi holds the context graph for one project: it reads and writes many
content formats, runs checks, and draws on local memory, all from files you
own, with no server or account. Bowrain is the platform that holds voice profiles,
vocabulary, and content memory once, across every project, surface, and
teammate, and adds real-time collaboration, connectors, automation, versioned
history, and corrections that become enforced checks.

kapi on its own is enough when one person works from one checkout. Reach for
Bowrain when content lives in systems beyond that checkout, when several
projects or surfaces should share one memory, when several people work on the
same content, or when corrections should compound. Bowrain is as much for a solo
builder with many surfaces as it is for a team. The full comparison is in
[How Bowrain and kapi fit together](/getting-started/kapi-vs-bowrain).

## Can I self-host Bowrain?

Yes. Bowrain is available under the AGPL-3.0 license, or under a commercial
license, and runs as a set of Docker services (server, worker, PostgreSQL, a job
queue, and the web UI) behind your own OIDC provider. A self-hosted deployment
runs without the billing pipeline: no credit limits, no plan gates. See
[Self-hosting](/server/self-hosting).

## Do bring-your-own-key runs consume credits?

No. When a workspace configures its own AI provider key, drafting and check
operations run against that provider account, and those runs are not metered
against Bowrain credits. (Usage is still counted internally, for abuse
protection.) Credits are only spent when an operation uses the shared platform
provider. See [Security and privacy](/server/security-and-privacy#bring-your-own-ai-keys).

## What happens to my data if I leave?

You take it with you. Content round-trips back to the source formats you imported,
and your assets export to open interchange formats through the kapi
CLI: `kapi memory export` writes TMX, and `kapi terms export` writes TBX, CSV, or
JSON. A self-hosted deployment additionally keeps everything in a PostgreSQL
database you can back up directly. There is no proprietary lock-in format holding
your content hostage.

## Which formats are supported?

Bowrain reads and writes the same formats the neokapi engine supports: document
formats, data formats, subtitle formats, office formats, and bilingual
interchange formats. Rather than repeat a list that changes as formats are added, see the
generated [format reference](https://neokapi.github.io/formats)
for the current, complete set.

## Does Bowrain train models on my content?

No. Bowrain has no model-training pipeline. Your content is sent to an AI provider
only to carry out an operation you initiated, a draft or a quality check,
and, with a bring-your-own key, that request goes to your own provider account.
The platform's analytics measure product usage, not content (see
[Security and privacy](/server/security-and-privacy#analytics)).

## How do review statuses work?

Review status is tracked per block and **per locale**, so reviewing the French
target of a block never changes its German target. A block moves through four
states, **Not Started**, **Draft**, **Translated**, and **Reviewed**, and you
can filter a file by state to work through one stage at a time. In the review
session, approving a block marks it Reviewed; rejecting it sends it back to Draft.
See [Review](/server/review).

## Can I use my own machine-translation provider?

Bowrain drafts with AI (large language model) providers, not dedicated
machine-translation engines. You can bring your own key for any provider the
engine's [translate tool](https://neokapi.github.io/reference/tools/translate)
supports, including a locally hosted model, and drafting runs against it. There
is no separate machine-translation provider integration to configure: dedicated
MT providers are not part of the product.

## I have a question that is not here

Contact **hello@bowrain.cloud**.
