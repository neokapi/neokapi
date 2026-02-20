---
sidebar_position: 2
title: "Connector Interfaces"
---
# Connector Interfaces

This note provides implementation details for [AD-005](/docs/ad/005-connector-system).

## IntegrationConnector (Server-Side)

Server-side connectors that Bowrain reaches into external systems. Uses Fetch/Publish terminology (from Bowrain's perspective):

```go
type IntegrationConnector interface {
    ConnectorBase

    // Fetch retrieves source content FROM the external system INTO Bowrain.
    Fetch(ctx context.Context, opts FetchOptions) ([]*ContentItem, error)

    // Publish sends translated content FROM Bowrain TO the external system.
    Publish(ctx context.Context, items []*ContentItem, opts PublishOptions) error
}
```

## SourceConnector (Client-Side)

Client-side connectors that push to and pull from Bowrain. Uses Push/Pull terminology (from the source system's perspective):

```go
type SourceConnector interface {
    ConnectorBase

    // Push sends source content FROM the source system TO Bowrain.
    Push(ctx context.Context, opts PushOptions) (*PushResult, error)

    // Pull retrieves translated content FROM Bowrain TO the source system.
    Pull(ctx context.Context, opts PullOptions) (*PullResult, error)
}
```

The primary implementation is `KapiSourceConnector` -- the file connector that syncs `.kapi/` projects with Bowrain Server via REST API.

## ConnectorBase (Shared)

Both interfaces share common identity and lifecycle methods:

```go
type ConnectorBase interface {
    ID() string
    Name() string
    Category() Category
    Status(ctx context.Context) (*SyncStatus, error)
    Configure(config map[string]string) error
    Close() error
}
```

## Connector Categories

```go
type ConnectorCategory string

const (
    CategoryCMS       ConnectorCategory = "cms"
    CategoryDesign    ConnectorCategory = "design"
    CategoryCode      ConnectorCategory = "code"
    CategoryMarketing ConnectorCategory = "marketing"
    CategoryFile      ConnectorCategory = "file"  // Kapi's category
    CategoryTMS       ConnectorCategory = "tms"   // External TMS integrations
)
```

## PullOptions and PushOptions

```go
type PullOptions struct {
    Items   []string          // content item IDs to pull (empty = all)
    Locales []model.LocaleID  // source locales to pull
    Config  map[string]any    // connector-specific configuration
}

type PushOptions struct {
    Items   []string          // content item IDs to push (empty = all)
    Locales []model.LocaleID  // target locales to push
    Message string            // commit message / changelog
    Config  map[string]any
}
```

## SyncStatus

```go
type SyncStatus struct {
    ConnectorID   string
    LastSynced    time.Time
    ItemsChanged  int       // items modified since last pull
    ItemsNew      int       // new items in source system
    ItemsDeleted  int       // items removed from source system
    PendingPush   int       // translated items awaiting push
}
```

`Sync()` provides a lightweight status check without performing a full pull. Bowrain uses this to show sync indicators in the connector management panel.

## ContentItem

Server-side connectors expose browsable content:

```go
type ContentItem struct {
    ID           string
    ExternalID   string            // ID in the source system
    Name         string
    Path         string            // hierarchical path in source system
    ContentType  string            // "page", "entry", "text-layer", "file"
    Locale       model.LocaleID
    LastModified time.Time
    Metadata     map[string]string // connector-specific metadata
}
```

`List()` returns available content items from the connected system. For a CMS connector, these are pages and entries. For a design connector, artboards and text layers. Bowrain displays content items as a browsable tree, allowing selective pull.

## ConnectorRegistry

Server-side connectors register into a `ConnectorRegistry`:

```go
type ConnectorRegistry struct {
    connectors map[string]ConnectorFactory
}

type ConnectorFactory func(config map[string]any) (Connector, error)

func (r *ConnectorRegistry) Register(id string, category ConnectorCategory, factory ConnectorFactory)
func (r *ConnectorRegistry) NewConnector(id string, config map[string]any) (Connector, error)
func (r *ConnectorRegistry) List() []ConnectorInfo
```

Built-in connectors register via `init()`. Plugin connectors register at runtime via gRPC discovery ([AD-007](/docs/ad/007-plugin-system)). Kapi does not interact with the ConnectorRegistry -- it is a file-based tool that syncs with the server via API.
