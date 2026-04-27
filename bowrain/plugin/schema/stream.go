package schema

import "strings"

const (
	// StreamAuto is the sentinel value meaning "auto-detect from git/CI".
	StreamAuto = "$auto"

	// StreamMain is the canonical default stream name.
	StreamMain = "main"
)

// NormalizeStreamName trims ref prefixes and maps "master" to "main".
func NormalizeStreamName(name string) string {
	name = strings.TrimPrefix(name, "refs/heads/")
	name = strings.TrimPrefix(name, "refs/tags/")
	name = strings.TrimSpace(name)
	if name == "master" {
		return StreamMain
	}
	if name == "" {
		return StreamMain
	}
	return name
}
