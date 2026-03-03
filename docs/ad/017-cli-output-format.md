---
id: 017-cli-output-format
sidebar_position: 17
title: "AD-017: CLI Output Format Flags"
---
# AD-017: CLI Output Format Flags

## Context

The Kapi CLI outputs information in various formats across different commands. Currently, output formatting is inconsistent:

- Some commands (`flow run`, tool commands) support `--json` flags
- Most commands default to human-readable text with no format option
- CI/CD pipelines need machine-readable output for scripting
- The existing `formatOutput()` helper in `output.go` handles JSON vs table formatting but is only used by a few commands

Users and automation scripts need a consistent, predictable way to request structured output from any command.

## Decision

### Unified Output Format Flags

All Kapi commands that produce output will support consistent format flags:

```bash
# Flag variants (all equivalent)
kapi status --json                    # Short form for JSON
kapi status --text                    # Short form for text (default)
kapi status --output-format=json      # Explicit long form
kapi status --output-format=text      # Explicit long form
kapi status -o json                   # Short flag form
```

**Supported formats:**
- `text` — Human-readable output (default). Tables, formatted text, colors if terminal.
- `json` — Machine-readable JSON. Single JSON object or array per command.

### Flag Precedence

1. `--json` flag sets format to JSON (highest precedence, for convenience)
2. `--text` flag sets format to text  
3. `--output-format=<format>` or `-o <format>` explicit format selection
4. Default: `text`

### Implementation

**Shared output package** (`bowrain/cmd/kapi/output.go`):

```go
// OutputFormat represents the CLI output format
type OutputFormat string

const (
    FormatText OutputFormat = "text"
    FormatJSON OutputFormat = "json"
)

// OutputConfig holds output formatting state
type OutputConfig struct {
    Format OutputFormat
}

// Global output configuration (set by PersistentPreRun)
var outputCfg OutputConfig

// AddOutputFlags registers --json, --text, and --output-format flags on a command
func AddOutputFlags(cmd *cobra.Command) {
    cmd.PersistentFlags().Bool("json", false, "Output in JSON format")
    cmd.PersistentFlags().Bool("text", false, "Output in text format (default)")
    cmd.PersistentFlags().StringP("output-format", "o", "", "Output format: json, text")
}

// GetOutputFormat resolves the output format from command flags
func GetOutputFormat(cmd *cobra.Command) OutputFormat {
    if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
        return FormatJSON
    }
    if textFlag, _ := cmd.Flags().GetBool("text"); textFlag {
        return FormatText
    }
    if format, _ := cmd.Flags().GetString("output-format"); format != "" {
        return OutputFormat(format)
    }
    return FormatText
}

// Print outputs data in the configured format
func Print(cmd *cobra.Command, data any) error {
    format := GetOutputFormat(cmd)
    return PrintWithFormat(format, data)
}

// PrintWithFormat outputs data in the specified format
func PrintWithFormat(format OutputFormat, data any) error {
    switch format {
    case FormatJSON:
        return printJSON(data)
    default:
        return printText(data)
    }
}
```

**Text formatting interface:**

```go
// TextFormatter is implemented by types that can render themselves as text
type TextFormatter interface {
    FormatText(w io.Writer) error
}

func printText(data any) error {
    if tf, ok := data.(TextFormatter); ok {
        return tf.FormatText(os.Stdout)
    }
    // Fallback: print as formatted JSON for types without text formatter
    return printJSON(data)
}
```

### Command Implementation Pattern

Commands use the shared output helpers:

```go
var statusCmd = &cobra.Command{
    Use:   "status",
    Short: "Show sync status",
    RunE: func(cmd *cobra.Command, args []string) error {
        status, err := getStatus()
        if err != nil {
            return err
        }
        return output.Print(cmd, status)
    },
}

func init() {
    output.AddOutputFlags(statusCmd)
}
```

### JSON Output Structure

Each command defines its JSON output structure. The structure should be:

1. **Consistent** — Same command always produces same structure
2. **Complete** — All relevant data included, not just what text mode shows
3. **Typed** — Use appropriate JSON types (numbers, booleans, arrays)

Example JSON outputs:

```json
// kapi status --json
{
  "project": {
    "path": "/path/to/project",
    "server": "https://bowrain.example.com",
    "project_id": "my-project"
  },
  "local_changes": [
    {"path": "src/locales/en.json", "status": "modified", "blocks": 3}
  ],
  "remote_changes": [
    {"path": "src/locales/fr.json", "status": "updated", "blocks": 1}
  ],
  "conflicts": [],
  "last_sync": "2026-02-16T10:30:00Z"
}

// kapi formats list --json
{
  "formats": [
    {
      "name": "okf_json",
      "display_name": "JSON Filter",
      "extensions": [".json"],
      "mime_types": ["application/json"],
      "source": "plugin:okapi-bridge"
    }
  ]
}

// kapi plugins list --json
{
  "plugins": [
    {
      "name": "okapi-bridge",
      "version": "1.5.0",
      "status": "running",
      "formats": 46,
      "path": "/home/user/.config/kapi/plugins/okapi-bridge"
    }
  ]
}
```

### Exit Codes

Exit codes are consistent regardless of output format:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (command failed) |
| 2 | Conflict (sync conflict detected) |

Errors in JSON mode are output as:

```json
{
  "error": "failed to connect to server",
  "code": "connection_failed"
}
```

### Commands to Update

All commands producing output should be updated:

| Command | Current Output | JSON Structure |
|---------|---------------|----------------|
| `status` | Text table | Project state, changes, conflicts |
| `diff` | Text diff | Array of diffs per file |
| `formats list` | Text table | Array of format objects |
| `tools list` | Text table | Array of tool objects |
| `plugins list` | Text table | Array of plugin objects |
| `plugins search` | Text table | Array of search results |
| `auth status` | Text lines | Auth state object |
| `flow list` | Text table | Array of flow definitions |
| `flow run` | Progress + result | Flow execution result |
| `termbase stats` | Text table | Statistics object |
| `termbase lookup` | Text entries | Array of term matches |
| `version` | Text line | Version object |

### Backward Compatibility

- Commands that already support `--json` continue to work
- Default output remains text (human-readable)
- New flags (`--text`, `--output-format`) are additive

## Alternatives Considered

- **`--format` flag only**: Less convenient than `--json` shorthand for the common case.

- **Structured logging**: Over-engineered for CLI output; logging is for debug, output is for results.

- **YAML output**: Less common in CLI tools, JSON is more universal for scripting.

- **Per-command flags**: Current state; inconsistent and harder to document.

## Consequences

- **Consistent UX**: Users learn one pattern for all commands.

- **CI/CD friendly**: `--json` flag enables easy scripting with `jq`, parsing in any language.

- **Documentation**: Single section documents output format for all commands.

- **Implementation cost**: Each command needs JSON structure defined and `TextFormatter` implemented.

- **Breaking changes**: None; existing commands continue to work, new flags are additive.

- **Type safety**: Commands must define concrete types for their output (no `map[string]any`).

- **Error handling**: Errors in JSON mode return structured error objects, not stderr text.
