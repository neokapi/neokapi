//go:build parity

package formats

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCsvBridgeConfig pins the neokapi→Okapi csv config translation.
// The translator emits one `fprmContent` Okapi ParametersString blob;
// these assertions verify each spec key lands as the right field line.
func TestCsvBridgeConfig(t *testing.T) {
	t.Run("defaults_with_no_overrides", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{})
		require.NoError(t, err)
		fprm := requireFprm(t, out)
		// neokapi-canonical CSV defaults.
		assert.Contains(t, fprm, "#v1\n")
		assert.Contains(t, fprm, "fieldDelimiter=,\n")
		assert.Contains(t, fprm, "textQualifier=\"\n")
		assert.Contains(t, fprm, "columnNamesLineNum.i=1\n")
		assert.Contains(t, fprm, "valuesStartLineNum.i=2\n")
		assert.Contains(t, fprm, "sendHeaderMode.i=0\n")
		assert.Contains(t, fprm, "sendColumnsMode.i=2\n") // SEND_COLUMNS_ALL
		assert.Contains(t, fprm, "trimMode.i=0\n")
		assert.Contains(t, fprm, "targetColumns=\n")
		assert.Contains(t, fprm, "parametersClass=net.sf.okapi.filters.table.csv.Parameters\n")
	})

	t.Run("separator_to_fieldDelimiter", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{"separator": "\t"})
		require.NoError(t, err)
		assert.Contains(t, requireFprm(t, out), "fieldDelimiter=\t\n")
	})

	t.Run("textQualifier_verbatim", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{"textQualifier": "'"})
		require.NoError(t, err)
		assert.Contains(t, requireFprm(t, out), "textQualifier='\n")
	})

	t.Run("hasHeader_true", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{"hasHeader": true})
		require.NoError(t, err)
		fprm := requireFprm(t, out)
		assert.Contains(t, fprm, "columnNamesLineNum.i=1\n")
		assert.Contains(t, fprm, "valuesStartLineNum.i=2\n")
		assert.Contains(t, fprm, "sendHeaderMode.i=0\n")
	})

	t.Run("hasHeader_false", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{"hasHeader": false})
		require.NoError(t, err)
		fprm := requireFprm(t, out)
		assert.Contains(t, fprm, "columnNamesLineNum.i=0\n")
		assert.Contains(t, fprm, "valuesStartLineNum.i=1\n")
	})

	t.Run("columnNamesRow_and_valuesStartRow", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{"columnNamesRow": 2, "valuesStartRow": 4})
		require.NoError(t, err)
		fprm := requireFprm(t, out)
		assert.Contains(t, fprm, "columnNamesLineNum.i=2\n")
		assert.Contains(t, fprm, "valuesStartLineNum.i=4\n")
	})

	t.Run("translatableColumns_zero_based_to_one_based_listed", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{"translatableColumns": []any{0, 2}})
		require.NoError(t, err)
		fprm := requireFprm(t, out)
		assert.Contains(t, fprm, "sourceColumns=1,3\n")
		assert.Contains(t, fprm, "sendColumnsMode.i=1\n") // SEND_COLUMNS_LISTED
	})

	t.Run("keyColumns_to_sourceIdColumns", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{"keyColumns": []any{0, 1}})
		require.NoError(t, err)
		assert.Contains(t, requireFprm(t, out), "sourceIdColumns=1,2\n")
	})

	t.Run("commentColumns_one_based", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{"commentColumns": []any{1}})
		require.NoError(t, err)
		assert.Contains(t, requireFprm(t, out), "commentColumns=2\n")
	})

	t.Run("trimValues_true", func(t *testing.T) {
		out, err := csvBridgeConfig(map[string]any{"trimValues": true})
		require.NoError(t, err)
		fprm := requireFprm(t, out)
		assert.Contains(t, fprm, "trimMode.i=2\n")
		assert.Contains(t, fprm, "trimLeading.b=true\n")
		assert.Contains(t, fprm, "trimTrailing.b=true\n")
	})

	t.Run("unknown_key_errors", func(t *testing.T) {
		_, err := csvBridgeConfig(map[string]any{"bogus": true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown spec key")
	})

	t.Run("wrong_type_errors", func(t *testing.T) {
		_, err := csvBridgeConfig(map[string]any{"hasHeader": "yes"})
		require.Error(t, err)
	})
}

// TestXmlstreamBridgeConfig pins the neokapi→Okapi xmlstream config
// translation: shorthand camelCase keys expand into the long-form
// Okapi YAML keys (elements / attributes / exclude_by_default /
// preserve_whitespace) that the bridge's YAML deep-merge applies.
func TestXmlstreamBridgeConfig(t *testing.T) {
	t.Run("translatableElements_to_include_with_exclude_by_default", func(t *testing.T) {
		out, err := xmlstreamBridgeConfig(map[string]any{
			"translatableElements": []any{"title", "description"},
		})
		require.NoError(t, err)
		assert.Equal(t, true, out["exclude_by_default"],
			"translatableElements is an opt-in whitelist → exclude_by_default")
		elements, ok := out["elements"].(map[string]any)
		require.True(t, ok, "elements map present")
		assert.Equal(t, ruleTypes("INCLUDE"), elements["title"])
		assert.Equal(t, ruleTypes("INCLUDE"), elements["description"])
	})

	t.Run("excludeByDefault_to_underscore", func(t *testing.T) {
		out, err := xmlstreamBridgeConfig(map[string]any{"excludeByDefault": true})
		require.NoError(t, err)
		assert.Equal(t, true, out["exclude_by_default"])
	})

	t.Run("excludeByDefault_false_is_emitted", func(t *testing.T) {
		out, err := xmlstreamBridgeConfig(map[string]any{"excludeByDefault": false})
		require.NoError(t, err)
		v, ok := out["exclude_by_default"]
		require.True(t, ok, "explicit false must be emitted")
		assert.Equal(t, false, v)
	})

	t.Run("explicit_excludeByDefault_wins_over_translatableElements_implication", func(t *testing.T) {
		out, err := xmlstreamBridgeConfig(map[string]any{
			"translatableElements": []any{"title"},
			"excludeByDefault":     false,
		})
		require.NoError(t, err)
		assert.Equal(t, false, out["exclude_by_default"],
			"an explicit excludeByDefault must override the translatableElements implication")
	})

	t.Run("preserveWhitespace_to_underscore", func(t *testing.T) {
		out, err := xmlstreamBridgeConfig(map[string]any{"preserveWhitespace": true})
		require.NoError(t, err)
		assert.Equal(t, true, out["preserve_whitespace"])
	})

	t.Run("parser_preserveWhitespace_subkey", func(t *testing.T) {
		out, err := xmlstreamBridgeConfig(map[string]any{
			"parser": map[string]any{"preserveWhitespace": true, "assumeWellformed": true},
		})
		require.NoError(t, err)
		assert.Equal(t, true, out["preserve_whitespace"])
		// assumeWellformed is intentionally dropped.
		_, hasAssume := out["assumeWellformed"]
		assert.False(t, hasAssume)
	})

	t.Run("element_shorthands_map_to_rule_types", func(t *testing.T) {
		out, err := xmlstreamBridgeConfig(map[string]any{
			"inlineElements":             []any{"b"},
			"excludedElements":           []any{"pre"},
			"preserveWhitespaceElements": []any{"code"},
			"groupElements":              []any{"ul"},
		})
		require.NoError(t, err)
		elements, ok := out["elements"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, ruleTypes("INLINE"), elements["b"])
		assert.Equal(t, ruleTypes("EXCLUDE"), elements["pre"])
		assert.Equal(t, ruleTypes("PRESERVE_WHITESPACE"), elements["code"])
		assert.Equal(t, ruleTypes("GROUP"), elements["ul"])
		// excludeByDefault is NOT implied by these shorthands.
		_, hasExclude := out["exclude_by_default"]
		assert.False(t, hasExclude)
	})

	t.Run("translatableAttributes_to_attribute_trans", func(t *testing.T) {
		out, err := xmlstreamBridgeConfig(map[string]any{
			"translatableAttributes": []any{"title", "alt"},
		})
		require.NoError(t, err)
		attrs, ok := out["attributes"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, ruleTypes("ATTRIBUTE_TRANS"), attrs["title"])
		assert.Equal(t, ruleTypes("ATTRIBUTE_TRANS"), attrs["alt"])
	})

	t.Run("explicit_elements_and_attributes_pass_through", func(t *testing.T) {
		elementRule := map[string]any{"ruleTypes": []any{"EXCLUDE"}}
		out, err := xmlstreamBridgeConfig(map[string]any{
			"elements": map[string]any{"pre": elementRule},
		})
		require.NoError(t, err)
		elements, ok := out["elements"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, elementRule, elements["pre"])
	})

	t.Run("unknown_key_errors", func(t *testing.T) {
		_, err := xmlstreamBridgeConfig(map[string]any{"bogus": true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown spec key")
	})

	t.Run("wrong_type_errors", func(t *testing.T) {
		_, err := xmlstreamBridgeConfig(map[string]any{"preserveWhitespace": "yes"})
		require.Error(t, err)
	})
}

// TestRegexBridgeConfig pins the neokapi→Okapi okf_regex config
// translation: the rule list is serialised to the reserved
// `regexRulesJson` parameter, the escape discriminator folds into the
// two bridge booleans, and regexOptions is forced to 0 (RE2-equivalent,
// no global DOTALL/MULTILINE).
func TestRegexBridgeConfig(t *testing.T) {
	t.Run("no_rules_forces_re2_defaults", func(t *testing.T) {
		out, err := regexBridgeConfig(map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, 0, out["regexOptions"])
		assert.Equal(t, false, out["useBSlashEscape"])
		assert.Equal(t, false, out["useDoubleCharEscape"])
		_, hasRules := out["regexRulesJson"]
		assert.False(t, hasRules, "no rules → no regexRulesJson key")
	})

	t.Run("source_and_id_rule", func(t *testing.T) {
		out, err := regexBridgeConfig(map[string]any{
			"rules": []any{
				map[string]any{
					"pattern":     `"([^"]*?)"\s*=\s*"(.*?)"\s*;`,
					"sourceGroup": 2,
					"idGroup":     1,
				},
			},
		})
		require.NoError(t, err)
		rules := requireRegexRules(t, out)
		require.Len(t, rules, 1)
		assert.Equal(t, `"([^"]*?)"\s*=\s*"(.*?)"\s*;`, rules[0].Expr)
		assert.Equal(t, 1, rules[0].RuleType) // RULETYPE_CONTENT
		assert.Equal(t, 2, rules[0].SourceGroup)
		assert.Equal(t, 1, rules[0].NameGroup)
		assert.Equal(t, -1, rules[0].NoteGroup) // unset → Okapi sentinel
	})

	t.Run("note_group_maps_to_okapi_minus_one_sentinel", func(t *testing.T) {
		out, err := regexBridgeConfig(map[string]any{
			"rules": []any{
				map[string]any{
					"pattern":     `/\*(.*?)\*/\s*"([^"]*?)"\s*=\s*"(.*?)"\s*;`,
					"sourceGroup": 3,
					"idGroup":     2,
					"noteGroup":   1,
				},
			},
		})
		require.NoError(t, err)
		rules := requireRegexRules(t, out)
		require.Len(t, rules, 1)
		assert.Equal(t, 3, rules[0].SourceGroup)
		assert.Equal(t, 2, rules[0].NameGroup)
		assert.Equal(t, 1, rules[0].NoteGroup)
	})

	t.Run("source_only_rule_uses_minus_one_for_name_and_note", func(t *testing.T) {
		out, err := regexBridgeConfig(map[string]any{
			"rules": []any{
				map[string]any{"pattern": `"(.*?)"`, "sourceGroup": 1},
			},
		})
		require.NoError(t, err)
		rules := requireRegexRules(t, out)
		require.Len(t, rules, 1)
		assert.Equal(t, 1, rules[0].SourceGroup)
		assert.Equal(t, -1, rules[0].NameGroup)
		assert.Equal(t, -1, rules[0].NoteGroup)
	})

	t.Run("multiple_rules_preserve_order", func(t *testing.T) {
		out, err := regexBridgeConfig(map[string]any{
			"rules": []any{
				map[string]any{"pattern": `(?m)^(\w+)=(.+)$`, "sourceGroup": 2, "idGroup": 1},
				map[string]any{"pattern": `LABEL\s+"([^"]+)"`, "sourceGroup": 1},
			},
		})
		require.NoError(t, err)
		rules := requireRegexRules(t, out)
		require.Len(t, rules, 2)
		assert.Equal(t, `(?m)^(\w+)=(.+)$`, rules[0].Expr)
		assert.Equal(t, 1, rules[0].NameGroup)
		assert.Equal(t, `LABEL\s+"([^"]+)"`, rules[1].Expr)
		assert.Equal(t, -1, rules[1].NameGroup)
	})

	t.Run("escape_backslash", func(t *testing.T) {
		out, err := regexBridgeConfig(map[string]any{
			"rules":      []any{map[string]any{"pattern": `"(.*?)"`, "sourceGroup": 1}},
			"escapeType": "backslash",
		})
		require.NoError(t, err)
		assert.Equal(t, true, out["useBSlashEscape"])
		assert.Equal(t, false, out["useDoubleCharEscape"])
	})

	t.Run("escape_doublechar_with_escapechar", func(t *testing.T) {
		out, err := regexBridgeConfig(map[string]any{
			"rules":      []any{map[string]any{"pattern": `"((?:[^"]|"")*)"`, "sourceGroup": 1}},
			"escapeType": "doublechar",
			"escapeChar": `"`,
		})
		require.NoError(t, err)
		assert.Equal(t, false, out["useBSlashEscape"])
		assert.Equal(t, true, out["useDoubleCharEscape"])
	})

	t.Run("escape_none_explicit", func(t *testing.T) {
		out, err := regexBridgeConfig(map[string]any{
			"rules":      []any{map[string]any{"pattern": `"(.*?)"`, "sourceGroup": 1}},
			"escapeType": "none",
		})
		require.NoError(t, err)
		assert.Equal(t, false, out["useBSlashEscape"])
		assert.Equal(t, false, out["useDoubleCharEscape"])
	})

	t.Run("missing_pattern_errors", func(t *testing.T) {
		_, err := regexBridgeConfig(map[string]any{
			"rules": []any{map[string]any{"sourceGroup": 1}},
		})
		require.Error(t, err)
	})

	t.Run("sourceGroup_below_one_errors", func(t *testing.T) {
		_, err := regexBridgeConfig(map[string]any{
			"rules": []any{map[string]any{"pattern": `(.*)`, "sourceGroup": 0}},
		})
		require.Error(t, err)
	})

	t.Run("unknown_key_errors", func(t *testing.T) {
		_, err := regexBridgeConfig(map[string]any{"bogus": true})
		require.Error(t, err)
	})

	t.Run("unknown_escape_type_errors", func(t *testing.T) {
		_, err := regexBridgeConfig(map[string]any{
			"rules":      []any{map[string]any{"pattern": `(.*)`, "sourceGroup": 1}},
			"escapeType": "rot13",
		})
		require.Error(t, err)
	})
}

// requireRegexRules decodes the regexRulesJson reserved param into the
// Okapi-shaped rule slice, failing the test if it's missing or malformed.
func requireRegexRules(t *testing.T, out map[string]any) []okapiRegexRule {
	t.Helper()
	v, ok := out["regexRulesJson"]
	require.True(t, ok, "regexRulesJson key present")
	s, ok := v.(string)
	require.True(t, ok, "regexRulesJson is a JSON string")
	var rules []okapiRegexRule
	require.NoError(t, json.Unmarshal([]byte(s), &rules))
	return rules
}

// requireFprm extracts the single fprmContent string the csv translator
// emits, failing the test if it's missing or the wrong type.
func requireFprm(t *testing.T, out map[string]any) string {
	t.Helper()
	require.Len(t, out, 1, "csvBridgeConfig emits exactly one key")
	v, ok := out["fprmContent"]
	require.True(t, ok, "fprmContent key present")
	s, ok := v.(string)
	require.True(t, ok, "fprmContent is a string")
	require.True(t, strings.HasPrefix(s, "#v1\n"), "fprm starts with #v1 header")
	return s
}

// ruleTypes builds the {ruleTypes:[t]} rule object the xmlstream
// translator emits for a shorthand element/attribute rule.
func ruleTypes(t string) map[string]any {
	return map[string]any{"ruleTypes": []any{t}}
}
