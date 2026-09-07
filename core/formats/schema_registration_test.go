package formats_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
	fschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A format's schema reaches a reader through its config; the schema registry is
// where every other surface reads it, so the two have to agree. Registration is
// one call per format in RegisterAll, and five formats were registered without
// it: epub, messageformat, mo, odf and tsv had a schema their reader knew and
// the registry did not.
//
// The cost of that was invisible. `kapi validate` accepted any parameter name
// for those five, and the builtins metadata document carried no parameter text
// for them, so every title and description on their reference pages stayed
// English in every locale while the neighbouring formats translated.
func TestEveryFormatSchemaIsRegistered(t *testing.T) {
	reg := registry.NewFormatRegistry()
	schemaReg := fschema.NewSchemaRegistry()
	formats.RegisterAll(reg, formats.RegisterOptions{
		SchemaReg: schemaReg,
		ConfigReg: config.NewRegistry(),
	})

	var checked int
	for _, info := range reg.FormatInfos() {
		if !info.HasReader {
			continue
		}
		reader, err := reg.NewReader(info.Name)
		require.NoError(t, err, "format %s", info.Name)
		if reader == nil {
			continue
		}
		provider, ok := reader.Config().(format.SchemaProvider)
		if !ok {
			continue
		}
		own := provider.Schema()
		if own == nil {
			continue
		}
		checked++

		registered, ok := schemaReg.GetSchema(string(info.Name))
		require.Truef(t, ok,
			"format %s has a schema on its config but none in the schema registry: "+
				"add registerSchemaAndDecoder for it in RegisterAll", info.Name)
		assert.Equal(t, own.Description, registered.Description, "format %s", info.Name)
		assert.Len(t, registered.Properties, len(own.Properties), "format %s", info.Name)
		assert.Len(t, registered.Groups, len(own.Groups), "format %s", info.Name)
	}

	assert.Greater(t, checked, 20, "expected the built-in formats to carry schemas")
}

// The five that were missing, named so a regression says which one came back.
func TestFormatsThatWentUnregistered(t *testing.T) {
	reg := registry.NewFormatRegistry()
	schemaReg := fschema.NewSchemaRegistry()
	formats.RegisterAll(reg, formats.RegisterOptions{
		SchemaReg: schemaReg,
		ConfigReg: config.NewRegistry(),
	})

	for _, id := range []string{"epub", "messageformat", "mo", "odf", "tsv"} {
		t.Run(id, func(t *testing.T) {
			s, ok := schemaReg.GetSchema(id)
			require.True(t, ok)
			require.NotNil(t, s)
		})
	}
}
