//go:build integration

package compat

import (
	"archive/zip"
	"bytes"
	"io"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// zipEntries extracts all files from a ZIP archive into a map of path → content.
func zipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err, "opening ZIP")

	entries := make(map[string][]byte)
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err, "opening ZIP entry %s", f.Name)
		content, err := io.ReadAll(rc)
		require.NoError(t, err, "reading ZIP entry %s", f.Name)
		rc.Close()
		entries[f.Name] = content
	}
	return entries
}

// assertZIPEqual compares two ZIP archives entry-by-entry, comparing XML content
// of .xml and .rels files and binary content of everything else.
func assertZIPEqual(t *testing.T, label string, expected, actual []byte) {
	t.Helper()

	expectedEntries := zipEntries(t, expected)
	actualEntries := zipEntries(t, actual)

	// Compare entry lists.
	var expectedKeys, actualKeys []string
	for k := range expectedEntries {
		expectedKeys = append(expectedKeys, k)
	}
	for k := range actualEntries {
		actualKeys = append(actualKeys, k)
	}
	sort.Strings(expectedKeys)
	sort.Strings(actualKeys)

	if !assert.Equal(t, expectedKeys, actualKeys, "%s: ZIP entry lists differ", label) {
		return
	}

	// Compare each entry.
	for _, name := range expectedKeys {
		ec := expectedEntries[name]
		ac := actualEntries[name]

		if isXMLEntry(name) {
			// Normalize whitespace for XML comparison.
			assert.Equal(t, normalizeXMLWhitespace(ec), normalizeXMLWhitespace(ac),
				"%s: XML content differs in %s", label, name)
		} else {
			assert.Equal(t, ec, ac, "%s: binary content differs in %s", label, name)
		}
	}
}

// isXMLEntry returns true for files that should be compared as XML.
func isXMLEntry(name string) bool {
	return strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels")
}

// normalizeXMLWhitespace trims leading/trailing whitespace from each line
// and collapses empty lines — a rough normalization for comparing XML across
// implementations that may differ in formatting.
func normalizeXMLWhitespace(data []byte) string {
	lines := strings.Split(string(data), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return strings.Join(result, "\n")
}

// --- HTML normalization ---
//
// The Okapi bridge and tikal introduce known differences during identity
// roundtrip. Native neokapi preserves the original bytes via skeleton store;
// the normalizer brings both outputs to a common form before comparison.
//
//  1. Encoding: bridge/tikal transcode to UTF-8 and rewrite charset decls.
//  2. Attribute whitespace: bridge collapses double spaces in attr values.
//  3. HTML entities: bridge decodes character references to literal chars
//     (e.g. &gt; → >, &copy; → ©) and re-encodes some differently.
//  4. Block whitespace: bridge collapses whitespace between block elements.
//  5. Text whitespace: bridge/tikal collapse runs of whitespace (including
//     newlines) inside text content to single spaces. Native preserves the
//     original whitespace verbatim via skeleton store.

var (
	// Matches <meta ... charset="..."> with double quotes.
	htmlCharsetMetaDQRe = regexp.MustCompile(`(?i)(<meta\s[^>]*charset\s*=\s*")([^"]*)(")`)
	// Matches <meta ... charset='...'> with single quotes.
	htmlCharsetMetaSQRe = regexp.MustCompile(`(?i)(<meta\s[^>]*charset\s*=\s*')([^']*)(')`)
	// Matches charset=VALUE in content-type strings (unquoted charset value).
	htmlCharsetContentRe = regexp.MustCompile(`(?i)(charset\s*=\s*)([^\s;"']+)`)

	// Matches double-quoted attribute values.
	htmlAttrDQRe = regexp.MustCompile(`(=\s*")([^"]*)(")`)
	// Matches single-quoted attribute values.
	htmlAttrSQRe = regexp.MustCompile(`(=\s*')([^']*)(')`)

	// Matches whitespace-only runs between tags: >  \n  <
	htmlInterTagWSRe = regexp.MustCompile(`(>)([ \t]*\n[ \t]*)(<)`)
)

// normalizeHTML applies normalization to HTML output to account for known
// bridge/tikal differences. Both native and bridge outputs are normalized
// before comparison so that benign differences are ignored.
func normalizeHTML(data []byte) []byte {
	s := string(data)

	// 1. Normalize charset declarations to UTF-8.
	s = htmlCharsetMetaDQRe.ReplaceAllString(s, `${1}UTF-8${3}`)
	s = htmlCharsetMetaSQRe.ReplaceAllString(s, `${1}UTF-8${3}`)
	s = htmlCharsetContentRe.ReplaceAllString(s, `${1}UTF-8`)

	// 2. Decode all HTML character references to their literal form.
	//    This normalizes entity differences (e.g. &gt; vs >, &copy; vs ©,
	//    &#39; vs &amp;#39;). We decode both sides so they match.
	s = decodeHTMLEntities(s)

	// 3. Collapse and trim whitespace in attribute values.
	s = htmlAttrDQRe.ReplaceAllStringFunc(s, func(match string) string {
		subs := htmlAttrDQRe.FindStringSubmatch(match)
		if subs == nil {
			return match
		}
		return subs[1] + strings.TrimSpace(collapseSpaces(subs[2])) + subs[3]
	})
	s = htmlAttrSQRe.ReplaceAllStringFunc(s, func(match string) string {
		subs := htmlAttrSQRe.FindStringSubmatch(match)
		if subs == nil {
			return match
		}
		return subs[1] + strings.TrimSpace(collapseSpaces(subs[2])) + subs[3]
	})

	// 4. Normalize whitespace between tags: collapse >ws< to ><
	//    to handle bridge reformatting around block elements.
	s = htmlInterTagWSRe.ReplaceAllString(s, `${1}${3}`)

	// 5. Collapse whitespace in text content (between tags).
	//    Bridge/tikal collapse newlines+tabs to single spaces in text nodes.
	s = collapseTextWhitespace(s)

	return []byte(s)
}

// decodeHTMLEntities decodes HTML character references in text content and
// attribute values, normalizing entity representation differences between
// implementations. We parse and re-render to get canonical output.
func decodeHTMLEntities(s string) string {
	// Use html.UnescapeString to decode all entities (&gt; &copy; &#39; etc.)
	// but we need to be careful not to break HTML structure. We only decode
	// text content between tags and attribute values.
	var buf strings.Builder
	buf.Grow(len(s))

	i := 0
	for i < len(s) {
		if s[i] == '<' {
			// Inside a tag — copy tag, decoding attribute values.
			tagEnd := strings.IndexByte(s[i:], '>')
			if tagEnd < 0 {
				buf.WriteString(s[i:])
				break
			}
			tagEnd += i + 1
			buf.WriteString(decodeTagAttrs(s[i:tagEnd]))
			i = tagEnd
		} else {
			// Text content — find the next tag and decode the text.
			nextTag := strings.IndexByte(s[i:], '<')
			if nextTag < 0 {
				buf.WriteString(html.UnescapeString(s[i:]))
				break
			}
			buf.WriteString(html.UnescapeString(s[i : i+nextTag]))
			i += nextTag
		}
	}
	return buf.String()
}

// decodeTagAttrs decodes HTML entities inside attribute values of a tag,
// preserving the tag structure.
func decodeTagAttrs(tag string) string {
	var buf strings.Builder
	buf.Grow(len(tag))

	i := 0
	for i < len(tag) {
		// Find next attribute value (="..." or ='...')
		eqPos := strings.IndexByte(tag[i:], '=')
		if eqPos < 0 {
			buf.WriteString(tag[i:])
			break
		}
		eqPos += i
		buf.WriteString(tag[i : eqPos+1])
		i = eqPos + 1

		// Skip whitespace after =
		for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t') {
			buf.WriteByte(tag[i])
			i++
		}
		if i >= len(tag) {
			break
		}

		quote := tag[i]
		if quote == '"' || quote == '\'' {
			buf.WriteByte(quote)
			i++
			// Find closing quote.
			end := strings.IndexByte(tag[i:], quote)
			if end < 0 {
				buf.WriteString(tag[i:])
				break
			}
			// Decode entities within attribute value.
			buf.WriteString(html.UnescapeString(tag[i : i+end]))
			buf.WriteByte(quote)
			i += end + 1
		}
	}
	return buf.String()
}

// collapseTextWhitespace collapses runs of whitespace (spaces, tabs, newlines)
// inside text content (between > and <) to single spaces.
func collapseTextWhitespace(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '<' {
			// Inside a tag — copy verbatim until >
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				buf.WriteString(s[i:])
				break
			}
			end += i + 1
			buf.WriteString(s[i:end])
			i = end
		} else {
			// Text content — collapse whitespace runs (including newlines) to single space.
			nextTag := strings.IndexByte(s[i:], '<')
			if nextTag < 0 {
				buf.WriteString(collapseSpaces(s[i:]))
				break
			}
			buf.WriteString(collapseSpaces(s[i : i+nextTag]))
			i += nextTag
		}
	}
	return buf.String()
}

// collapseSpaces collapses runs of whitespace (spaces, tabs, newlines) to a
// single space. Used for normalizing attribute values and text content.
func collapseSpaces(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				buf.WriteByte(' ')
				inSpace = true
			}
		} else {
			buf.WriteRune(r)
			inSpace = false
		}
	}
	return buf.String()
}
