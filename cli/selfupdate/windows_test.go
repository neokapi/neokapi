package selfupdate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/pluginhost/registry"
)

// indexWithPlatforms builds a CLI release index whose single version carries
// an entry for each of the given platform keys.
func indexWithPlatforms(platforms ...string) *registry.IndexV2 {
	plats := map[string]registry.PlatformEntry{}
	for _, p := range platforms {
		plats[p] = registry.PlatformEntry{URL: "https://example.test/" + p, SHA256: "deadbeef"}
	}
	return &registry.IndexV2{Plugins: map[string]registry.PluginEntry{
		indexName: {Versions: map[string]registry.VersionEntry{
			"1.2.0": {Channel: "stable", Platforms: plats},
		}},
	}}
}

// The Windows build is updated through winget, so an index with no Windows
// entry is the expected state there and not a missing release. Any other OS
// without an entry, and a Windows entry once published, keep the ordinary path.
func TestWindowsUnindexed(t *testing.T) {
	cases := []struct {
		name      string
		idx       *registry.IndexV2
		goos      string
		platKey   string
		unindexed bool
	}{
		{"windows with no entry", indexWithPlatforms("darwin/arm64", "linux/amd64"), "windows", "windows/amd64", true},
		{"windows with an empty index", &registry.IndexV2{}, "windows", "windows/amd64", true},
		{"windows with an entry", indexWithPlatforms("linux/amd64", "windows/amd64"), "windows", "windows/amd64", false},
		{"windows arm64 when only amd64 is indexed", indexWithPlatforms("windows/amd64"), "windows", "windows/arm64", true},
		{"darwin with no entry", indexWithPlatforms("linux/amd64"), "darwin", "darwin/amd64", false},
		{"linux with an entry", indexWithPlatforms("linux/amd64"), "linux", "linux/amd64", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.unindexed, windowsUnindexed(tc.idx, tc.goos, tc.platKey))
		})
	}
}

// fetchLatest on Windows reports ErrWindowsViaWinget when the index has no
// Windows entry, and the ordinary resolution error on any other OS. The index
// names a platform no test host runs on, so Resolve fails everywhere.
func TestFetchLatestWindowsWithoutAnEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"schema":"v2","plugins":{"kapi":{"channels":["stable"],"versions":{"1.2.0":{"channel":"stable","platforms":{"plan9/mips":{"url":"https://example.test/kapi.tar.gz","sha256":"deadbeef"}}}}}}}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("KAPI_CLI_INDEX_URL", srv.URL)

	cases := []struct {
		name     string
		goos     string
		platKey  string
		wantErr  error
		wantText string
	}{
		{"windows", "windows", "windows/amd64", ErrWindowsViaWinget, ""},
		{"darwin", "darwin", "darwin/arm64", nil, "no version of plugin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel, err := fetchLatest(context.Background(), "stable", tc.goos, tc.platKey)
			require.Error(t, err)
			assert.Nil(t, rel)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NotErrorIs(t, err, ErrWindowsViaWinget)
			require.ErrorContains(t, err, tc.wantText)
		})
	}
}
