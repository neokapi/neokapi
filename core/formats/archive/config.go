package archive

import (
	"path"
	"strings"

	"github.com/neokapi/neokapi/core/format"
)

// Config holds configuration for the archive format. An archive (ZIP / TAR /
// TAR.GZ) is treated as a folder of sub-documents: each entry whose content
// kapi recognises and can faithfully rewrite is parsed through its own format
// reader; everything else rides through the round-trip byte-for-byte.
type Config struct {
	// Include is an optional set of glob patterns (matched against the slash-
	// separated entry path, e.g. "locales/en.json"). When non-empty, only
	// entries matching at least one pattern are considered for sub-filtering;
	// the rest pass through unchanged. Empty means "consider every entry".
	Include []string `json:"include" schema:"title=Include,description=Glob patterns of archive entry paths to process; empty means all entries"`
	// Exclude is an optional set of glob patterns that are never sub-filtered
	// (they pass through byte-for-byte). Exclude takes precedence over Include.
	Exclude []string `json:"exclude" schema:"title=Exclude,description=Glob patterns of archive entry paths to never process (pass through unchanged)"`
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "archive" }

// Reset restores default values.
func (c *Config) Reset() {
	c.Include = nil
	c.Exclude = nil
}

// Validate checks configuration validity. The glob patterns are validated lazily
// at match time (an invalid pattern simply never matches), so there is nothing
// to reject up front.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	return format.ApplyMapViaJSON(c, values)
}

// matches reports whether an entry path is eligible for sub-filtering under the
// include/exclude rules. The path is normalised to forward slashes (the native
// separator inside both ZIP and TAR containers) before matching.
func (c *Config) matches(name string) bool {
	name = strings.TrimPrefix(name, "./")
	for _, pat := range c.Exclude {
		if globMatch(pat, name) {
			return false
		}
	}
	if len(c.Include) == 0 {
		return true
	}
	for _, pat := range c.Include {
		if globMatch(pat, name) {
			return true
		}
	}
	return false
}

// globMatch matches a glob pattern against an entry path. A "**" segment is
// expanded to also match across path separators (path.Match alone stops at "/"),
// so "locales/**" covers nested entries.
func globMatch(pattern, name string) bool {
	if pattern == name {
		return true
	}
	if strings.Contains(pattern, "**") {
		// Treat "**" as "match anything including slashes": compare the
		// surrounding literal prefix/suffix around the wildcard.
		prefix, suffix, _ := strings.Cut(pattern, "**")
		prefix = strings.TrimSuffix(prefix, "/")
		suffix = strings.TrimPrefix(suffix, "/")
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			return false
		}
		if suffix != "" && !strings.HasSuffix(name, suffix) {
			return false
		}
		return true
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}
