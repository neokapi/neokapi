package project

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// parseRecipe decodes a recipe body without validation — these tests are about
// what ExecSurface sees in the shape, not about whether the recipe is loadable.
func parseRecipe(t *testing.T, body string) *KapiProject {
	t.Helper()
	var p KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(body), &p))
	return &p
}

// TestExecSurfaceEmptyForOrdinaryRecipes: the gate must be invisible to the
// recipes people actually write. Every recipe in this repository is one of
// these, so a non-empty surface here would mean prompting on the dogfood
// project.
func TestExecSurfaceEmptyForOrdinaryRecipes(t *testing.T) {
	cases := map[string]string{
		"bare": "version: v2\n",
		"content and flows": `version: v2
defaults:
  source_language: en
  target_languages: [nb, de]
collections:
  - path: docs/**/*.md
flows:
  default:
    steps:
      - tool: translate
      - tool: term-check
`,
		"tool presets": `version: v2
defaults:
  tools:
    translate:
      model: some-model
  locales:
    nb:
      tools:
        redact:
          entities: true
`,
		"named format": `version: v2
collections:
  - path: app/**/*.json
    format:
      name: i18next
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, ExecSurface(parseRecipe(t, body)))
			assert.Empty(t, ExecSurfaceDigest(ExecSurface(parseRecipe(t, body))))
		})
	}
	assert.Empty(t, ExecSurface(nil), "a nil recipe has no surface")
}

// TestExecSurfaceFindsEveryArm: a step that runs code must be found wherever it
// is written, including the places a reader skims past — a nested parallel
// branch, the retired source_transforms stage, a per-locale tool preset, and a
// format binding on a single content item.
func TestExecSurfaceFindsEveryArm(t *testing.T) {
	sites := ExecSurface(parseRecipe(t, `version: v2
flows:
  default:
    steps:
      - tool: translate
      - tool: external-command
        config:
          command: /bin/sh
          args: ["-c", "id"]
      - parallel:
          - tool: term-check
          - tool: script
            config:
              script: "1 + 1"
    source_transforms:
      - tool: external-command
        config:
          command: /usr/bin/tr
defaults:
  tools:
    external-command:
      command: /bin/echo
  locales:
    nb:
      tools:
        script:
          scriptFile: ./hook.js
collections:
  - path: a/**
    format:
      name: exec
      config:
        command: /bin/cat
    content:
      - path: b.txt
        format:
          name: exec
`))

	var where []string
	for _, s := range sites {
		where = append(where, s.Where)
	}
	assert.ElementsMatch(t, []string{
		"flows.default.steps[1]",
		"flows.default.steps[2].parallel[1]",
		"flows.default.source_transforms[0]",
		"defaults.tools.external-command",
		"defaults.locales.nb.tools.script",
		"collections[0].format",
		"collections[0].content[0].format",
	}, where)
}

// TestExecSurfaceDetailNamesWhatWouldRun: the prompt's whole value is that a
// person can judge the argv rather than the tool's name, so the summary has to
// carry it.
func TestExecSurfaceDetailNamesWhatWouldRun(t *testing.T) {
	sites := ExecSurface(parseRecipe(t, `version: v2
flows:
  default:
    steps:
      - tool: external-command
        config:
          command: /bin/sh
          args: ["-c", "whoami"]
`))
	require.Len(t, sites, 1)
	assert.Contains(t, sites[0].Detail, "command=/bin/sh")
	assert.Contains(t, sites[0].Detail, "whoami")
}

// TestExecSurfaceDetailStripsTerminalControls: the detail is attacker-authored
// text on its way to a terminal. Escape sequences in it could erase or repaint
// the prompt that is asking about it, so no control character survives.
func TestExecSurfaceDetailStripsTerminalControls(t *testing.T) {
	p := &KapiProject{
		Defaults: Defaults{Tools: map[string]map[string]any{
			"external-command": {"command": "/bin/sh\x1b[2K\rkapi: trusted project\n"},
		}},
	}
	sites := ExecSurface(p)
	require.Len(t, sites, 1)
	assert.NotContains(t, sites[0].Detail, "\x1b")
	assert.NotContains(t, sites[0].Detail, "\r")
	assert.NotContains(t, sites[0].Detail, "\n")
	assert.Contains(t, sites[0].Detail, "/bin/sh", "the readable part survives")
}

// TestExecSurfaceDetailIsClipped: a recipe cannot flood the terminal to push
// the question out of view.
func TestExecSurfaceDetailIsClipped(t *testing.T) {
	p := &KapiProject{
		Defaults: Defaults{Tools: map[string]map[string]any{
			"external-command": {"command": strings.Repeat("a", 5000)},
		}},
	}
	sites := ExecSurface(p)
	require.Len(t, sites, 1)
	assert.Less(t, len([]rune(sites[0].Detail)), 300)
	assert.Contains(t, sites[0].Detail, "…")
}

// TestExecSurfaceDigestTracksTheArgvNotTheRecipe is the property the stored
// decision depends on. Editing the parts of a recipe people edit daily must
// keep a decision valid; editing what would run must invalidate it.
func TestExecSurfaceDigestTracksTheArgvNotTheRecipe(t *testing.T) {
	const base = `version: v2
defaults:
  target_languages: [nb]
flows:
  default:
    steps:
      - tool: external-command
        config:
          command: /bin/echo
          args: ["hello"]
`
	digestOf := func(body string) string {
		return ExecSurfaceDigest(ExecSurface(parseRecipe(t, body)))
	}
	baseDigest := digestOf(base)
	require.NotEmpty(t, baseDigest)

	t.Run("stable across runs", func(t *testing.T) {
		assert.Equal(t, baseDigest, digestOf(base))
	})

	t.Run("unchanged by an unrelated edit", func(t *testing.T) {
		assert.Equal(t, baseDigest, digestOf(strings.Replace(
			base, "target_languages: [nb]", "target_languages: [nb, de, fr]", 1)))
	})

	t.Run("unchanged by config key order", func(t *testing.T) {
		assert.Equal(t, baseDigest, digestOf(`version: v2
defaults:
  target_languages: [nb]
flows:
  default:
    steps:
      - tool: external-command
        config:
          args: ["hello"]
          command: /bin/echo
`))
	})

	t.Run("changed by a new argument", func(t *testing.T) {
		assert.NotEqual(t, baseDigest, digestOf(strings.Replace(
			base, `args: ["hello"]`, `args: ["hello", "; curl example.invalid"]`, 1)))
	})

	t.Run("changed by a different command", func(t *testing.T) {
		assert.NotEqual(t, baseDigest, digestOf(strings.Replace(
			base, "command: /bin/echo", "command: /bin/sh", 1)))
	})

	t.Run("changed by an added step", func(t *testing.T) {
		assert.NotEqual(t, baseDigest, digestOf(base+`      - tool: script
        config:
          script: "0"
`))
	})
}
