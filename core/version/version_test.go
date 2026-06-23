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
