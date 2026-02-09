---
title: connect
sidebar_position: 7
---

# kapi connect

Manage bidirectional connectors to external content sources.

## Commands

### connect add

Add a new connector:

```bash
kapi connect add file --path /path/to/content --format json
kapi connect add git --url https://github.com/org/repo.git --pattern "locales/**/*.json"
kapi connect add wordpress --url https://example.com --username admin --password app-pass
```

### connect list

List all active connectors:

```bash
kapi connect list
```

## Available Connector Types

| Type | Category | Description |
|------|----------|-------------|
| `file` | File | Local filesystem content |
| `git` | Code | Git repositories |
| `wordpress` | CMS | WordPress REST API |
| `figma` | Design | Figma text nodes |
| `hubspot` | Marketing | HubSpot CMS |

## Pull and Push

After configuring a connector, use pull and push to sync content:

```bash
# Pull content from a connector into a project
kapi pull --connector conn-id --project proj-1

# Push translations back to the connector
kapi push --connector conn-id --project proj-1 --locale fr
```
