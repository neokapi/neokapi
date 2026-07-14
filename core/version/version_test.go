package version

import "testing"

func TestIsPrereleaseAndChannel(t *testing.T) {
	saved := Version
	defer func() { Version = saved }()

	cases := []struct {
		v       string
		pre     bool
		channel string
	}{
		{"1.2.0", false, "stable"},
		{"v1.2.0", false, "stable"},
		{"1.2.0-rc.1", true, "beta"},
		{"1.2.0-beta.2", true, "beta"},
		{"v1.2.0-rc.1", true, "beta"},
		{"1.2.0+build.5", false, "stable"},   // build metadata only, not a prerelease
		{"1.2.0-rc.1+build.5", true, "beta"}, // prerelease + build metadata
		{"dev", false, "stable"},

		// Working-tree builds take the channel of the tag they were built from,
		// not of the git-describe decorations.
		{"v1.2.0-rc9-238-gb1a3033dc-dirty", true, "beta"},
		{"v1.2.0-238-gb1a3033dc", false, "stable"},
		{"v1.2.0-dirty", false, "stable"},
		// Not a version at all (an unfiltered describe, or a bare sha).
		{"contract-types-v0.1.0-82-gb1a3033dc-dirty", false, "stable"},
		{"b1a3033dc", false, "stable"},
	}
	for _, c := range cases {
		Version = c.v
		if got := IsPrerelease(); got != c.pre {
			t.Errorf("IsPrerelease(%q) = %v, want %v", c.v, got, c.pre)
		}
		if got := Channel(); got != c.channel {
			t.Errorf("Channel(%q) = %q, want %q", c.v, got, c.channel)
		}
	}
}

func TestIsDevBuild(t *testing.T) {
	saved := Version
	defer func() { Version = saved }()

	cases := []struct {
		v   string
		dev bool
	}{
		// Released tags.
		{"1.2.0", false},
		{"v1.2.0", false},
		{"v1.2.0-rc9", false},
		{"1.2.0-rc.1+build.5", false},

		// Unstamped builds.
		{"", true},
		{"dev", true},
		{"unknown", true},

		// git describe: ahead of the tag, dirty, or both.
		{"v1.2.0-rc9-238-gb1a3033dc", true},
		{"v1.2.0-rc9-238-gb1a3033dc-dirty", true},
		{"v1.2.0-dirty", true},

		// The unfiltered-describe regression: a package tag is the nearest tag
		// to HEAD, so the CLI reported itself as "contract-types-v0.1.0-…" and
		// — being unparseable — looked older than every published release.
		{"contract-types-v0.1.0-82-gb1a3033dc-dirty", true},
		{"sat-v1.0.2", true},
		{"vision-models-v1", true},
		// git describe --always with no matching tag: a bare commit sha.
		{"b1a3033dc", true},
	}
	for _, c := range cases {
		Version = c.v
		if got := IsDevBuild(); got != c.dev {
			t.Errorf("IsDevBuild(%q) = %v, want %v", c.v, got, c.dev)
		}
	}
}
