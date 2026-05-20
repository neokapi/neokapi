package openxml

import "bytes"

// run_merge_proto.go — PROTOTYPE (not wired into the writer yet; #602/#603).
//
// A byte-preserving, single-pass run-merge. It locates <w:r>…</w:r>
// boundaries and SPLICES the original bytes — it never decodes/re-encodes
// through encoding/xml (which would mangle self-closing tags, attribute
// order, and namespace prefixes, defeating byte parity). Adjacent runs
// whose <w:rPr> byte-payloads are identical (including both-absent) are
// fused into one <w:r> by concatenating their children under the shared
// rPr. Memory is O(one pending run), independent of document size; one
// pass over the buffer; the only large allocation is the output buffer,
// which the writer already materialises today.
//
// This is the cost model for replacing the 4 remaining post-serialization
// fuse regexes. It is intentionally NOT the production rule yet: the real
// version needs the field/annotation/content gating (a bare br→text and a
// fldChar-end→text fuse differently from two arbitrary equal-rPr runs).
// The merge SHAPE — scan boundaries, compare rPr bytes, splice — is what
// determines cost, so this faithfully characterises the runtime/allocation
// profile.
func streamingRunMerge(data []byte) []byte {
	out := make([]byte, 0, len(data))

	// Pending run state (the last run we emitted, still open for fusion).
	var pendRPr []byte // rPr inner payload of the pending run (nil = no rPr)
	var pendHasRPr bool
	pendOpen := false // we have a pending run buffered in `out` we can extend
	pendChildrenEnd := -1 // index in `out` just before the pending run's </w:r>

	i := 0
	for i < len(data) {
		rs := indexRunOpen(data[i:])
		if rs < 0 {
			out = append(out, data[i:]...)
			break
		}
		runStart := i + rs
		// Emit any gap bytes before this run. A non-empty gap breaks the
		// pending-run adjacency (something sits between the two runs).
		if runStart > i {
			out = append(out, data[i:runStart]...)
			pendOpen = false
		}
		// Find this run's matching </w:r> (runs nest inside txbxContent).
		bodyStart := runStart + len(runOpenTag)
		closeRel := indexRunClose(data[bodyStart:])
		if closeRel < 0 {
			// Malformed; emit the rest verbatim.
			out = append(out, data[runStart:]...)
			break
		}
		bodyEnd := bodyStart + closeRel              // index of "</w:r>"
		runEnd := bodyEnd + len(runCloseTag)         // just past "</w:r>"
		body := data[bodyStart:bodyEnd]              // between <w:r> and </w:r>
		rpr, children, hasRPr := splitRPr(body)

		if pendOpen && rprEqual(pendHasRPr, pendRPr, hasRPr, rpr) {
			// Fuse: drop the pending run's </w:r> and this run's <w:r>+rPr,
			// append this run's children, re-close. Splice in `out`.
			out = out[:pendChildrenEnd]              // cut pending </w:r>
			out = append(out, children...)
			out = append(out, runCloseTag...)
			pendChildrenEnd = len(out) - len(runCloseTag)
		} else {
			// Emit this run as a fresh pending run.
			out = append(out, runOpenTag...)
			if hasRPr {
				out = append(out, rprWrap(rpr)...)
			}
			out = append(out, children...)
			pendChildrenEnd = len(out)
			out = append(out, runCloseTag...)
			pendRPr, pendHasRPr, pendOpen = rpr, hasRPr, true
		}
		i = runEnd
	}
	return out
}

var (
	runOpenTag  = []byte("<w:r>")
	runCloseTag = []byte("</w:r>")
	rPrOpenTag  = []byte("<w:rPr>")
	rPrCloseTag = []byte("</w:rPr>")
	rPrEmptyTag = []byte("<w:rPr/>")
)

// indexRunOpen returns the offset of the next bare "<w:r>" run-open tag,
// or -1. It deliberately matches only the bare form (no attributes) — the
// shape the fuses target; attributed <w:r ...> runs are left alone.
func indexRunOpen(b []byte) int { return bytes.Index(b, runOpenTag) }

// indexRunClose returns the offset of the "</w:r>" that closes the current
// run, accounting for runs nested inside the body (e.g. <w:pict> →
// <w:txbxContent> → <w:p> → <w:r>). Depth tracks nested <w:r>.
func indexRunClose(b []byte) int {
	depth := 0
	i := 0
	for i < len(b) {
		oc := bytes.Index(b[i:], runCloseTag)
		if oc < 0 {
			return -1
		}
		closeAt := i + oc
		// Count nested run-opens strictly before this close.
		nested := bytes.Count(b[i:closeAt], runOpenTag)
		depth += nested
		if depth == 0 {
			return closeAt
		}
		depth--
		i = closeAt + len(runCloseTag)
	}
	return -1
}

// splitRPr splits a run body into (rprInner, children, hasRPr). When the
// body begins with <w:rPr>…</w:rPr> or <w:rPr/>, that leading block is the
// run properties; the remainder is the children.
func splitRPr(body []byte) (rprInner, children []byte, hasRPr bool) {
	if bytes.HasPrefix(body, rPrEmptyTag) {
		return nil, body[len(rPrEmptyTag):], true
	}
	if bytes.HasPrefix(body, rPrOpenTag) {
		end := bytes.Index(body, rPrCloseTag)
		if end >= 0 {
			inner := body[len(rPrOpenTag):end]
			return inner, body[end+len(rPrCloseTag):], true
		}
	}
	return nil, body, false
}

// rprWrap reconstructs the <w:rPr>…</w:rPr> (or <w:rPr/>) byte form.
func rprWrap(inner []byte) []byte {
	if len(inner) == 0 {
		return rPrEmptyTag
	}
	out := make([]byte, 0, len(rPrOpenTag)+len(inner)+len(rPrCloseTag))
	out = append(out, rPrOpenTag...)
	out = append(out, inner...)
	out = append(out, rPrCloseTag...)
	return out
}

func rprEqual(aHas bool, a []byte, bHas bool, b []byte) bool {
	if aHas != bHas {
		return false
	}
	return bytes.Equal(a, b)
}
