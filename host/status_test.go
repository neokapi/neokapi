package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// writeCollectionProject creates a project with two named collections (docs, ui)
// and a collection-scoped gate: docs requires nothing (always shippable), ui
// requires 100% translated. Both sources are untranslated.
func writeCollectionProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "ui"), 0o755))

	recipe := `version: v1
name: coll
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: docs
    content:
      - path: docs/a.md
        target: "{lang}/docs/a.md"
  - name: ui
    content:
      - path: ui/b.json
        target: "{lang}/ui/b.json"
ship_gates:
  - when: { collections: [docs] }
    gate: { translated: 0 }
  - gate: { translated: 100 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("# Title\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ui", "b.json"), []byte(`{"k":"Save"}`), 0o644))
	return root
}

func scope(o StatusOutput, locale, collection string) (LocaleCoverage, bool) {
	for _, lc := range o.Locales {
		if lc.Locale == locale && lc.Collection == collection {
			return lc, true
		}
	}
	return LocaleCoverage{}, false
}

func TestStatus_CollectionScopedGates(t *testing.T) {
	t.Chdir(writeCollectionProject(t))
	out := runStatusJSON(t)

	docs, ok := scope(out, "nb", "docs")
	require.True(t, ok, "expected a nb/docs row")
	assert.True(t, docs.Shippable, "docs gate requires nothing → shippable even untranslated")

	ui, ok := scope(out, "nb", "ui")
	require.True(t, ok, "expected a nb/ui row")
	assert.False(t, ui.Shippable, "ui gate requires 100% translated")
}

// writeSourceGateProject creates a project with a source_gate. Its source is a
// JSON catalog — a format with nowhere to write a per-block source status, so
// the rung it reports can only come from settling the source, never from what
// the file carries.
func writeSourceGateProject(t *testing.T, sourceGate string) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: src
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - path: en.json
    target: "{lang}.json"
` + sourceGate + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana","c":"Cherry"}`), 0o644))
	return root
}

// TestStatus_SourceReadiness: the source axis reports the rung the source
// actually settles at, on a format that cannot carry a per-block status in its
// own bytes. Reading `SourceStatus` off a freshly parsed JSON catalog reports
// the presence baseline for every block, so `kapi status` printed `checked 0%`
// seconds after the same run's settle line said every block was checked — a
// number contradicted by its own sibling, and a `source_gate` above `authored`
// that could never pass on a catalog format at all.
func TestStatus_SourceReadiness(t *testing.T) {
	t.Chdir(writeSourceGateProject(t, "source_gate: { checked: 100 }"))
	out := runStatusJSON(t)

	require.NotNil(t, out.Source, "source readiness must be reported")
	assert.Equal(t, 3, out.Source.Total)
	assert.Equal(t, 100, out.Source.Pct["authored"], "all source present → authored baseline")
	assert.Equal(t, 100, out.Source.Pct["checked"],
		"clean source settles to checked; the catalog has nowhere to carry the stamp, so it must be derived")
	assert.Equal(t, 0, out.Source.Pct["approved"], "approval is authored, never derived from content")
	assert.True(t, out.Source.Gated)
	assert.True(t, out.Source.Shippable, "checked:100 is met once the clean source settles")
}

// TestStatus_SourceReadiness_ApprovalIsNotDerived: settling reaches `checked`
// and stops. `approved` is somebody's decision, so a gate that asks for it stays
// pending however clean the source is.
func TestStatus_SourceReadiness_ApprovalIsNotDerived(t *testing.T) {
	t.Chdir(writeSourceGateProject(t, "source_gate: { approved: 100 }"))
	out := runStatusJSON(t)

	require.NotNil(t, out.Source)
	assert.Equal(t, 100, out.Source.Pct["checked"])
	assert.False(t, out.Source.Shippable, "approved:100 is unmet by a settle")
}

func TestStatus_SourceReadiness_AuthoredGateClears(t *testing.T) {
	// An {authored: 100} gate is satisfied by the presence baseline.
	t.Chdir(writeSourceGateProject(t, "source_gate: { authored: 100 }"))
	out := runStatusJSON(t)
	require.NotNil(t, out.Source)
	assert.True(t, out.Source.Shippable, "authored:100 is met when all source is present")
}

func TestVerify_SourceGate(t *testing.T) {
	t.Chdir(writeSourceGateProject(t, "source_gate: { approved: 100 }"))

	// Without --ship: the source gate is not evaluated (source drift is non-blocking).
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	out, _ := captureStdout(t, func() error { return a.RunVerify(cmd, nil) })
	var parsed verifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.False(t, hasGate(parsed, gateSource), "source gate must be opt-in (--ship)")

	// With --ship: the source gate runs and fails (the source settles to
	// `checked`; nobody has approved it).
	a2 := &App{}
	cmd2 := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd2)
	AddVerifyFlags(cmd2)
	require.NoError(t, cmd2.Flags().Set("json", "true"))
	require.NoError(t, cmd2.Flags().Set("ship", "true"))
	out2, runErr := captureStdout(t, func() error { return a2.RunVerify(cmd2, nil) })
	var parsed2 verifyOutput
	require.NoError(t, json.Unmarshal([]byte(out2), &parsed2))
	sg, has := findGate(parsed2, gateSource)
	require.True(t, has, "--ship adds the source gate")
	assert.False(t, sg.Pass, "source is below approved:100")
	assert.NotEmpty(t, sg.Findings)
	assert.Error(t, runErr, "a failed source gate exits non-zero")
}

func hasGate(out verifyOutput, name string) bool {
	_, ok := findGate(out, name)
	return ok
}

func findGate(out verifyOutput, name string) (verifyGateResult, bool) {
	for _, g := range out.Gates {
		if g.Gate == name {
			return g, true
		}
	}
	return verifyGateResult{}, false
}

// TestStatus_UnreadableTargetFallsBackToPresence: a content target whose format
// is write-only (a compiled .mo catalog) cannot be read back to measure, so
// status must not crash — it counts the materialized target by presence and
// still reports the other collections.
func TestStatus_UnreadableTargetFallsBackToPresence(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: mo
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - path: en.json
    target: "{lang}.mo"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	// A compiled .mo target exists but is not readable through the pipeline.
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.mo"), []byte("\xde\x12\x04\x95compiled"), 0o644))

	t.Chdir(root)
	out := runStatusJSON(t) // must not error
	nb, ok := localeCoverage(out, "nb")
	require.True(t, ok)
	assert.Equal(t, 2, nb.Total)
	assert.Equal(t, 100, nb.Pct["translated"], "a present but unreadable target counts by presence")
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

func localeCoverage(o StatusOutput, loc string) (LocaleCoverage, bool) {
	for _, lc := range o.Locales {
		if lc.Locale == loc {
			return lc, true
		}
	}
	return LocaleCoverage{}, false
}

func TestStatus_Coverage(t *testing.T) {
	t.Chdir(writeStatusProject(t))
	out := runStatusJSON(t)

	nb, ok := localeCoverage(out, "nb")
	require.True(t, ok)
	assert.Equal(t, 3, nb.Total)
	assert.Equal(t, 67, nb.Pct["translated"], "2 of 3 keys translated")
	assert.True(t, nb.Gated)
	assert.False(t, nb.Shippable, "default gate needs 100% translated + 80% reviewed")

	ja, ok := localeCoverage(out, "ja")
	require.True(t, ok)
	assert.Equal(t, 3, ja.Total)
	assert.Equal(t, 0, ja.Pct["translated"], "no ja file yet")
	assert.True(t, ja.Gated, "ja matches its locale rule")
	assert.False(t, ja.Shippable)
}

// writeVerifiedGateProject builds a project where everything ships (a trivial
// ship gate) but only fully-translated locales are verified (verified_gate:
// {translated: 100}). nb is fully translated (verified); de is 2 of 3
// (shippable but unverified — the AI case). Using `translated` as the verified
// bar keeps the test to file-scan coverage — the evaluation mechanism is what
// matters, and it is identical to the ship gate's.
func writeVerifiedGateProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: verified
defaults:
  source_language: en
  target_languages: [nb, de]
collections:
  - path: en.json
    target: "{lang}.json"
ship_gate: { translated: 0 }
verified_gate: { translated: 100 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana","c":"Cherry"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan","c":"Kirsebær"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "de.json"),
		[]byte(`{"a":"Apfel","b":"Banane"}`), 0o644))
	return root
}

func TestStatus_VerifiedGate(t *testing.T) {
	t.Chdir(writeVerifiedGateProject(t))
	out := runStatusJSON(t)

	nb, ok := localeCoverage(out, "nb")
	require.True(t, ok)
	assert.True(t, nb.Shippable, "trivial ship gate → shippable")
	assert.True(t, nb.Verified, "fully translated clears verified_gate: {translated: 100}")

	de, ok := localeCoverage(out, "de")
	require.True(t, ok)
	assert.True(t, de.Shippable, "ships under the translated:0 gate")
	assert.False(t, de.Verified, "2 of 3 translated → not verified (the AI case)")
}

func TestStatus_NoVerifiedGate_NothingVerified(t *testing.T) {
	// The default: writeStatusProject declares no verified gate, so every locale
	// reads unverified regardless of coverage.
	t.Chdir(writeStatusProject(t))
	out := runStatusJSON(t)
	require.NotEmpty(t, out.Locales)
	for _, lc := range out.Locales {
		assert.False(t, lc.Verified, "%s: no verified gate configured → unverified", lc.Locale)
	}
}

func TestStatus_ShipManifestEmit(t *testing.T) {
	root := writeVerifiedGateProject(t)
	t.Chdir(root)
	shipPath := filepath.Join(root, "ship.json")

	a := &App{}
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("ship", "true"))
	require.NoError(t, cmd.Flags().Set("emit", shipPath))
	_, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)

	raw, err := os.ReadFile(shipPath)
	require.NoError(t, err)
	var manifest map[string]ShipEntry
	require.NoError(t, json.Unmarshal(raw, &manifest), "ship.json must be valid JSON: %s", raw)

	assert.Equal(t, ShipEntry{Shippable: true, Verified: true}, manifest["nb"])
	assert.Equal(t, ShipEntry{Shippable: true, Verified: false}, manifest["de"],
		"shippable but unverified — the AI case")
}

func TestStatus_ShipManifestStdout(t *testing.T) {
	// Without --emit, --ship prints the minimal manifest to stdout so a build can
	// redirect it into ship.json.
	t.Chdir(writeVerifiedGateProject(t))
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("ship", "true"))
	out, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)
	var manifest map[string]ShipEntry
	require.NoError(t, json.Unmarshal([]byte(out), &manifest), "stdout must be the ship manifest: %s", out)
	assert.True(t, manifest["nb"].Verified)
	assert.False(t, manifest["de"].Verified)
}

func shipGate(out verifyOutput) (verifyGateResult, bool) {
	for _, g := range out.Gates {
		if g.Gate == gateShip {
			return g, true
		}
	}
	return verifyGateResult{}, false
}

func TestVerify_ShipGate(t *testing.T) {
	t.Chdir(writeStatusProject(t))

	// Without --ship: no ship gate is evaluated (drift is non-blocking here).
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	out, _ := captureStdout(t, func() error { return a.RunVerify(cmd, nil) })
	var parsed verifyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	_, has := shipGate(parsed)
	assert.False(t, has, "ship gate must be opt-in (--ship)")

	// With --ship: the ship gate runs and fails (nb/ja are below their gates).
	a2 := &App{}
	cmd2 := NewEnvCommand(context.Background(), "verify")
	AddProjectFlag(cmd2)
	AddVerifyFlags(cmd2)
	require.NoError(t, cmd2.Flags().Set("json", "true"))
	require.NoError(t, cmd2.Flags().Set("ship", "true"))
	out2, runErr := captureStdout(t, func() error { return a2.RunVerify(cmd2, nil) })
	var parsed2 verifyOutput
	require.NoError(t, json.Unmarshal([]byte(out2), &parsed2))
	sg, has := shipGate(parsed2)
	require.True(t, has, "--ship adds the ship gate")
	assert.False(t, sg.Pass, "nb/ja are not shippable")
	assert.NotEmpty(t, sg.Findings)
	assert.Error(t, runErr, "a failed ship gate exits non-zero (the pre-release bar)")
}

func TestStatus_NeverFails(t *testing.T) {
	// status is informational: a behind locale is reported, not an error.
	t.Chdir(writeStatusProject(t))
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	_, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	assert.NoError(t, err, "status must never return a non-nil error for drift")
}

// A recipe with a venue block reports the effective convergence venue —
// degraded to local (with a note) when no plugin provides the server-up
// plumbing. A plain local recipe reports no venue at all: nothing ambiguous.
func TestStatus_VenueLine(t *testing.T) {
	registerVenue(t)

	root := writeStatusProject(t)
	recipePath := filepath.Join(root, "kapi.yaml")
	recipe, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	withVenue := string(recipe) + "\nbowrain:\n  url: https://bowrain.example/acme/demo\n  converge: on-push\n"
	require.NoError(t, os.WriteFile(recipePath, []byte(withVenue), 0o644))

	t.Chdir(root)
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))

	out, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)

	var got StatusOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.NotNil(t, got.Venue, "a venue-bound recipe must report its venue")
	assert.Equal(t, "local", got.Venue.Venue, "without the server-up plumbing the venue degrades to local")
	assert.Equal(t, "on-push", got.Venue.ConvergePolicy)
	assert.Contains(t, got.Venue.Note, "plugin not installed")
}

func TestStatus_NoVenueWithoutServer(t *testing.T) {
	t.Chdir(writeStatusProject(t))
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("json", "true"))

	out, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)

	var got StatusOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Nil(t, got.Venue, "a plain local project has no venue ambiguity to report")
}
