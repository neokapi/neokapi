//go:build parity

package parity

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// CompareEvents asserts that two part streams agree on their canonical
// projection (see CanonicalPart). It logs a structured diff on mismatch
// and returns whether the streams matched, so callers can choose
// between t.Fatal and t.Error.
func CompareEvents(t *testing.T, native, bridge []*model.Part) bool {
	t.Helper()
	gotNative := Canonicalize(native)
	gotBridge := Canonicalize(bridge)

	if diff := canonicalDiff(gotBridge, gotNative); diff != "" {
		t.Errorf("event-level parity mismatch (-bridge +native):\n%s\n%s", diff, partSummary(native, bridge))
		return false
	}
	return true
}

// canonicalDiff returns a multi-line diff between two CanonicalPart
// slices. Returns "" when they match.
func canonicalDiff(want, got []CanonicalPart) string {
	if len(want) != len(got) {
		var buf strings.Builder
		fmt.Fprintf(&buf, "len mismatch: bridge=%d native=%d\n", len(want), len(got))
		n := max(len(got), len(want))
		for i := range n {
			var w, g string
			if i < len(want) {
				w = canonicalLine(want[i])
			} else {
				w = "<missing>"
			}
			if i < len(got) {
				g = canonicalLine(got[i])
			} else {
				g = "<missing>"
			}
			marker := "  "
			if w != g {
				marker = "* "
			}
			fmt.Fprintf(&buf, "%s[%d]\n  - %s\n  + %s\n", marker, i, w, g)
		}
		return buf.String()
	}
	var buf strings.Builder
	mismatched := 0
	for i := range want {
		w, g := canonicalLine(want[i]), canonicalLine(got[i])
		if w != g {
			mismatched++
			fmt.Fprintf(&buf, "[%d]\n  - %s\n  + %s\n", i, w, g)
		}
	}
	if mismatched == 0 {
		return ""
	}
	return buf.String()
}

// canonicalLine renders a CanonicalPart on a single line for diff output.
func canonicalLine(c CanonicalPart) string {
	var fields []string
	add := func(name, value string) {
		if value == "" {
			return
		}
		fields = append(fields, fmt.Sprintf("%s=%q", name, value))
	}
	addBool := func(name string, value bool) {
		if !value {
			return
		}
		fields = append(fields, fmt.Sprintf("%s=%t", name, value))
	}
	addInt := func(name string, value int) {
		if value == 0 {
			return
		}
		fields = append(fields, fmt.Sprintf("%s=%d", name, value))
	}
	add("BlockID", c.BlockID)
	addBool("Translatable", c.Translatable)
	add("Source", c.Source)
	add("Targets", c.Targets)
	add("GroupID", c.GroupID)
	add("GroupType", c.GroupType)
	add("LayerID", c.LayerID)
	add("LayerName", c.LayerName)
	add("DataID", c.DataID)
	add("MediaMime", c.MediaMime)
	addInt("MediaSize", c.MediaSize)
	if len(fields) == 0 {
		return c.Type.String()
	}
	return c.Type.String() + " " + strings.Join(fields, " ")
}

// CompareBytes asserts byte-exact equality, with a hex-style diff on
// mismatch.
func CompareBytes(t *testing.T, want, got []byte) bool {
	t.Helper()
	if bytes.Equal(want, got) {
		return true
	}
	t.Errorf("byte-level parity mismatch:\n  want: %d bytes (sha=%s)\n  got:  %d bytes (sha=%s)\n  first diff: %s",
		len(want), shortHash(want), len(got), shortHash(got), firstDiff(want, got))
	return false
}

// CompareBlockText asserts that the two streams contain the same
// rendered translatable text in the same order, ignoring everything
// else. Useful when implementations differ on segmentation but must
// agree on extracted content.
func CompareBlockText(t *testing.T, native, bridge []*model.Part) bool {
	t.Helper()
	wantText := joinBlockText(native)
	gotText := joinBlockText(bridge)
	if wantText == gotText {
		return true
	}
	t.Errorf("block-text parity mismatch:\n  bridge: %q\n  native: %q", gotText, wantText)
	return false
}

// CompareBlockTextSoft is the non-failing variant of CompareBlockText.
// It logs the mismatch but doesn't call t.Errorf, so the surrounding
// subtest still passes as far as `go test` is concerned. Use for
// auto-generated fixtures that explore divergence without blocking
// CI; the boolean return drives the parity-report status so the
// dashboard still shows the truth.
func CompareBlockTextSoft(t *testing.T, native, bridge []*model.Part) bool {
	t.Helper()
	wantText := joinBlockText(native)
	gotText := joinBlockText(bridge)
	if wantText == gotText {
		return true
	}
	t.Logf("block-text parity mismatch (informational):\n  bridge: %q\n  native: %q", gotText, wantText)
	return false
}

// joinBlockText concatenates rendered block text in stream order.
func joinBlockText(parts []*model.Part) string {
	var buf strings.Builder
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || !b.Translatable {
			continue
		}
		buf.WriteString(renderBlockSource(b))
		buf.WriteByte('\n')
	}
	return buf.String()
}

func partSummary(native, bridge []*model.Part) string {
	var buf strings.Builder
	buf.WriteString("native parts:\n")
	for i, p := range native {
		fmt.Fprintf(&buf, "  [%d] %s\n", i, p.Type)
	}
	buf.WriteString("bridge parts:\n")
	for i, p := range bridge {
		fmt.Fprintf(&buf, "  [%d] %s\n", i, p.Type)
	}
	return buf.String()
}

func firstDiff(a, b []byte) string {
	n := min(len(b), len(a))
	for i := range n {
		if a[i] != b[i] {
			lo := max(i-8, 0)
			hi := min(i+24, n)
			return fmt.Sprintf("offset %d: want %q got %q", i, a[lo:hi], b[lo:hi])
		}
	}
	return fmt.Sprintf("length differs at %d", n)
}

func shortHash(b []byte) string {
	const fnvOffset uint64 = 14695981039346656037
	const fnvPrime uint64 = 1099511628211
	h := fnvOffset
	for _, c := range b {
		h ^= uint64(c)
		h *= fnvPrime
	}
	return fmt.Sprintf("%016x", h)
}
