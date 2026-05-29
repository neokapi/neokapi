---
title: Connectors
sidebar_position: 5
---

# Connectors in Bowrain

Bowrain's Connector panel lets you connect to **remote** content sources and sync translations bidirectionally. Sourcing from a local filesystem or a git checkout is a server-side concern — the file and git connectors are configured on the server, and a local codebase syncs in through kapi (`kapi push` / `kapi pull`).

## Accessing Connectors

Click **Connectors** in the left sidebar to open the Connector panel.

## Adding a Connector

1. Select a connector type from the dropdown (file, git, wordpress, figma, hubspot)
2. Configure the connector with a path, URL, or API credentials
3. Click **Add** to create the connector

## Content Browser

After adding a connector, click on it to browse its content items. Each item shows:

- File name or content title
- Number of translatable blocks

## Pull and Push

- **Pull**: Import content from the connector into the active project
- **Push**: Export translations from the project back to the connector

## Sync Status

The sync status indicator shows:

- **Synced**: All content is up to date
- **Pending**: Changes are available to pull or push
- **Error**: A sync error occurred

## Supported Connectors

| Type      | What it connects to                    |
| --------- | -------------------------------------- |
| WordPress | WordPress posts and pages via REST API |
| Figma     | Text nodes in Figma designs            |
| HubSpot   | HubSpot CMS pages                      |

File and git sources are configured **server-side**; see [Connectors](/server/connectors).
