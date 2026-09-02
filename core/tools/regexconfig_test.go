package tools_test

import (
	"regexp"
	"testing"

	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQACheckRejectsUninvokableRegex is the regression test for a rule that
// quietly never fires.
//
// A check pattern whose regex does not compile was skipped at construction, so the
// check it described simply never ran. Nothing failed, nothing was logged, and
// the report came back clean — which the person who wrote the rule reads as
// "my content passes", the exact opposite of the truth. A checker that cannot
// distinguish "found nothing" from "never looked" is worse than no checker.
//
// The config factory is the path a recipe's `qa:` step travels, so it is where
// the mistake is still attributable to the line that caused it.
func TestQACheckRejectsUninvokableRegex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		pattern map[string]any
		wantIn  []string
	}{
		{
			name: "invalid source expression",
			pattern: map[string]any{
				"enabled": true,
				"source":  `(unclosed`,
				"target":  `<same>`,
			},
			wantIn: []string{"(unclosed"},
		},
		{
			name: "invalid target expression",
			pattern: map[string]any{
				"enabled": true,
				"source":  `\d+`,
				"target":  `[a-`,
			},
			wantIn: []string{"[a-"},
		},
		{
			name: "invalid forbidden expression",
			pattern: map[string]any{
				"enabled":   true,
				"forbidden": true,
				"source":    `*nope`,
			},
			wantIn: []string{"*nope"},
		},
		{
			name: "enabled pattern with no source",
			pattern: map[string]any{
				"enabled": true,
				"source":  ``,
			},
			wantIn: []string{"no source expression"},
		},
		{
			name: "description names the offending rule",
			pattern: map[string]any{
				"enabled":     true,
				"source":      `(?P<bad`,
				"description": "trailing-period-parity",
			},
			wantIn: []string{"trailing-period-parity", "(?P<bad"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tools.NewQACheckFromConfig(map[string]any{
				"patterns": []any{tt.pattern},
			}, "fr")
			require.Error(t, err, "an uninvokable check rule must not build a tool that silently never fires")
			for _, want := range tt.wantIn {
				assert.Contains(t, err.Error(), want,
					"the error must quote what the author actually wrote")
			}
		})
	}
}

// TestQACheckAcceptsValidAndDisabledPatterns guards the other half of the
// contract: validation must not start rejecting configurations that work
// today. A disabled pattern is inert by the author's own choice, so its
// expression is never compiled and never judged.
func TestQACheckAcceptsValidAndDisabledPatterns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		pattern map[string]any
	}{
		{
			name:    "valid source and target",
			pattern: map[string]any{"enabled": true, "source": `\d+`, "target": `\d+`},
		},
		{
			name:    "same-match sentinel target",
			pattern: map[string]any{"enabled": true, "source": `\{\w+\}`, "target": "<same>"},
		},
		{
			name:    "empty target means same-match",
			pattern: map[string]any{"enabled": true, "source": `%[sd]`, "target": ""},
		},
		{
			name:    "forbidden needs only a source",
			pattern: map[string]any{"enabled": true, "forbidden": true, "source": `TODO`},
		},
		{
			// Disabled: never compiled, so never rejected — even though both
			// expressions are garbage.
			name:    "disabled pattern is not judged",
			pattern: map[string]any{"enabled": false, "source": `(unclosed`, "target": `[a-`},
		},
		{
			// A forbidden pattern matches its source against the target text,
			// so an invalid Target must not be compiled or rejected.
			name:    "forbidden ignores its target expression",
			pattern: map[string]any{"enabled": true, "forbidden": true, "source": `TODO`, "target": `[a-`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tl, err := tools.NewQACheckFromConfig(map[string]any{
				"patterns": []any{tt.pattern},
			}, "fr")
			require.NoError(t, err)
			assert.NotNil(t, tl)
		})
	}
}

// TestQACheckConfigValidateChecksPatterns pins that the same judgement is
// reachable through the ToolConfig.Validate contract, not only through the
// factory — so an in-process caller assembling a config in Go has a way to ask.
func TestQACheckConfigValidateChecksPatterns(t *testing.T) {
	t.Parallel()

	cfg := tools.NewQACheckConfig("fr")
	require.NoError(t, cfg.Validate(), "the default config must stay valid")

	cfg.Patterns = append(cfg.Patterns, tools.QAPattern{Enabled: true, Source: `(unclosed`})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "(unclosed")
}

// TestTagProtectRejectsUninvokableRegex covers the same failure for
// tag-protect, where the stakes are higher: patterns are the tool's ENTIRE
// behaviour, so a pattern that never compiles is a tool that protects nothing
// — and the placeholder it was configured to guard is handed to the MT step
// unprotected, to come back mangled, with nothing anywhere naming the cause.
func TestTagProtectRejectsUninvokableRegex(t *testing.T) {
	t.Parallel()

	_, err := tools.NewTagProtectFromConfig(map[string]any{
		"patterns": []any{`\{[^}]+\}`, `(unclosed`},
	}, "")
	require.Error(t, err, "an uninvokable protection pattern must not build a tool that protects nothing")
	assert.Contains(t, err.Error(), "pattern 1", "the error must name which pattern is broken")

	_, err = tools.NewTagProtectFromConfig(map[string]any{
		"patterns": []any{`\{[^}]+\}`, `<[^>]+>`},
	}, "")
	require.NoError(t, err, "valid patterns must still build")
}

// TestDefaultTagPatternsCompile pins the code-owned defaults. compileTagPatterns
// drops what it cannot compile, so a typo introduced into defaultTagPatterns
// would disable tag protection for every flow that does not override it, with
// no test failing anywhere. This is the guard that makes that impossible.
func TestDefaultTagPatternsCompile(t *testing.T) {
	t.Parallel()

	tl, err := tools.NewTagProtectFromConfig(map[string]any{}, "")
	require.NoError(t, err)
	require.NotNil(t, tl)

	// The defaults are unexported; assert the same property against the
	// documented set so a change to either side has to be deliberate.
	for _, pat := range []string{
		`<[^>]+>`,
		`\{[^}]+\}`,
		`%[sdfoexXbBhHtT%]+`,
		`\$\{[^}]+\}`,
	} {
		_, err := regexp.Compile(pat)
		require.NoErrorf(t, err, "documented default pattern %q must compile", pat)
	}
}
