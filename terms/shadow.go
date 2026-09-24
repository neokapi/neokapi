package terms

import (
	"strconv"
	"strings"
)

// ShadowIDPrefix reserves IDs for stream-scoped copies of live concepts and
// relations used to evaluate proposals.
//
// The tables key on ID alone; stream is an ordinary column. Each combination of
// proposal, stream and original ID therefore needs a distinct shadow ID.
// Reusing a live ID would move the live row onto the proposal's branch.
//
// Reads without a stream filter must exclude shadow IDs. Otherwise unapproved
// terms could affect checks on other streams or be synchronized into a
// project's terms store.
const ShadowIDPrefix = "__pilot__"

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
