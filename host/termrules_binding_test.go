package host

import (
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/schema"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTermRulesReachEveryGovernedTool closes the loop the config key sits in:
// the binder writes it, and each governed tool's config factory reads it back.
//
// The two halves live in different modules and are joined only by a string.
// When they disagree, nothing fails — the tool decodes a config with no rules,
// finds no violations, and reports a pass. A project's terminology simply stops
// being enforced, and the gate that exists to say so says nothing. That is what
// happened when `glossary` became `preferred_terms` and one producer's map
// literal did not match the pattern the rename swept with.
//
// So this test names the key exactly once — through the tools that own it —
// and asserts the rules survive the round trip for every tool the binder feeds.
func TestTermRulesReachEveryGovernedTool(t *testing.T) {
	t.Parallel()

	rules := []coreprofile.TermRule{{Term: "Save", Replacement: "Enregistrer"}}
	b := &ProjectBindings{termRules: rules}

	for _, tc := range []struct {
		tool  string
		req   string
		rules func(map[string]any) ([]coreprofile.TermRule, error)
	}{
		{
			tool: "term-check", req: schema.RequiresTerms,
			rules: func(cfg map[string]any) ([]coreprofile.TermRule, error) {
				var c coretools.TermCheckConfig
				err := schema.ApplyConfig(cfg, &c)
				return c.TermRules, err
			},
		},
		{
			tool: "recycle",
			rules: func(cfg map[string]any) ([]coreprofile.TermRule, error) {
				var c coretools.MemoryLeverageConfig
				err := schema.ApplyConfig(cfg, &c)
				return c.TermRules, err
			},
		},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			s := &schema.ComponentSchema{ToolMeta: &schema.ToolMeta{ID: tc.tool}}
			if tc.req != "" {
				s.ToolMeta.Requires = []string{tc.req}
			}

			config := (&App{}).applyBindings(b, tc.tool, s, map[string]any{})
			require.NotEmpty(t, config, "the binder must inject the project's term rules")

			got, err := tc.rules(config)
			require.NoError(t, err, "the tool must decode the key the binder wrote")
			assert.Equal(t, rules, got,
				"%s decoded no rules from a config the binder filled — the key drifted on one side", tc.tool)
		})
	}
}
