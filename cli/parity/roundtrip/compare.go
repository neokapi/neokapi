//go:build parity

package roundtrip

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"sort"
)

// Divergence captures one engine's failure to match the tikal
// reference. Reported per-engine so the harness lists every
// disagreement at once instead of bailing on the first.
type Divergence struct {
	// Engine identifies which implementation diverged.
	Engine string
	// Reason is a human-readable summary (length mismatch, entry
	// mismatch, byte position of first diff, …).
	Reason string
}

// String renders a divergence for test output.
func (d Divergence) String() string {
	return fmt.Sprintf("%s: %s", d.Engine, d.Reason)
}

// compareToReference returns the empty string when got matches the
// tikal reference exactly, or a one-line diagnostic suitable for a
// Divergence.Reason when they differ. isZip toggles per-entry zip
// comparison: byte-equal across entries (sorted by name), ignoring
// zip metadata like mtime and central-directory ordering that vary
// across implementations.
func compareToReference(got, reference []byte, isZip bool) string {
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
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// snippet returns a short human-readable window of bytes around the
// given offset, bounded so the diagnostic line stays readable.
func snippet(b []byte, offset int) string {
	const window = 32
	start := offset
	end := offset + window
	if end > len(b) {
		end = len(b)
	}
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
