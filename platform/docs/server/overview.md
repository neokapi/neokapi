---
sidebar_position: 1
title: Overview
---

# Bowrain Server

Bowrain Server is the integration platform that connects translation tools, services, and team members into a unified workflow.

## What is Bowrain Server?

Bowrain Server is to Kapi as **GitHub is to git** — a centralized collaboration platform that:

- Hosts translation projects for team collaboration
- Connects to external systems (CMS, design tools, code repos)
- Orchestrates automation workflows and quality gates
- Provides REST API for Kapi CLI and Bowrain apps

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  Bowrain Server                      │
│                                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────┐ │
│  │ Content Store│  │ Event System │  │ Workflows │ │
│  └──────────────┘  └──────────────┘  └───────────┘ │
│                                                      │
│  ┌────────────────────────────────────────────────┐ │
│  │              REST API                          │ │
│  └────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
           │                  │                │
           ▼                  ▼                ▼
      Kapi CLI          Bowrain Web      Bowrain Desktop
```

## Key Features

### Multi-Tenancy

Workspaces provide isolated environments for teams:

```
workspace/acme
├── Project: Website
├── Project: Mobile App
└── Members: alice, bob, carol

workspace/contoso
├── Project: Documentation
└── Members: dave, eve
```

### Connectors

Integrate with external systems:

| Connector Type | Examples | Purpose |
|----------------|----------|---------|
| **CMS** | Contentful, Sanity, Strapi | Source content |
| **Design** | Figma, Sketch | UI text strings |
| **Code** | GitHub, GitLab | Localization files |
| **Marketing** | HubSpot, Marketo | Campaign content |
| **File** | Kapi CLI | Local file sync |

### Automation

Event-driven workflows:

```
Event: New content pushed
  → Auto-translate with AI
  → Run QA checks
  → Notify translators
  → Export to CMS when approved
```

### Quality Gates

Enforce standards before content goes live:

- Terminology compliance
- TM fuzzy match thresholds
- AI confidence scores
- Custom validation rules

## When to Deploy Bowrain Server

Deploy Bowrain Server when you need:

- **Team collaboration** — Multiple translators, reviewers, project managers
- **Integration** — Connect CMS, design tools, code repositories
- **Automation** — Trigger workflows on content changes
- **Centralized TM** — Share translation memory across projects
- **Access control** — Role-based permissions (workspace admin, translator, reviewer)

For solo work or local-only workflows, use **Kapi CLI** instead — no server required.

## Deployment Options

### Docker (Recommended)

```bash
docker run -p 8080:8080 \
  -e DATABASE_URL=postgres://... \
  -e OIDC_ISSUER=https://dex.example.com \
  ghcr.io/gokapi/bowrain-server:latest
```

### Kubernetes

Helm chart for production deployments:

```bash
helm install bowrain gokapi/bowrain-server \
  --set database.url=postgres://... \
  --set oidc.issuer=https://dex.example.com
```

### systemd

Run as a native service on Linux:

```bash
bowrain-server \
  --database postgres://... \
  --oidc-issuer https://dex.example.com \
  --port 8080
```

## Components

### Content Store

Block-based storage with content addressing:

- Deduplication via SHA-256 hashing
- Version snapshots
- Change tracking
- KAZ export/import

### Event Bus

Publish-subscribe system for automation:

```
Publisher          Event Bus          Subscriber
---------          ---------          ----------
Connector  →  ContentPushed  →  AI Translation
Connector  →  ContentPushed  →  QA Check
Translator →  ContentApproved → CMS Export
```

### Workspaces

Multi-tenant isolation:

- Separate projects per workspace
- Independent members and roles
- Isolated TM and terminology
- Per-workspace automation rules

## Next Steps

- [Installation](/bowrain/server/installation)
- [Configuration](/bowrain/server/configuration)
- [Workspaces](/bowrain/server/workspaces)
- [Connectors](/bowrain/server/connectors)
- [Automation](/bowrain/server/automation)
- [Self-Hosting](/bowrain/server/self-hosting)
