package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/telemetry"
)

// telemetryTestTree builds a miniature kapi command tree: two built-in verbs
// (stats, tm>import), a plugin-attached verb (push), the excluded built-ins
// (help, completion, telemetry), and returns the tree plus the built-in verb
// set as kapi/cmd/kapi would snapshot it before plugin attach.
func telemetryTestTree() (root *cobra.Command, lookup map[string]*cobra.Command, builtins map[string]bool) {
	root = &cobra.Command{Use: "kapi"}
	stats := &cobra.Command{Use: "stats"}
	tm := &cobra.Command{Use: "tm"}
	tmImport := &cobra.Command{Use: "import"}
	tm.AddCommand(tmImport)
	help := &cobra.Command{Use: "help"}
	completion := &cobra.Command{Use: "completion"}
	telemetryCmd := &cobra.Command{Use: "telemetry"}
	telemetryOff := &cobra.Command{Use: "off"}
	telemetryCmd.AddCommand(telemetryOff)
	root.AddCommand(stats, tm, help, completion, telemetryCmd)

	builtins = make(map[string]bool)
	for _, c := range root.Commands() {
		builtins[c.Name()] = true
	}

	// Plugin command attaches after the built-in snapshot.
	push := &cobra.Command{Use: "push"}
	pushRemote := &cobra.Command{Use: "remote"}
	push.AddCommand(pushRemote)
	root.AddCommand(push)

	lookup = map[string]*cobra.Command{
		"stats":         stats,
		"tm.import":     tmImport,
		"push":          push,
		"push.remote":   pushRemote,
		"help":          help,
		"completion":    completion,
		"telemetry.off": telemetryOff,
	}
	return root, lookup, builtins
}

func TestTelemetryCommandLabel(t *testing.T) {
	root, lookup, builtins := telemetryTestTree()

	tests := []struct {
		name     string
		executed string // key into lookup; "" means root itself
		err      error
		want     string
	}{
		{name: "built-in top-level verb", executed: "stats", want: "stats"},
		{name: "built-in nested verb reports dotted path", executed: "tm.import", want: "tm.import"},
		{name: "plugin verb reports as plugin", executed: "push", want: "plugin"},
		{name: "plugin subcommand reports as plugin", executed: "push.remote", want: "plugin"},
		{name: "help is excluded", executed: "help", want: ""},
		{name: "completion is excluded", executed: "completion", want: ""},
		{name: "telemetry group is excluded", executed: "telemetry.off", want: ""},
		{name: "bare root", executed: "", want: "root"},
		{name: "unknown verb reports as other", executed: "",
			err:  errors.New(`unknown command "frobnicate" for "kapi"`),
			want: "other"},
		{name: "root error that is not unknown-command stays root", executed: "",
			err:  errors.New("boom"),
			want: "root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executed := root
			if tt.executed != "" {
				executed = lookup[tt.executed]
				require.NotNil(t, executed)
			}
			assert.Equal(t, tt.want, telemetryCommandLabel(root, executed, tt.err, builtins))
		})
	}
}

func TestTelemetryCommandLabelHelpFlag(t *testing.T) {
	root, lookup, builtins := telemetryTestTree()
	for _, c := range []*cobra.Command{root, lookup["stats"]} {
		c.InitDefaultHelpFlag()
		require.NoError(t, c.Flags().Set("help", "true"))
	}
	assert.Empty(t, telemetryCommandLabel(root, root, nil, builtins),
		"kapi --help must not report")
	assert.Empty(t, telemetryCommandLabel(root, lookup["stats"], nil, builtins),
		"kapi stats --help must not report")
}

func TestTelemetryCommandLabelBusyboxRoot(t *testing.T) {
	_, _, builtins := telemetryTestTree()
	builtins["kcat"] = true

	kcat := &cobra.Command{Use: "kcat"}
	assert.Equal(t, "kcat", telemetryCommandLabel(kcat, kcat, nil, builtins))

	stranger := &cobra.Command{Use: "not-ours"}
	assert.Equal(t, "other", telemetryCommandLabel(stranger, stranger, nil, builtins))

	assert.Empty(t, telemetryCommandLabel(nil, nil, nil, builtins))
}

func TestTelemetryExitClass(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{ExitOK, "ok"},
		{ExitUsage, "usage"},
		{ExitError, "error"},
		{ExitGate, "error"},
		{ExitSignal, "error"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, telemetryExitClass(tt.code), "code %d", tt.code)
	}
}

func TestMaybeShowTelemetryNoticeShownExactlyOnce(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	cfg := config.NewAppConfig()
	require.NoError(t, cfg.Load())

	var buf bytes.Buffer
	require.True(t, MaybeShowTelemetryNotice(cfg, &buf), "first call must show the notice")
	out := buf.String()
	assert.Contains(t, out, "anonymous usage telemetry")
	assert.Contains(t, out, "kapi telemetry off")
	assert.Contains(t, out, "KAPI_TELEMETRY=0")
	assert.Contains(t, out, "DO_NOT_TRACK")
	assert.Contains(t, out, telemetryDocsURL)
	assert.Contains(t, out, "Never:")

	buf.Reset()
	assert.False(t, MaybeShowTelemetryNotice(cfg, &buf), "second call must not show the notice")
	assert.Empty(t, buf.String())

	// The marker persists: a fresh config load still suppresses it.
	fresh := config.NewAppConfig()
	require.NoError(t, fresh.Load())
	buf.Reset()
	assert.False(t, MaybeShowTelemetryNotice(fresh, &buf))
	assert.Empty(t, buf.String())
}

// telemetryTestServer records every batch body it receives.
func telemetryTestServer(t *testing.T) (*httptest.Server, func() [][]byte) {
	t.Helper()
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)
	return srv, func() [][]byte {
		mu.Lock()
		defer mu.Unlock()
		return append([][]byte(nil), bodies...)
	}
}

// setupReporterEnv isolates config and neutralizes the env lattice so the
// reporter resolves as enabled (or as directed by the individual test).
func setupReporterEnv(t *testing.T, endpoint string) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(telemetry.EnvOptOut, "")
	t.Setenv(telemetry.EnvDoNotTrack, "")
	t.Setenv(telemetry.EnvCI, "")
	t.Setenv("KAPI_TELEMETRY_ENABLED", "")
	telemetry.Configure("phc_test", endpoint)
	t.Cleanup(func() { telemetry.Configure("", "") })
}

func TestTelemetryReporterOptedOutAttemptsNoHTTP(t *testing.T) {
	srv, bodies := telemetryTestServer(t)
	setupReporterEnv(t, srv.URL)
	t.Setenv(telemetry.EnvOptOut, "0")

	root, lookup, builtins := telemetryTestTree()
	report := CommandReport{
		Root:     root,
		Executed: lookup["stats"],
		ExitCode: ExitOK,
		Duration: 5 * time.Millisecond,
	}
	NewTelemetryReporter(&App{}, builtins)(report)

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, bodies(), "KAPI_TELEMETRY=0 must suppress every HTTP call")

	// And the notice must not have been marked as shown.
	cfg := config.NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.False(t, cfg.GetBool(telemetry.KeyNoticeShown))
}

func TestTelemetryReporterEmitsFirstRunThenCommandRun(t *testing.T) {
	srv, bodies := telemetryTestServer(t)
	setupReporterEnv(t, srv.URL)

	root, lookup, builtins := telemetryTestTree()
	reporter := NewTelemetryReporter(&App{}, builtins)

	// First eligible run: cli_first_run + cli_command_run.
	reporter(CommandReport{Root: root, Executed: lookup["tm.import"], ExitCode: ExitOK, Duration: 2 * time.Second})

	events := func() []string {
		var names []string
		for _, body := range bodies() {
			var payload struct {
				Batch []struct {
					Event      string         `json:"event"`
					Properties map[string]any `json:"properties"`
				} `json:"batch"`
			}
			require.NoError(t, json.Unmarshal(body, &payload))
			for _, ev := range payload.Batch {
				names = append(names, ev.Event)
				// Privacy invariants on every event.
				for _, banned := range []string{"args", "flags", "path", "project"} {
					assert.NotContains(t, ev.Properties, banned)
				}
			}
		}
		return names
	}

	require.Eventually(t, func() bool { return len(events()) == 2 }, 2*time.Second, 10*time.Millisecond)
	assert.ElementsMatch(t, []string{"cli_first_run", "cli_command_run"}, events())

	// Check the command_run payload.
	var payload struct {
		Batch []struct {
			Event      string         `json:"event"`
			Properties map[string]any `json:"properties"`
		} `json:"batch"`
	}
	all := bodies()
	require.NoError(t, json.Unmarshal(all[len(all)-1], &payload))
	for _, ev := range payload.Batch {
		if ev.Event != "cli_command_run" {
			continue
		}
		assert.Equal(t, "tm.import", ev.Properties["command"])
		assert.Equal(t, "<10s", ev.Properties["duration_bucket"])
		assert.Equal(t, "ok", ev.Properties["exit_class"])
		assert.Equal(t, "cli", ev.Properties["surface"])
	}

	// Second run: only cli_command_run — the notice (and cli_first_run) never repeat.
	reporter(CommandReport{Root: root, Executed: lookup["stats"], ExitCode: ExitError, Duration: time.Millisecond})
	require.Eventually(t, func() bool { return len(events()) == 3 }, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, countString(events(), "cli_first_run"))
	assert.Equal(t, 2, countString(events(), "cli_command_run"))
}

func TestTelemetryReporterSkipsExcludedCommands(t *testing.T) {
	srv, bodies := telemetryTestServer(t)
	setupReporterEnv(t, srv.URL)

	root, lookup, builtins := telemetryTestTree()
	reporter := NewTelemetryReporter(&App{}, builtins)
	for _, name := range []string{"help", "completion", "telemetry.off"} {
		reporter(CommandReport{Root: root, Executed: lookup[name], ExitCode: ExitOK})
	}

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, bodies(), "help/completion/telemetry must emit nothing")

	cfg := config.NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.False(t, cfg.GetBool(telemetry.KeyNoticeShown),
		"excluded commands must not consume the first-run notice")
}

func countString(list []string, want string) int {
	n := 0
	for _, s := range list {
		if s == want {
			n++
		}
	}
	return n
}

func TestTelemetryOnOffStatusCommands(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(telemetry.EnvOptOut, "")
	t.Setenv(telemetry.EnvDoNotTrack, "")
	t.Setenv(telemetry.EnvCI, "")
	t.Setenv("KAPI_TELEMETRY_ENABLED", "")
	telemetry.Configure("phc_test", "")
	t.Cleanup(func() { telemetry.Configure("", "") })

	run := func(args ...string) string {
		var out bytes.Buffer
		cmd := NewTelemetryCmd(&App{})
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
		return out.String()
	}

	assert.Contains(t, run("status"), "Telemetry: enabled")

	assert.Contains(t, run("off"), "disabled")
	assert.Contains(t, run("status"), telemetry.ReasonConfigOff)

	assert.Contains(t, run("on"), "enabled")
	status := run("status")
	assert.Contains(t, status, "Telemetry: enabled")
	assert.Contains(t, status, "cli_command_run")
	assert.Contains(t, status, "Never sent")
}
