package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// pseudoNewRunes is the destination set of Okapi's SCRIPT_EXT_LATIN
// substitution table — the Latin Extended glyphs that replace ASCII
// letters during pseudo-translation. Kept verbatim in sync with
// cli/parity/roundtrip/pseudo.go's extLatinNewChars (the upstream
// TextModificationStep TYPE_EXTREPLACE map). The set is the canonical
// signature of "this content has been pseudo-translated".
const pseudoNewRunes = "ÀàßƀĆćĎďĒēƑƒĜĝĤĥ" +
	"ĨĩĵĴĶķĹĺŃńŌōƤƥǪǫŔŕ" +
	"ŚśŢţŨũŴŵŶŷŹź"

// pseudoRuneSet is the runtime lookup table.
var pseudoRuneSet = func() map[rune]struct{} {
	m := make(map[rune]struct{})
	for _, r := range pseudoNewRunes {
		m[r] = struct{}{}
	}
	return m
}()

// VerifyResult records what verifyPseudoOutput found for one file.
// PseudoChars is the count of SCRIPT_EXT_LATIN destination runes in
// the (possibly-unzipped) output. ScannedBytes is the total UTF-8
// content scanned, useful for spotting empty outputs. Verified is
// false when PseudoChars == 0 and the input had any letters at all —
// a signal that pseudo-translate ran but produced no substitutions
// (silent no-op / dropped content / wrong filter).
type VerifyResult struct {
	PseudoChars  int    `json:"pseudoChars"`
	ScannedBytes int64  `json:"scannedBytes"`
	Verified     bool   `json:"verified"`
	Reason       string `json:"reason,omitempty"`
}

// verifyPseudoOutput counts SCRIPT_EXT_LATIN destination runes in the
// given output file. For zip-based formats (PK\x03\x04 magic), it
// recursively scans every text-like entry (.xml / .rels / .xhtml /
// any entry whose first non-whitespace byte is `<`). Binary entries
// (images, fonts) are skipped — they can't hold pseudo'd text.
//
// `inputLetterCount` is the number of ASCII letters in the input
// (lower-bound estimate of how many pseudo substitutions could fire).
// When it's > 0 and PseudoChars == 0, Verified is false: the engine
// either dropped the translatable content or short-circuited pseudo.
func verifyPseudoOutput(outputPath string, inputLetterCount int) (VerifyResult, error) {
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("read %s: %w", outputPath, err)
	}
	res := VerifyResult{ScannedBytes: int64(len(data))}

	if isZipBytes(data) {
		count, scanned, err := countPseudoRunesInZip(data)
		if err != nil {
			return res, fmt.Errorf("scan zip %s: %w", outputPath, err)
		}
		res.PseudoChars = count
		res.ScannedBytes = scanned
	} else {
		res.PseudoChars = countPseudoRunes(data)
	}

	// Heuristic: pseudo'd output should contain SOME pseudo chars when
	// the input contained letters. Empty fixtures (no extractable
	// translatable content) get Verified=true regardless.
	switch {
	case inputLetterCount == 0:
		res.Verified = true
		res.Reason = "input has no letters to pseudo-translate"
	case res.PseudoChars == 0:
		res.Verified = false
		res.Reason = fmt.Sprintf("zero pseudo runes in output (input has %d letters) — pseudo may have silently no-op'd or content was dropped", inputLetterCount)
	default:
		res.Verified = true
	}
	return res, nil
}

// countPseudoRunes returns the number of SCRIPT_EXT_LATIN destination
// runes in the given UTF-8 byte slice. Invalid UTF-8 sequences are
// skipped — they can't carry pseudo runes anyway.
func countPseudoRunes(b []byte) int {
	count := 0
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError {
			b = b[1:]
			continue
		}
		if _, ok := pseudoRuneSet[r]; ok {
			count++
		}
		b = b[size:]
	}
	return count
}

// countLetters returns the number of ASCII letters in the input
// bytes. For zip-based formats, drills into text-like inner entries
// so a .odt baseline matches what a downstream pseudo-translate could
// possibly substitute (only the text inside content.xml/styles.xml/
// meta.xml has ASCII content worth pseudoing — the zip's binary
// entries don't). Returns 0 on read error so the caller falls back
// to "input has no letters" Verified=true (conservative).
func countLetters(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	if isZipBytes(data) {
		total := 0
		_, _, _ = walkZipText(data, func(name string, body []byte) {
			total += countASCIILetters(body)
		})
		return total
	}
	return countASCIILetters(data)
}

func countASCIILetters(b []byte) int {
	count := 0
	for _, c := range b {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			count++
		}
	}
	return count
}

// isZipBytes returns true if the byte slice starts with the zip
// local-file-header magic. Catches .odt/.docx/.idml/.epub regardless
// of file extension.
func isZipBytes(b []byte) bool {
	return len(b) >= 4 && b[0] == 'P' && b[1] == 'K' && b[2] == 3 && b[3] == 4
}

// countPseudoRunesInZip opens the zip and sums pseudo-rune counts
// across all text-like entries.
func countPseudoRunesInZip(zipBytes []byte) (count int, scanned int64, err error) {
	_, scanned, err = walkZipText(zipBytes, func(name string, body []byte) {
		count += countPseudoRunes(body)
	})
	return count, scanned, err
}

// walkZipText invokes `fn` for each text-like entry in the zip. An
// entry is text-like if its name ends in .xml/.rels/.xhtml/.txt OR
// its first non-whitespace byte is `<`. Returns the number of entries
// visited and the total bytes scanned (the callback decides how to
// aggregate). Binary entries (images, fonts) are skipped.
func walkZipText(zipBytes []byte, fn func(name string, body []byte)) (entries int, scanned int64, err error) {
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return 0, 0, err
	}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		if !isLikelyTextEntry(f.Name, body) {
			continue
		}
		fn(f.Name, body)
		entries++
		scanned += int64(len(body))
	}
	return entries, scanned, nil
}

func isLikelyTextEntry(name string, body []byte) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".xml"),
		strings.HasSuffix(lower, ".rels"),
		strings.HasSuffix(lower, ".xhtml"),
		strings.HasSuffix(lower, ".txt"),
		strings.HasSuffix(lower, ".json"),
		strings.HasSuffix(lower, ".html"),
		strings.HasSuffix(lower, ".htm"):
		return true
	}
	for _, b := range body {
		switch b {
		case ' ', '\t', '\r', '\n', 0xef, 0xbb, 0xbf:
			continue
		case '<':
			return true
		default:
			return false
		}
	}
	return false
}
