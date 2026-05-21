//go:build parity

package roundtrip

// CanonClass records WHY an engine reached canonical-equal instead of
// byte-equal. The distinction is the load-bearing fact for honest parity
// reporting: the large majority of canonical gaps are the tested engine
// being *more faithful to the source* than okapi — okapi re-serializes on
// round-trip (reorders attributes, strips revision IDs, rewrites the XML
// declaration, normalizes line endings, reflows whitespace, alphabetizes
// container children) while native writes byte-exact from a skeleton and
// only rewrites translated segments. Counting every canonical case as
// "remaining work toward byte-equal" therefore understates native quality
// and would reward fidelity *regressions* (degrading native to mimic
// okapi's destructive re-serialization).
//
// CanonClass is declared per-format on the coverage scan and threaded down
// to every fixture's parity record so the report can split canon into
// "faithful" (expected, don't chase) and "closeable" (native genuinely
// loses source information, worth fixing).
type CanonClass int

const (
	// CanonUnclassified: no judgment recorded for this format. Counted
	// separately and never folded into the faithful-parity figure, so an
	// unclassified canon case can only ever *under*-state native quality —
	// the conservative default.
	CanonUnclassified CanonClass = iota

	// CanonFaithful: native preserves the source's serialization and the
	// canonical gap exists only because okapi re-serializes/normalizes on
	// round-trip. Native is the more faithful side; driving these to
	// byte-equal would regress source fidelity, so they are *expected*
	// canon, not a backlog.
	CanonFaithful

	// CanonCloseable: native itself alters or loses source information that
	// it could preserve (e.g. inconsistent line endings). Driving these to
	// byte-equal is a genuine fidelity improvement and is real remaining
	// work.
	CanonCloseable
)

// String renders the class for diagnostics and JSON.
func (c CanonClass) String() string {
	switch c {
	case CanonFaithful:
		return "faithful"
	case CanonCloseable:
		return "closeable"
	default:
		return "unclassified"
	}
}
