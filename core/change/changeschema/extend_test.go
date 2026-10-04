package changeschema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frozenRequest is the frozen kapi.change/v1 schema, decoded.
func frozenRequest(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "schema.v1.frozen.json"))
	require.NoError(t, err)
	return b
}

type node = map[string]any

func decode(t *testing.T, b []byte) node {
	t.Helper()
	var n node
	require.NoError(t, json.Unmarshal(b, &n))
	return n
}

func encode(t *testing.T, n node) []byte {
	t.Helper()
	b, err := json.Marshal(n)
	require.NoError(t, err)
	return b
}

// operation returns the oneOf member of the operation named op.
func operation(t *testing.T, root node, op string) node {
	t.Helper()
	members := root["properties"].(node)["ops"].(node)["items"].(node)["oneOf"].([]any)
	for _, m := range members {
		member := m.(node)
		if member["properties"].(node)["op"].(node)["const"] == op {
			return member
		}
	}
	t.Fatalf("no operation %s", op)
	return nil
}

func withoutString(list any, s string) []any {
	return slices.DeleteFunc(slices.Clone(list.([]any)), func(v any) bool { return v == s })
}

// The comparator refuses what breaks a sender of the frozen schema and allows
// what only extends it, each case a mutation of the frozen schema itself.
func TestExtends_RequestSide(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(t *testing.T, root node)
		refused string
	}{
		{"removing replace_text's edits", func(t *testing.T, root node) {
			m := operation(t, root, "replace_text")
			delete(m["properties"].(node), "edits")
			m["required"] = withoutString(m["required"], "edits")
		}, `property "edits" was removed`},
		{"renaming set_attribute's name to attribute", func(t *testing.T, root node) {
			props := operation(t, root, "set_attribute")["properties"].(node)
			props["attribute"] = props["name"]
			delete(props, "name")
		}, `property "name" was removed`},
		{"requiring basis on set_content", func(t *testing.T, root node) {
			m := operation(t, root, "set_content")
			m["required"] = append(m["required"].([]any), "basis")
		}, `property "basis" became required`},
		{"changing if_match's pattern", func(t *testing.T, root node) {
			operation(t, root, "set_content")["properties"].(node)["if_match"].(node)["pattern"] = "^r:[0-9a-f]{32}$"
		}, "pattern changed"},
		{"changing path's type", func(t *testing.T, root node) {
			root["$defs"].(node)["path"].(node)["type"] = "object"
		}, "type changed"},
		{"opening additionalProperties", func(t *testing.T, root node) {
			operation(t, root, "set_content")["additionalProperties"] = true
		}, "additionalProperties changed"},
		{"removing preview from mode", func(t *testing.T, root node) {
			mode := root["properties"].(node)["mode"].(node)
			mode["enum"] = withoutString(mode["enum"], "preview")
		}, `enum value "preview" was removed`},
		{"removing an operation", func(t *testing.T, root node) {
			ops := root["properties"].(node)["ops"].(node)["items"].(node)
			ops["oneOf"] = slices.DeleteFunc(slices.Clone(ops["oneOf"].([]any)), func(m any) bool {
				return m.(node)["properties"].(node)["op"].(node)["const"] == "mark"
			})
		}, "oneOf member op=mark was removed"},
		{"removing a $defs entry", func(t *testing.T, root node) {
			delete(root["$defs"].(node), "block")
		}, `$defs entry "block" was removed`},
		{"adding an optional property", func(t *testing.T, root node) {
			operation(t, root, "set_content")["properties"].(node)["comment"] = node{"type": "string"}
		}, ""},
		{"adding an operation", func(t *testing.T, root node) {
			ops := root["properties"].(node)["ops"].(node)["items"].(node)
			ops["oneOf"] = append(ops["oneOf"].([]any), node{
				"type": "object", "required": []any{"op"}, "additionalProperties": false,
				"properties": node{"op": node{"type": "string", "const": "split_block"}},
			})
		}, ""},
		{"adding an enum value", func(t *testing.T, root node) {
			mode := root["properties"].(node)["mode"].(node)
			mode["enum"] = append(mode["enum"].([]any), "propose")
		}, ""},
		{"adding a $defs entry and changing descriptions", func(t *testing.T, root node) {
			root["$defs"].(node)["anchor"] = node{"type": "object"}
			root["description"] = "another description"
			operation(t, root, "mark")["description"] = "another summary"
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frozen := frozenRequest(t)
			current := decode(t, frozen)
			tc.mutate(t, current)
			problems, err := Extends(frozen, encode(t, current), Writer)
			require.NoError(t, err)
			if tc.refused == "" {
				assert.Empty(t, problems)
				return
			}
			require.NotEmpty(t, problems, "refused")
			found := false
			for _, p := range problems {
				found = found || strings.Contains(p, tc.refused)
			}
			assert.True(t, found, "%q in %v", tc.refused, problems)
		})
	}
}

// From the reader's side a property a reply stops promising breaks a reader,
// and one it starts promising does not.
func TestExtends_ReaderSide(t *testing.T) {
	frozen := ResultSchema()
	current := decode(t, frozen)
	current["required"] = withoutString(current["required"], "status")
	problems, err := Extends(frozen, encode(t, current), Reader)
	require.NoError(t, err)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], `property "status" is no longer required`)

	current = decode(t, frozen)
	current["properties"].(node)["warnings"] = node{"type": "array"}
	current["required"] = append(current["required"].([]any), "warnings")
	problems, err = Extends(frozen, encode(t, current), Reader)
	require.NoError(t, err)
	assert.Empty(t, problems, "a new field a reply always carries extends it")

	current = decode(t, frozen)
	status := current["properties"].(node)["status"].(node)
	status["enum"] = append(status["enum"].([]any), "queued")
	problems, err = Extends(frozen, encode(t, current), Reader)
	require.NoError(t, err)
	assert.Empty(t, problems, "a new status extends it")
}

// A schema extends itself, and the current schemas extend the frozen ones
// (TestSchemaExtendsFrozenV1 in package change says so with its message).
func TestExtends_ItselfAndTheGenerated(t *testing.T) {
	for _, b := range [][]byte{Schema(), ResultSchema(), ReadSchema()} {
		problems, err := Extends(b, b, Writer)
		require.NoError(t, err)
		assert.Empty(t, problems)
	}
	problems, err := Extends(frozenRequest(t), Schema(), Writer)
	require.NoError(t, err)
	assert.Empty(t, problems)
}
