// Package telemetry implements anonymous, opt-out usage telemetry for the
// kapi CLI and other local clients built on the host runtime (roadmap epic
// 018, workstream G).
//
// The package is deliberately minimal and cobra-free: events are captured
// into an in-memory queue, posted to a PostHog-compatible /batch endpoint by
// a background goroutine over plain net/http, and flushed best-effort (with
// a caller-bounded wait, ≤100 ms by convention) from the process exit path.
// Failures are silent; capture never blocks the command being run.
//
// Privacy invariants (see web/docs/reference/telemetry.mdx):
//   - the only identifier is a random machine UUID persisted in the kapi
//     config store — no hostname, no username, no PII;
//   - event properties never include arguments, flag values, file paths,
//     project names, or content;
//   - telemetry is disabled if ANY switch in the opt-out lattice fires
//     (see Resolve), including builds without a baked-in key and builds
//     made with the `notelemetry` tag.
package telemetry

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/neokapi/neokapi/host/config"
)

// apiKey and endpoint are baked into release binaries via -ldflags
// (see the root Makefile: KAPI_POSTHOG_KEY / KAPI_POSTHOG_ENDPOINT).
// Both default to empty so development builds are silent: with no key,
// Resolve reports telemetry as disabled and New returns nil.
var (
	apiKey   string
	endpoint string
)

// DefaultEndpoint is the ingestion host used when no endpoint is baked in
// alongside the key.
const DefaultEndpoint = "https://eu.i.posthog.com"

// Config keys in the kapi config store (~/.config/kapi/kapi.yaml).
const (
	// KeyEnabled persists the `kapi telemetry on|off` choice. Absent means on
	// (telemetry is opt-out); any of "false", "0", "off", "no" means off.
	KeyEnabled = "telemetry.enabled"
	// KeyNoticeShown records that the one-time first-run notice was printed.
	KeyNoticeShown = "telemetry.notice_shown"
	// KeyMachineID stores the random anonymous machine UUID.
	KeyMachineID = "telemetry.machine_id"
)

// Environment variables in the opt-out lattice.
const (
	// EnvOptOut disables telemetry when set to "0" or "false".
	EnvOptOut = "KAPI_TELEMETRY"
	// EnvDoNotTrack is the cross-tool console DNT convention: any non-empty
	// value other than "0" disables telemetry.
	EnvDoNotTrack = "DO_NOT_TRACK"
	// EnvCI marks a CI environment; telemetry auto-disables there.
	EnvCI = "CI"
)

// Stable reasons reported by Resolve for `kapi telemetry status`.
const (
	ReasonEnabled    = "enabled"
	ReasonBuildTag   = "this binary was built without telemetry (notelemetry build tag)"
	ReasonConfigOff  = "turned off in config (kapi telemetry off)"
	ReasonEnvOptOut  = "KAPI_TELEMETRY is set to 0/false"
	ReasonDoNotTrack = "DO_NOT_TRACK is set"
	ReasonCI         = "CI environment detected"
	ReasonNoKey      = "no telemetry key baked into this build (development build)"
)

// Status is the resolved telemetry state plus the first opt-out switch that
// fired (or ReasonEnabled).
type Status struct {
	Enabled bool
	Reason  string
}

// Options configures a Client. Surface is the taxonomy surface property
// (e.g. "cli"); Version is the application version stamped on every event.
// DistinctID overrides the persisted machine ID (tests); when empty the
// machine ID from the config store is used.
type Options struct {
	Surface    string
	Version    string
	DistinctID string
}

// Configure overrides the baked-in API key and endpoint. Intended for tests
// and for embedders that supply their own project key at runtime; release
// binaries get both via ldflags.
func Configure(key, ep string) {
	apiKey = key
	endpoint = ep
}

// Resolve evaluates the opt-out lattice. Telemetry is disabled if ANY of the
// switches fires; the returned Status carries the first match so status
// output can explain why. cfg may be nil (only the persisted-config switch
// is skipped then).
func Resolve(cfg *config.AppConfig) Status {
	if compiledOut {
		return Status{Reason: ReasonBuildTag}
	}
	if cfg != nil && configOff(cfg.GetString(KeyEnabled)) {
		return Status{Reason: ReasonConfigOff}
	}
	if v := os.Getenv(EnvOptOut); v == "0" || strings.EqualFold(v, "false") {
		return Status{Reason: ReasonEnvOptOut}
	}
	if v := os.Getenv(EnvDoNotTrack); v != "" && v != "0" {
		return Status{Reason: ReasonDoNotTrack}
	}
	if v := os.Getenv(EnvCI); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return Status{Reason: ReasonCI}
	}
	if apiKey == "" {
		return Status{Reason: ReasonNoKey}
	}
	return Status{Enabled: true, Reason: ReasonEnabled}
}

// configOff reports whether a persisted telemetry.enabled value means "off".
// An absent value (empty string) means on — telemetry is opt-out.
func configOff(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "off", "no":
		return true
	default:
		return false
	}
}

// MachineID returns the anonymous machine UUID, generating and persisting a
// fresh random one via the kapi config store on first use. Persistence is
// best-effort: if the config file cannot be written the ID is still returned
// (and simply regenerated next run).
func MachineID(cfg *config.AppConfig) string {
	if cfg != nil {
		if id := cfg.GetString(KeyMachineID); id != "" {
			return id
		}
	}
	id := uuid.NewString()
	if err := config.SetGlobalConfig(KeyMachineID, id); err != nil {
		debugf("persist machine id: %v", err)
	}
	if cfg != nil {
		cfg.Set(KeyMachineID, id)
	}
	return id
}

// DurationBucket maps a command duration onto the coarse buckets reported in
// cli_command_run — never the raw duration.
func DurationBucket(d time.Duration) string {
	switch {
	case d < 100*time.Millisecond:
		return "<100ms"
	case d < time.Second:
		return "<1s"
	case d < 10*time.Second:
		return "<10s"
	case d < time.Minute:
		return "<60s"
	default:
		return ">=60s"
	}
}

// resolvedEndpoint returns the ingestion host: the ldflags override when
// set, else DefaultEndpoint.
func resolvedEndpoint() string {
	if endpoint != "" {
		return strings.TrimRight(endpoint, "/")
	}
	return DefaultEndpoint
}

// debugf logs telemetry internals to stderr when KAPI_TELEMETRY_DEBUG is
// set. All telemetry failures are silent otherwise.
func debugf(format string, args ...any) {
	if os.Getenv("KAPI_TELEMETRY_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "telemetry: "+format+"\n", args...)
}
