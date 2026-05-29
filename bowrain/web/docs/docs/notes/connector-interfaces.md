---
sidebar_position: 2
title: "Connector Interfaces"
---

# Connector Interfaces

This note provides implementation details for [AD-008](/architecture-decisions/008-connector-system).

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

One client-side implementation is `BowrainSourceConnector` -- the kapi connector that syncs `.kapi` projects (recipe + state dir) with Bowrain Server via REST API. Server-side connectors (CMS, design tools, git) implement the same interfaces.

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
type Category string

const (
    CategoryFile      Category = "file"
    CategoryCode      Category = "code"
    CategoryCMS       Category = "cms"
    CategoryDesign    Category = "design"
    CategoryMarketing Category = "marketing"
    CategoryTMS       Category = "tms"
)
```

## PushOptions and PullOptions

```go
type PushOptions struct {
    Paths  []string // Specific file paths to push (empty = all)
    Force  bool     // Push all blocks, ignoring sync cache
    DryRun bool     // Report what would be pushed without sending
}

type PullOptions struct {
    Locales []model.LocaleID // Target locales to pull (empty = all)
    Force   bool             // Overwrite local changes
    DryRun  bool             // Report what would be pulled without writing
}
```

## SyncStatus

```go
type SyncStatus struct {
    ConnectorID string
    LastSync    time.Time
    ItemCount   int
    FileCount   int // total local files scanned
    WordCount   int // total source words across all local blocks
    PendingPull int // Items changed externally since last pull
    PendingPush int // Items changed locally since last push
    Errors      []string
}
```

`Status()` provides a lightweight status check without performing a full pull. Bowrain uses this to show sync indicators in the connector management panel.

## ContentItem

Server-side connectors expose browsable content:

```go
type ContentItem struct {
    ID          string
    Name        string
    Path        string
    Format      string
    Locale      model.LocaleID
    Blocks      []*model.Block
    Metadata    map[string]string
    LastChanged time.Time
}
```

`List()` returns available content items from the connected system. For a CMS connector, these are pages and entries. For a design connector, artboards and text layers. Bowrain displays content items as a browsable tree, allowing selective pull.

## Registry

Server-side connectors register into a `Registry`:

```go
type Factory func(config map[string]string) (IntegrationConnector, error)

type Info struct {
    Name     string
    Category Category
}

type Registry struct {
    mu        sync.RWMutex
    factories map[string]Factory
    infos     map[string]Info
}

func (r *Registry) Register(name string, category Category, factory Factory)
func (r *Registry) NewConnector(name string, config map[string]string) (IntegrationConnector, error)
func (r *Registry) List() []Info
```

Built-in connectors register via `init()`. Plugin connectors register at runtime via gRPC discovery ([Framework AD-007](https://neokapi.github.io/web/neokapi/docs/architecture/007-plugin-system)). The bowrain CLI does not interact with the Registry -- it is a file-based tool that syncs with the server via API.
