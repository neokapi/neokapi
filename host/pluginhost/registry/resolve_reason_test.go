package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexOf builds an index holding one plugin named "okapi-bridge".
func indexOf(versions map[string]VersionEntry) *IndexV2 {
	return &IndexV2{Plugins: map[string]PluginEntry{"okapi-bridge": {Versions: versions}}}
}

// here is a build for the running platform.
func here() map[string]PlatformEntry {
	return map[string]PlatformEntry{PlatformKey(): {URL: "https://example.invalid/a.tgz"}}
}

// When no version resolves, the error is the one reason that decided it, in
// one sentence: never a platform blamed for a kapi version, nor an empty
// constraint quoted back.
func TestResolve_NamesTheDecisiveReason(t *testing.T) {
	other := map[string]PlatformEntry{"plan9/mips": {URL: "https://example.invalid/b.tgz"}}
	for _, tc := range []struct {
		name       string
		versions   map[string]VersionEntry
		constraint string
		kapi       string
		want       string
		not        []string
	}{
		{
			name: "every version needs a newer kapi than a release",
			versions: map[string]VersionEntry{
				"1.47.0": {Channel: "stable", MinKapiVersion: "1.5.0", Platforms: here()},
				"1.46.0": {Channel: "stable", MinKapiVersion: "1.4.2", Platforms: here()},
			},
			kapi: "1.4.0",
			want: `plugin "okapi-bridge" needs kapi 1.4.2 or newer, and this kapi is 1.4.0: update kapi`,
			not:  []string{"platform", `constraint ""`},
		},
		{
			name: "a development build satisfies no minimum",
			versions: map[string]VersionEntry{
				"1.47.0": {Channel: "stable", MinKapiVersion: "1.3.0", Platforms: here()},
			},
			kapi: "dev",
			want: `plugin "okapi-bridge" needs kapi 1.3.0 or newer, and this kapi is a development build (version "dev") that satisfies no minimum: build it with ` + "`make build`" + `, which stamps a version`,
		},
		{
			name: "no build for this platform",
			versions: map[string]VersionEntry{
				"1.47.0": {Channel: "stable", Platforms: other},
			},
			kapi: "1.9.0",
			want: `plugin "okapi-bridge" has no build for ` + PlatformKey(),
		},
		{
			name: "no release on the channel",
			versions: map[string]VersionEntry{
				"1.47.0-rc1": {Channel: "beta", Platforms: here()},
			},
			kapi: "1.9.0",
			want: `plugin "okapi-bridge" has no release on the "stable" channel`,
		},
		{
			name: "no version matches the constraint",
			versions: map[string]VersionEntry{
				"1.47.0": {Channel: "stable", Platforms: here()},
			},
			constraint: "^2.0",
			kapi:       "1.9.0",
			want:       `no version of plugin "okapi-bridge" matches "^2.0"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := indexOf(tc.versions).Resolve("okapi-bridge", tc.constraint, "stable", tc.kapi)
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
			for _, n := range tc.not {
				assert.NotContains(t, err.Error(), n)
			}
		})
	}
}

// The platform without a build is reported even when the versions that have
// one need a newer kapi: no version could install here at all.
func TestResolve_APlatformWithNoBuildOutranksTheKapiVersion(t *testing.T) {
	idx := indexOf(map[string]VersionEntry{
		"1.47.0": {Channel: "stable", MinKapiVersion: "9.0.0", Platforms: map[string]PlatformEntry{"plan9/mips": {}}},
	})
	_, _, err := idx.Resolve("okapi-bridge", "", "stable", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no build for")
}

func TestIsDevelopmentVersion(t *testing.T) {
	for v, want := range map[string]bool{
		"dev": true, "": true, "unknown": true, "1.x": true,
		"1.4.0": false, "v1.4.0": false, "1.4.0-rc5-173-g997ab7890-dirty": false, "1.4": false,
	} {
		assert.Equal(t, want, IsDevelopmentVersion(v), v)
	}
}
