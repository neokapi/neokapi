//go:build parity

package roundtrip

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"sort"
)

// Tier classifies how close an engine's output got to the okapi
// reference. Lower values are stricter; the harness reports the lowest
// (= strictest) tier the engine reached.
type Tier int

const (
	// TierByteEqual: outputs match byte-for-byte (or per-zip-entry for
	// IsZip cases). The strictest tier.
	TierByteEqual Tier = iota

	// TierCanonicalEqual: outputs match after running both through the
	// case's Normalizer (XML C14N, JSON canonicalization, line-ending
	// normalization, …). Reaching this tier without TierByteEqual means
	// the outputs are semantically equivalent but stylistically different.
	TierCanonicalEqual

	// TierSemanticEqual: outputs match through a domain-aware
	// structural comparator (e.g. PO comment-tolerant diff). Reserved
	// for cases that need parsing rather than text canonicalization.
	TierSemanticEqual

	// TierDivergent: outputs differ at every tier we can check.
	TierDivergent
)

// String renders the tier for diagnostics.
func (t Tier) String() string {
	switch t {
	case TierByteEqual:
		return "byte-equal"
	case TierCanonicalEqual:
		return "canonical-equal"
	case TierSemanticEqual:
		return "semantic-equal"
	case TierDivergent:
		return "divergent"
	default:
		return fmt.Sprintf("tier-%d", int(t))
	}
}

// Normalizer rewrites bytes into a canonical form so two stylistically
// different but semantically equivalent outputs compare byte-equal.
// Implementations should be deterministic and idempotent. Returning an
// error means the input couldn't be parsed/canonicalized — the harness
// records that as TierDivergent rather than treating it as a match.
type Normalizer interface {
	Name() string
	Normalize(in []byte) ([]byte, error)
}

// ComparisonResult is the structured outcome of one engine's output vs
// the reference. The harness uses Achieved against the engine's
// required tier to decide pass/fail and records the whole struct in
// the parity report so we can see *how close* divergent engines got.
type ComparisonResult struct {
	// Achieved is the strictest tier this output reached.
	Achieved Tier

	// Reason is a one-line diagnostic suitable for test output (the
	// first-diff offset and a small context window). Empty when
	// Achieved == TierByteEqual.
	Reason string

	// GotSize / RefSize are the raw byte sizes of engine output and
	// okapi reference, respectively.
	GotSize int
	RefSize int

	// RawDiffOffset is the first byte offset where got and reference
	// differ. -1 when byte-equal. n/a (=0) for IsZip cases.
	RawDiffOffset int

	// NormDiffOffset is the first byte offset where normalize(got) and
	// normalize(reference) differ. -1 when canonical-equal or when no
	// normalizer was configured.
	NormDiffOffset int

	// Normalizer is the name of the normalizer that was tried (empty
	// when no normalizer is configured for this case).
	Normalizer string
}

// Divergence captures one engine's failure to meet its required tier.
// Reported per-engine so the harness lists every disagreement at once
// instead of bailing on the first.
type Divergence struct {
	Engine string
	Reason string
}

// String renders a divergence for test output.
func (d Divergence) String() string {
	return fmt.Sprintf("%s: %s", d.Engine, d.Reason)
}

// compareTiered runs the byte (and, when norm != nil, canonical)
// comparisons and returns the strictest tier reached along with the
// diff metrics needed to populate the parity report.
func compareTiered(got, reference []byte, isZip bool, norm Normalizer) ComparisonResult {
	res := ComparisonResult{
		GotSize:        len(got),
		RefSize:        len(reference),
		RawDiffOffset:  -1,
		NormDiffOffset: -1,
	}
	reason := compareBytesOrZip(got, reference, isZip)
	if reason == "" {
		res.Achieved = TierByteEqual
		return res
	}
	res.Reason = reason
	if !isZip {
		res.RawDiffOffset = firstDiff(got, reference)
	}

	if norm != nil {
		res.Normalizer = norm.Name()
		normGot, err := norm.Normalize(got)
		if err != nil {
			res.Reason = fmt.Sprintf("%s; normalizer %q failed on got: %v", res.Reason, norm.Name(), err)
			res.Achieved = TierDivergent
			return res
		}
		normRef, err := norm.Normalize(reference)
		if err != nil {
			res.Reason = fmt.Sprintf("%s; normalizer %q failed on reference: %v", res.Reason, norm.Name(), err)
			res.Achieved = TierDivergent
			return res
		}
		if reason := compareBytesOrZip(normGot, normRef, isZip); reason == "" {
			res.Achieved = TierCanonicalEqual
			return res
		} else if !isZip {
			res.NormDiffOffset = firstDiff(normGot, normRef)
		} else {
			// For zip cases, replace the raw-comparison Reason with
			// the post-normalize one so the report explains why
			// canonical-equal wasn't reached.
			res.Reason = "[after " + norm.Name() + "] " + reason
		}
	}

	res.Achieved = TierDivergent
	return res
}

// compareBytesOrZip is the byte-level (or per-zip-entry) check.
// Returns "" on match, a one-line diagnostic on divergence.
func compareBytesOrZip(got, reference []byte, isZip bool) string {
	if isZip {
		return compareZip(got, reference)
	}
	return compareBytes(got, reference)
}

// compareBytes does a byte-equal compare and returns a short
// diagnostic with the first differing offset and a small context
// window when they diverge.
func compareBytes(got, reference []byte) string {
	if bytes.Equal(got, reference) {
		return ""
	}
	if len(got) != len(reference) {
		offset := firstDiff(got, reference)
		return fmt.Sprintf("byte length differs: got %d, reference %d (first diff at offset %d: got %q vs %q)",
			len(got), len(reference), offset, snippet(got, offset), snippet(reference, offset))
	}
	offset := firstDiff(got, reference)
	return fmt.Sprintf("byte content differs at offset %d: got %q vs %q",
		offset, snippet(got, offset), snippet(reference, offset))
}

// firstDiff returns the first byte index where a and b differ. When
// one slice is a prefix of the other, it returns the shorter length.
func firstDiff(a, b []byte) int {
	n := min(len(b), len(a))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// snippet returns a human-readable window of bytes around the given
// offset. The window includes a small lead-in before the divergence
// (so the dashboard can show "what came right before the change" as
// muted common context — the same bytes appear on both got and ref,
// so the token-level diff naturally marks them as common) plus a
// larger trailing window for the actual diff context. 32 bytes total
// only ever caught the divergent token itself, which forced re-running
// the test to understand what was around it. The Markdown drill-down
// trims at render time when needed.
func snippet(b []byte, offset int) string {
	const before = 32
	const after = 256
	start := max(offset-before, 0)
	end := min(offset+after, len(b))
	if start > len(b) {
		start = len(b)
	}
	return string(b[start:end])
}

// compareZip compares two zip archives entry-by-entry. Entries match
// when their names match and their uncompressed contents are
// byte-equal. Zip metadata (mtime, central-directory order,
// compression level) is ignored because two correct round-trippers
// can produce different metadata for semantically identical archives.
func compareZip(got, reference []byte) string {
	gotEntries, err := readZipEntries(got)
	if err != nil {
		return fmt.Sprintf("got is not a valid zip: %v", err)
	}
	refEntries, err := readZipEntries(reference)
	if err != nil {
		return fmt.Sprintf("reference is not a valid zip: %v", err)
	}
	if len(gotEntries) != len(refEntries) {
		gotNames := zipEntryNames(gotEntries)
		refNames := zipEntryNames(refEntries)
		return fmt.Sprintf("zip entry count differs: got %d %v, reference %d %v",
			len(gotEntries), gotNames, len(refEntries), refNames)
	}
	for name, refContent := range refEntries {
		gotContent, ok := gotEntries[name]
		if !ok {
			return fmt.Sprintf("zip entry %q present in reference, missing from got", name)
		}
		if !bytes.Equal(gotContent, refContent) {
			offset := firstDiff(gotContent, refContent)
			return fmt.Sprintf("zip entry %q differs at offset %d: got %q vs %q",
				name, offset, snippet(gotContent, offset), snippet(refContent, offset))
		}
	}
	return ""
}

// readZipEntries returns a map of entry name → uncompressed bytes
// for every file in the archive (directories skipped).
func readZipEntries(data []byte) (map[string][]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(r.File))
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", f.Name, err)
		}
		out[f.Name] = content
	}
	return out, nil
}

// zipEntryNames returns sorted entry names for diagnostics.
func zipEntryNames(entries map[string][]byte) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
