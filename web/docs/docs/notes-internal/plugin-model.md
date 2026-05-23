---
id: plugin-model
title: "Note: Plugin model — the in-process registry contract"
sidebar_position: 50
---

# Plugin model — the in-process registry contract

This implementation note covers the **in-process registry mechanism** a plugin binary uses to wire its features into the shared `cli.App`. Plugin packages are blank-imported by a plugin binary's `main`; their `init()` functions register features against process-global registries via direct function calls — no gRPC, no dynamic loading inside the binary.

This is one half of the plugin story: how the Go code _inside_ a plugin binary is composed. How `kapi` then **discovers** that binary on disk and **dispatches** to it at runtime (the `manifest.json` model and the A/B/C transport modes) lives in [AD-007: Plugin System](../architecture/007-plugin-system). The `kapi` binary itself links no vendor plugins; the registries below populate inside the plugin binary — for bowrain, that's `kapi-bowrain` (built from `bowrain/cli/cmd/kapi-bowrain/`).

This note is the reference for: how the registries work, how to write the Go side of a new plugin, and when to use the schema-only registry vs. the heavier ones.

## When to use which registry

The framework and shared CLI module expose four registries. A plugin can use any subset.

| Registry                         | Lives in          | Plugin extends with                                              |
| -------------------------------- | ----------------- | ---------------------------------------------------------------- |
| `core/project.RegisterExtension` | framework         | Recipe schema (typed YAML decoders for keys under `Extras`)      |
| `cli.RegisterCommandFactory`     | shared CLI module | Top-level cobra subcommands (`push`, `pull`, …)                  |
| `cli.RegisterAppInitializer`     | shared CLI module | Mutates `*cli.App` after construction (sets fields, wires hooks) |
| `cli.RegisterMCPToolFactory`     | shared CLI module | MCP tools served by the shared `mcp` subcommand                  |

A plugin that only declares schema can be a tiny module with just one decoder file and no CLI deps — that's how `bowrain/plugin/schema/` is structured. A plugin that adds full UX (commands, MCP tools, source connector) layers more on top, but the schema part can ship independently.

## The schema registry

```go
type ExtensionDecoder interface {
    Decode(node yaml.Node) error
}

type Scope int  // ScopeProject | ScopeDefaults | ScopeCollection | ScopeItem

type Extension struct {
    Name    string
    Scope   Scope
    Group   string
    Decoder ExtensionDecoder
}

func RegisterExtension(ext Extension)
func RegisterExtensionGroup(group string, exts []Extension)
func HasExtensionGroup(group string) bool
```

`Scope` decides which `Extras` map a key is matched against:

- `ScopeProject` — top-level keys on the recipe (e.g. `server:`, `hooks:`)
- `ScopeDefaults` — keys nested under `defaults:`
- `ScopeCollection` — keys on a `ContentCollection` (named-collection wrapper or bare entry)
- `ScopeItem` — keys on a `ContentItem` (per-item fields)

Re-registering the same `(Scope, Name)` pair panics — competing init functions are almost always a bug. Pure-name matches across scopes don't conflict (`collection` at `ScopeItem` and `ScopeDefaults` are distinct).

`Group` lets a recipe declare `requires: [bowrain]` and have validation fail when no extension under that group has been registered. Use this when a recipe is meaningless without the platform's behavior — `kapi push`/`kapi pull` won't work without the bowrain plugin installed, so a `.kapi` recipe with `server:` typically declares `requires: [bowrain]`.

`HasExtensionGroup` is consulted by `KapiProject.Validate()` to enforce `requires:`. Plugins typically don't need to call it directly.

### Forward compatibility

A recipe with an unknown extension key (no decoder registered for `(scope, key)`) loads successfully; the value sits in `Extras` and round-trips through `Save`. This is intentional — a kapi binary built without the bowrain plugin can still load a bowrain recipe (it just can't validate or act on it). The `requires:` declaration is the recipe author's opt-in for "fail loudly if the extension is missing."

## The CLI registries

```go
type CommandFactory func(parent *cobra.Command, app *App)
type AppInitializer func(app *App)
type MCPToolFactory func(server *mcp.Server, app *App)

func RegisterCommandFactory(f CommandFactory)
func RegisterAppInitializer(f AppInitializer)
func RegisterMCPToolFactory(f MCPToolFactory)
```

`CommandFactory` is invoked by the plugin binary once after the built-in command tree is constructed. For bowrain that happens inside `kapi-bowrain`, which builds its `command` subtree from the registered factories:

```go
// bowrain/cli/cmd/kapi-bowrain/main.go
cli.ApplyCommandFactories(cmd, app)
```

Plugins typically register one factory per command file, e.g. `bowrain/plugin/commands/push.go`:

```go
func init() {
    cli.RegisterCommandFactory(func(parent *cobra.Command, a *cli.App) {
        parent.AddCommand(buildPushCmd(a))
    })
}
```

`AppInitializer` is invoked from the host's `PersistentPreRun` after `app.Init()`. Plugins use it to install fields like `app.FallbackRunE` (project flow resolution) that need to see the fully-initialized `App`.

`MCPToolFactory` is invoked by the shared `app.NewMCPCmd("kapi")` when the `mcp` subcommand starts. Each registered factory is given the `*mcp.Server` and the `*cli.App` and adds its tools.

## Writing a new plugin: a worked example

Suppose you want to add a `gitlab` source connector that pushes content to a GitLab repo as locale-suffixed branches. The plugin:

```
gitlab-plugin/
├── go.mod
├── plugin.go         // anchor: imports the sub-packages
├── schema/
│   ├── extension.go  // init() registers RegisterExtensionGroup("gitlab", ...)
│   └── server.go     // GitLabServer { URL, Token, Branch, ... } + Validate
└── commands/
    └── push.go       // init() registers a "gitlab-push" command factory
```

### `gitlab-plugin/schema/server.go`

```go
package schema

import (
    "errors"
    "net/url"
    "gopkg.in/yaml.v3"
)

type GitLabServer struct {
    URL    string `yaml:"url"`
    Branch string `yaml:"branch,omitempty"`
}

func (s *GitLabServer) Validate() error {
    if s == nil { return nil }
    u, err := url.Parse(s.URL)
    if err != nil || u.Scheme == "" {
        return errors.New("url is required and must be a valid URL")
    }
    return nil
}
```

### `gitlab-plugin/schema/extension.go`

```go
package schema

import (
    coreproj "github.com/neokapi/neokapi/core/project"
    "gopkg.in/yaml.v3"
)

const Group = "gitlab"

func init() {
    coreproj.RegisterExtensionGroup(Group, []coreproj.Extension{
        {
            Name:  "gitlab",
            Scope: coreproj.ScopeProject,
            Decoder: coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
                var s GitLabServer
                if err := n.Decode(&s); err != nil { return err }
                return s.Validate()
            }),
        },
    })
}
```

### Recipe

```yaml
version: v1
name: my-app
requires: [gitlab]
gitlab:
  url: https://gitlab.example.com/team/project
  branch: localization
content:
  - path: src/locales/**/*.json
    format: json
```

When loaded by a binary that links `gitlab-plugin/schema` (the plugin binary, or a desktop app that blank-imports the schema for validation), the recipe validates. When loaded by a `kapi` with no gitlab plugin installed, the recipe fails at parse time:

```
recipe requires extension group "gitlab" but no matching extension is registered
(this binary was not built with the "gitlab" extension linked in)
```

### Adding a command

```go
// gitlab-plugin/commands/push.go
package commands

import (
    "github.com/neokapi/neokapi/cli"
    "github.com/spf13/cobra"
)

func init() {
    cli.RegisterCommandFactory(func(parent *cobra.Command, a *cli.App) {
        parent.AddCommand(buildGitLabPushCmd(a))
    })
}

func buildGitLabPushCmd(a *cli.App) *cobra.Command {
    return &cobra.Command{
        Use:   "gitlab-push",
        Short: "Push translations to a GitLab project",
        RunE: func(cmd *cobra.Command, args []string) error {
            // ... implementation that loads the recipe and pushes
            return nil
        },
    }
}
```

To ship the plugin, build it as its own binary that blank-imports the anchor, alongside a `manifest.json` declaring the `gitlab-push` command (and any formats/tools/connectors). `kapi` discovers the binary and dispatches to it:

```go
// gitlab-plugin/cmd/kapi-gitlab/main.go
package main

import _ "example.com/gitlab-plugin"
```

## Packaging and the license boundary

A plugin ships as its own binary plus a `manifest.json`, installed into a kapi plugin directory rather than linked into `kapi`. Because the plugin runs as a separate process, its license is independent of `kapi`'s: the `kapi` binary stays Apache-2.0 and links no vendor-plugin code, while the bowrain plugin binary (`kapi-bowrain`) carries the bowrain packages. There is no `-tags pure` / `kapi-pure` split — `kapi` is always plugin-free, and bowrain is something you install into it.

See [AD-007: Plugin System](../architecture/007-plugin-system) for the manifest schema, discovery precedence, install paths (`kapi plugins install <name>`, Homebrew), and the A/B/C transport modes.

## Initialization order

Within one Go binary:

1. All `init()` functions run during binary startup, in package import order. Cross-package init order is undefined — the registries are append-only, so order shouldn't matter for correctness.
2. The binary's `main()` constructs `*cli.App`, calls `app.InitRegistries()` (built-in formats/tools).
3. Cobra's `init()` builds the command tree; the host calls `cli.ApplyCommandFactories(root, app)` after.
4. `cobra.Execute()` runs. `PersistentPreRun` calls `app.Init()` then `cli.ApplyAppInitializers(app)`.
5. The chosen subcommand's `RunE` runs.

`AppInitializer` runs _after_ `Init`, so plugins can rely on the registries (FormatReg, ToolReg, Credentials, Config) being populated. `CommandFactory` runs at `init()`-time of the host's main package — registries must be ready by then, which is why `InitRegistries` runs at cobra init too.

## Goroutines and async work

Plugins that need their own background goroutines can spawn them inside their commands or connectors — the registries don't impose any model. The bowrain source connector, for example, runs concurrent block uploads via `errgroup` inside `BowrainSourceConnector.Push`. Nothing in the plugin contract requires the plugin to expose its async work to the host.

## Testing plugins

Unit-test the schema decoders directly:

```go
func TestGitLabServerDecodes(t *testing.T) {
    var node yaml.Node
    require.NoError(t, yaml.Unmarshal([]byte(`url: https://gitlab.example.com/team/proj`), &node))
    var s GitLabServer
    require.NoError(t, node.Decode(&s))
    require.NoError(t, s.Validate())
}
```

Tests that mutate the global registries should call `coreproj.ResetExtensionsForTest()` and `cli.ResetPluginRegistriesForTest()` in setup so they don't leak state across tests.

## Relationship to runtime dispatch

The registries on this page are purely in-process: they assemble the command tree, MCP tools, and recipe schema _inside_ a single plugin binary. They say nothing about how that binary is found or invoked — that is the runtime concern owned by [AD-007: Plugin System](../architecture/007-plugin-system): `manifest.json` discovery and the A/B/C transport modes (one-shot command exec, an MCP-over-stdio session, and the gRPC daemon used by formats/tools/source connectors, including the Java Okapi bridge).

In short: this note is the contract for the Go _inside_ a plugin; AD-007 is the contract for the process _around_ it.
