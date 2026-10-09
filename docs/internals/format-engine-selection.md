# Format engine selection

How kapi decides which engine reads a file whose extension more than one
format claims, where that decision is made, and where a person steers it.

This is a working note for contributors. The user-facing description is the
[Format engines](../../web/docs/reference/project-file.mdx) section of the
recipe reference, and the architecture is in
[E-02](../../web/docs/contribute/architecture/engine/e-02-format-system.md).

## Engines

An engine is a provider of formats. `native` is the set of formats kapi ships;
every plugin is one engine, named as the registry records its source
(`okapi-bridge`, `kapi-av`). A format belongs to exactly one engine
(`FormatInfo.Source`). Native and plugin formats for the same logical format
carry different ids (`json` and `okf_json`, `xml` and `okf_xml`) and collide on
the extension, never on the name. A plugin format registered under a built-in
format's own name is a different case: it replaces the built-in's reader and
writer (`host/pluginhost/format_factory.go`).

## The resolver

`registry.FormatRegistry.Resolve(path, DetectOptions)` is the one resolver
(`core/registry/engine.go`). `Detect` is `Resolve` without the trace. Every
entry point that detects a format for a named file calls one of the two: the
tool runner (`host/toolrun.go`), the flow runners for one file and for a batch
(`host/flow.go`, `core/flow/filerunner.go`), the toolbox (`host/toolbox.go`),
the change service (`host/changes.go`), project content resolution
(`core/project/context.go`) and the desktop backend. The deprecated
`DetectByExtension` family are wrappers over `Detect`.

Resolution has two stages.

1. **Engine.** The formats claiming the extension are grouped by engine, and
   the engine order is walked until an engine with a claimant is found.
2. **Format within the engine.** When the engine has one claimant, that is the
   format. When it has several and the file can be read, the file head is
   sniffed and the result is taken if it names one of the engine's claimants
   (`.xliff` 1.x or 2.x, `.xml` as Android, RESX or plain XML). Otherwise the
   highest priority wins and a tie falls to name order.

A priority never crosses an engine. `format.DefaultPluginPriority` (25) sits
below `format.DefaultBuiltInPriority` (50) so that the detector's own
priority-only ranking (MIME types, content sniffing) agrees with the engine
order for the callers that ask it directly.

The trace (`registry.Resolution`) names the format, its engine, the rule that
chose the engine, whether content decided within it, the engine order consulted
and every claimant. `kapi formats explain` is the planned surface for it
(#640, phase 3).

## The engine order

First to last. Each line names the option the stage reads and the surface that
fills it.

| Stage | `DetectOptions` or registry state | Filled by |
| --- | --- | --- |
| Per-format pin | `FormatEngines[ext or format id]` | recipe `defaults.formats.<name>.engine`; `WriterFormatFor` pins the reader's engine |
| Host override | `SetEngineOverride` | `--engine` (`host/engine.go`, `App.ApplyEngineSelection`) |
| Detection preference | `Engine` | recipe `defaults.engine` (`ProjectContext.DetectOptions`) |
| Host default | `SetDefaultEngine` | config `formats.engine` (`host/config`, `KeyFormatsEngine`) |
| Native | always | |
| Ranked plugins | `EngineOrder` | recipe `plugins.<name>.format_priority`, highest first, then name (`ProjectContext.EngineOrder`) |
| Everything else | | the remaining sources in name order |

`native` and `built-in` both name the built-in formats (`registry.NormalizeEngine`).

`AllowedSources` bounds the whole order: a project detects within
`["built-in"] + plugins:`, so a plugin installed on the machine but not declared
serves none of the project's files, and an engine preference for it selects
nothing. The recipe's validation (`KapiProject.validateEngines`) rejects
`defaults.engine` and `defaults.formats.<name>.engine` values that are neither
`native` nor a declared plugin. `--engine` is checked against the installed
engines once the plugin host has registered its formats
(`FormatRegistry.CheckEngine`).

## Precedence, as a person sees it

1. A format named outright: `--format`, `--map '<glob>=<format>'`, a content
   item's `format:`. Read by that format, whatever the engine order says.
2. A per-format engine pin: `defaults.formats.<name>.engine`.
3. The engine-wide preference: `--engine`, then `defaults.engine`, then
   `formats.engine`.
4. Native first, then the plugins.

## Reader and writer

`WriterFormatFor(readerFormat, outputPath)` resolves the writer for an output
path whose extension differs from the input's. It pins the reader's engine
first (`FormatEngines[ext] = source of readerFormat`) and takes only formats
with a writer (`DetectOptions.Writer`), so `-f json` writing `.xml` gets the
built-in XML writer and `-f okf_json` gets `okf_xml`. A same-extension round
trip keeps the reader's format without re-detecting.

## The lazy loader

`SetOnMiss` registers a loader the registry calls once when nothing claims an
extension or a reader or writer name is unknown. A built-in format satisfying
the request never reaches it: `Resolve` asks the loader only when no format at
all claims the extension and no source restriction is in force.

## Commands

Every command that takes `--format`/`-f` for its input takes `--engine`
(`cli/engine_flag_test.go` walks the command tree and checks). A tool whose own
parameters are named `engine` (entity-extract, translate) keeps its parameter
under `kapi exec <tool>`; the format engine then comes from the recipe or the
config (`App.AddInputFlagsWithoutEngine`).

## Tests

- `core/registry/engine_test.go`: the order, the pin, scoping, the onMiss
  guard, the writer pin.
- `core/project/context_test.go`: `defaults.engine`,
  `defaults.formats.<name>.engine`, `format_priority` ranking.
- `host/engine_test.go`: the same file resolves to the same engine from
  `kapi exec`, a single-file flow run and a project run, with an in-process
  plugin format registered beside the built-in JSON.
- `core/format/detect_test.go`: the detector's own ranking is native-first.
