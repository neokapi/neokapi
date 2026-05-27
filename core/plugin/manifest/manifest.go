// Package manifest defines the per-plugin sidecar manifest.json format
// used by the unified plugin model (issue #438).
//
// Every plugin directory contains a manifest.json declaring everything
// the plugin provides — commands, MCP tools, format readers/writers,
// flow tools, source connectors, and recipe schema extensions.
//
// Kapi reads all manifests at startup and builds dispatch tables from
// them; there is no name fall-through or "command not found" magic.
//
// This is distinct from the registry index entry in
// cli/pluginhost/registry — the registry index points at downloadable
// plugin tarballs, while a manifest.json sits inside an installed plugin.
package manifest

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CurrentVersion is the manifest schema version this binary supports.
// Bumped on breaking schema changes; kapi rejects manifests whose
// manifest_version is not in SupportedVersions.
const CurrentVersion = "1"

// SupportedVersions is the set of manifest_version strings this binary
// can parse. Add to this when introducing a backwards-compatible bump.
var SupportedVersions = []string{"1"}

//go:embed schema.json
var schemaJSON []byte

// SchemaJSON returns the embedded JSON Schema describing the manifest
// format. Plugin authors can validate their manifest.json against this.
func SchemaJSON() []byte {
	out := make([]byte, len(schemaJSON))
	copy(out, schemaJSON)
	return out
}

// Manifest is the root type for a plugin's manifest.json.
type Manifest struct {
	// ManifestVersion is the schema version of this manifest. Currently
	// always "1".
	ManifestVersion string `json:"manifest_version"`

	// Plugin is the plugin's unique name (e.g., "bowrain", "okapi-bridge").
	// Used as the install dir name and as the key in dispatch tables.
	Plugin string `json:"plugin"`

	// Version is the plugin's semver (e.g., "1.4.0").
	Version string `json:"version"`

	// Description is a one-line human-readable description.
	Description string `json:"description,omitempty"`

	// Homepage is the plugin's documentation URL.
	Homepage string `json:"homepage,omitempty"`

	// Author is the plugin's author/maintainer (free-form, typically
	// "Name <email>").
	Author string `json:"author,omitempty"`

	// License is the SPDX license identifier (e.g., "Apache-2.0").
	License string `json:"license,omitempty"`

	// Binary is the executable name within the plugin dir (e.g.,
	// "kapi-bowrain"). Resolved relative to the manifest's directory.
	Binary string `json:"binary"`

	// MinKapiVersion is the minimum kapi version this plugin supports.
	// kapi refuses to register plugins whose constraint excludes the
	// running kapi binary.
	MinKapiVersion string `json:"min_kapi_version,omitempty"`

	// Group is a logical grouping for related plugins (e.g., "bowrain"
	// or "neokapi"). Currently informational; may be used for UI grouping.
	Group string `json:"group,omitempty"`

	// Capabilities lists everything the plugin provides. Each capability
	// section is independent — a plugin may declare commands but no
	// formats, or formats but no commands, etc.
	Capabilities Capabilities `json:"capabilities"`

	// Daemon is present only for plugins that use Mode C (daemon over
	// local socket). Mode A (commands) and Mode B (MCP) plugins omit
	// this field.
	Daemon *DaemonConfig `json:"daemon,omitempty"`
}

// Capabilities groups every capability section a plugin can declare.
//
// Each section is a slice; a plugin populates only the sections it
// implements. Empty sections are omitted.
type Capabilities struct {
	// Commands declares top-level CLI commands the plugin provides
	// (Mode A — one-shot subprocess).
	Commands []Command `json:"commands,omitempty"`

	// MCPTools declares MCP tools the plugin exposes (Mode B —
	// session subprocess speaking MCP-over-stdio).
	MCPTools []MCPTool `json:"mcp_tools,omitempty"`

	// Formats declares format readers/writers the plugin provides
	// (Mode C — daemon over local socket).
	Formats []Format `json:"formats,omitempty"`

	// Tools declares flow tools the plugin provides (Mode C).
	Tools []Tool `json:"tools,omitempty"`

	// SourceConnectors declares source connectors the plugin provides
	// (Mode C). A connector typically implements project synchronization
	// (push/pull) against an external system.
	SourceConnectors []SourceConnector `json:"source_connectors,omitempty"`

	// SchemaExtensions declares recipe schema keys the plugin owns.
	// Each entry pairs a YAML key (at a given scope) with a JSON Schema
	// file shipped in the plugin dir; kapi validates Extras against the
	// schema at recipe parse time.
	SchemaExtensions []SchemaExtension `json:"schema_extensions,omitempty"`

	// CommandContributions declares hooks a plugin adds to a built-in kapi
	// command (e.g. extending `kapi init` to connect the project to a
	// platform). Unlike Commands (which a plugin fully owns), a contribution
	// augments a command kapi already provides: kapi registers the declared
	// flags on the built-in command and, after it runs, dispatches the
	// plugin's handler when the contribution is engaged.
	CommandContributions []CommandContribution `json:"command_contributions,omitempty"`
}

// CommandContribution declares a plugin hook into a built-in kapi command.
//
// kapi adds the contribution's Flags to the named built-in command and, when
// the contribution is engaged (EngageWhen flag set, or — if EngageWhen is empty
// — any contributed flag set), dispatches the plugin handler after the built-in
// command's own action runs. The handler is dispatched as a Mode-A subcommand
// (`<binary> command <Handler> <engaged flags>`) in the command's project
// directory (KAPI_PROJECT_DIR). Handlers must be idempotent.
type CommandContribution struct {
	// Command is the built-in command this contribution extends (e.g. "init").
	Command string `json:"command"`

	// Handler is the plugin subcommand kapi dispatches (e.g. "init-connect").
	Handler string `json:"handler"`

	// Short is an optional one-line description of what the contribution adds.
	Short string `json:"short,omitempty"`

	// Flags are added to the built-in command's flag set. The plugin handler
	// parses them itself; engaged flags are forwarded to the handler.
	Flags []FlagSpec `json:"flags,omitempty"`

	// EngageWhen names the flag whose presence triggers dispatching the
	// handler. When empty, the contribution engages if any of its Flags is set.
	EngageWhen string `json:"engage_when,omitempty"`
}

// Command describes one top-level CLI command the plugin provides.
type Command struct {
	// Name is the command name as the user types it (e.g., "push").
	Name string `json:"name"`

	// Short is the short help line (one sentence).
	Short string `json:"short,omitempty"`

	// Long is the long help text shown by --help.
	Long string `json:"long,omitempty"`

	// Args declares positional arguments. Used to render usage strings
	// and shell completion. Argument parsing happens in the plugin
	// subprocess; this is purely declarative metadata.
	Args []ArgSpec `json:"args,omitempty"`

	// Flags declares command flags. Used for shell completion and the
	// usage string. The plugin subprocess parses flags itself.
	Flags []FlagSpec `json:"flags,omitempty"`

	// Subcommands lists nested subcommand names (e.g., for "auth": ["login",
	// "logout", "status"]). Subcommands inherit the parent's transport.
	Subcommands []string `json:"subcommands,omitempty"`
}

// ArgSpec describes one positional argument.
type ArgSpec struct {
	Name     string `json:"name"`
	Optional bool   `json:"optional,omitempty"`
	Variadic bool   `json:"variadic,omitempty"`
}

// FlagSpec describes one command flag.
type FlagSpec struct {
	Name        string `json:"name"`
	Short       string `json:"short,omitempty"`
	Type        string `json:"type"` // "bool" | "string" | "int" | "stringSlice"
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
}

// MCPTool describes one MCP tool the plugin exposes.
type MCPTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Format describes one format reader/writer (Mode C).
type Format struct {
	// Name is the format identifier (e.g., "okf_idml", "okf_html").
	Name string `json:"name"`

	// DisplayName is the human-readable name (e.g., "Adobe IDML").
	DisplayName string `json:"display_name,omitempty"`

	// Description is a short description of the format.
	Description string `json:"description,omitempty"`

	// Extensions lists file extensions this format handles (e.g., [".idml"]).
	Extensions []string `json:"extensions,omitempty"`

	// MimeTypes lists MIME types this format handles.
	MimeTypes []string `json:"mime_types,omitempty"`

	// Capabilities lists the supported operations: "read", "write".
	Capabilities []string `json:"capabilities,omitempty"`

	// Schema is the path (relative to the plugin dir) to the format's
	// parameter JSON Schema, if any.
	Schema string `json:"schema,omitempty"`
}

// HasCapability reports whether this format supports the named operation.
func (f *Format) HasCapability(name string) bool {
	for _, c := range f.Capabilities {
		if strings.EqualFold(c, name) {
			return true
		}
	}
	return false
}

// Tool describes one flow tool (Mode C).
type Tool struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Inputs      []string `json:"inputs,omitempty"`
	Outputs     []string `json:"outputs,omitempty"`
	Schema      string   `json:"schema,omitempty"`
}

// SourceConnector describes one source connector (Mode C).
type SourceConnector struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
}

// SchemaExtension binds a recipe YAML key (at a given scope) to a JSON
// Schema file shipped in the plugin dir.
type SchemaExtension struct {
	// Name is the YAML key at the given scope (e.g., "server").
	Name string `json:"name"`

	// Scope is one of "project", "defaults", "collection", "item".
	Scope string `json:"scope"`

	// Group is a logical grouping for related extensions (e.g.,
	// "bowrain"). A recipe's `requires:` map is matched against
	// these group names.
	Group string `json:"group,omitempty"`

	// JSONSchema is the path (relative to the plugin dir) to the
	// JSON Schema validating values under this key.
	JSONSchema string `json:"json_schema,omitempty"`
}

// DaemonConfig declares Mode-C daemon behavior. Only present for
// plugins that provide formats, tools, or source connectors.
type DaemonConfig struct {
	// IdleTimeoutSeconds bounds how long the daemon may sit idle before
	// kapi terminates it. Defaults to 300 (5 minutes) when zero.
	IdleTimeoutSeconds int `json:"idle_timeout_seconds,omitempty"`

	// StartupTimeoutSeconds bounds how long kapi waits for the daemon's
	// handshake line on stdout. Defaults to 30 seconds when zero.
	StartupTimeoutSeconds int `json:"startup_timeout_seconds,omitempty"`

	// Handshake describes the handshake protocol the daemon uses to
	// advertise its socket address.
	Handshake *Handshake `json:"handshake,omitempty"`
}

// Handshake describes how a daemon advertises its socket. Currently
// only the "stdio-handshake" type is supported: the daemon prints one
// JSON line on stdout containing the listed fields, then keeps stdout
// open for log output.
type Handshake struct {
	Type   string   `json:"type"`             // currently always "stdio-handshake"
	Fields []string `json:"fields,omitempty"` // typically ["socket", "version"]
}

// IsModeC reports whether the manifest declares Mode-C transport.
// True when the manifest provides any format, tool, or source connector.
func (m *Manifest) IsModeC() bool {
	c := m.Capabilities
	return len(c.Formats) > 0 || len(c.Tools) > 0 || len(c.SourceConnectors) > 0
}

// IsModeB reports whether the manifest declares Mode-B transport.
// True when the manifest provides at least one MCP tool.
func (m *Manifest) IsModeB() bool {
	return len(m.Capabilities.MCPTools) > 0
}

// IsModeA reports whether the manifest declares Mode-A transport.
// True when the manifest provides at least one command.
func (m *Manifest) IsModeA() bool {
	return len(m.Capabilities.Commands) > 0
}

// Validate performs lightweight structural validation. Heavy validation
// (JSON Schema conformance) is done by callers using SchemaJSON.
func (m *Manifest) Validate() error {
	if m.ManifestVersion == "" {
		return errors.New("manifest_version is required")
	}
	supported := false
	for _, v := range SupportedVersions {
		if v == m.ManifestVersion {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("unsupported manifest_version %q (supported: %s)", m.ManifestVersion, strings.Join(SupportedVersions, ", "))
	}
	if m.Plugin == "" {
		return errors.New("plugin is required")
	}
	if !validPluginName(m.Plugin) {
		return fmt.Errorf("invalid plugin name %q (must match [a-z0-9][a-z0-9-]*)", m.Plugin)
	}
	if m.Version == "" {
		return errors.New("version is required")
	}
	if m.Binary == "" {
		return errors.New("binary is required")
	}
	if err := validateBinaryPath(m.Binary); err != nil {
		return err
	}
	if m.IsModeC() && m.Daemon == nil {
		return errors.New("daemon block is required when capabilities include formats, tools, or source_connectors")
	}
	if m.Daemon != nil && !m.IsModeC() {
		return errors.New("daemon block is only valid when capabilities include formats, tools, or source_connectors")
	}
	for i, c := range m.Capabilities.Commands {
		if c.Name == "" {
			return fmt.Errorf("capabilities.commands[%d]: name is required", i)
		}
	}
	for i, t := range m.Capabilities.MCPTools {
		if t.Name == "" {
			return fmt.Errorf("capabilities.mcp_tools[%d]: name is required", i)
		}
	}
	for i, f := range m.Capabilities.Formats {
		if f.Name == "" {
			return fmt.Errorf("capabilities.formats[%d]: name is required", i)
		}
	}
	for i, t := range m.Capabilities.Tools {
		if t.Name == "" {
			return fmt.Errorf("capabilities.tools[%d]: name is required", i)
		}
	}
	for i, sc := range m.Capabilities.SourceConnectors {
		if sc.ID == "" {
			return fmt.Errorf("capabilities.source_connectors[%d]: id is required", i)
		}
	}
	for i, ext := range m.Capabilities.SchemaExtensions {
		if ext.Name == "" {
			return fmt.Errorf("capabilities.schema_extensions[%d]: name is required", i)
		}
		switch ext.Scope {
		case "project", "defaults", "collection", "item":
		default:
			return fmt.Errorf("capabilities.schema_extensions[%d]: invalid scope %q (want project|defaults|collection|item)", i, ext.Scope)
		}
	}
	for i, cc := range m.Capabilities.CommandContributions {
		if cc.Command == "" {
			return fmt.Errorf("capabilities.command_contributions[%d]: command is required", i)
		}
		if cc.Handler == "" {
			return fmt.Errorf("capabilities.command_contributions[%d]: handler is required", i)
		}
		for j, fl := range cc.Flags {
			if fl.Name == "" {
				return fmt.Errorf("capabilities.command_contributions[%d].flags[%d]: name is required", i, j)
			}
			switch fl.Type {
			case "bool", "string", "int", "stringSlice":
			default:
				return fmt.Errorf("capabilities.command_contributions[%d].flags[%d]: invalid type %q (want bool|string|int|stringSlice)", i, j, fl.Type)
			}
		}
		if cc.EngageWhen != "" {
			found := false
			for _, fl := range cc.Flags {
				if fl.Name == cc.EngageWhen {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("capabilities.command_contributions[%d]: engage_when %q is not one of the declared flags", i, cc.EngageWhen)
			}
		}
	}
	return nil
}

// validateBinaryPath enforces that `binary` is a relative path inside
// the plugin dir. Forward slashes are allowed (jpackage produces
// e.g. "bin/kapi-okapi-bridge" on Linux, "Contents/MacOS/..." on macOS).
// Windows-style backslashes, absolute paths, and `..` segments are
// rejected to keep the host from escaping the plugin dir at exec time.
func validateBinaryPath(p string) error {
	if strings.ContainsRune(p, '\\') {
		return fmt.Errorf("binary %q: backslashes are not allowed; use forward slashes (kapi normalizes them per-platform at exec time)", p)
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("binary %q: absolute paths are not allowed", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			return fmt.Errorf("binary %q: empty path segment", p)
		}
		if seg == ".." {
			return fmt.Errorf("binary %q: parent-dir segments are not allowed", p)
		}
	}
	return nil
}

// validPluginName matches [a-z0-9][a-z0-9-]*.
func validPluginName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (i > 0 && r == '-')
		if !ok {
			return false
		}
	}
	return true
}

// Parse decodes a manifest from raw JSON.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest: parse: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("manifest: validate: %w", err)
	}
	return &m, nil
}

// Marshal encodes the manifest to indented JSON.
func (m *Manifest) Marshal() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}
