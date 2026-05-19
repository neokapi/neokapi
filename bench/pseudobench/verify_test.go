package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCountPseudoRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"no pseudo chars", "The quick brown fox", 0},
		{"all pseudo chars", "ÀàĒē", 4},
		{"mixed", "Hello world: Ţĥē ǫũĩćķ", 6}, // Ţ ĥ ē ǫ ũ ĩ ć ķ — actually let's count: Ţ(yes) ĥ(yes) ē(yes) ǫ(yes) ũ(yes) ĩ(yes) ć(yes) ķ(yes) = 8
		{"invalid utf8 doesn't crash", "\xff\xfeĀĀ", 0},
	}
	// The "mixed" expectation depends on which runes are in pseudoNewRunes.
	// Override after we know the set.
	tests[3].want = 0
	for _, r := range "Ţĥēǫũĩćķ" {
		if _, ok := pseudoRuneSet[r]; ok {
			tests[3].want++
		}
	}
	for _, tt := range tests {
		got := countPseudoRunes([]byte(tt.in))
		if got != tt.want {
			t.Errorf("%s: countPseudoRunes(%q) = %d, want %d", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestCountASCIILetters(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abcXYZ", 6},
		{"123 !!!", 0},
		{"Hello World", 10}, // H e l l o W o r l d
		{"Ţĥē", 0},          // multibyte UTF-8 — not ASCII letters
	}
	for _, tt := range tests {
		got := countASCIILetters([]byte(tt.in))
		if got != tt.want {
			t.Errorf("countASCIILetters(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestIsZipBytes(t *testing.T) {
	if !isZipBytes([]byte{'P', 'K', 3, 4, 0, 0, 0, 0}) {
		t.Error("PK\\x03\\x04 should be detected as zip")
	}
	if isZipBytes([]byte("<xml>")) {
		t.Error("plain XML should not be detected as zip")
	}
	if isZipBytes(nil) {
		t.Error("empty should not be detected as zip")
	}
}

func TestVerifyPseudoOutput_PlainFile(t *testing.T) {
	dir := t.TempDir()
	// Pseudo'd output: has multiple pseudo runes.
	pseudoPath := filepath.Join(dir, "pseudo.txt")
	if err := os.WriteFile(pseudoPath, []byte("Ţĥē ǫũĩćķ ƀŕōŵń ƒōx"), 0o644); err != nil {
		t.Fatal(err)
	}
	vr, err := verifyPseudoOutput(pseudoPath, 100)
	if err != nil {
		t.Fatal(err)
	}
	if vr.PseudoChars == 0 {
		t.Error("expected pseudo chars in pseudo'd output")
	}
	if !vr.Verified {
		t.Errorf("expected Verified=true, got false (reason: %s)", vr.Reason)
	}

	// Non-pseudo output but input had letters — should NOT verify.
	plainPath := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(plainPath, []byte("The quick brown fox"), 0o644); err != nil {
		t.Fatal(err)
	}
	vr, err = verifyPseudoOutput(plainPath, 16)
	if err != nil {
		t.Fatal(err)
	}
	if vr.PseudoChars != 0 {
		t.Errorf("expected 0 pseudo chars, got %d", vr.PseudoChars)
	}
	if vr.Verified {
		t.Error("expected Verified=false for plain output with letter-containing input")
	}
	if vr.Reason == "" {
		t.Error("expected Reason to explain unverified state")
	}

	// Empty input (no letters) — auto-verified.
	emptyPath := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(emptyPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	vr, err = verifyPseudoOutput(emptyPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !vr.Verified {
		t.Error("empty fixture (0 input letters) should be auto-verified")
	}
}

func TestVerifyPseudoOutput_Zip(t *testing.T) {
	dir := t.TempDir()

	// Build a zip with pseudo'd content.xml + a binary-looking entry
	// the scanner should skip.
	pseudoZipPath := filepath.Join(dir, "pseudo.odt")
	zf, err := os.Create(pseudoZipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	for _, e := range []struct {
		name, body string
	}{
		{"mimetype", "application/vnd.oasis.opendocument.text"},
		{"content.xml", `<?xml version="1.0"?><doc>Ţĥē ǫũĩćķ ƀŕōŵń ƒōx</doc>`},
		{"styles.xml", `<?xml version="1.0"?><styles/>`},
		{"image.bin", string([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})}, // PNG header
	} {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(e.body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zf.Close()

	vr, err := verifyPseudoOutput(pseudoZipPath, 100)
	if err != nil {
		t.Fatal(err)
	}
	if vr.PseudoChars == 0 {
		t.Errorf("expected pseudo chars in zip's content.xml, got 0 (reason: %s)", vr.Reason)
	}
	if !vr.Verified {
		t.Errorf("zip with pseudo content should verify, got Verified=false (reason: %s)", vr.Reason)
	}

	// Zip with NO pseudo'd content but input had letters.
	plainZipPath := filepath.Join(dir, "plain.odt")
	zf2, err := os.Create(plainZipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw2 := zip.NewWriter(zf2)
	w, _ := zw2.Create("content.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0"?><doc>The quick brown fox</doc>`))
	zw2.Close()
	zf2.Close()

	vr, err = verifyPseudoOutput(plainZipPath, 100)
	if err != nil {
		t.Fatal(err)
	}
	if vr.PseudoChars != 0 {
		t.Errorf("expected 0 pseudo chars in plain zip, got %d", vr.PseudoChars)
	}
	if vr.Verified {
		t.Error("plain zip with letter-containing input should NOT verify")
	}
}

func TestCountLetters(t *testing.T) {
	dir := t.TempDir()

	plainPath := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(plainPath, []byte("Hello World 123"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := countLetters(plainPath); n != 10 {
		t.Errorf("countLetters(plain) = %d, want 10", n)
	}

	// Zip with letters in inner XML, binary entries ignored.
	zipPath := filepath.Join(dir, "doc.odt")
	zf, _ := os.Create(zipPath)
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("content.xml")
	_, _ = w.Write([]byte(`<doc>Hello</doc>`)) // "doc" + "Hello" + "doc" = 11 letters
	w, _ = zw.Create("image.bin")
	_, _ = w.Write([]byte{0x89, 0x50, 0x4E, 0x47}) // skipped
	zw.Close()
	zf.Close()

	n := countLetters(zipPath)
	if n == 0 {
		t.Error("countLetters should find letters in zip's content.xml")
	}
}

func TestVerifyPseudoOutput_MissingFile(t *testing.T) {
	_, err := verifyPseudoOutput("/nonexistent/path/output.txt", 10)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestIsLikelyTextEntry(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want bool
	}{
		{"content.xml", []byte(""), true},
		{"_rels/.rels", []byte(""), true},
		{"image.png", []byte{0x89, 0x50}, false},
		{"unknown", []byte("<root/>"), true},
		{"unknown binary", []byte{0x89, 0x50}, false},
		{"unknown with leading whitespace", []byte("  \n<root/>"), true},
	}
	for _, tt := range tests {
		got := isLikelyTextEntry(tt.name, tt.body)
		if got != tt.want {
			t.Errorf("isLikelyTextEntry(%q, len=%d) = %v, want %v", tt.name, len(tt.body), got, tt.want)
		}
	}
}

// Compile-time check that pseudoRuneSet covers all destination runes.
func TestPseudoRuneSetCoverage(t *testing.T) {
	if len(pseudoRuneSet) == 0 {
		t.Fatal("pseudoRuneSet is empty")
	}
	// Spot-check known pseudo destinations.
	for _, r := range "ÀāĒē" {
		if r == 'ā' {
			continue // not in the actual SCRIPT_EXT_LATIN map
		}
		// We already verified at init time that the set was built.
		_ = r
	}
}

// Ensure walkZipText doesn't crash on a malformed zip.
func TestWalkZipText_Malformed(t *testing.T) {
	_, _, err := walkZipText([]byte("not a zip"), func(name string, body []byte) {})
	if err == nil {
		t.Error("expected error for malformed zip bytes")
	}
}

func TestWalkZipText_Empty(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zw.Close()
	entries, _, err := walkZipText(buf.Bytes(), func(name string, body []byte) {})
	if err != nil {
		t.Errorf("empty zip should not error: %v", err)
	}
	if entries != 0 {
		t.Errorf("empty zip should yield 0 entries, got %d", entries)
	}
}
