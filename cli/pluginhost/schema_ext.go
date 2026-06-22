package pluginhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/neokapi/neokapi/core/project"
	"gopkg.in/yaml.v3"
)

// RegisterSchemaExtensions takes every schema_extension declared in
// any discovered plugin's manifest and registers it with
// core/project.RegisterExtension so recipe Validate sees them.
//
// For every extension that ships a json_schema file, the schema is
// loaded and compiled at register time. The returned decoder then
// validates the YAML value against the compiled schema each time a
// recipe is loaded.
//
// If a schema file is missing or fails to compile, a warning is emitted
// via onWarn and the extension falls back to a structural-only decoder.
// This matches the previous "decoder is optional" behavior so that a
// broken plugin schema cannot prevent recipe loading entirely.
//
// Errors are reported via onWarn but never abort registration.
func RegisterSchemaExtensions(host *Host, onWarn func(msg string)) {
	if host == nil {
		return
	}
	if onWarn == nil {
		onWarn = func(string) {}
	}
	for _, reg := range host.SchemaExtensions() {
		scope, err := scopeFromString(reg.Extension.Scope)
		if err != nil {
			onWarn(fmt.Sprintf("plugin %q: schema_extension %q: %v", reg.Plugin.Name(), reg.Extension.Name, err))
			continue
		}
		group := reg.Extension.Group
		if group == "" {
			group = reg.Plugin.Name()
		}

		// Idempotent re-registration: a binary that compiles in a
		// platform's schema package (blank import) and then rediscovers
		// the same plugin through its manifest sees every (scope, name)
		// twice. When the existing claim belongs to the same group this
		// is benign — keep the compiled-in decoder and move on silently.
		// Only a genuine cross-plugin clash (a different group claiming a
		// key we own) is worth a warning.
		if existing, ok := project.ExtensionRegistered(scope, reg.Extension.Name); ok {
			if existing != group {
				onWarn(fmt.Sprintf("plugin %q: schema_extension %q at scope %s already registered by %q — keeping existing entry", reg.Plugin.Name(), reg.Extension.Name, reg.Extension.Scope, existing))
			}
			continue
		}

		decoder := loadSchemaDecoder(reg, onWarn)
		ext := project.Extension{
			Name:    reg.Extension.Name,
			Scope:   scope,
			Group:   group,
			Decoder: decoder,
		}

		// Belt-and-suspenders: registration isn't synchronized against a
		// concurrent registrar, so keep recovering from the duplicate
		// panic even though the check above handles the common case.
		func() {
			defer func() {
				if r := recover(); r != nil {
					onWarn(fmt.Sprintf("plugin %q: schema_extension %q at scope %s already registered — keeping existing entry", reg.Plugin.Name(), reg.Extension.Name, reg.Extension.Scope))
				}
			}()
			project.RegisterExtension(ext)
		}()
	}
}

// loadSchemaDecoder returns a decoder for one schema_extension entry.
//
// When the manifest references a JSON Schema file (json_schema), the
// schema is read from disk and compiled at register time. The decoder
// returned then validates each recipe value against the compiled schema.
//
// On any failure to load or compile the schema, a warning is emitted
// via onWarn and the decoder falls back to a structural-only check
// (YAML must parse, but no schema is enforced).
//
// When no json_schema is declared, a structural-only decoder is used.
func loadSchemaDecoder(reg SchemaExtensionRegistration, onWarn func(msg string)) project.ExtensionDecoder {
	// Structural fallback: ensure the YAML node is parseable.
	structural := project.ExtensionDecoderFunc(func(node yaml.Node) error {
		var any any
		if err := node.Decode(&any); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		return nil
	})

	if reg.Extension.JSONSchema == "" {
		return structural
	}

	schemaPath := filepath.Join(reg.Plugin.Dir, reg.Extension.JSONSchema)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		onWarn(fmt.Sprintf(
			"plugin %q: schema_extension %q: cannot read JSON Schema %q: %v — falling back to structural validation",
			reg.Plugin.Name(), reg.Extension.Name, schemaPath, err,
		))
		return structural
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		onWarn(fmt.Sprintf(
			"plugin %q: schema_extension %q: cannot parse JSON Schema %q: %v — falling back to structural validation",
			reg.Plugin.Name(), reg.Extension.Name, schemaPath, err,
		))
		return structural
	}

	resolved, err := schema.Resolve(nil)
	if err != nil {
		onWarn(fmt.Sprintf(
			"plugin %q: schema_extension %q: cannot compile JSON Schema %q: %v — falling back to structural validation",
			reg.Plugin.Name(), reg.Extension.Name, schemaPath, err,
		))
		return structural
	}

	pluginName := reg.Plugin.Name()
	extName := reg.Extension.Name

	return project.ExtensionDecoderFunc(func(node yaml.Node) error {
		// Decode the YAML node into a JSON-shaped Go value (map[string]any,
		// []any, primitives) so jsonschema can validate it.
		instance, err := yamlNodeToJSONValue(node)
		if err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		if err := resolved.Validate(instance); err != nil {
			return formatSchemaError(pluginName, extName, err)
		}
		return nil
	})
}

// yamlNodeToJSONValue normalizes a YAML node into a Go value that mirrors
// the result of unmarshaling JSON: map keys are strings, numbers are
// float64, booleans are bool, and nulls are nil. We do this by round-
// tripping through encoding/json so jsonschema sees identical types to
// what it would see from a JSON document.
func yamlNodeToJSONValue(node yaml.Node) (any, error) {
	var raw any
	if err := node.Decode(&raw); err != nil {
		return nil, err
	}
	// gopkg.in/yaml.v3 decodes mappings as map[string]any (string keys are
	// the common case for JSON-Schema-validated payloads), so we usually
	// don't need to fix up keys. But map[any]any can still appear for
	// non-string keys; round-tripping through JSON enforces the JSON model
	// and surfaces any non-JSON types as an error early.
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("not JSON-compatible: %w", err)
	}
	var out any
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("re-decode: %w", err)
	}
	return out, nil
}

// formatSchemaError renders a jsonschema validation failure into a
// concise multi-line error suitable for surfacing at the recipe location.
// jsonschema returns a tree of errors; we flatten the first few causes
// into bullet points so the user sees the relevant constraints rather
// than a single opaque "doesn't validate" line.
func formatSchemaError(plugin, extension string, err error) error {
	if err == nil {
		return nil
	}
	parts := []string{fmt.Sprintf("does not match JSON Schema for %s.%s:", plugin, extension)}
	for _, line := range flattenSchemaErrors(err) {
		parts = append(parts, "  - "+line)
	}
	return errors.New(strings.Join(parts, "\n"))
}

// flattenSchemaErrors walks an error chain (errors.Join, fmt.Errorf %w,
// nested validation errors) into a list of human-readable lines.
func flattenSchemaErrors(err error) []string {
	if err == nil {
		return nil
	}
	type unwrapMany interface{ Unwrap() []error }
	if u, ok := err.(unwrapMany); ok {
		var out []string
		for _, sub := range u.Unwrap() {
			out = append(out, flattenSchemaErrors(sub)...)
		}
		if len(out) > 0 {
			return out
		}
	}
	if inner := errors.Unwrap(err); inner != nil {
		if nested := flattenSchemaErrors(inner); len(nested) > 0 {
			// Prefer the most specific cause when single-wrapped.
			return nested
		}
	}
	return []string{err.Error()}
}

func scopeFromString(s string) (project.Scope, error) {
	switch s {
	case "project":
		return project.ScopeProject, nil
	case "defaults":
		return project.ScopeDefaults, nil
	case "collection":
		return project.ScopeCollection, nil
	case "item":
		return project.ScopeItem, nil
	default:
		return 0, fmt.Errorf("invalid scope %q (want project|defaults|collection|item)", s)
	}
}
