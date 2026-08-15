package terms

import (
	"strconv"
	"strings"
)

// ShadowIDPrefix reserves an ID namespace for entries that exist only as the
// stream-scoped shadow of a live one.
//
// A caller exercising a proposed change on a branch writes the concepts and
// relations the proposal would produce onto that branch, under shadowed IDs, so
// the live graph stays untouched and the shadow is a deterministic delete. The
// concept and relation tables key on ID alone — stream is a plain column — so
// without a distinct ID per (proposal, stream, original) the shadow write would
// re-home the single live row onto the branch and destroy the graph.
//
// **Every stream-blind read filters the namespace out.** That is the half a
// store must not forget: a shadow is visible only to a reader that names the
// stream it was written to. A workspace-wide lookup that returned one would let
// a proposal nobody adopted govern content nobody bound it to — the checks read
// terms stream-blind, so the branch's draft would decide what main flags, and
// the sync would carry machine-written duplicates into a project's own terms.
const ShadowIDPrefix = "__pilot__"

// IsShadowID reports whether id sits in the shadow namespace.
func IsShadowID(id string) bool { return strings.HasPrefix(id, ShadowIDPrefix) }

// NotShadowSQL is the predicate a stream-blind query adds to exclude the shadow
// namespace, plus the argument it binds. col names the ID column ("c.id",
// "t.concept_id", "source_id"), and placeholder is the driver's parameter
// marker: "?" for SQLite, "$N" for Postgres.
//
// It compares a prefix-length slice rather than using LIKE, because the reserved
// prefix is made of underscores and `_` is a LIKE wildcard — `LIKE '__pilot__%'`
// matches any two characters followed by "pilot" followed by any two, which is
// both wrong and quietly wrong.
func NotShadowSQL(col, placeholder string) (predicate string, arg any) {
	return "substr(" + col + ", 1, " + strconv.Itoa(len(ShadowIDPrefix)) + ") <> " + placeholder, ShadowIDPrefix
}
