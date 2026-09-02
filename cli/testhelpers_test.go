package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/leonelquinteros/gotext"
	"github.com/neokapi/neokapi/core/formats/mo"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

// Cobra-side copies of test fixtures whose canonical definitions moved to
// the host package with the runtime/service extraction.

// writeVerifyProject creates a temp project that binds a voice profile and a
// project terms store, with an English source file and a French target file. The
// returned root is the project directory; the target file is returned so the
// test can rewrite it for the passing case.
func writeVerifyProject(t *testing.T) (root, targetFile string) {
	t.Helper()
	// Hermetic: neutralise an inherited KAPI_NO_PROJECT (the in-repo dogfood
	// contract encourages devs to set it) so discovery finds the temp project
	// written below. An empty value does NOT disable discovery — only a
	// non-empty KAPI_NO_PROJECT does.
	t.Setenv("KAPI_NO_PROJECT", "")
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "fr"), 0o755))

	recipe := `version: v1
name: verify
defaults:
  source_language: en
  target_languages: [fr]
  voice:
    profile_file: voice.yaml
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(verifyVoiceYAML), 0o644))

	// Source: contains the competitor term "Globex" (brand fail) and a
	// {name} placeholder plus a ruled term "Save".
	src := `{
  "greeting": "Hello {name}, welcome to Globex!",
  "save": "Save"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "en", "app.json"), []byte(src), 0o644))

	// Target: drops the {name} placeholder (a check failure) and mistranslates
	// "Save" (terminology fail — the rule requires "Enregistrer").
	bad := `{
  "greeting": "Bonjour, bienvenue chez Globex!",
  "save": "Sauvegarder"
}
`
	targetFile = filepath.Join(root, "locales", "fr", "app.json")
	require.NoError(t, os.WriteFile(targetFile, []byte(bad), 0o644))

	seedProjectTerms(t, root)
	return root, targetFile
}

// writeStatusProject creates a temp project with a JSON catalog source, a
// partially-translated nb target (2 of 3 keys), no ja target, and ship gates
// (ja machine-ships, the default needs 80% review).
func writeStatusProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()

	recipe := `version: v1
name: status
defaults:
  source_language: en
  target_languages: [nb, ja]
collections:
  - path: en.json
    target: "{lang}.json"
ship_gates:
  - when: { locales: [ja] }
    gate: { translated: 100, reviewed: 0 }
  - gate: { translated: 100, reviewed: 80 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana","c":"Cherry"}`), 0o644))
	// nb has 2 of 3 keys → 67% translated.
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

func runStatusJSON(t *testing.T) StatusOutput {
	t.Helper()
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	out, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)
	var parsed StatusOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed), "status must emit valid JSON: %s", out)
	return parsed
}

func locale(o StatusOutput, loc string) (LocaleCoverage, bool) {
	for _, lc := range o.Locales {
		if lc.Locale == loc {
			return lc, true
		}
	}
	return LocaleCoverage{}, false
}

// writeReviewProject writes a project with a fully-translated nb target and a
// gate that needs 50% reviewed, plus a bound (initially absent) .memory.json source.
func writeReviewProject(t *testing.T) string {
	t.Helper()
	// Dogfood isolation contract (CLAUDE.md). Discovery stays ON — one test in
	// this family chdirs into the fixture and relies on the upward walk finding
	// its recipe — so every OTHER root a run could inherit is pinned instead:
	// the developer's ~/.config/kapi, the plugin roots, the shared caches.
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	root := t.TempDir()
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
  memory_source: memory.json
collections:
  - path: en.json
    target: "{lang}.json"
ship_gate: { translated: 100, reviewed: 50 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

// writeReviewedCorrection approves the unit whose source matches srcText (for nb)
// through the real state-store approval path — ApproveReviewUnit records the
// decision in the project state store, the authoritative carrier of review state.
// The target argument is ignored: approval blesses the translation already in the
// file. (Named for historical continuity with the prior .memory.json-based helper.)
func writeReviewedCorrection(t *testing.T, root, srcText, _ string) {
	t.Helper()
	a := &App{}
	proj := filepath.Join(root, "kapi.yaml")
	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	for _, it := range rep.Review {
		if it.Source == srcText {
			ok, err := a.ApproveReviewUnit(context.Background(), proj, "en", it.Locale, it.File, it.Key, "reviewed")
			require.NoError(t, err)
			require.True(t, ok)
			return
		}
	}
	t.Fatalf("no review unit with source %q in %v", srcText, rep.Review)
}

// makeMoCatalog compiles a tiny MO for the given (scope, source, target)
// triples using the real writer so the round-trip behaviour is tested
// end-to-end, not via the package-internal shortcut.
func makeMoCatalog(t *testing.T, locale string, entries ...[3]string) *gotext.Mo {
	t.Helper()
	w := mo.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(model.LocaleID(locale))

	parts := make(chan *model.Part, len(entries))
	for i, e := range entries {
		b := model.NewBlock(("tu" + iToA(i+1)), e[1])
		b.Name = e[0]
		b.SetTargetText(model.LocaleID(locale), e[2])
		parts <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(parts)
	require.NoError(t, w.Write(context.Background(), parts))

	m := gotext.NewMo()
	m.Parse(buf.Bytes())
	return m
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written along with fn's return value.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String(), runErr
}

// verifyVoiceYAML binds a voice profile with a critical competitor term, so a
// single occurrence in the source drops the compliance score below the default
// threshold (100 - 25 = 75 < 80) and fails the voice gate.
const verifyVoiceYAML = `name: Verify Brand
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: minor
  competitor_terms:
    - term: Globex
      replacement: our platform
      severity: critical
`

func iToA(i int) string { return strconv.Itoa(i) }
