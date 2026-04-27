package pluginhost

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
	"gopkg.in/yaml.v3"
)

// RegisterSchemaExtensions takes every schema_extension declared in
// any discovered plugin's manifest and registers it with
// core/project.RegisterExtension so recipe Validate sees them.
//
// The decoder loaded from the manifest is a structural pass-through —
// it does not (yet) validate the payload against the JSON Schema
// referenced in the manifest. That validation will be wired in by
// Phase 4 once a JSON Schema library is selected. For Phase 2, the
// extension is registered as a marker so `requires:` validation
// passes and the recipe Extras round-trip.
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
		decoder := loadSchemaDecoder(reg)
		group := reg.Extension.Group
		if group == "" {
			group = reg.Plugin.Name()
		}
		ext := project.Extension{
			Name:    reg.Extension.Name,
			Scope:   scope,
			Group:   group,
			Decoder: decoder,
		}

		// Avoid panicking on duplicate registration: if a previously
		// loaded plugin (or build-time path) already claimed this
		// (scope, name), we keep the existing registration and warn.
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
// When the manifest references a JSON Schema file, the decoder still
// parses the YAML node (to surface YAML-level errors) but does not
// validate against the schema yet — that's deferred to Phase 4.
//
// When the schema file is missing or unreadable, the decoder logs to
// stderr and accepts any value; this matches the previous build-time
// extension behavior of "decoder is optional".
func loadSchemaDecoder(reg SchemaExtensionRegistration) project.ExtensionDecoder {
	schemaPath := ""
	if reg.Extension.JSONSchema != "" {
		schemaPath = filepath.Join(reg.Plugin.Dir, reg.Extension.JSONSchema)
	}
	// Pre-stat the schema file so we can warn once at registration
	// rather than on every recipe load.
	if schemaPath != "" {
		if _, err := os.Stat(schemaPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: plugin %q schema file %q: %v\n", reg.Plugin.Name(), schemaPath, err)
		}
	}

	return project.ExtensionDecoderFunc(func(node yaml.Node) error {
		// Phase 2: structural decode only. We make sure the value
		// is parseable as YAML; we don't enforce JSON Schema yet.
		var any any
		if err := node.Decode(&any); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		return nil
	})
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
