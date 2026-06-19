package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/neokapi/neokapi/cli/pluginhost/registry"
	"github.com/neokapi/neokapi/core/version"
)

// indexName is the entry key for the kapi CLI within the CLI release index.
// The index reuses the plugin registry's v2 shape (name → versions →
// per-platform url/sha256/signature/cert identity), published as a separate
// document so the CLI never appears in `kapi plugin list`.
const indexName = "kapi"

// DefaultIndexURL is where the CLI release index lives. Override with
// KAPI_CLI_INDEX_URL (tests, mirrors, enterprise).
const DefaultIndexURL = "https://neokapi.github.io/registry/cli.json"

// defaultChannel is the release track checked by default.
const defaultChannel = "stable"

// cacheTTL is how long a cached latest-version answer stays fresh. The check is
// a courtesy, not a guarantee — once a day is plenty and keeps us off the
// network on the hot path.
const cacheTTL = 24 * time.Hour

// IndexURL returns the configured CLI release index URL.
func IndexURL() string {
	if v := strings.TrimSpace(os.Getenv("KAPI_CLI_INDEX_URL")); v != "" {
		return v
	}
	return DefaultIndexURL
}

// Release describes the newest available CLI build for this platform.
type Release struct {
	Version  string
	Platform registry.PlatformEntry
}

// FetchLatest resolves the newest CLI version for the running platform from the
// release index. It performs network I/O and does not consult the cache.
func FetchLatest(ctx context.Context, channel string) (*Release, error) {
	if channel == "" {
		channel = defaultChannel
	}
	idx, err := registry.FetchIndex(ctx, IndexURL())
	if err != nil {
		return nil, err
	}
	v, plat, err := idx.Resolve(indexName, "", channel, version.Version)
	if err != nil {
		return nil, err
	}
	return &Release{Version: v, Platform: plat}, nil
}

// IsNewer reports whether latest is a strictly newer release than the running
// binary. Dev / source builds (version "dev", "", "unknown") are never told to
// update — there is no meaningful "latest" to compare them against.
func IsNewer(latest string) bool {
	cur := strings.TrimSpace(version.Version)
	if cur == "" || cur == "dev" || cur == "unknown" {
		return false
	}
	return registry.CompareSemver(strings.TrimPrefix(latest, "v"), strings.TrimPrefix(cur, "v")) > 0
}

// ---- background-notification gating + cache ----

// cacheState persists the last latest-version answer so the background notifier
// hits the network at most once per cacheTTL.
type cacheState struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	Channel   string    `json:"channel"`
}

// NotifyDisabled reports whether the background update check should be skipped
// entirely. We stay silent in non-interactive contexts and honor the
// cross-tool DO_NOT_TRACK convention plus a kapi-specific opt-out.
//
// A version check leaks the current version + IP + timing, so it is opt-out-able
// like telemetry; it never sends anything identifying beyond the HTTP request.
func NotifyDisabled() bool {
	if isTruthy(os.Getenv("KAPI_NO_UPDATE_CHECK")) || isTruthy(os.Getenv("DO_NOT_TRACK")) {
		return true
	}
	// Continuous-integration / automation: never nag.
	for _, k := range []string{"CI", "GITHUB_ACTIONS", "BUILD_NUMBER", "RUN_ID"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	// Only notify on an interactive stderr (we render the notice to stderr so
	// it never contaminates piped stdout / machine-readable output).
	return !isatty.IsTerminal(os.Stderr.Fd()) && !isatty.IsCygwinTerminal(os.Stderr.Fd())
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// CachedLatest returns the latest known version, fetching from the network only
// when the cache is missing or older than cacheTTL. A fetch error is reported
// but never fatal — callers treat any error as "no notice".
func CachedLatest(ctx context.Context, channel string) (latest string, err error) {
	if channel == "" {
		channel = defaultChannel
	}
	path := cachePath()
	if st, ok := readCache(path); ok && st.Channel == channel && time.Since(st.CheckedAt) < cacheTTL {
		return st.Latest, nil
	}
	rel, err := FetchLatest(ctx, channel)
	if err != nil {
		return "", err
	}
	writeCache(path, cacheState{CheckedAt: time.Now(), Latest: rel.Version, Channel: channel})
	return rel.Version, nil
}

// ---- background notifier (root-command integration) ----
//
// The flow is split so the command's hot path is never blocked by the network:
//
//   - StartBackgroundRefresh kicks off a detached, time-bounded fetch that only
//     writes the cache file. Started in the root PreRun, it has the whole
//     command's runtime to complete.
//   - RenderNotice reads the cache file only (never the network) and prints the
//     "update available" line. Run in the root PostRun, it shows whatever the
//     latest completed refresh found — this run's or a prior run's.
//
// Net effect, like npm's update-notifier: zero added latency, and the notice
// surfaces on this or a subsequent invocation.

// backgroundRefreshTimeout bounds the detached check so a slow/hung network
// can't keep a goroutine (and its sockets) alive indefinitely.
const backgroundRefreshTimeout = 5 * time.Second

// StartBackgroundRefresh launches a detached cache refresh unless notifications
// are disabled. It returns immediately; the goroutine is best-effort and its
// result is discarded (it only updates the on-disk cache).
func StartBackgroundRefresh(channel string) {
	if NotifyDisabled() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), backgroundRefreshTimeout)
		defer cancel()
		_, _ = CachedLatest(ctx, channel)
	}()
}

// RenderNotice prints an "update available" line to w if the cache (already on
// disk, no network) shows a newer release for the given channel. It is silent
// when notifications are disabled, when no cache exists, or when up to date.
func RenderNotice(w io.Writer, channel string) {
	if NotifyDisabled() {
		return
	}
	renderNotice(w, channel)
}

// renderNotice is the cache+version core of RenderNotice, separated from the
// TTY/opt-out gate so it can be tested without a terminal.
func renderNotice(w io.Writer, channel string) {
	if channel == "" {
		channel = defaultChannel
	}
	st, ok := readCache(cachePath())
	if !ok || st.Channel != channel || !IsNewer(st.Latest) {
		return
	}
	fmt.Fprintln(w, Notice(Detect(), channel, version.Version, st.Latest))
}

// cachePath is the update-check state file, alongside the kapi config.
func cachePath() string {
	if dir := strings.TrimSpace(os.Getenv("KAPI_CONFIG_DIR")); dir != "" {
		return filepath.Join(dir, "update-check.json")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "kapi", "update-check.json")
}

func readCache(path string) (cacheState, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheState{}, false
	}
	var st cacheState
	if err := json.Unmarshal(data, &st); err != nil {
		return cacheState{}, false
	}
	return st, true
}

// writeCache best-effort persists the cache; failures are silently ignored
// (a missing cache just means we re-check next time).
func writeCache(path string, st cacheState) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	// Write atomically (temp + rename in the same dir) so the PostRun reader
	// never sees a half-written file while the background refresh writes.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-check-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
	}
}
