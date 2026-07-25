package conformance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/plugin/conformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Fixture plumbing ────────────────────────────────────────────────────────

// Fixture binaries are compiled once for the whole package: `go build` of the
// probe daemon costs more than every check in this file combined, and the
// binaries are read-only once built.
var (
	probeBin      string
	helloBin      string
	helloManifest []byte
)

const (
	probePkg = "./core/plugin/conformance/testdata/probedaemon"
	helloPkg = "./examples/plugins/hello"
)

func TestMain(m *testing.M) {
	root, err := findModuleRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "conformance tests:", err)
		os.Exit(1)
	}
	buildDir, err := os.MkdirTemp("", "kapi-conformance-build-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "conformance tests: build dir:", err)
		os.Exit(1)
	}

	// No `defer` for the cleanup: os.Exit below would skip it. Every exit path
	// removes the build dir explicitly instead.
	fail := func(err error) {
		fmt.Fprintln(os.Stderr, "conformance tests:", err)
		_ = os.RemoveAll(buildDir)
		os.Exit(1)
	}

	ctx := context.Background()
	if probeBin, err = build(ctx, root, buildDir, probePkg, "kapi-probe"); err != nil {
		fail(err)
	}
	if helloBin, err = build(ctx, root, buildDir, helloPkg, "kapi-hello"); err != nil {
		fail(err)
	}
	if helloManifest, err = os.ReadFile(
		filepath.Join(root, "examples", "plugins", "hello", "manifest.json"),
	); err != nil {
		fail(fmt.Errorf("read hello manifest: %w", err))
	}

	code := m.Run()
	_ = os.RemoveAll(buildDir)
	os.Exit(code)
}

func build(ctx context.Context, root, outDir, pkg, name string) (string, error) {
	out := filepath.Join(outDir, name)
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, pkg)
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build %s: %w\n%s", pkg, err, combined)
	}
	return out, nil
}

// findModuleRoot walks up from the working directory to the framework module
// root, so `go build ./examples/...` resolves regardless of where tests run.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// pluginDir stages an install directory named for the plugin, containing the
// manifest and the binary under the manifest's declared name.
func pluginDir(t *testing.T, plugin string, man map[string]any, binSrc string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, plugin)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	if binSrc != "" {
		binName, _ := man["binary"].(string)
		require.NotEmpty(t, binName, "manifest must declare a binary to stage one")
		data, err := os.ReadFile(binSrc)
		require.NoError(t, err)
		dst := filepath.Join(dir, filepath.FromSlash(binName))
		require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
		require.NoError(t, os.WriteFile(dst, data, 0o755))
	}
	writeManifest(t, dir, man)
	return dir
}

func writeManifest(t *testing.T, dir string, man map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(man, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600))
}

// probeManifest is a Mode-C manifest for the probe daemon fixture.
func probeManifest() map[string]any {
	return map[string]any{
		"manifest_version": "1",
		"plugin":           "probe",
		"version":          "1.0.0",
		"binary":           "kapi-probe",
		"license":          "Apache-2.0",
		"capabilities": map[string]any{
			"commands": []any{
				map[string]any{"name": "echo", "short": "Echo the arguments"},
			},
			"formats": []any{
				map[string]any{
					"name":         "probefmt",
					"display_name": "Probe format",
					"extensions":   []any{".probe"},
					"capabilities": []any{"read"},
				},
			},
			"segmenters": []any{
				map[string]any{"name": "probeseg", "display_name": "Probe segmenter"},
			},
			"selfcheck": true,
		},
		"daemon": map[string]any{
			"idle_timeout_seconds":    120,
			"startup_timeout_seconds": 20,
			"handshake": map[string]any{
				"type":   "stdio-handshake",
				"fields": []any{"socket", "version"},
			},
		},
	}
}

func probeDir(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	man := probeManifest()
	if mutate != nil {
		mutate(man)
	}
	plugin, _ := man["plugin"].(string)
	return pluginDir(t, plugin, man, probeBin)
}

// suite is the common Suite for fixture runs: short timeouts keep the negative
// paths (which wait out a deadline on purpose) fast.
func suite(dir string) conformance.Suite {
	return conformance.Suite{Dir: dir, Timeout: 20 * time.Second}
}

func run(t *testing.T, s conformance.Suite) *conformance.Report {
	t.Helper()
	rep, err := conformance.Run(context.Background(), s)
	require.NoError(t, err)
	t.Logf("\n%s", rep.Text())
	return rep
}

// result fetches one check result, failing the test when the check did not run.
func result(t *testing.T, rep *conformance.Report, id string) conformance.Result {
	t.Helper()
	res, ok := rep.Result(id)
	require.Truef(t, ok, "check %q is absent from the report", id)
	return res
}

func requireStatus(t *testing.T, rep *conformance.Report, id string, want conformance.Status) conformance.Result {
	t.Helper()
	res := result(t, rep, id)
	assert.Equalf(t, want, res.Status, "check %q: %s", id, res.Detail)
	return res
}

// ── Catalogue ───────────────────────────────────────────────────────────────

func TestChecksCatalogue(t *testing.T) {
	checks := conformance.Checks()
	require.NotEmpty(t, checks)

	seen := map[string]bool{}
	for _, c := range checks {
		assert.NotEmpty(t, c.ID, "every check needs an ID")
		assert.NotEmpty(t, c.Title, "check %q needs a title", c.ID)
		assert.NotEmpty(t, c.Why, "check %q needs a Why line so a failure is actionable", c.ID)
		assert.Containsf(t, c.ID, ".", "check %q must be namespaced <group>.<name>", c.ID)
		assert.Falsef(t, seen[c.ID], "duplicate check ID %q", c.ID)
		seen[c.ID] = true
	}

	// Group order is load-bearing: manifest parsing feeds everything, and the
	// Mode-C teardown checks stop the shared daemon.
	var groups []string
	for _, c := range checks {
		if g := c.Group(); len(groups) == 0 || groups[len(groups)-1] != g {
			groups = append(groups, g)
		}
	}
	assert.Equal(t, []string{"manifest", "binary", "modeA", "modeB", "modeC"}, groups)

	last := checks[len(checks)-1]
	assert.Equal(t, "modeC.sigterm-exit", last.ID, "daemon teardown must be the final check")
}

func TestRunRequiresDir(t *testing.T) {
	_, err := conformance.Run(context.Background(), conformance.Suite{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Dir is required")
}

// ── The in-repo reference plugin (Mode A + B) ───────────────────────────────

// helloDir stages examples/plugins/hello, the repository's own reference
// plugin. If the reference plugin ever stops satisfying the published protocol,
// this test is the thing that says so.
func helloDir(t *testing.T) string {
	t.Helper()
	var man map[string]any
	require.NoError(t, json.Unmarshal(helloManifest, &man))
	return pluginDir(t, "hello", man, helloBin)
}

func TestReferencePluginIsConformant(t *testing.T) {
	rep := run(t, suite(helloDir(t)))

	assert.Truef(t, rep.OK(), "the reference plugin must be conformant:\n%s", rep.Text())
	require.NoError(t, rep.Err())
	assert.Equal(t, "hello", rep.Plugin)
	assert.Equal(t, []conformance.Mode{conformance.ModeA, conformance.ModeB}, rep.Modes)

	// Mode A + B exercised; Mode C skipped because nothing declares it.
	requireStatus(t, rep, "manifest.parse", conformance.Pass)
	requireStatus(t, rep, "binary.version", conformance.Pass)
	requireStatus(t, rep, "binary.version-matches-manifest", conformance.Pass)
	requireStatus(t, rep, "binary.unknown-verb-rejected", conformance.Pass)
	requireStatus(t, rep, "modeA.unknown-command-rejected", conformance.Pass)
	requireStatus(t, rep, "modeB.initialize", conformance.Pass)
	requireStatus(t, rep, "modeB.tools-list", conformance.Pass)
	requireStatus(t, rep, "modeB.stdin-close-exits", conformance.Pass)

	skipped := requireStatus(t, rep, "modeC.handshake", conformance.Skip)
	assert.Contains(t, skipped.Detail, "declares no formats")
	skipped = requireStatus(t, rep, "binary.doctor", conformance.Skip)
	assert.Contains(t, skipped.Detail, "selfcheck")
}

func TestReferencePluginCommandProbe(t *testing.T) {
	s := suite(helloDir(t))
	s.CommandProbe = &conformance.CommandProbe{
		Command:    "hello",
		Args:       []string{"conformance"},
		WantExit:   0,
		WantStdout: "Hello, conformance!",
	}
	rep := run(t, s)
	requireStatus(t, rep, "modeA.command-probe", conformance.Pass)
	assert.True(t, rep.OK())
}

func TestCommandProbeRejectsUndeclaredCommand(t *testing.T) {
	s := suite(helloDir(t))
	s.CommandProbe = &conformance.CommandProbe{Command: "not-declared"}
	rep := run(t, s)
	res := requireStatus(t, rep, "modeA.command-probe", conformance.Fail)
	assert.Contains(t, res.Detail, "the manifest does not declare")
	assert.False(t, rep.OK())
}

func TestCommandProbeReportsWrongExitCode(t *testing.T) {
	s := suite(helloDir(t))
	s.CommandProbe = &conformance.CommandProbe{Command: "hello", WantExit: 3}
	rep := run(t, s)
	res := requireStatus(t, rep, "modeA.command-probe", conformance.Fail)
	assert.Contains(t, res.Detail, "exited 0, want 3")
}

// ── Manifest checks ─────────────────────────────────────────────────────────

func TestManifestMissing(t *testing.T) {
	dir := t.TempDir()
	rep := run(t, suite(dir))

	requireStatus(t, rep, "manifest.present", conformance.Fail)
	// Every downstream check skips rather than piling on with the same cause.
	requireStatus(t, rep, "manifest.name-matches-dir", conformance.Skip)
	requireStatus(t, rep, "binary.version", conformance.Skip)
	assert.False(t, rep.OK())
	assert.Len(t, rep.Failures(), 1, "a missing manifest is one failure, not many")
}

func TestManifestUnparseable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "broken")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{not json"), 0o600))

	rep := run(t, suite(dir))
	requireStatus(t, rep, "manifest.present", conformance.Pass)
	requireStatus(t, rep, "manifest.parse", conformance.Fail)
	assert.Empty(t, rep.Plugin)
}

func TestManifestUnknownField(t *testing.T) {
	dir := probeDir(t, func(m map[string]any) { m["capabilties"] = map[string]any{} })
	rep := run(t, conformance.Suite{Dir: dir, Timeout: 20 * time.Second, Only: []string{"manifest"}})

	res := requireStatus(t, rep, "manifest.no-unknown-fields", conformance.Fail)
	assert.Contains(t, res.Detail, "capabilties")
	// A typo'd key still parses, so the rest of the manifest group is unaffected.
	requireStatus(t, rep, "manifest.parse", conformance.Pass)
}

func TestManifestNameMustMatchDir(t *testing.T) {
	man := probeManifest()
	// Stage under a directory name that disagrees with the manifest.
	dir := pluginDir(t, "not-probe", man, probeBin)
	rep := run(t, conformance.Suite{Dir: dir, Timeout: 20 * time.Second, Only: []string{"manifest"}})

	res := requireStatus(t, rep, "manifest.name-matches-dir", conformance.Fail)
	assert.Contains(t, res.Detail, `"probe"`)
	assert.Contains(t, res.Detail, `"not-probe"`)
}

func TestManifestBinaryMustExistAndBeExecutable(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		man := probeManifest()
		dir := pluginDir(t, "probe", man, "")
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second})
		requireStatus(t, rep, "manifest.binary-present", conformance.Fail)
		requireStatus(t, rep, "binary.version", conformance.Skip)
	})

	t.Run("not executable", func(t *testing.T) {
		man := probeManifest()
		dir := pluginDir(t, "probe", man, "")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi-probe"), []byte("#!/bin/sh\n"), 0o644))
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.binary-present", conformance.Fail)
		assert.Contains(t, res.Detail, "not executable")
	})

	t.Run("escapes the plugin dir", func(t *testing.T) {
		man := probeManifest()
		man["binary"] = "../outside/kapi-probe"
		dir := pluginDir(t, "probe", man, "")
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.binary-present", conformance.Fail)
		assert.Contains(t, res.Detail, "escapes the plugin directory")
	})
}

func TestManifestDaemonBlockConsistency(t *testing.T) {
	t.Run("mode C without a daemon block", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) { delete(m, "daemon") })
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.daemon-block", conformance.Fail)
		assert.Contains(t, res.Detail, "no daemon block")
	})

	t.Run("daemon block without mode C", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			m["capabilities"] = map[string]any{
				"commands": []any{map[string]any{"name": "echo"}},
			}
		})
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.daemon-block", conformance.Fail)
		assert.Contains(t, res.Detail, "no format, flow tool, segmenter, or source connector")
	})

	t.Run("unsupported handshake type", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			m["daemon"].(map[string]any)["handshake"] = map[string]any{"type": "tcp-port"}
		})
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.daemon-block", conformance.Fail)
		assert.Contains(t, res.Detail, "tcp-port")
	})

	t.Run("handshake fields omit socket", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			m["daemon"].(map[string]any)["handshake"] = map[string]any{
				"type": "stdio-handshake", "fields": []any{"version"},
			}
		})
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.daemon-block", conformance.Fail)
		assert.Contains(t, res.Detail, "socket")
	})
}

func TestManifestSchemaFiles(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			caps := m["capabilities"].(map[string]any)
			caps["formats"] = []any{map[string]any{
				"name": "probefmt", "schema": "schemas/probefmt.json",
			}}
		})
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.schema-files", conformance.Fail)
		assert.Contains(t, res.Detail, "formats[probefmt]")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			caps := m["capabilities"].(map[string]any)
			caps["formats"] = []any{map[string]any{
				"name": "probefmt", "schema": "schemas/probefmt.json",
			}}
		})
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas", "probefmt.json"), []byte("{oops"), 0o600))
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.schema-files", conformance.Fail)
		assert.Contains(t, res.Detail, "invalid JSON")
	})

	t.Run("valid", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			caps := m["capabilities"].(map[string]any)
			caps["formats"] = []any{map[string]any{
				"name": "probefmt", "schema": "schemas/probefmt.json",
			}}
		})
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas", "probefmt.json"),
			[]byte(`{"type":"object"}`), 0o600))
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		requireStatus(t, rep, "manifest.schema-files", conformance.Pass)
	})
}

func TestManifestModelsMustBePinned(t *testing.T) {
	t.Run("unpinned digest", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			m["models"] = []any{map[string]any{
				"id": "probe-model", "version": "1", "default": true,
				"files": []any{map[string]any{
					"path": "weights.bin",
					"url":  "https://example.invalid/weights.bin",
					// A short/uppercase digest is the realistic authoring slip.
					"sha256": "ABC123",
				}},
			}}
		})
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.models", conformance.Fail)
		assert.Contains(t, res.Detail, "64-character lowercase hex")
	})

	t.Run("non-https url", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			m["models"] = []any{map[string]any{
				"id": "probe-model", "version": "1", "default": true,
				"files": []any{map[string]any{
					"path":   "weights.bin",
					"url":    "http://example.invalid/weights.bin",
					"sha256": strings.Repeat("a", 64),
				}},
			}}
		})
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.models", conformance.Fail)
		assert.Contains(t, res.Detail, "https")
	})

	t.Run("bundled file must exist", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			m["models"] = []any{map[string]any{
				"id": "probe-model", "version": "1", "bundled": true, "default": true,
				"files": []any{map[string]any{"path": "weights.bin"}},
			}}
		})
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		res := requireStatus(t, rep, "manifest.models", conformance.Fail)
		assert.Contains(t, res.Detail, "bundled file missing")
	})

	t.Run("pinned and bundled", func(t *testing.T) {
		dir := probeDir(t, func(m map[string]any) {
			m["models"] = []any{
				map[string]any{
					"id": "downloaded", "version": "1", "default": true,
					"files": []any{map[string]any{
						"path":   "weights.bin",
						"url":    "https://example.invalid/rev/abc/weights.bin",
						"sha256": strings.Repeat("a", 64),
					}},
				},
				map[string]any{
					"id": "bundled", "version": "1", "bundled": true,
					"files": []any{map[string]any{"path": "bundled.bin"}},
				},
			}
		})
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bundled.bin"), []byte("x"), 0o600))
		rep := run(t, conformance.Suite{Dir: dir, Timeout: 5 * time.Second, Only: []string{"manifest"}})
		requireStatus(t, rep, "manifest.models", conformance.Pass)
	})
}

// ── Binary verbs ────────────────────────────────────────────────────────────

func TestBinaryVersionMismatchIsAdvisory(t *testing.T) {
	dir := probeDir(t, nil)
	s := suite(dir)
	s.Only = []string{"binary", "manifest"}
	s.Env = []string{"PROBE_VERSION=9.9.9"}

	rep := run(t, s)
	res := requireStatus(t, rep, "binary.version-matches-manifest", conformance.Fail)
	assert.False(t, res.Required, "a version mismatch is advisory, not disqualifying")
	assert.Contains(t, res.Detail, "9.9.9")
	assert.True(t, rep.OK(), "an advisory failure must not clear conformance")
	assert.Len(t, rep.Advisories(), 1)
}

func TestBinaryCatchAllVerbIsRejected(t *testing.T) {
	dir := probeDir(t, nil)
	s := suite(dir)
	s.Only = []string{"binary"}
	s.Env = []string{"PROBE_UNKNOWN_VERB_OK=1"}

	rep := run(t, s)
	res := requireStatus(t, rep, "binary.unknown-verb-rejected", conformance.Fail)
	assert.True(t, res.Required)
	assert.Contains(t, res.Detail, "exited 0")
}

func TestModeACatchAllCommandIsRejected(t *testing.T) {
	dir := probeDir(t, nil)
	s := suite(dir)
	s.Only = []string{"modeA"}
	s.Env = []string{"PROBE_ANY_COMMAND_OK=1"}

	rep := run(t, s)
	requireStatus(t, rep, "modeA.unknown-command-rejected", conformance.Fail)
}

// ── Filtering ───────────────────────────────────────────────────────────────

func TestOnlyOmitsAndSkipReports(t *testing.T) {
	dir := helloDir(t)

	only := run(t, conformance.Suite{Dir: dir, Timeout: 20 * time.Second, Only: []string{"manifest.parse"}})
	require.Len(t, only.Results, 1)
	assert.Equal(t, "manifest.parse", only.Results[0].ID)

	skipped := run(t, conformance.Suite{
		Dir: dir, Timeout: 20 * time.Second,
		Only: []string{"manifest"}, Skip: []string{"manifest.models"},
	})
	res := requireStatus(t, skipped, "manifest.models", conformance.Skip)
	assert.Contains(t, res.Detail, "Suite.Skip")
}

func TestLogfReceivesOneLinePerCheck(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	s := conformance.Suite{
		Dir: helloDir(t), Timeout: 20 * time.Second, Only: []string{"manifest"},
		Logf: func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			lines = append(lines, format)
		},
	}
	rep, err := conformance.Run(context.Background(), s)
	require.NoError(t, err)
	assert.Len(t, lines, len(rep.Results))
}

// ── Report rendering ────────────────────────────────────────────────────────

func TestReportTextAndJSON(t *testing.T) {
	rep := run(t, suite(helloDir(t)))

	text := rep.Text()
	assert.Contains(t, text, "kapi plugin protocol v"+conformance.ProtocolVersion)
	assert.Contains(t, text, "plugin:  hello 0.1.0")
	assert.Contains(t, text, "CONFORMANT")
	assert.Contains(t, text, "modeB.tools-list")

	data, err := rep.JSON()
	require.NoError(t, err)

	var decoded struct {
		ProtocolVersion string   `json:"protocol_version"`
		Plugin          string   `json:"plugin"`
		Version         string   `json:"version"`
		Modes           []string `json:"modes"`
		Conformant      bool     `json:"conformant"`
		Summary         struct {
			Passed, Failed, Advisory, Skipped, Total int
		} `json:"summary"`
		Results []struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Required bool   `json:"required"`
			Why      string `json:"why"`
			Detail   string `json:"detail"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, conformance.ProtocolVersion, decoded.ProtocolVersion)
	assert.Equal(t, "hello", decoded.Plugin)
	assert.Equal(t, []string{"A", "B"}, decoded.Modes)
	assert.True(t, decoded.Conformant)
	assert.Equal(t, len(rep.Results), decoded.Summary.Total)
	assert.Equal(t, 0, decoded.Summary.Failed)
	require.NotEmpty(t, decoded.Results)
	for _, r := range decoded.Results {
		assert.NotEmpty(t, r.ID)
		assert.NotEmpty(t, r.Status)
		assert.NotEmpty(t, r.Detail, "check %q must always explain itself", r.ID)
		assert.NotEmpty(t, r.Why, "check %q must carry its Why into the artifact", r.ID)
	}
}

func TestReportErrSummarisesFailures(t *testing.T) {
	rep := run(t, suite(t.TempDir()))
	err := rep.Err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin protocol v"+conformance.ProtocolVersion)
	assert.Contains(t, err.Error(), "manifest.present")
}

// ── RunT adapter ────────────────────────────────────────────────────────────

// recordingT captures what RunT would have reported to a *testing.T.
type recordingT struct {
	mu      sync.Mutex
	errors  []string
	logs    []string
	helpers int
}

func (r *recordingT) Helper() { r.mu.Lock(); r.helpers++; r.mu.Unlock() }
func (r *recordingT) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, strings.TrimSpace(fmt.Sprintf(format, args...)))
}
func (r *recordingT) Logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

func TestRunTPassesForConformantPlugin(t *testing.T) {
	rt := &recordingT{}
	rep := conformance.RunT(rt, suite(helloDir(t)))
	require.NotNil(t, rep)
	assert.Empty(t, rt.errors, "a conformant plugin must not fail the test")
	assert.NotEmpty(t, rt.logs, "the transcript is always logged")
	assert.Positive(t, rt.helpers)
}

func TestRunTReportsOneErrorPerRequiredFailure(t *testing.T) {
	rt := &recordingT{}
	rep := conformance.RunT(rt, suite(t.TempDir()))
	require.NotNil(t, rep)
	assert.Len(t, rt.errors, len(rep.Failures()))
	assert.Contains(t, strings.Join(rt.errors, "\n"), "manifest.present")
}

func TestRunTReportsAnUnrunnableSuite(t *testing.T) {
	rt := &recordingT{}
	rep := conformance.RunT(rt, conformance.Suite{})
	assert.Nil(t, rep)
	require.Len(t, rt.errors, 1)
	assert.Contains(t, rt.errors[0], "could not run")
}

// ── Mode C ──────────────────────────────────────────────────────────────────

func skipUnlessModeCSupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Mode C is a Unix-socket transport")
	}
}

func TestModeCDaemonIsConformant(t *testing.T) {
	skipUnlessModeCSupported(t)
	rep := run(t, suite(probeDir(t, nil)))

	assert.Truef(t, rep.OK(), "the probe daemon must be conformant:\n%s", rep.Text())
	assert.Equal(t,
		[]conformance.Mode{conformance.ModeA, conformance.ModeC},
		rep.Modes)

	requireStatus(t, rep, "binary.doctor", conformance.Pass)
	requireStatus(t, rep, "modeC.handshake", conformance.Pass)
	requireStatus(t, rep, "modeC.socket-listening", conformance.Pass)
	requireStatus(t, rep, "modeC.socket-private", conformance.Pass)
	requireStatus(t, rep, "modeC.grpc-ready", conformance.Pass)
	requireStatus(t, rep, "modeC.bridge-service", conformance.Pass)
	requireStatus(t, rep, "modeC.segment-rpc", conformance.Pass)
	requireStatus(t, rep, "modeC.shutdown-rpc", conformance.Pass)
	requireStatus(t, rep, "modeC.sigterm-exit", conformance.Pass)

	// Mode B is not declared, so it is skipped rather than failed.
	requireStatus(t, rep, "modeB.initialize", conformance.Skip)
	// The format probe is opt-in.
	res := requireStatus(t, rep, "modeC.format-probe", conformance.Skip)
	assert.Contains(t, res.Detail, "FormatProbe")
}

func TestModeCHandshakeFailures(t *testing.T) {
	skipUnlessModeCSupported(t)

	cases := []struct {
		name    string
		env     string
		wantSub string
	}{
		{"no handshake at all", "PROBE_NO_HANDSHAKE=1", "no handshake line within"},
		{"non-JSON first line", "PROBE_BAD_HANDSHAKE=1", "parse handshake"},
		{"missing socket field", "PROBE_NO_SOCKET_FIELD=1", `omits "socket"`},
		{"relative socket path", "PROBE_RELATIVE_SOCKET=1", "not an absolute path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := conformance.Suite{
				Dir:     probeDir(t, nil),
				Timeout: 4 * time.Second, // the no-handshake case waits this out
				Only:    []string{"modeC"},
				Env:     []string{tc.env},
			}
			rep := run(t, s)
			res := requireStatus(t, rep, "modeC.handshake", conformance.Fail)
			assert.Contains(t, res.Detail, tc.wantSub)

			// Every dependent Mode-C check skips: one root cause, one failure.
			for _, id := range []string{
				"modeC.socket-listening", "modeC.grpc-ready",
				"modeC.bridge-service", "modeC.sigterm-exit",
			} {
				requireStatus(t, rep, id, conformance.Skip)
			}
			assert.Len(t, rep.Failures(), 1)
		})
	}
}

func TestModeCUnregisteredServiceFails(t *testing.T) {
	skipUnlessModeCSupported(t)
	s := suite(probeDir(t, nil))
	s.Only = []string{"modeC"}
	s.Env = []string{"PROBE_NO_SERVICE=1"}

	rep := run(t, s)
	requireStatus(t, rep, "modeC.handshake", conformance.Pass)
	requireStatus(t, rep, "modeC.grpc-ready", conformance.Pass)
	res := requireStatus(t, rep, "modeC.bridge-service", conformance.Fail)
	assert.Contains(t, res.Detail, "Unimplemented")
	// The daemon still has to die on SIGTERM.
	requireStatus(t, rep, "modeC.sigterm-exit", conformance.Pass)
}

func TestModeCLooseSocketIsAdvisory(t *testing.T) {
	skipUnlessModeCSupported(t)
	s := suite(probeDir(t, nil))
	s.Only = []string{"modeC"}
	s.Env = []string{"PROBE_LOOSE_SOCKET=1"}

	rep := run(t, s)
	res := requireStatus(t, rep, "modeC.socket-private", conformance.Fail)
	assert.False(t, res.Required)
	assert.Contains(t, res.Detail, "group/other")
	assert.True(t, rep.OK(), "a permissive socket is advisory, not disqualifying")
}

func TestModeCMissingShutdownIsAdvisoryButSigtermIsNot(t *testing.T) {
	skipUnlessModeCSupported(t)

	t.Run("no Shutdown RPC", func(t *testing.T) {
		s := suite(probeDir(t, nil))
		s.Only = []string{"modeC"}
		s.Env = []string{"PROBE_NO_SHUTDOWN=1"}

		rep := run(t, s)
		res := requireStatus(t, rep, "modeC.shutdown-rpc", conformance.Fail)
		assert.False(t, res.Required)
		// SIGTERM still works, so the plugin stays conformant.
		requireStatus(t, rep, "modeC.sigterm-exit", conformance.Pass)
		assert.True(t, rep.OK())
	})

	t.Run("ignores SIGTERM", func(t *testing.T) {
		s := conformance.Suite{
			Dir:     probeDir(t, nil),
			Timeout: 4 * time.Second,
			Only:    []string{"modeC"},
			Env:     []string{"PROBE_NO_SHUTDOWN=1", "PROBE_IGNORE_SIGTERM=1"},
		}
		rep := run(t, s)
		res := requireStatus(t, rep, "modeC.sigterm-exit", conformance.Fail)
		assert.True(t, res.Required)
		assert.Contains(t, res.Detail, "still running")
		assert.False(t, rep.OK())
	})
}

func TestModeCLeakedSocketFails(t *testing.T) {
	skipUnlessModeCSupported(t)
	s := suite(probeDir(t, nil))
	s.Only = []string{"modeC"}
	s.Env = []string{"PROBE_NO_SHUTDOWN=1", "PROBE_LEAK_SOCKET=1"}

	rep := run(t, s)
	res := requireStatus(t, rep, "modeC.sigterm-exit", conformance.Fail)
	assert.Contains(t, res.Detail, "left")
}

func TestModeCFormatProbe(t *testing.T) {
	skipUnlessModeCSupported(t)

	t.Run("reads the staged document", func(t *testing.T) {
		s := suite(probeDir(t, nil))
		s.FormatProbe = &conformance.FormatProbe{
			Format:    "probefmt",
			Input:     []byte("first line\nsecond line\nthird line\n"),
			Filename:  "sample.probe",
			MinBlocks: 3,
		}
		rep := run(t, s)
		res := requireStatus(t, rep, "modeC.format-probe", conformance.Pass)
		assert.Contains(t, res.Detail, "3 translatable block")
		assert.True(t, rep.OK())
	})

	t.Run("too few blocks", func(t *testing.T) {
		s := suite(probeDir(t, nil))
		s.FormatProbe = &conformance.FormatProbe{
			Format:    "probefmt",
			Input:     []byte("only one line\n"),
			MinBlocks: 5,
		}
		rep := run(t, s)
		res := requireStatus(t, rep, "modeC.format-probe", conformance.Fail)
		assert.Contains(t, res.Detail, "want at least 5")
	})

	t.Run("undeclared format", func(t *testing.T) {
		s := suite(probeDir(t, nil))
		s.FormatProbe = &conformance.FormatProbe{Format: "nope", Input: []byte("x")}
		rep := run(t, s)
		res := requireStatus(t, rep, "modeC.format-probe", conformance.Fail)
		assert.Contains(t, res.Detail, "the manifest does not declare")
	})
}

func TestModeCSegmentProbe(t *testing.T) {
	skipUnlessModeCSupported(t)

	t.Run("returns boundaries", func(t *testing.T) {
		s := suite(probeDir(t, nil))
		s.SegmentProbe = &conformance.SegmentProbe{
			Engine:        "probeseg",
			Text:          "One. Two. Three.",
			MinBoundaries: 2,
		}
		rep := run(t, s)
		res := requireStatus(t, rep, "modeC.segment-rpc", conformance.Pass)
		assert.Contains(t, res.Detail, "2 boundary")
	})

	t.Run("engine error fails a probed run", func(t *testing.T) {
		s := suite(probeDir(t, nil))
		s.Only = []string{"modeC"}
		s.Env = []string{"PROBE_SEGMENT_ERROR=1"}
		s.SegmentProbe = &conformance.SegmentProbe{Engine: "probeseg", Text: "One. Two."}
		rep := run(t, s)
		res := requireStatus(t, rep, "modeC.segment-rpc", conformance.Fail)
		assert.Contains(t, res.Detail, "PROBE_SEGMENT_ERROR")
	})

	t.Run("engine error only reports reachability without a probe", func(t *testing.T) {
		s := suite(probeDir(t, nil))
		s.Only = []string{"modeC"}
		s.Env = []string{"PROBE_SEGMENT_ERROR=1"}
		rep := run(t, s)
		res := requireStatus(t, rep, "modeC.segment-rpc", conformance.Pass)
		assert.Contains(t, res.Detail, "reachable")
	})

	t.Run("undeclared engine", func(t *testing.T) {
		s := suite(probeDir(t, nil))
		s.SegmentProbe = &conformance.SegmentProbe{Engine: "nope"}
		rep := run(t, s)
		res := requireStatus(t, rep, "modeC.segment-rpc", conformance.Fail)
		assert.Contains(t, res.Detail, "the manifest does not declare")
	})
}

func TestModeCSkippedWhenNotDeclared(t *testing.T) {
	dir := probeDir(t, func(m map[string]any) {
		m["capabilities"] = map[string]any{
			"commands": []any{map[string]any{"name": "echo"}},
		}
		delete(m, "daemon")
	})
	rep := run(t, suite(dir))
	for _, id := range []string{
		"modeC.handshake", "modeC.grpc-ready", "modeC.bridge-service",
		"modeC.segment-rpc", "modeC.shutdown-rpc", "modeC.sigterm-exit",
	} {
		requireStatus(t, rep, id, conformance.Skip)
	}
	assert.True(t, rep.OK())
}

// TestNoDaemonSurvivesTheRun asserts the suite always reaps the daemon it
// spawned, including on the paths where a check returns early.
func TestNoDaemonSurvivesTheRun(t *testing.T) {
	skipUnlessModeCSupported(t)
	before := probeDaemonPIDs(t)

	// A run that stops before the teardown checks: Only excludes them.
	s := suite(probeDir(t, nil))
	s.Only = []string{"modeC.handshake", "modeC.grpc-ready"}
	rep := run(t, s)
	requireStatus(t, rep, "modeC.handshake", conformance.Pass)

	// The pool is torn down by runner.cleanup when Run returns.
	assert.Eventually(t, func() bool {
		return len(probeDaemonPIDs(t)) <= len(before)
	}, 10*time.Second, 200*time.Millisecond, "Run must not leave a daemon behind")
}

// probeDaemonPIDs lists the running probe-daemon processes, so a leak is
// observable rather than inferred.
func probeDaemonPIDs(t *testing.T) []string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "pgrep", "-f", "kapi-probe daemon").Output()
	if err != nil {
		// pgrep exits 1 when nothing matches, which is the common case.
		return nil
	}
	var pids []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			pids = append(pids, line)
		}
	}
	return pids
}
