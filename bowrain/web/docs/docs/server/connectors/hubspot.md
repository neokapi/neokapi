---
title: HubSpot
sidebar_position: 2
description: The HubSpot connector reads CMS site pages from HubSpot, delivers their titles and meta descriptions to Bowrain, and writes approved text back.
---

# HubSpot connector

The HubSpot connector is a marketing connector. It reads CMS site pages from
HubSpot over the CMS API, delivers their title and meta description to Bowrain,
and writes approved text back to the same pages.

:::note

This connector is added in a project's **Connectors** view in the web app, or
through the workspace **connectors API** described below. Saved credentials are
write-only: used for sync, never displayed again. All connector operations
require the **manage connectors** permission. It is one route into a workspace
among several; see [Connectors](/server/connectors) for the full row.

:::

## What it syncs

The connector works against HubSpot CMS site pages through the
`cms/v3/pages/site-pages` API.

| Direction | Content |
| --------- | ------- |
| Fetch (in) | For each site page: the HTML title, and the meta description (when the page has one). |
| Publish (out) | The HTML title and meta description of a previously fetched page. |

The connector reads the page title and meta description only. Page body
content, modules, blog posts, emails, and landing pages are not included.

## Prerequisites and credentials

You need a HubSpot account with the CMS pages API available, and a private app
access token that can read and write CMS pages.

The connector accepts the following configuration keys:

| Key | Required | Description |
| --- | -------- | ----------- |
| `api_key` | Yes | A HubSpot [private app](https://developers.hubspot.com/docs/api/private-apps) access token. It is sent as a bearer token in the `Authorization` header. |
| `name` | No | A human-readable name for the connector instance. |
| `id` | No | A stable identifier for the connector instance. When omitted, it defaults to `hubspot`. |

## Setup

Add the connector from a project's **Connectors** view in the web app, or by
posting its type and configuration to `POST /api/v1/{workspace}/connectors`.
Connectors are scoped to the workspace they are added in.

```json
{
  "type": "hubspot",
  "config": {
    "name": "Marketing pages",
    "api_key": "pat-xx-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  }
}
```

The response returns the connector's `id`, `name`, and `category`. Use that `id`
for subsequent fetch, publish, status, and remove calls.

## How sync works

Sync is explicit: content moves only when you fetch or publish (**Fetch now**
and **Publish** in the Connectors view, or the endpoints below). The connector
does not poll HubSpot on a schedule and does not receive HubSpot webhooks.

**Fetch** requests site pages and stores each page's title and meta description
as blocks on the target project's main content stream. Call
`POST /api/v1/{workspace}/connectors/{id}/fetch` with the connector and project:

```json
{ "connector_id": "{id}", "project_id": "{project}" }
```

The response reports how many items were fetched. Fetching stores source
content; it does not itself start a run. Catch the fetched content up the
way you would any project content, for example by starting a run or by letting
the project's `bowrain.converge` policy produce and review targets.

**Publish** sends the project's stored title and meta-description text back to
the matching pages. Call `POST /api/v1/{workspace}/connectors/{id}/publish` with
the same fields:

```json
{ "connector_id": "{id}", "project_id": "{project}" }
```

Each page is updated in place with a PATCH to the CMS pages API, matched by its
HubSpot page ID. The connector writes to the page object; it does not perform a
separate HubSpot publish step, so changes may need to be published from within
HubSpot to appear on the live site.

Call `GET /api/v1/{workspace}/connectors/{id}/status` to see the connector's
current view of HubSpot.

## Limitations

- Only the HTML title and meta description are synced. Page body content and
  modules are out of scope.
- Fetch reads the first page of site pages returned by the API and does not
  paginate; accounts with more pages are not fully covered.
- Publishing updates the page object through a PATCH and does not push the page
  live; a separate publish action in HubSpot may be required.
