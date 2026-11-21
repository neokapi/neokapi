package terms

import (
	"errors"
	"strconv"
	"strings"
)

// ShadowIDPrefix reserves IDs for proposal copies of concepts and relations.
// The terms tables key on ID within a workspace. Each project's proposal on a
// stream therefore needs its own namespace. Reads without a stream filter
// exclude these IDs so checks and synchronization see the live terms.
const ShadowIDPrefix = "__pilot__"

// ShadowNamespacePrefix limits a bulk operation to one namespace within the reserved shadow IDs.
func ShadowNamespacePrefix(namespace string) (string, error) {
	if strings.TrimSpace(namespace) == "" {
		return "", errors.New("a shadow namespace is required")
	}
	return ShadowIDPrefix + namespace, nil
}

// IsShadowID reports whether id sits in the shadow namespace.
func IsShadowID(id string) bool { return strings.HasPrefix(id, ShadowIDPrefix) }

// NotShadowSQL returns a predicate and argument that exclude shadow IDs from a
// query without a stream filter. col names the ID column ("c.id", "t.concept_id",
// "source_id"); placeholder is "?" for SQLite or "$N" for Postgres.
//
// The predicate compares a prefix-length substring. LIKE would interpret the
// underscores in the reserved prefix as single-character wildcards.
func NotShadowSQL(col, placeholder string) (predicate string, arg any) {
	return "substr(" + col + ", 1, " + strconv.Itoa(len(ShadowIDPrefix)) + ") <> " + placeholder, ShadowIDPrefix
}
