---
title: Connectors
sidebar_position: 12
---

# Connector System

Connectors provide bidirectional sync between neokapi and external content sources. They pull content into the Content Store and push results back. This page covers the Go interfaces; for configuring connectors on a server, see [Connectors](/server/connectors).

## Connector Interfaces

The interfaces live in `bowrain/core/connector`. Two perspectives require two interfaces, both sharing a common base:

```go
// ConnectorBase contains shared connector identity and lifecycle methods.
type ConnectorBase interface {
    ID() string
    Name() string
    Category() Category
    Status(ctx context.Context) (*SyncStatus, error)
    Configure(config map[string]string) error
    Close() error
}

// IntegrationConnector represents a system that Bowrain reaches into.
// Used by server-side integrations (WordPress, Figma, HubSpot, filesystem, Git).
type IntegrationConnector interface {
    ConnectorBase

    // Fetch retrieves source content FROM the external system INTO Bowrain.
    Fetch(ctx context.Context, opts FetchOptions) ([]*ContentItem, error)

    // Publish sends content FROM Bowrain TO the external system.
    Publish(ctx context.Context, items []*ContentItem, opts PublishOptions) error

    // List returns available content items without fetching full content.
    List(ctx context.Context) ([]*ContentItem, error)
}

// SourceConnector represents a content source that pushes to and pulls from
// Bowrain. Used by systems outside Bowrain (kapi CLI, Git hooks, CI/CD).
type SourceConnector interface {
    ConnectorBase

    // Push sends source content FROM the source system TO Bowrain.
    Push(ctx context.Context, opts PushOptions) (*PushResult, error)

    // Pull retrieves content FROM Bowrain TO the source system.
    Pull(ctx context.Context, opts PullOptions) (*PullResult, error)
}
```

`IntegrationConnector` uses fetch/publish terminology from Bowrain's perspective; `SourceConnector` uses push/pull terminology from the source system's perspective. The full option and result types are declared alongside the interfaces in the connector package.

## Categories

Connectors are organized by category:

| Category    | Description                | Built-in                    |
| ----------- | -------------------------- | --------------------------- |
| `file`      | Local filesystem content   | FileConnector               |
| `code`      | Source code repositories   | GitConnector, ForgeConnector |
| `cms`       | Content management systems | WordPressConnector          |
| `design`    | Design tools               | FigmaConnector              |
| `marketing` | Marketing platforms        | HubSpotConnector            |
| `analytics` | Product analytics          | PostHogConnector (locale demand; carries no content) |

## Built-in Connectors

### FileConnector

Wraps the `FormatRegistry` to read/write content files. Single-tenant hosts only; see [Connector Registry](#connector-registry):

```go
config := map[string]string{
    "path":   "/path/to/content",
    "format": "json",  // Optional: auto-detected from extensions
}
```

### GitConnector

Clone/pull repositories and discover resource files via glob patterns:

```go
config := map[string]string{
    "url":     "https://github.com/org/repo.git",
    "branch":  "main",
    "pattern": "src/locales/**/*.json",
}
```

### ForgeConnector

Wraps the git connector for GitHub/GitLab repositories and adds branch-and-pull-request delivery: a push webhook triggers the server, and results come back as one pull/merge request that every delivery updates in place. See [GitHub / GitLab connector](/server/connectors/github) for configuration.

### WordPressConnector

REST API integration for WordPress posts. Use a WordPress [application password](https://wordpress.org/documentation/article/application-passwords/), not the account login password (see the [WordPress connector guide](/server/connectors/wordpress)):

```go
config := map[string]string{
    "url":      "https://example.com",
    "username": "editor",
    "password": "xxxx xxxx xxxx xxxx xxxx xxxx", // application password
}
```

### FigmaConnector

REST API for Figma text nodes with DisplayHints from bounding boxes:

```go
config := map[string]string{
    "token":    "figma-personal-access-token",
    "file_key": "abc123def456",
}
```

### HubSpotConnector

REST API for HubSpot CMS pages:

```go
config := map[string]string{
    "api_key": "hubspot-api-key",
}
```

## Connector Registry

The registry lives in `bowrain/core/connector`; the concrete implementations and their registration helpers live in `bowrain/connector`.

Which types a registry offers is a security boundary, not a capability list, so there is no "register everything" helper; each host names the surface it is:

| Helper             | Registers                                     | Used by                     |
| ------------------ | --------------------------------------------- | --------------------------- |
| `RegisterServer`   | forge, wordpress, figma, hubspot              | Bowrain server, ingest worker |
| `RegisterForgeApp` | the forge connector in GitHub App mode        | Bowrain server, when a GitHub App is configured |
| `RegisterLocal`    | file, git, forge, wordpress, figma, hubspot   | single-tenant hosts          |
| `RegisterRemote`   | wordpress, figma, hubspot                     | desktop app                 |

A connector is built from a config map, and on a multi-tenant server that map arrives over the workspace connectors API from anyone holding `PermManageConnectors`. The `file` and `git` connectors take a filesystem path from that config, so `RegisterServer` omits them: a tenant must not be able to name a path on the host the server runs on. Server-side work that genuinely needs a local checkout (the context scan's repository harvest, for example) constructs its connector directly in Go with a path the server chose.

```go
reg := connector.NewRegistry()
bowrainconn.RegisterServer(reg, formatReg) // forge, wordpress, figma, hubspot

// Create a connector instance
c, err := reg.NewConnector("wordpress", config)

// List available types
types := reg.List()
```

Every remote connector's HTTP client is built from a `bowrain/safehttp` policy, because a base URL in a connector config is tenant input aimed at the network the server sits on. The policy resolves and re-checks the host at connect time, disables environment proxies, re-applies itself to every redirect hop, and refuses loopback, private, link-local, CGNAT, multicast, and unspecified addresses. A new connector that dials a configured host must use it; `core/httputil` is about TLS and retries, and is not a substitute.

At the API level, connector instances are workspace-scoped: they are created with `POST /api/v1/{workspace}/connectors` and addressed under that workspace. See [Connectors](/server/connectors) for the setup guides.

## Implementing a Custom Connector

1. Create a type implementing `connector.IntegrationConnector`
2. Register a factory function on the registry, with the connector's category:

```go
reg.Register("my-connector", connector.CategoryCMS,
    func(config map[string]string) (connector.IntegrationConnector, error) {
        return &MyConnector{config: config}, nil
    })
```
