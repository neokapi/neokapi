---
sidebar_position: 5
title: Connectors
---

# Server Connectors

Connectors integrate Bowrain Server with external systems (CMS, design tools, code repositories, marketing platforms).

## Connector Types

| Type          | Examples                   | Purpose                        |
| ------------- | -------------------------- | ------------------------------ |
| **CMS**       | Contentful, Sanity, Strapi | Source content for translation |
| **Design**    | Figma, Sketch              | UI text strings from designs   |
| **Code**      | GitHub, GitLab, Bitbucket  | Localization files in repos    |
| **Marketing** | HubSpot, Marketo           | Campaign and email content     |
| **File**      | kapi (bowrain plugin)      | Local file sync                |

## How Connectors Work

1. **Pull**: Fetch content from external system → Bowrain Server
2. **Process**: Translate, review, QA within Bowrain
3. **Push**: Send translations back to external system

```
External System ←→ Connector ←→ Bowrain Server ←→ Translators
```

## File Connector (kapi)

kapi (with the bowrain plugin) acts as a file connector:

```bash
# Initialize connection to server
kapi init --server https://bowrain.example.com --project abc123

# Pull translations from server
kapi pull

# Push local changes to server
kapi push -m "Translate new features"
```

See the [CLI documentation](/cli/overview) for details.

## Server-Side Connectors

Server-side connectors run on Bowrain Server and integrate with external APIs.

### Configuration

Connectors are configured per workspace in the web UI or via API:

```bash
# Create a CMS connector
POST /api/v1/workspaces/:ws/connectors

{
  "type": "contentful",
  "name": "Production CMS",
  "config": {
    "space_id": "abc123",
    "access_token": "...",
    "environment": "master"
  },
  "mappings": [
    {
      "content_type": "blogPost",
      "fields": ["title", "body", "excerpt"],
      "locale_mapping": {
        "en-US": "en",
        "fr-FR": "fr"
      }
    }
  ]
}
```

### Automation

Connectors can trigger automatic workflows:

```yaml
# On new content in Contentful
event: connector.content_pushed
source: contentful-prod
action:
  - translate_with_ai
  - run_qa_checks
  - notify_reviewers
```

## Implementation Status

:::warning Work in Progress

Server-side connectors are under development. Currently supported:

- **File connector** via kapi (placeholder)
- **GitHub connector** (in progress)
- **Contentful connector** (planned)

:::

## Next Steps

- [CLI Overview](/cli/overview)
- [Automation](/server/automation)
- [Workspaces](/server/workspaces)
