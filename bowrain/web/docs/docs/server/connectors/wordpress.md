---
title: WordPress
sidebar_position: 1
description: The WordPress connector reads posts from a WordPress site over the REST API, delivers them to Bowrain, and writes approved text back to the same posts.
---

# WordPress connector

The WordPress connector is a CMS connector. It reads posts from a WordPress
site over the REST API, delivers them to Bowrain, and writes approved text
back to the same posts.

:::note

This connector is added in a project's **Connectors** view in the web app, or
through the workspace **connectors API** described below. Saved credentials are
write-only: used for sync, never displayed again. All connector operations
require the **manage connectors** permission. It is one route into a workspace
among several; see [Connectors](/server/connectors) for the full row.

:::

## What it syncs

The connector works against WordPress posts through the `wp/v2/posts` REST
endpoint.

| Direction | Content |
| --------- | ------- |
| Fetch (in) | For each post: the title, the content body, and the excerpt (when the post has one). Content is read from the REST API's rendered fields and treated as HTML. |
| Publish (out) | The title, content, and excerpt fields of a post that was previously fetched. |

The connector reads posts only. Pages, custom post types, taxonomies, media,
and custom fields are not included.

## Prerequisites and credentials

You need a WordPress site whose REST API is reachable from Bowrain Server, and,
for publishing, an account that may edit the posts you intend to route through
Bowrain.

The connector accepts the following configuration keys:

| Key | Required | Description |
| --- | -------- | ----------- |
| `url` | Yes | The site's base URL, for example `https://blog.example.com`. A trailing slash is trimmed. |
| `username` | For publishing | The WordPress username used for authentication. |
| `password` | For publishing | The credential paired with `username`. Use a WordPress [application password](https://wordpress.org/documentation/article/application-passwords/) rather than the account login password. |
| `name` | No | A human-readable name for the connector instance. |
| `id` | No | A stable identifier for the connector instance. When omitted, one is derived from the URL. |

Authentication uses HTTP Basic auth. If `username` is empty the connector makes
unauthenticated requests, which is sufficient to fetch public posts but not to
publish.

## Setup

Add the connector from a project's **Connectors** view in the web app, or by
posting its type and configuration to `POST /api/v1/{workspace}/connectors`.
Connectors are scoped to the workspace they are added in.

```json
{
  "type": "wordpress",
  "config": {
    "name": "Company blog",
    "url": "https://blog.example.com",
    "username": "editor",
    "password": "xxxx xxxx xxxx xxxx xxxx xxxx"
  }
}
```

The response returns the connector's `id`, `name`, and `category`. Use that `id`
for subsequent fetch, publish, status, and remove calls.

## How sync works

Sync is explicit: content moves only when you fetch or publish (**Fetch now**
and **Publish** in the Connectors view, or the endpoints below). The connector
does not poll the site on a schedule and does not receive WordPress webhooks.

**Fetch** requests the first 100 posts from the site and stores their title,
content, and excerpt as blocks on the target project's main content stream. Call
`POST /api/v1/{workspace}/connectors/{id}/fetch` with the connector and project:

```json
{ "connector_id": "{id}", "project_id": "{project}" }
```

The response reports how many items were fetched. Fetching stores source
content; it does not itself start a run. Catch the fetched content up
the way you would any project content, for example by starting a run or by
letting the project's `bowrain.converge` policy produce and review targets.

**Publish** sends the project's stored block text back to the matching posts.
Call `POST /api/v1/{workspace}/connectors/{id}/publish` with the same fields:

```json
{ "connector_id": "{id}", "project_id": "{project}" }
```

Each post is updated in place through the REST API. The connector matches a post
by the WordPress post ID recorded when the content was fetched, so a post must
have been fetched through the connector before it can be published back. Posts
are updated, never created, and their publication status is left unchanged.

Call `GET /api/v1/{workspace}/connectors/{id}/status` to see the connector's
current view of the site.

## Limitations

- Fetch returns the first 100 posts in one request and does not paginate; sites
  with more posts are not fully covered.
- Only posts are supported. Pages, custom post types, media, and custom fields
  are out of scope.
- Publishing writes the fields of an already-fetched post. It does not create
  posts and does not change post status.
