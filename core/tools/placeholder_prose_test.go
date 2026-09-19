package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CLI help quotes JSON, and a translated word inside one is prose rather than a
// placeholder that moved. Two committed Norwegian help strings are reported
// today for exactly that: one omits an example line carrying
// `{"first":3,"last":4}`, and one renders "findings" as "funn" inside
// `{"decision":"block","reason":"…findings…"}`. Neither is a placeholder a
// program interpolates into, and a reader loses nothing by the difference.
//
// `scripts/check-derived-content.mjs` has always drawn this line and says why.
// These pin the same line here.

func TestPlaceholderCheck_QuotedJSONIsProse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		target string
	}{
		{
			name:   "a word translated inside a JSON literal",
			source: `emits {"decision":"block","reason":"…findings…"} (exit 0) when a gate fails`,
			target: `sender ut {"decision":"block","reason":"…funn…"} (avslutt 0) når en port feiler`,
		},
		{
			name:   "an example line the target does not repeat",
			source: "  kapi apply changeset.jsonl\n  echo '{\"kind\":\"comment\",\"lines\":{\"first\":3,\"last\":4}}' | kapi apply",
			target: "  kapi apply changeset.jsonl",
		},
		{
			name:   "prose inside braces",
			source: "a {pattern, format} pair",
			target: "et {mønster, format} par",
		},
		{
			name:   "a JSON shape carrying angle brackets",
			source: `{"socket": "<path>", "version": "<kapi version>"}`,
			target: `{"socket": "<sti>", "version": "<kapi versjon>"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, runPlaceholder(t, tc.source, tc.target, true),
				"quoted JSON in help text is prose, not a placeholder")
		})
	}
}

// The other half, and the one that must not move: narrowing what counts as a
// placeholder may not cost the check a single interpolation style it catches
// today. Every case here drops a real placeholder and must still be reported.
func TestPlaceholderCheck_GenuineStylesStillCaught(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		target string
		token  string
	}{
		{name: "braced name", source: "Add {name}", target: "Legg til", token: "{name}"},
		{name: "positional", source: "Item {0}", target: "Element", token: "{0}"},
		{name: "dotted", source: "{row.done} done", target: "ferdig", token: "{row.done}"},
		{name: "double brace", source: "Hi {{name}}", target: "Hei", token: "{{name}}"},
		{name: "go template", source: "Hi {{.WorkspaceName}}", target: "Hei", token: "{{.WorkspaceName}}"},
		{name: "shell style", source: "Hi ${name}", target: "Hei", token: "${name}"},
		{name: "printf verb", source: "read %s", target: "leste", token: "%s"},
		{name: "positional printf", source: "read %1$s", target: "leste", token: "%1$s"},
		{name: "python named", source: "read %(who)s", target: "leste", token: "%(who)s"},
		{name: "numbered tag", source: "a <0>link</0>", target: "en lenke", token: "<0>"},
		{name: "element marker", source: "Added {=m0}slug{/=m0}.", target: "Lagt til.", token: "{=m0}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			findings := runPlaceholder(t, tc.source, tc.target, true)
			require.NotEmpty(t, findings, "a dropped %s must still be reported", tc.token)

			var tokens []string
			for _, f := range findings {
				assert.Equal(t, check.SeverityCritical, f.Severity,
					"a dropped placeholder stays release-blocking")
				tokens = append(tokens, f.OriginalText)
			}
			assert.Contains(t, tokens, tc.token, "the finding names the token that went missing")
		})
	}
}
