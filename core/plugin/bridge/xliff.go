package bridge

import (
	"os"
	"regexp"
	"strings"
)

// emptyTargetLangRe matches empty target-language="" or target-language=” attributes
// in XLIFF content. These must be stripped before sending to Okapi because Okapi's
// XLIFF writer sets target-language from the RawDocument target locale — if the input
// already has an empty target-language, Okapi produces a duplicate attribute that
// breaks the output XML.
var emptyTargetLangRe = regexp.MustCompile(`\s+target-language\s*=\s*["']['"]`)

// isXLIFFFilter returns true if the filter class is an XLIFF filter
// (either XLIFF 1.x or XLIFF 2.x).
func isXLIFFFilter(filterClass string) bool {
	return strings.Contains(filterClass, "XLIFFFilter") || strings.Contains(filterClass, "XLIFF2Filter")
}

// stripEmptyTargetLanguage removes empty target-language="" attributes from
// XLIFF content bytes.
func stripEmptyTargetLanguage(content []byte) []byte {
	if !emptyTargetLangRe.Match(content) {
		return content
	}
	return emptyTargetLangRe.ReplaceAll(content, nil)
}

// stripEmptyTargetLanguageFile checks if a file contains empty target-language
// attributes. If so, it creates a temp file with the attributes stripped and
// returns the temp file path. If no stripping is needed, returns the original
// path and no cleanup function.
//
// The caller must call the returned cleanup function (if non-nil) to remove
// the temp file when done.
func stripEmptyTargetLanguageFile(path string) (resultPath string, cleanup func(), err error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	if !emptyTargetLangRe.Match(content) {
		return path, nil, nil
	}

	stripped := emptyTargetLangRe.ReplaceAll(content, nil)

	tmp, err := os.CreateTemp("", "neokapi-xliff-*")
	if err != nil {
		return "", nil, err
	}

	if _, err := tmp.Write(stripped); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", nil, err
	}

	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
}
