# Workspaces API

A workspace is the container for boards and members.

## List workspaces

`GET /v1/workspaces`

Returns the workspaces the access token can read, newest first.

| Field | Type | Description |
| --- | --- | --- |
| `id` | string | The workspace's identifier, such as `ws_8f2k1`. |
| `name` | string | The workspace's display name. |
| `member_count` | integer | People with access to the workspace. |

## Get one workspace

`GET /v1/workspaces/{id}`

Returns one workspace, or `404` when the token cannot read it.
