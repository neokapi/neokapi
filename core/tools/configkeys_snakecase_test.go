package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/schema"
)

// Every built-in tool's config keys are snake_case, the spelling of every
// other recipe key. A camelCase json tag on a config field would put a
// camelCase key in the schema, the reference pages, the MCP surface and the
// flags `kapi exec` derives, and StepConfigError would refuse it in a recipe.
func TestBuiltInToolConfigKeysAreSnakeCase(t *testing.T) {
	reg := builtInRegistry(t)
	for _, name := range builtInNames(t, reg) {
		s := reg.Schema(name)
		if s == nil {
			continue
		}
		assertSnakeCaseProperties(t, string(name), s.Properties)
		for _, g := range s.Groups {
			for _, f := range g.Fields {
				assert.Truef(t, schema.IsSnakeCase(f), "%s: group %s lists %s, which is not snake_case", name, g.ID, f)
			}
		}
		if info := reg.ToolInfo(name); info != nil && info.Group != nil {
			for _, m := range info.Group.Members {
				if ms := reg.MemberSchema(name, m.Name); ms != nil {
					assertSnakeCaseProperties(t, string(name)+"."+m.Name, ms.Properties)
				}
			}
		}
	}
}

func assertSnakeCaseProperties(t *testing.T, at string, props map[string]schema.PropertySchema) {
	t.Helper()
	for k, p := range props {
		assert.Truef(t, schema.IsSnakeCase(k), "%s: config key %s is not snake_case; write %s", at, k, schema.SnakeCase(k))
		assertSnakeCaseProperties(t, at+"."+k, p.Properties)
		if p.Items != nil {
			assertSnakeCaseProperties(t, at+"."+k+"[]", p.Items.Properties)
		}
	}
}
