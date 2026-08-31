---
title: Figma
sidebar_position: 3
description: The Figma connector reads the text layers of a Figma design file over the Figma REST API and delivers each one to Bowrain.
---

# Figma connector

The Figma connector is a design connector. It reads the text of a Figma design
file over the Figma REST API and delivers each text layer to Bowrain. It is a
one-directional connector: it fetches text but does not write results back to
Figma.

:::note

This connector is added in a project's **Connectors** view in the web app, or
through the workspace **connectors API** described below. Saved credentials are
write-only: used for sync, never displayed again. All connector operations
require the **manage connectors** permission. It is one route into a workspace
among several; see [Connectors](/server/connectors) for the full row.

:::

## What it syncs

The connector reads a single Figma file, identified by its file key.

| Direction | Content |
| --------- | ------- |
| Fetch (in) | Every text layer in the file with non-empty content. Each text layer becomes one block, named after the layer. Where the layer has a bounding box, the block carries a display hint recording the frame position and a rough character-length estimate, which downstream tools can use for length-aware drafting. |
| Publish (out) | Not supported. |

## Prerequisites and credentials

You need a Figma file and a Figma personal access token that can read it.

The connector accepts the following configuration keys:

| Key | Required | Description |
| --- | -------- | ----------- |
| `file_key` | Yes | The file key from the Figma file URL: the segment after `/file/` or `/design/` in `https://www.figma.com/design/<file_key>/...`. |
| `token` | Yes | A Figma [personal access token](https://www.figma.com/developers/api#access-tokens). It is sent in the `X-Figma-Token` request header. |
| `name` | No | A human-readable name for the connector instance. |
| `id` | No | A stable identifier for the connector instance. When omitted, one is derived from the file key. |

## Setup

Add the connector from a project's **Connectors** view in the web app, or by
posting its type and configuration to `POST /api/v1/{workspace}/connectors`.
Connectors are scoped to the workspace they are added in.

```json
{
  "type": "figma",
  "config": {
    "name": "Marketing site design",
    "file_key": "AbCdEf123456",
    "token": "figd_xxxxxxxxxxxxxxxxxxxx"
  }
}
```

The response returns the connector's `id`, `name`, and `category`. Use that `id`
for subsequent fetch and status calls.

## How sync works

Sync is explicit: text is read only when you fetch (**Fetch now** in the
Connectors view, or the fetch endpoint). The connector does not poll Figma on a
schedule and does not receive Figma webhooks.

**Fetch** reads the file, walks its layer tree, and stores each non-empty text
layer as a block on the target project's main content stream. Call
`POST /api/v1/{workspace}/connectors/{id}/fetch` with the connector and project:

```json
{ "connector_id": "{id}", "project_id": "{project}" }
```

The response reports how many items were fetched. Fetching stores source
content; it does not itself start a run. Catch the fetched content up the
way you would any project content, for example by starting a run or by letting
the project's `bowrain.converge` policy produce and review targets.

Because the connector does not publish, translated strings are consumed
elsewhere: export them from Bowrain, or read them through the API, and apply
them to the design by hand or through a separate Figma integration.

Call `GET /api/v1/{workspace}/connectors/{id}/status` to see the connector's
current view of the file.

## Limitations

- The connector is pull-only. Figma's general REST API does not support writing
  text back, so there is no publish path; a publish call returns an error.
- It reads one file per connector instance, identified by `file_key`.
- Only text layers are read. Component names, variables, and other non-text
  content are out of scope.
