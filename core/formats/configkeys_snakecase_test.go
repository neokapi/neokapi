package formats_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/config"
	fschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Every native format's config keys are snake_case, the spelling of every
// other recipe key: the properties at every depth, the fields a group lists,
// and the keys a preset sets. A format's ApplyMap refuses a camelCase spelling
// of one of these keys with the key to write (format.UnknownParameterError).
func TestNativeFormatConfigKeysAreSnakeCase(t *testing.T) {
	reg := registry.NewFormatRegistry()
	schemaReg := fschema.NewSchemaRegistry()
	formats.RegisterAll(reg, formats.RegisterOptions{SchemaReg: schemaReg, ConfigReg: config.NewRegistry()})

	var checked int
	for _, info := range reg.FormatInfos() {
		s, ok := schemaReg.GetSchema(string(info.Name))
		if !ok || s == nil {
			continue
		}
		checked++
		assertSnakeCaseFormatProperties(t, string(info.Name), s.Properties)
		for _, g := range s.Groups {
			for _, f := range g.Fields {
				assert.Truef(t, coreschema.IsSnakeCase(f), "%s: group %s lists %s, which is not snake_case", info.Name, g.ID, f)
			}
		}
		for name, preset := range s.Presets {
			for k := range preset {
				assert.Truef(t, coreschema.IsSnakeCase(k), "%s: preset %q sets %s, which is not snake_case", info.Name, name, k)
			}
		}
	}
	require.Greater(t, checked, 20, "expected the built-in formats to carry schemas")
}

func assertSnakeCaseFormatProperties(t *testing.T, at string, props map[string]fschema.PropertySchema) {
	t.Helper()
	for k, p := range props {
		assert.Truef(t, coreschema.IsSnakeCase(k), "%s: config key %s is not snake_case; write %s", at, k, coreschema.SnakeCase(k))
		assertSnakeCaseFormatProperties(t, at+"."+k, p.Properties)
	}
}
