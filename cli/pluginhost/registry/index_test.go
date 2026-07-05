package registry

import "testing"

func entryWithVersions(versions ...string) PluginEntry {
	m := make(map[string]VersionEntry, len(versions))
	for _, v := range versions {
		m[v] = VersionEntry{}
	}
	return PluginEntry{Versions: m}
}

func TestHighestVersion(t *testing.T) {
	cases := []struct {
		name     string
		versions []string
		want     string
	}{
		{"empty", nil, ""},
		{"single", []string{"1.0.0"}, "1.0.0"},
		// The lexicographic trap: "1.9.0" > "1.10.0" as strings, but
		// 1.10.0 is the newer release.
		{"double digit minor", []string{"1.9.0", "1.10.0"}, "1.10.0"},
		{"double digit patch", []string{"0.2.9", "0.2.10", "0.2.2"}, "0.2.10"},
		{"major beats high minor", []string{"1.9.9", "2.0.0"}, "2.0.0"},
		{"v prefix tolerated", []string{"v1.2.0", "1.10.0"}, "1.10.0"},
		// Pre-release/build metadata is ignored for ordering (CompareSemver
		// semantics), so 2.0.0-beta counts as 2.0.0 and outranks 1.9.0.
		{"prerelease ranks as its release", []string{"1.9.0", "2.0.0-beta"}, "2.0.0-beta"},
		{"release outranks lower prerelease", []string{"1.4.0-rc1", "1.5.0"}, "1.5.0"},
		// Semver tie ("1.4.0" vs "1.4.0-rc1" compare equal) breaks
		// lexicographically for determinism.
		{"semver tie breaks lexicographically", []string{"1.4.0", "1.4.0-rc1"}, "1.4.0-rc1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HighestVersion(entryWithVersions(c.versions...)); got != c.want {
				t.Errorf("HighestVersion(%v)=%q want %q", c.versions, got, c.want)
			}
		})
	}
}

func TestMatchQuery(t *testing.T) {
	cases := []struct {
		name, description, query string
		want                     bool
	}{
		{"okapi-bridge", "Okapi Framework filters", "", true}, // empty query matches all
		{"okapi-bridge", "Okapi Framework filters", "bridge", true},
		{"okapi-bridge", "Okapi Framework filters", "BRIDGE", true},  // case-insensitive on name
		{"okapi-bridge", "Okapi Framework filters", "filters", true}, // description match
		{"okapi-bridge", "Okapi Framework filters", "FRAMEWORK", true},
		{"okapi-bridge", "Okapi Framework filters", "okapi", true}, // matches lowercased description too
		{"okapi-bridge", "Okapi Framework filters", "pdfium", false},
		{"", "", "anything", false},
	}
	for _, c := range cases {
		if got := MatchQuery(c.name, c.description, c.query); got != c.want {
			t.Errorf("MatchQuery(%q,%q,%q)=%v want %v", c.name, c.description, c.query, got, c.want)
		}
	}
}
