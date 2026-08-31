package selfupdate

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/pluginhost/registry"
)

func TestIsNewer(t *testing.T) {
	orig := version.Version
	t.Cleanup(func() { version.Version = orig })

	version.Version = "1.0.0"
	if !IsNewer("1.1.0") {
		t.Error("IsNewer(1.1.0) over 1.0.0 = false, want true")
	}
	if !IsNewer("v1.0.1") {
		t.Error("IsNewer(v1.0.1) over 1.0.0 = false, want true (v-prefix tolerated)")
	}
	if IsNewer("1.0.0") {
		t.Error("IsNewer(1.0.0) over 1.0.0 = true, want false")
	}
	if IsNewer("0.9.0") {
		t.Error("IsNewer(0.9.0) over 1.0.0 = true, want false")
	}

	// Dev / source builds are never told to update. A git-describe version is
	// the case that bit us: it isn't semver, so it sorted below every release
	// and the CLI nagged a locally built binary to "update" to a release older
	// than the code it was running.
	for _, dev := range []string{
		"dev", "", "unknown",
		"v1.2.0-rc9-238-gb1a3033dc-dirty",
		"contract-types-v0.1.0-82-gb1a3033dc-dirty",
	} {
		version.Version = dev
		if IsNewer("99.0.0") {
			t.Errorf("IsNewer(99.0.0) for dev build %q = true, want false", dev)
		}
	}
}

// A source build must not even ask: no notice, and no request to the index.
func TestNotifyDisabled_DevBuild(t *testing.T) {
	orig := version.Version
	t.Cleanup(func() { version.Version = orig })

	for _, k := range []string{"KAPI_NO_UPDATE_CHECK", "DO_NOT_TRACK", "CI", "GITHUB_ACTIONS", "BUILD_NUMBER", "RUN_ID"} {
		t.Setenv(k, "")
	}
	version.Version = "v1.2.0-rc9-238-gb1a3033dc-dirty"
	if !NotifyDisabled() {
		t.Errorf("NotifyDisabled() = false for a git-describe source build, want true")
	}
}

func TestNotifyDisabled(t *testing.T) {
	cases := []struct{ key, val string }{
		{"KAPI_NO_UPDATE_CHECK", "1"},
		{"DO_NOT_TRACK", "1"},
		{"CI", "true"},
		{"GITHUB_ACTIONS", "true"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			// Neutralize the others so each case is isolated.
			for _, k := range []string{"KAPI_NO_UPDATE_CHECK", "DO_NOT_TRACK", "CI", "GITHUB_ACTIONS", "BUILD_NUMBER", "RUN_ID"} {
				t.Setenv(k, "")
			}
			t.Setenv(c.key, c.val)
			if !NotifyDisabled() {
				t.Errorf("NotifyDisabled() = false with %s=%s, want true", c.key, c.val)
			}
		})
	}
}

func TestIsTruthy(t *testing.T) {
	for _, v := range []string{"", "0", "false", "no", "off", "FALSE"} {
		if isTruthy(v) {
			t.Errorf("isTruthy(%q) = true, want false", v)
		}
	}
	for _, v := range []string{"1", "true", "yes", "on", "anything"} {
		if !isTruthy(v) {
			t.Errorf("isTruthy(%q) = false, want true", v)
		}
	}
}

// indexJSON builds a CLI release index advertising version v for the running
// platform.
func indexJSON(v string) string {
	return fmt.Sprintf(`{"schema":"v2","plugins":{"kapi":{"channels":["stable"],"versions":{%q:{"channel":"stable","platforms":{%q:{"url":"https://example.test/kapi.tar.gz","sha256":"deadbeef"}}}}}}}`, v, registry.PlatformKey())
}

func TestCachedLatest_FetchesThenCaches(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		fmt.Fprint(w, indexJSON("1.2.0"))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("KAPI_CLI_INDEX_URL", srv.URL)
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())

	ctx := context.Background()
	got, err := CachedLatest(ctx, "stable")
	if err != nil {
		t.Fatalf("CachedLatest() error: %v", err)
	}
	if got != "1.2.0" {
		t.Fatalf("CachedLatest() = %q, want 1.2.0", got)
	}
	// Second call within TTL must serve from cache (no extra HTTP hit).
	if _, err := CachedLatest(ctx, "stable"); err != nil {
		t.Fatalf("CachedLatest() second call error: %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("index fetched %d times, want 1 (second call should hit the cache)", n)
	}
}

func TestRenderNotice(t *testing.T) {
	orig := version.Version
	t.Cleanup(func() { version.Version = orig })
	version.Version = "1.0.0"

	cfg := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", cfg)
	t.Setenv("KAPI_INSTALL_SOURCE", "homebrew")

	// No cache yet → no notice.
	var buf bytes.Buffer
	renderNotice(&buf, "stable")
	if buf.Len() != 0 {
		t.Errorf("renderNotice with no cache printed %q, want nothing", buf.String())
	}

	// Cache a newer version → notice with the brew command.
	writeCache(cachePath(), cacheState{CheckedAt: time.Now(), Latest: "1.1.0", Channel: "stable"})
	buf.Reset()
	renderNotice(&buf, "stable")
	if got := buf.String(); !strings.Contains(got, "1.1.0") || !strings.Contains(got, "brew upgrade kapi-cli") {
		t.Errorf("renderNotice = %q, want a brew-upgrade notice for 1.1.0", got)
	}

	// Cache an older version → no notice.
	writeCache(cachePath(), cacheState{CheckedAt: time.Now(), Latest: "0.9.0", Channel: "stable"})
	buf.Reset()
	renderNotice(&buf, "stable")
	if buf.Len() != 0 {
		t.Errorf("renderNotice for older cached version printed %q, want nothing", buf.String())
	}

	// Channel mismatch → no notice.
	writeCache(cachePath(), cacheState{CheckedAt: time.Now(), Latest: "2.0.0", Channel: "beta"})
	buf.Reset()
	renderNotice(&buf, "stable")
	if buf.Len() != 0 {
		t.Errorf("renderNotice for mismatched channel printed %q, want nothing", buf.String())
	}
}

func TestFetchLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, indexJSON("2.0.0"))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("KAPI_CLI_INDEX_URL", srv.URL)

	rel, err := FetchLatest(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchLatest() error: %v", err)
	}
	if rel.Version != "2.0.0" {
		t.Errorf("FetchLatest().Version = %q, want 2.0.0", rel.Version)
	}
	if rel.Platform.URL == "" {
		t.Error("FetchLatest().Platform.URL is empty")
	}
}
