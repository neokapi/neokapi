---
title: Connectors
sidebar_position: 12
---

# Connector System

Connectors provide bidirectional sync between neokapi and external content sources. They pull content into the Content Store and push translations back. This page covers the Go interfaces; for configuring connectors on a server, see [Connectors](/server/connectors).

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

    // Publish sends translated content FROM Bowrain TO the external system.
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

    // Pull retrieves translated content FROM Bowrain TO the source system.
    Pull(ctx context.Context, opts PullOptions) (*PullResult, error)
}
```

`IntegrationConnector` uses fetch/publish terminology from Bowrain's perspective; `SourceConnector` uses push/pull terminology from the source system's perspective. See the [Connector Interfaces](/notes/connector-interfaces) note for the full option and result types.

## Categories

Connectors are organized by category:

| Category    | Description                | Built-in                    |
| ----------- | -------------------------- | --------------------------- |
| `file`      | Local filesystem content   | FileConnector               |
| `code`      | Source code repositories   | GitConnector, ForgeConnector |
| `cms`       | Content management systems | WordPressConnector          |
| `design`    | Design tools               | FigmaConnector              |
| `marketing` | Marketing platforms        | HubSpotConnector            |

## Built-in Connectors

### FileConnector

Wraps the `FormatRegistry` to read/write content files:

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

Wraps the git connector for GitHub/GitLab repositories and adds branch-and-pull-request delivery: a push webhook triggers the server, and translations come back as one pull/merge request that every delivery updates in place. See [GitHub / GitLab connector](/server/connectors/github) for configuration.

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

The registry lives in `bowrain/core/connector`; the concrete implementations and their registration helpers live in `bowrain/connector`. `RegisterAll` registers every built-in type (the server/worker surface); `RegisterRemote` registers only the remote/CMS types (the desktop surface — local-filesystem sourcing is a server-side concern):

```go
reg := connector.NewRegistry()
bowrainconn.RegisterAll(reg, formatReg) // file, forge, git, wordpress, figma, hubspot

// Create a connector instance
c, err := reg.NewConnector("file", config)

// List available types
types := reg.List()
```

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
