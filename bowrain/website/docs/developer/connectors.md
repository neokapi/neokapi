---
title: Connectors
sidebar_position: 12
---

# Connector System

Connectors provide bidirectional sync between neokapi and external content sources. They pull content into the Content Store and push translations back.

## Connector Interface

```go
type Connector interface {
    ID() string
    Name() string
    Category() Category

    Pull(ctx context.Context, opts PullOptions) ([]*ContentItem, error)
    Push(ctx context.Context, items []*ContentItem, opts PushOptions) error
    List(ctx context.Context) ([]*ContentItem, error)
    Sync(ctx context.Context) (*SyncStatus, error)

    Configure(config map[string]string) error
    Close() error
}
```

## Categories

Connectors are organized by category:

| Category    | Description                | Built-in           |
| ----------- | -------------------------- | ------------------ |
| `File`      | Local filesystem content   | FileConnector      |
| `Code`      | Source code repositories   | GitConnector       |
| `CMS`       | Content management systems | WordPressConnector |
| `Design`    | Design tools               | FigmaConnector     |
| `Marketing` | Marketing platforms        | HubSpotConnector   |

## Built-in Connectors

### FileConnector

Wraps the `FormatRegistry` to read/write localization files:

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

### WordPressConnector

REST API integration for WordPress posts and pages:

```go
config := map[string]string{
    "url":      "https://example.com",
    "username": "admin",
    "password": "app-password",
}
```

### FigmaConnector

REST API for Figma text nodes with DisplayHints from bounding boxes:

```go
config := map[string]string{
    "token":   "figma-personal-access-token",
    "file_id": "abc123def456",
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

Connectors use a factory pattern for registration:

```go
reg := connector.NewRegistry()
connector.RegisterAll(reg, formatReg)

// Create a connector instance
c, err := reg.NewConnector("file", config)

// List available types
types := reg.List()
```

## Implementing a Custom Connector

1. Create a type implementing `connector.Connector`
2. Register a factory function in the registry:

```go
func init() {
    connector.RegisterFactory("my-connector", func(config map[string]string) (connector.Connector, error) {
        return &MyConnector{config: config}, nil
    })
}
```
