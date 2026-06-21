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
	"slices"
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

	// Models declares large model/data assets the plugin needs at runtime.
	// Unlike Capabilities (things the plugin provides TO kapi), these are data
	// DEPENDENCIES the host fetches, verifies, and caches on the plugin's
	// behalf — so the plugin binary stays a pure compute engine and asset
	// downloads are uniform with the rest of kapi (shared progress bar,
	// concurrency, integrity, one cache). Because the manifest ships inside the
	// cosign + SHA-256 verified plugin tarball, the per-file digests below are
	// trust-rooted, and URLs pin an immutable upstream revision rather than a
	// moving branch. The host stages a model under
	// $XDG_CACHE_HOME/kapi/models/<plugin>/<id>/<version>/ and hands the plugin
	// that directory (it never reaches the network itself).
	Models []ModelAsset `json:"models,omitempty"`
}

// ModelAsset describes one downloadable model the plugin can load. A plugin may
// declare several (e.g. size/quantization variants); exactly one may be marked
// Default, which the host fetches when the user does not name a specific model.
type ModelAsset struct {
	// ID is the model identifier the user/recipe selects and the cache key
	// (e.g. "gemma-4-e2b"). Must match [a-z0-9][a-z0-9._-]*.
	ID string `json:"id"`

	// Version pins the asset revision so the cache is content-stable and a
	// model update is an explicit, observable change (e.g. "1" or an upstream
	// commit short-rev). Part of the cache path.
	Version string `json:"version"`

	// Default marks the model the host fetches when none is named. At most one
	// model per manifest may set this.
	Default bool `json:"default,omitempty"`

	// Bundled marks a model that ships inside the plugin tarball rather than
	// being downloaded by the host. Bundled assets are surfaced by `kapi models`
	// for visibility but cannot be pulled or pruned (there is nothing to fetch
	// or remove), and their files need only a Path — no URL or SHA-256, since
	// integrity is already covered by the signed tarball.
	Bundled bool `json:"bundled,omitempty"`

	// Description is a one-line human summary shown by `kapi models`.
	Description string `json:"description,omitempty"`

	// License is the SPDX identifier (or upstream license name) for the model
	// weights — surfaced so users can see terms before a multi-GB download.
	License string `json:"license,omitempty"`

	// Files are the artifacts to fetch. Every file is stored flat under its
	// Path basename in the model cache dir, so sibling references (e.g. ONNX
	// external-data) resolve.
	Files []ModelFile `json:"files"`
}

// ModelFile is one artifact within a ModelAsset.
type ModelFile struct {
	// Path is the basename the file is stored under in the cache dir
	// (e.g. "decoder_model_merged_q4.onnx"). Must be a bare name — no slashes,
	// no "..".
	Path string `json:"path"`

	// URL is the immutable download URL. It SHOULD pin a fixed upstream
	// revision (e.g. HuggingFace .../resolve/<commit>/...), never a moving
	// branch like main.
	URL string `json:"url"`

	// SHA256 is the lowercase-hex digest the downloaded bytes must match. It is
	// mandatory: the host refuses to install a model file without a pinned
	// digest (no --unsafe escape hatch for declared assets).
	SHA256 string `json:"sha256"`

	// Size, when > 0, is the expected byte length, checked alongside SHA256 and
	// used to pre-size the progress bar before the response arrives.
	Size int64 `json:"size,omitempty"`
}

// DefaultModel returns the manifest's default ModelAsset and true, or a zero
// value and false when the plugin declares no models or none is marked default.
func (m *Manifest) DefaultModel() (ModelAsset, bool) {
	for _, a := range m.Models {
		if a.Default {
			return a, true
		}
	}
	// A single declared model is unambiguously the default even without the flag.
	if len(m.Models) == 1 {
		return m.Models[0], true
	}
	return ModelAsset{}, false
}

// Model returns the declared ModelAsset with the given id, or false.
func (m *Manifest) Model(id string) (ModelAsset, bool) {
	for _, a := range m.Models {
		if a.ID == id {
			return a, true
		}
	}
	return ModelAsset{}, false
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

	// Segmenters declares segmentation engines the plugin provides (Mode C).
	// Each becomes a selectable engine in the segmentation tool, with its own
	// config schema, dispatched over the daemon's Segment RPC.
	Segmenters []Segmenter `json:"segmenters,omitempty"`

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

	// SelfCheck indicates the plugin implements the standard `<binary> doctor`
	// self-check that `kapi plugins doctor` invokes. Plugins that bundle native
	// binaries, models, or in-process engines set this so doctor can confirm
	// those runtime dependencies resolve; plugins without runtime dependencies
	// omit it (doctor still verifies their binary presence and version). This
	// replaces the per-plugin self-check verbs that used to clutter the
	// top-level command surface.
	SelfCheck bool `json:"selfcheck,omitempty"`
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

	// Subcommands lists nested subcommands (e.g., for "auth": ["login",
	// "logout", "status"]). Subcommands inherit the parent's transport and
	// may themselves nest (e.g., "auth token create"). Each entry is either
	// a bare string (a leaf subcommand) or an object carrying its own
	// nested subcommands; see Subcommand.UnmarshalJSON.
	Subcommands []Subcommand `json:"subcommands,omitempty"`
}

// Subcommand describes one nested subcommand under a Command (or under
// another Subcommand). It supports two JSON forms for ergonomics:
//
//	"login"                                         // leaf, no children
//	{"name": "token", "subcommands": ["create"]}    // parent with children
//
// The bare-string form keeps existing manifests (and the common case of
// flat command groups like auth login/logout/status) terse, while the
// object form expresses multi-level trees such as `auth token create`.
type Subcommand struct {
	// Name is the subcommand name as the user types it (e.g., "token").
	Name string `json:"name"`

	// Subcommands are the nested children of this subcommand, if any.
	Subcommands []Subcommand `json:"subcommands,omitempty"`
}

// UnmarshalJSON decodes a Subcommand from either a bare string (leaf) or an
// object with name + nested subcommands.
func (s *Subcommand) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}
		s.Name = name
		s.Subcommands = nil
		return nil
	}
	// Object form. Use an alias to avoid recursing into this method.
	type subcommandAlias Subcommand
	var alias subcommandAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*s = Subcommand(alias)
	return nil
}

// SubcommandNames returns the immediate child subcommand names, preserving
// the bare-string accessor the flat-group call sites relied on.
func (c Command) SubcommandNames() []string {
	names := make([]string, len(c.Subcommands))
	for i, sub := range c.Subcommands {
		names[i] = sub.Name
	}
	return names
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

	// Capabilities lists the supported operations: "read", "write", and
	// "generative". A "write" + "generative" format can serialize a complete
	// document from the content model alone, so it is a valid cross-format
	// conversion target; a "write" format without "generative" is skeleton-bound
	// (round-trips / merges into its own original file only). See AD-005.
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

// Segmenter describes one segmentation engine (Mode C). It mirrors the
// in-process segment.EngineDescriptor: the host registers each as a selectable
// engine carrying its own parameter schema, and dispatches segmentation to the
// daemon's Segment RPC keyed by Name.
type Segmenter struct {
	// Name is the engine id the recipe selects (e.g. "sat"). It is the engine
	// value passed back to the daemon's Segment RPC.
	Name string `json:"name"`

	// DisplayName is the human label shown in the engine selector.
	DisplayName string `json:"display_name,omitempty"`

	// Description is the selector help text.
	Description string `json:"description,omitempty"`

	// Order hints the engine's position in the selector (lower sorts first).
	// Plugin engines default to sorting after the built-ins.
	Order int `json:"order,omitempty"`

	// Schema is the path (relative to the plugin dir) to the engine's parameter
	// JSON Schema, if any.
	Schema string `json:"schema,omitempty"`
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
// True when the manifest provides any format, tool, segmenter, or source
// connector.
func (m *Manifest) IsModeC() bool {
	c := m.Capabilities
	return len(c.Formats) > 0 || len(c.Tools) > 0 || len(c.Segmenters) > 0 || len(c.SourceConnectors) > 0
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
	supported := slices.Contains(SupportedVersions, m.ManifestVersion)
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
		return errors.New("daemon block is required when capabilities include formats, tools, segmenters, or source_connectors")
	}
	if m.Daemon != nil && !m.IsModeC() {
		return errors.New("daemon block is only valid when capabilities include formats, tools, segmenters, or source_connectors")
	}
	for i, c := range m.Capabilities.Commands {
		if c.Name == "" {
			return fmt.Errorf("capabilities.commands[%d]: name is required", i)
		}
		if err := validateSubcommands(c.Subcommands, fmt.Sprintf("capabilities.commands[%d]", i)); err != nil {
			return err
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
	for i, s := range m.Capabilities.Segmenters {
		if s.Name == "" {
			return fmt.Errorf("capabilities.segmenters[%d]: name is required", i)
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
	defaults := 0
	seenModel := map[string]bool{}
	for i, a := range m.Models {
		if a.ID == "" {
			return fmt.Errorf("models[%d]: id is required", i)
		}
		if !validModelID(a.ID) {
			return fmt.Errorf("models[%d]: invalid id %q (must match [a-z0-9][a-z0-9._-]*)", i, a.ID)
		}
		if seenModel[a.ID] {
			return fmt.Errorf("models[%d]: duplicate model id %q", i, a.ID)
		}
		seenModel[a.ID] = true
		if a.Version == "" {
			return fmt.Errorf("models[%d] (%s): version is required", i, a.ID)
		}
		if a.Default {
			defaults++
		}
		if len(a.Files) == 0 {
			return fmt.Errorf("models[%d] (%s): at least one file is required", i, a.ID)
		}
		seenFile := map[string]bool{}
		for j, f := range a.Files {
			if f.Path == "" {
				return fmt.Errorf("models[%d] (%s).files[%d]: path is required", i, a.ID, j)
			}
			if f.Path != filepathBase(f.Path) {
				return fmt.Errorf("models[%d] (%s).files[%d]: path %q must be a bare basename (no directories)", i, a.ID, j, f.Path)
			}
			if seenFile[f.Path] {
				return fmt.Errorf("models[%d] (%s).files[%d]: duplicate file path %q", i, a.ID, j, f.Path)
			}
			seenFile[f.Path] = true
			if a.Bundled {
				continue // bundled files ship in the (signed) tarball — no URL/digest
			}
			if f.URL == "" {
				return fmt.Errorf("models[%d] (%s).files[%d] (%s): url is required", i, a.ID, j, f.Path)
			}
			if !validSHA256(f.SHA256) {
				return fmt.Errorf("models[%d] (%s).files[%d] (%s): a 64-hex sha256 is required (declared model files must be pinned)", i, a.ID, j, f.Path)
			}
		}
	}
	if defaults > 1 {
		return fmt.Errorf("models: at most one model may be marked default (found %d)", defaults)
	}
	return nil
}

// filepathBase returns the final path element, treating both "/" and "\" as
// separators so a Windows-style "dir\file" path is also rejected as non-bare.
func filepathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// validModelID matches [a-z0-9][a-z0-9._-]*.
func validModelID(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			(i > 0 && (r == '-' || r == '.' || r == '_'))
		if !ok {
			return false
		}
	}
	return true
}

// validSHA256 reports whether s is exactly 64 lowercase-or-uppercase hex digits.
func validSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

// validateSubcommands recursively checks that every subcommand carries a
// name. The path is used to build a precise error location.
func validateSubcommands(subs []Subcommand, path string) error {
	for i, sub := range subs {
		if sub.Name == "" {
			return fmt.Errorf("%s.subcommands[%d]: name is required", path, i)
		}
		if err := validateSubcommands(sub.Subcommands, fmt.Sprintf("%s.subcommands[%d]", path, i)); err != nil {
			return err
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
	for seg := range strings.SplitSeq(p, "/") {
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
