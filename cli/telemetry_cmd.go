package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/telemetry"
)

// telemetryDocsURL is the transparency page listing every event, the privacy
// invariants, and all opt-out mechanisms.
const telemetryDocsURL = "https://neokapi.github.io/reference/telemetry"

// telemetryFlushWait bounds the best-effort flush on the exit path. Telemetry
// may never delay process exit beyond this.
const telemetryFlushWait = 100 * time.Millisecond

// NewTelemetryCmd builds the `kapi telemetry` command group: status shows the
// resolved state (and why), on/off persist the choice in the global config
// file. The events themselves are documented at telemetryDocsURL.
func NewTelemetryCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Show or change anonymous usage telemetry (status, on, off)",
		Long: `kapi collects anonymous usage telemetry to guide development: the name of the
built-in command that ran, a coarse duration bucket, the exit class, the kapi
version, and the operating system and architecture, keyed by a random
anonymous ID. It never collects arguments, flag values, file paths, file
contents, or project names.

Telemetry is disabled by any of: 'kapi telemetry off', KAPI_TELEMETRY=0,
DO_NOT_TRACK=1, a CI environment, a build made with -tags notelemetry, or a
build without a telemetry key (any build you compile yourself).

See ` + telemetryDocsURL + ` for the full event list.`,
		Example: "  kapi telemetry status\n  kapi telemetry off",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newTelemetryStatusCmd(a), newTelemetryOnCmd(a), newTelemetryOffCmd(a))
	return cmd
}

func newTelemetryStatusCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether telemetry is enabled, why, and the exact payload shape",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := telemetryConfig(a)
			st := telemetry.Resolve(cfg)
			out := cmd.OutOrStdout()
			if st.Enabled {
				fmt.Fprintln(out, "Telemetry: enabled")
			} else {
				fmt.Fprintf(out, "Telemetry: disabled, %s\n", st.Reason)
			}
			id := cfg.GetString(telemetry.KeyMachineID)
			if id == "" {
				id = "(generated on first eligible command)"
			}
			fmt.Fprintf(out, "Anonymous ID: %s\n", id)
			fmt.Fprint(out, `Events: cli_first_run (once), cli_command_run
Payload shape (cli_command_run):
  {
    "command":         "<built-in verb, or plugin/other>",
    "duration_bucket": "<100ms | <1s | <10s | <60s | >=60s",
    "exit_class":      "ok | error | usage",
    "app_version":     "<kapi version>",
    "os":              "<GOOS>", "arch": "<GOARCH>", "surface": "cli"
  }
Never sent: arguments, flag values, file paths, file contents, project names.
Opt out: kapi telemetry off, KAPI_TELEMETRY=0, DO_NOT_TRACK=1, CI, or a
notelemetry build. Details: `+telemetryDocsURL+"\n")
			return nil
		},
	}
}

func newTelemetryOnCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "on",
		Short: "Enable anonymous usage telemetry (persists to the global config file)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.SetGlobalConfig(telemetry.KeyEnabled, "true"); err != nil {
				return fmt.Errorf("enable telemetry: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Telemetry enabled. Environment switches (KAPI_TELEMETRY=0, DO_NOT_TRACK, CI) still take precedence.")
			return nil
		},
	}
}

func newTelemetryOffCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Disable anonymous usage telemetry (persists to the global config file)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.SetGlobalConfig(telemetry.KeyEnabled, "false"); err != nil {
				return fmt.Errorf("disable telemetry: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Telemetry disabled.")
			return nil
		},
	}
}

// telemetryConfig returns the loaded app config, falling back to a fresh
// load when the App hasn't initialized one (e.g. the command failed before
// PersistentPreRunE ran).
func telemetryConfig(a *App) *config.AppConfig {
	if a != nil && a.Config != nil {
		return a.Config
	}
	cfg := config.NewAppConfig()
	_ = cfg.Load()
	return cfg
}

// NewTelemetryReporter returns the CommandReporter that kapi/cmd/kapi wires
// into Run: after each top-level command completes it shows the one-time
// first-run notice (when eligible) and emits cli_command_run, then flushes
// best-effort within telemetryFlushWait. builtins is the set of top-level
// verbs registered by KapiCommandSet, captured BEFORE plugin commands attach —
// anything outside it reports as "plugin"/"other", never by name.
func NewTelemetryReporter(a *App, builtins map[string]bool) func(CommandReport) {
	return func(r CommandReport) {
		label := telemetryCommandLabel(r.Root, r.Executed, r.Err, builtins)
		if label == "" {
			return // help, completion, or the telemetry group itself
		}
		cfg := telemetryConfig(a)
		client := telemetry.New(cfg, telemetry.Options{
			Surface: "cli",
			Version: version.Version,
		})
		if client == nil {
			return // disabled by the opt-out lattice: no notice, no events
		}
		if MaybeShowTelemetryNotice(cfg, os.Stderr) {
			client.Capture("cli_first_run", nil)
		}
		client.Capture("cli_command_run", map[string]any{
			"command":         label,
			"duration_bucket": telemetry.DurationBucket(r.Duration),
			"exit_class":      telemetryExitClass(r.ExitCode),
		})
		client.Close(telemetryFlushWait)
	}
}

// telemetryExitClass buckets a process exit code into ok/usage/error. Gate
// failures, silent exits, and signals all class as "error" — the code
// itself is never reported.
func telemetryExitClass(code int) string {
	switch code {
	case ExitOK:
		return "ok"
	case ExitUsage:
		return "usage"
	default:
		return "error"
	}
}

// telemetryCommandLabel sanitizes what cli_command_run reports as "command".
// Only built-in verbs are ever named: the dotted command path (e.g.
// "memory.import") for a command whose top-level verb is in builtins, "plugin"
// for plugin-attached verbs, "other" for unknown verbs, "root" for a bare
// invocation. An empty return means "emit nothing" (help, shell completion,
// and the telemetry group itself).
func telemetryCommandLabel(root, executed *cobra.Command, err error, builtins map[string]bool) string {
	if root == nil {
		return ""
	}
	// A --help/-h invocation of any command renders help and exits: not an
	// eligible run — no notice, no event (cobra marks the flag Changed).
	if executed != nil {
		if f := executed.Flags().Lookup("help"); f != nil && f.Changed {
			return ""
		}
	}
	if executed == nil || executed == root {
		// A verb that matched no command never reaches a subcommand: cobra
		// reports it as an unknown-command error on the root.
		if err != nil && strings.HasPrefix(err.Error(), "unknown command") {
			return "other"
		}
		if root.Name() != "kapi" {
			// Multi-call (busybox) roots: kcat/kgrep/… are built-in toolbox
			// verbs; anything else is not ours to name.
			if builtins[root.Name()] {
				return root.Name()
			}
			return "other"
		}
		return "root"
	}

	// Find the top-level verb under root.
	top := executed
	for top.Parent() != nil && top.Parent() != root {
		top = top.Parent()
	}
	switch top.Name() {
	case "help", "completion", "telemetry",
		cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return ""
	}
	if !builtins[top.Name()] {
		return "plugin"
	}
	// CommandPath is space-joined and starts with the root name; report the
	// dotted path without it ("kapi memory import" → "memory.import").
	//
	// The label is derived from the live command path, so renaming a command
	// renames its telemetry series. `kapi tm *` became `kapi memory *` and
	// `kapi termbase *` became `kapi terms *`, which means those series start
	// fresh rather than continuing the old ones; any dashboard querying the
	// `tm.*` / `termbase.*` labels needs to union both spellings across the
	// release boundary. That is a consequence of the deliberate CLI rename, not
	// something this function can paper over.
	parts := strings.Fields(executed.CommandPath())
	if len(parts) < 2 {
		return "root"
	}
	return strings.Join(parts[1:], ".")
}

// MaybeShowTelemetryNotice prints the one-time first-run notice to w and
// persists the shown marker. Returns true when the notice was printed this
// call. Callers must only invoke it when telemetry is enabled — a disabled
// lattice means no notice and no events.
func MaybeShowTelemetryNotice(cfg *config.AppConfig, w io.Writer) bool {
	if cfg.GetBool(telemetry.KeyNoticeShown) {
		return false
	}
	// Persist first: if the write fails we still show the notice, and a
	// failure to persist must not re-notify forever without also failing
	// everything else about the config store.
	if err := config.SetGlobalConfig(telemetry.KeyNoticeShown, "true"); err == nil {
		cfg.Set(telemetry.KeyNoticeShown, true)
	}
	fmt.Fprintf(w, `
kapi collects anonymous usage telemetry to guide development.
  Collected:  built-in command name, duration bucket, exit class, kapi
              version, OS and architecture, keyed by a random anonymous ID.
  Never:      arguments, flag values, file paths, file contents, project
              names, or any personal information.
  Opt out:    kapi telemetry off   (or KAPI_TELEMETRY=0 / DO_NOT_TRACK=1)
  Details:    %s
This notice is shown once.

`, telemetryDocsURL)
	return true
}
