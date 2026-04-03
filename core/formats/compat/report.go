//go:build integration

package compat

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/pmezard/go-difflib/difflib"
)

// comparisonResult holds the output of one format × file × engine-pair comparison.
type comparisonResult struct {
	Format   string
	File     string
	Pair     string // e.g. "native vs bridge"
	Match    bool
	Info     bool           // true for informational comparisons (don't affect pass/fail)
	NativeOK bool           // false if native roundtrip errored
	OtherOK  bool           // false if the other engine errored
	Diff     string         // unified diff (normalized if normalizer was used)
	RawDiff  string         // unified diff of raw output (empty if no normalizer or raw already matches)
	RawA     string         // raw output A (for side-by-side diff)
	RawB     string         // raw output B (for side-by-side diff)
	NormA    string         // normalized output A (empty if no normalizer)
	NormB    string         // normalized output B (empty if no normalizer)
	Entries  []zipEntryDiff // non-nil for ZIP formats
}

// zipEntryDiff is a per-entry diff inside a ZIP archive.
type zipEntryDiff struct {
	Name   string
	Match  bool
	Binary bool   // true if content is not valid UTF-8 text
	Diff   string // unified diff or binary summary
	SizeA  int
	SizeB  int
}

// reportCollector accumulates comparison results and writes an HTML report.
type reportCollector struct {
	mu      sync.Mutex
	results []comparisonResult
}

// groupKey identifies a format+file group in the report.
type groupKey struct{ format, file string }

// groupEntry is one comparison result within a group.
type groupEntry struct {
	index  int
	result comparisonResult
}

func (rc *reportCollector) add(r comparisonResult) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.results = append(rc.results, r)
}

// compareText compares two outputs, records the result, and returns whether they match.
func (rc *reportCollector) compareText(format, file, pair string, a, b []byte) bool {
	if bytes.Equal(a, b) {
		rc.add(comparisonResult{Format: format, File: file, Pair: pair, Match: true, NativeOK: true, OtherOK: true})
		return true
	}
	diff := unifiedDiff(pair, string(a), string(b))
	rc.add(comparisonResult{Format: format, File: file, Pair: pair, Match: false, NativeOK: true, OtherOK: true,
		Diff: diff, RawA: string(a), RawB: string(b)})
	return false
}

// compareTextInfo records an informational comparison that doesn't affect pass/fail.
// Used for input-vs-output comparisons that show how much each engine deviates
// from the original.
func (rc *reportCollector) compareTextInfo(format, file, pair string, a, b []byte) {
	if bytes.Equal(a, b) {
		rc.add(comparisonResult{Format: format, File: file, Pair: pair, Match: true, Info: true, NativeOK: true, OtherOK: true})
		return
	}
	diff := unifiedDiff(pair, string(a), string(b))
	rc.add(comparisonResult{Format: format, File: file, Pair: pair, Match: false, Info: true, NativeOK: true, OtherOK: true,
		Diff: diff, RawA: string(a), RawB: string(b)})
}

// compareTextNormalized compares using normalized outputs but records both the
// raw and normalized diffs in the report so reviewers can see exactly what
// changed vs what the normalizer absorbed.
func (rc *reportCollector) compareTextNormalized(format, file, pair string, rawA, rawB, normA, normB []byte) bool {
	rawMatch := bytes.Equal(rawA, rawB)
	normMatch := bytes.Equal(normA, normB)

	r := comparisonResult{
		Format:   format,
		File:     file,
		Pair:     pair,
		Match:    normMatch,
		NativeOK: true,
		OtherOK:  true,
	}

	if !normMatch {
		r.Diff = unifiedDiff(pair+" (normalized)", string(normA), string(normB))
	}
	if !rawMatch {
		r.RawDiff = unifiedDiff(pair+" (raw)", string(rawA), string(rawB))
		r.RawA = string(rawA)
		r.RawB = string(rawB)
	}
	// Always store normalized outputs when a normalizer is used, so both
	// side-by-side and unified views can toggle between raw and normalized.
	r.NormA = string(normA)
	r.NormB = string(normB)

	rc.add(r)
	return normMatch
}

// compareParts compares extracted block texts from two implementations.
// This is the event-level comparison (Okapi's approach): it checks that both
// implementations extract the same translatable content, regardless of byte
// differences in the serialized output.
//
// The comparison checks that every substantial block text (≥10 chars) from
// each side appears in the other's concatenated output. This handles
// segmentation differences (one implementation may split content differently)
// and focuses on "no translatable content lost."
func (rc *reportCollector) compareParts(format, file, pair string, textsA, textsB []string) bool {
	concatA := blockTextSet(textsA)
	concatB := blockTextSet(textsB)

	const minLen = 10 // skip trivial blocks (IDs, short labels)

	// Strip non-alphanumeric characters for fuzzy matching fallback.
	// This handles segmentation artifacts where inline code removal leaves
	// punctuation patterns (", , .") that don't appear in the other side.
	alphaA := stripNonAlpha(concatA)
	alphaB := stripNonAlpha(concatB)

	var missingInB, missingInA []string
	for _, t := range textsA {
		if len(t) >= minLen && !strings.Contains(concatB, t) {
			// Fallback: check alphanumeric-only match
			if !strings.Contains(alphaB, stripNonAlpha(t)) {
				missingInB = append(missingInB, t)
			}
		}
	}
	for _, t := range textsB {
		if len(t) >= minLen && !strings.Contains(concatA, t) {
			if !strings.Contains(alphaA, stripNonAlpha(t)) {
				missingInA = append(missingInA, t)
			}
		}
	}

	match := len(missingInB) == 0 && len(missingInA) == 0

	if match {
		rc.add(comparisonResult{
			Format: format, File: file, Pair: pair, Match: true, NativeOK: true, OtherOK: true,
			RawA: fmt.Sprintf("%d blocks", len(textsA)),
			RawB: fmt.Sprintf("%d blocks", len(textsB)),
		})
		return true
	}

	var diffBuf strings.Builder
	if len(missingInB) > 0 {
		fmt.Fprintf(&diffBuf, "--- %d blocks in A missing from B:\n", len(missingInB))
		for _, t := range missingInB {
			if len(t) > 120 {
				t = t[:120] + "..."
			}
			fmt.Fprintf(&diffBuf, "  - %s\n", t)
		}
	}
	if len(missingInA) > 0 {
		fmt.Fprintf(&diffBuf, "+++ %d blocks in B missing from A:\n", len(missingInA))
		for _, t := range missingInA {
			if len(t) > 120 {
				t = t[:120] + "..."
			}
			fmt.Fprintf(&diffBuf, "  + %s\n", t)
		}
	}

	rc.add(comparisonResult{
		Format: format, File: file, Pair: pair, Match: false, NativeOK: true, OtherOK: true,
		Diff: diffBuf.String(),
		RawA: fmt.Sprintf("%d blocks", len(textsA)),
		RawB: fmt.Sprintf("%d blocks", len(textsB)),
	})
	return false
}

// compareZIP compares two ZIP archives entry-by-entry and records the result.
func (rc *reportCollector) compareZIP(format, file, pair string, a, b []byte) bool {
	entriesA := readZIPEntries(a)
	entriesB := readZIPEntries(b)

	allKeys := mergeKeys(entriesA, entriesB)
	sort.Strings(allKeys)

	allMatch := true
	var diffs []zipEntryDiff

	for _, name := range allKeys {
		ca, okA := entriesA[name]
		cb, okB := entriesB[name]

		if !okA || !okB {
			allMatch = false
			summary := "missing in "
			if !okA {
				summary += "first"
			} else {
				summary += "second"
			}
			diffs = append(diffs, zipEntryDiff{Name: name, Match: false, Diff: summary, SizeA: len(ca), SizeB: len(cb)})
			continue
		}

		if bytes.Equal(ca, cb) {
			diffs = append(diffs, zipEntryDiff{Name: name, Match: true, SizeA: len(ca), SizeB: len(cb)})
			continue
		}

		allMatch = false
		if isText(ca) && isText(cb) {
			d := unifiedDiff(name, string(ca), string(cb))
			diffs = append(diffs, zipEntryDiff{Name: name, Match: false, Diff: d, SizeA: len(ca), SizeB: len(cb)})
		} else {
			hashA := sha256sum(ca)
			hashB := sha256sum(cb)
			summary := fmt.Sprintf("binary differs: %d bytes (sha256:%s) vs %d bytes (sha256:%s)", len(ca), hashA[:16], len(cb), hashB[:16])
			diffs = append(diffs, zipEntryDiff{Name: name, Match: false, Binary: true, Diff: summary, SizeA: len(ca), SizeB: len(cb)})
		}
	}

	rc.add(comparisonResult{Format: format, File: file, Pair: pair, Match: allMatch, NativeOK: true, OtherOK: true, Entries: diffs})
	return allMatch
}

// writeReport writes the compat report as a static site to a directory.
// Structure: dir/index.html (summary) + dir/<format>/<file>.html (detail pages).
func (rc *reportCollector) writeReport(dir string) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Sort results by format, file, pair.
	sort.Slice(rc.results, func(i, j int) bool {
		a, b := rc.results[i], rc.results[j]
		if a.Format != b.Format {
			return a.Format < b.Format
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Pair < b.Pair
	})

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Group results by format+file.
	var groupOrder []groupKey
	groups := make(map[groupKey][]groupEntry)
	for i, r := range rc.results {
		key := groupKey{r.Format, r.File}
		if _, ok := groups[key]; !ok {
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], groupEntry{i, r})
	}

	// Write detail pages.
	for _, key := range groupOrder {
		entries := groups[key]
		hasDetail := false
		for _, e := range entries {
			if !e.result.Match || e.result.RawDiff != "" {
				hasDetail = true
				break
			}
		}
		if !hasDetail {
			continue
		}
		pageDir := filepath.Join(dir, key.format)
		if err := os.MkdirAll(pageDir, 0o755); err != nil {
			return err
		}
		pagePath := filepath.Join(pageDir, key.file+".html")
		if err := rc.writeDetailPage(pagePath, key.format, key.file, entries); err != nil {
			return fmt.Errorf("writing detail page %s: %w", pagePath, err)
		}
	}

	// Write index page.
	return rc.writeIndexPage(filepath.Join(dir, "index.html"), groupOrder, groups)
}

// writeIndexPage writes the summary index.html.
func (rc *reportCollector) writeIndexPage(path string, groupOrder []groupKey, groups map[groupKey][]groupEntry) error {
	var buf bytes.Buffer
	buf.WriteString(pageHeader("Format Compatibility Report"))

	passCount, failCount := 0, 0
	for _, r := range rc.results {
		if r.Info {
			continue
		}
		if r.Match {
			passCount++
		} else {
			failCount++
		}
	}
	fmt.Fprintf(&buf, `<div class="summary"><h2>Summary</h2>`)
	fmt.Fprintf(&buf, `<p><span class="badge pass">%d PASS</span> <span class="badge fail">%d FAIL</span> across %d comparisons</p>`,
		passCount, failCount, passCount+failCount)

	buf.WriteString(`<table class="summary-table"><tr><th>Format</th><th>File</th><th>Pair</th><th>Result</th></tr>`)
	for _, r := range rc.results {
		cls := "pass"
		label := "PASS"
		if !r.Match {
			cls = "fail"
			label = "FAIL"
		}
		if r.Info {
			cls = "info"
			if r.Match {
				label = "IDENTICAL"
			} else {
				label = "DIFFERS"
			}
		}
		if r.Match && !r.Info && r.RawDiff != "" {
			label = "PASS (normalized)"
		}
		link := ""
		hasDetail := !r.Match || r.RawDiff != "" || (r.Info && !r.Match)
		if hasDetail {
			link = fmt.Sprintf(` <a href="%s/%s.html">details</a>`, r.Format, r.File)
		}
		fmt.Fprintf(&buf, `<tr class="%s"><td>%s</td><td>%s</td><td>%s</td><td><span class="badge %s">%s</span>%s</td></tr>`,
			cls, html.EscapeString(r.Format), html.EscapeString(r.File), html.EscapeString(r.Pair), cls, label, link)
	}
	buf.WriteString(`</table></div>`)
	buf.WriteString(pageFooter)

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// writeDetailPage writes a single detail page with tabbed comparisons.
func (rc *reportCollector) writeDetailPage(path, format, file string, entries []groupEntry) error {
	var buf bytes.Buffer
	title := fmt.Sprintf("%s / %s", format, file)
	buf.WriteString(pageHeader(title))
	fmt.Fprintf(&buf, `<p><a href="../index.html">← Back to summary</a></p>`)

	// Filter to entries that have diffs.
	var withDiff []groupEntry
	for _, e := range entries {
		if !e.result.Match || e.result.RawDiff != "" {
			withDiff = append(withDiff, e)
		}
	}

	if len(withDiff) == 0 {
		fmt.Fprintf(&buf, `<h2>%s / %s</h2>`, html.EscapeString(format), html.EscapeString(file))
		buf.WriteString(`<p>All comparisons identical.</p>`)
		buf.WriteString(pageFooter)
		return os.WriteFile(path, buf.Bytes(), 0o644)
	}

	// Tab bar.
	buf.WriteString(`<div class="tabs">`)
	for j, e := range withDiff {
		active := ""
		if j == 0 {
			active = " active"
		}
		cls := "pass"
		if !e.result.Match {
			if e.result.Info {
				cls = "info"
			} else {
				cls = "fail"
			}
		}
		fmt.Fprintf(&buf, `<button class="tab-btn%s %s" onclick="switchTab(%d)">%s</button>`,
			active, cls, j, html.EscapeString(e.result.Pair))
	}
	buf.WriteString(`</div>`)

	// Tab panels — each includes its own heading so it updates with the tab.
	for j, e := range withDiff {
		display := "none"
		if j == 0 {
			display = "block"
		}
		fmt.Fprintf(&buf, `<div class="tab-panel" data-tab="%d" style="display:%s">`, j, display)
		fmt.Fprintf(&buf, `<h2>%s / %s — %s</h2>`, html.EscapeString(format), html.EscapeString(file), html.EscapeString(e.result.Pair))
		labelA, labelB := pairLabels(e.result.Pair)
		writeResultPanel(&buf, e.index, e.result, labelA, labelB)
		buf.WriteString(`</div>`)
	}

	buf.WriteString(pageFooter)
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// writeResultPanel renders the diff content for one comparison result.
func writeResultPanel(buf *bytes.Buffer, idx int, r comparisonResult, labelA, labelB string) {
	if len(r.Entries) > 0 {
		// ZIP entry-level comparison.
		buf.WriteString(`<table class="zip-table"><tr><th>Entry</th><th>Result</th><th>Size A</th><th>Size B</th></tr>`)
		for j, e := range r.Entries {
			cls := "pass"
			label := "MATCH"
			if !e.Match {
				cls = "fail"
				label = "DIFF"
			}
			link := ""
			if !e.Match {
				link = fmt.Sprintf(` <a href="#entry-%d-%d">diff</a>`, idx, j)
			}
			fmt.Fprintf(buf, `<tr class="%s"><td>%s</td><td><span class="badge %s">%s</span>%s</td><td>%d</td><td>%d</td></tr>`,
				cls, html.EscapeString(e.Name), cls, label, link, e.SizeA, e.SizeB)
		}
		buf.WriteString(`</table>`)

		for j, e := range r.Entries {
			if e.Match {
				continue
			}
			fmt.Fprintf(buf, `<div class="entry-diff" id="entry-%d-%d">`, idx, j)
			fmt.Fprintf(buf, `<h4>%s</h4>`, html.EscapeString(e.Name))
			if e.Binary {
				fmt.Fprintf(buf, `<pre class="diff binary">%s</pre>`, html.EscapeString(e.Diff))
			} else {
				buf.WriteString(`<pre class="diff">`)
				writeDiffHTML(buf, e.Diff)
				buf.WriteString(`</pre>`)
			}
			buf.WriteString(`</div>`)
		}
		return
	}

	hasNorm := r.NormA != "" || r.NormB != ""
	panelID := fmt.Sprintf("p%d", idx)

	// Toggle bar: layout (side-by-side / unified) + normalization (raw / normalized).
	buf.WriteString(`<div class="view-toggles">`)
	buf.WriteString(`<div class="toggle-group">`)
	fmt.Fprintf(buf, `<button class="view-btn active" onclick="switchLayout('%s','sbs')">Side by side</button>`, panelID)
	fmt.Fprintf(buf, `<button class="view-btn" onclick="switchLayout('%s','unified')">Unified</button>`, panelID)
	buf.WriteString(`</div>`)
	if hasNorm {
		buf.WriteString(`<div class="toggle-group">`)
		fmt.Fprintf(buf, `<button class="view-btn active" data-norm="raw" onclick="switchNorm('%s','raw')">Raw</button>`, panelID)
		fmt.Fprintf(buf, `<button class="view-btn" data-norm="norm" onclick="switchNorm('%s','norm')">Normalized</button>`, panelID)
		buf.WriteString(`</div>`)
		if r.Match {
			fmt.Fprintf(buf, `<span class="norm-hint" data-panel="%s" style="display:none;color:var(--pass-fg);font-size:0.8rem;align-self:center">✓ Equivalent after normalization</span>`, panelID)
		}
	}
	buf.WriteString(`</div>`)

	// Side-by-side raw.
	fmt.Fprintf(buf, `<div class="view-panel" data-panel="%s" data-layout="sbs" data-norm="raw" style="display:block">`, panelID)
	writeSideBySideDiff(buf, r.RawA, r.RawB, labelA, labelB)
	buf.WriteString(`</div>`)

	// Unified raw.
	rawDiff := r.RawDiff
	if rawDiff == "" {
		rawDiff = r.Diff
	}
	if rawDiff == "" && r.RawA != "" {
		rawDiff = unifiedDiff("", r.RawA, r.RawB)
	}
	fmt.Fprintf(buf, `<div class="view-panel" data-panel="%s" data-layout="unified" data-norm="raw" style="display:none">`, panelID)
	buf.WriteString(`<pre class="diff">`)
	writeDiffHTML(buf, rawDiff)
	buf.WriteString(`</pre></div>`)

	if hasNorm {
		// Side-by-side normalized.
		fmt.Fprintf(buf, `<div class="view-panel" data-panel="%s" data-layout="sbs" data-norm="norm" style="display:none">`, panelID)
		if r.Match {
			buf.WriteString(`<p style="color:var(--pass-fg);padding:1rem">All differences were absorbed by normalization — outputs are equivalent.</p>`)
		} else {
			writeSideBySideDiff(buf, r.NormA, r.NormB, labelA+" (normalized)", labelB+" (normalized)")
		}
		buf.WriteString(`</div>`)

		// Unified normalized.
		fmt.Fprintf(buf, `<div class="view-panel" data-panel="%s" data-layout="unified" data-norm="norm" style="display:none">`, panelID)
		if r.Match {
			buf.WriteString(`<p style="color:var(--pass-fg);padding:1rem">All differences were absorbed by normalization — outputs are equivalent.</p>`)
		} else {
			buf.WriteString(`<pre class="diff">`)
			writeDiffHTML(buf, r.Diff)
			buf.WriteString(`</pre>`)
		}
		buf.WriteString(`</div>`)
	}
}

// pairLabels splits a pair string like "native vs bridge" into its two sides.
func pairLabels(pair string) (string, string) {
	parts := strings.SplitN(pair, " vs ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "A", "B"
}

// unifiedDiff produces a unified diff between two strings.
func unifiedDiff(name, a, b string) string {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(a),
		B:        difflib.SplitLines(b),
		FromFile: "expected",
		ToFile:   "actual",
		Context:  3,
	}
	text, _ := difflib.GetUnifiedDiffString(diff)
	if len(text) > 50000 {
		text = text[:50000] + "\n... (truncated)\n"
	}
	return text
}

// writeDiffHTML writes diff text with colored +/- lines.
func writeDiffHTML(buf *bytes.Buffer, diff string) {
	for _, line := range strings.Split(diff, "\n") {
		escaped := html.EscapeString(line)
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			fmt.Fprintf(buf, `<span class="diff-header">%s</span>`+"\n", escaped)
		case strings.HasPrefix(line, "@@"):
			fmt.Fprintf(buf, `<span class="diff-hunk">%s</span>`+"\n", escaped)
		case strings.HasPrefix(line, "+"):
			fmt.Fprintf(buf, `<span class="diff-add">%s</span>`+"\n", escaped)
		case strings.HasPrefix(line, "-"):
			fmt.Fprintf(buf, `<span class="diff-del">%s</span>`+"\n", escaped)
		default:
			buf.WriteString(escaped + "\n")
		}
	}
}

// readZIPEntries extracts all files from a ZIP archive into a map.
func readZIPEntries(data []byte) map[string][]byte {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	entries := make(map[string][]byte)
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		entries[f.Name] = content
	}
	return entries
}

func mergeKeys(a, b map[string][]byte) []string {
	seen := make(map[string]bool)
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys
}

func isText(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// stripNonAlpha removes non-alphanumeric characters for fuzzy text matching.
func stripNonAlpha(s string) string {
	var buf strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// writeSideBySideDiff renders a side-by-side diff table with character-level
// highlighting of the differences within each changed line.
func writeSideBySideDiff(buf *bytes.Buffer, a, b, labelA, labelB string) {
	linesA := difflib.SplitLines(a)
	linesB := difflib.SplitLines(b)

	matcher := difflib.NewMatcher(linesA, linesB)
	opcodes := matcher.GetOpCodes()

	buf.WriteString(`<div class="sbs-diff"><table class="sbs-table">`)
	buf.WriteString(`<colgroup><col class="sbs-num"><col class="sbs-content"><col class="sbs-num"><col class="sbs-content"></colgroup>`)
	fmt.Fprintf(buf, `<thead><tr><th colspan="2">%s</th><th colspan="2">%s</th></tr></thead><tbody>`,
		html.EscapeString(labelA), html.EscapeString(labelB))

	for _, op := range opcodes {
		switch op.Tag {
		case 'e': // equal
			// Show up to 3 context lines around changes.
			start := op.I1
			end := op.I2
			if end-start > 6 {
				// Show first 3 and last 3.
				for i := start; i < start+3 && i < end; i++ {
					writeSBSRow(buf, i+1, op.J1+(i-start)+1, linesA[i], linesB[op.J1+(i-start)], "sbs-eq")
				}
				buf.WriteString(`<tr class="sbs-skip"><td colspan="4">`)
				fmt.Fprintf(buf, "⋮ %d identical lines", end-start-6)
				buf.WriteString(`</td></tr>`)
				for i := end - 3; i < end; i++ {
					writeSBSRow(buf, i+1, op.J1+(i-start)+1, linesA[i], linesB[op.J1+(i-start)], "sbs-eq")
				}
			} else {
				for i := start; i < end; i++ {
					writeSBSRow(buf, i+1, op.J1+(i-start)+1, linesA[i], linesB[op.J1+(i-start)], "sbs-eq")
				}
			}

		case 'r': // replace — pair up lines and highlight char diffs
			maxLen := op.I2 - op.I1
			if op.J2-op.J1 > maxLen {
				maxLen = op.J2 - op.J1
			}
			for k := 0; k < maxLen; k++ {
				var lineA, lineB string
				var numA, numB int
				if op.I1+k < op.I2 {
					lineA = linesA[op.I1+k]
					numA = op.I1 + k + 1
				}
				if op.J1+k < op.J2 {
					lineB = linesB[op.J1+k]
					numB = op.J1 + k + 1
				}
				writeSBSRowHighlighted(buf, numA, numB, lineA, lineB)
			}

		case 'd': // delete — lines only on left
			for i := op.I1; i < op.I2; i++ {
				writeSBSRowDel(buf, i+1, linesA[i])
			}

		case 'i': // insert — lines only on right
			for j := op.J1; j < op.J2; j++ {
				writeSBSRowIns(buf, j+1, linesB[j])
			}
		}
	}

	buf.WriteString(`</tbody></table></div>`)
}

func writeSBSRow(buf *bytes.Buffer, numA, numB int, lineA, lineB, cls string) {
	lineA = strings.TrimRight(lineA, "\n\r")
	lineB = strings.TrimRight(lineB, "\n\r")
	fmt.Fprintf(buf, `<tr class="%s"><td class="sbs-num">%d</td><td class="sbs-code">%s</td>`+
		`<td class="sbs-num">%d</td><td class="sbs-code">%s</td></tr>`,
		cls, numA, html.EscapeString(lineA), numB, html.EscapeString(lineB))
}

func writeSBSRowDel(buf *bytes.Buffer, num int, line string) {
	line = strings.TrimRight(line, "\n\r")
	fmt.Fprintf(buf, `<tr class="sbs-del"><td class="sbs-num">%d</td><td class="sbs-code">%s</td>`+
		`<td class="sbs-num"></td><td class="sbs-code"></td></tr>`,
		num, html.EscapeString(line))
}

func writeSBSRowIns(buf *bytes.Buffer, num int, line string) {
	line = strings.TrimRight(line, "\n\r")
	fmt.Fprintf(buf, `<tr class="sbs-ins"><td class="sbs-num"></td><td class="sbs-code"></td>`+
		`<td class="sbs-num">%d</td><td class="sbs-code">%s</td></tr>`,
		num, html.EscapeString(line))
}

// writeSBSRowHighlighted writes a replace row with character-level highlighting.
func writeSBSRowHighlighted(buf *bytes.Buffer, numA, numB int, lineA, lineB string) {
	lineA = strings.TrimRight(lineA, "\n\r")
	lineB = strings.TrimRight(lineB, "\n\r")

	// Find common prefix and suffix to isolate the changed region.
	runesA := []rune(lineA)
	runesB := []rune(lineB)

	prefixLen := 0
	minLen := len(runesA)
	if len(runesB) < minLen {
		minLen = len(runesB)
	}
	for prefixLen < minLen && runesA[prefixLen] == runesB[prefixLen] {
		prefixLen++
	}

	suffixLen := 0
	for suffixLen < minLen-prefixLen &&
		runesA[len(runesA)-1-suffixLen] == runesB[len(runesB)-1-suffixLen] {
		suffixLen++
	}

	// Build highlighted HTML for each side.
	htmlA := renderHighlighted(runesA, prefixLen, len(runesA)-suffixLen, "sbs-char-del")
	htmlB := renderHighlighted(runesB, prefixLen, len(runesB)-suffixLen, "sbs-char-ins")

	numAStr := ""
	if numA > 0 {
		numAStr = fmt.Sprintf("%d", numA)
	}
	numBStr := ""
	if numB > 0 {
		numBStr = fmt.Sprintf("%d", numB)
	}

	fmt.Fprintf(buf, `<tr class="sbs-chg"><td class="sbs-num">%s</td><td class="sbs-code">%s</td>`+
		`<td class="sbs-num">%s</td><td class="sbs-code">%s</td></tr>`,
		numAStr, htmlA, numBStr, htmlB)
}

// renderHighlighted renders runes with the changed region [start, end) wrapped
// in a highlight span.
func renderHighlighted(runes []rune, start, end int, cls string) string {
	var buf strings.Builder
	if start > 0 {
		buf.WriteString(html.EscapeString(string(runes[:start])))
	}
	if start < end {
		buf.WriteString(`<span class="`)
		buf.WriteString(cls)
		buf.WriteString(`">`)
		buf.WriteString(html.EscapeString(string(runes[start:end])))
		buf.WriteString(`</span>`)
	}
	if end < len(runes) {
		buf.WriteString(html.EscapeString(string(runes[end:])))
	}
	return buf.String()
}

func pageHeader(title string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>` + html.EscapeString(title) + `</title>` + `
<style>
  :root {
    --bg: #0d1117; --fg: #c9d1d9; --border: #30363d;
    --pass-bg: #1a3a2a; --pass-fg: #3fb950;
    --fail-bg: #3a1a1a; --fail-fg: #f85149;
    --diff-add-bg: #1a3a2a; --diff-del-bg: #3a1a1a;
    --hunk-fg: #79c0ff; --header-fg: #8b949e;
    --card-bg: #161b22; --link: #58a6ff;
  }
  @media (prefers-color-scheme: light) {
    :root {
      --bg: #fff; --fg: #1f2328; --border: #d0d7de;
      --pass-bg: #dafbe1; --pass-fg: #1a7f37;
      --fail-bg: #ffebe9; --fail-fg: #cf222e;
      --diff-add-bg: #e6ffec; --diff-del-bg: #ffebe9;
      --hunk-fg: #0550ae; --header-fg: #656d76;
      --card-bg: #f6f8fa; --link: #0969da;
    }
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif;
         background: var(--bg); color: var(--fg); padding: 2rem; max-width: 1400px; margin: 0 auto; }
  h1 { margin-bottom: 0.25rem; }
  h1 small { font-weight: 400; color: var(--header-fg); font-size: 0.5em; }
  h2 { margin: 2rem 0 1rem; border-bottom: 1px solid var(--border); padding-bottom: 0.5rem; }
  h3 { margin: 1.5rem 0 0.75rem; }
  h4 { margin: 1rem 0 0.5rem; color: var(--header-fg); }
  a { color: var(--link); text-decoration: none; }
  a:hover { text-decoration: underline; }

  .badge { display: inline-block; padding: 0.15em 0.6em; border-radius: 1em;
           font-size: 0.75rem; font-weight: 600; }
  .badge.pass { background: var(--pass-bg); color: var(--pass-fg); }
  .badge.fail { background: var(--fail-bg); color: var(--fail-fg); }
  .badge.info { background: var(--card-bg); color: var(--header-fg); border: 1px solid var(--border); }
  tr.info td:last-child { background: var(--card-bg); }

  table { border-collapse: collapse; width: 100%; margin: 1rem 0; }
  th, td { text-align: left; padding: 0.5rem 0.75rem; border: 1px solid var(--border); }
  th { background: var(--card-bg); font-weight: 600; }
  tr.pass td:last-child { background: var(--pass-bg); }
  tr.fail td:last-child { background: var(--fail-bg); }

  .detail { background: var(--card-bg); border: 1px solid var(--border);
            border-radius: 6px; padding: 1rem 1.5rem; margin: 1.5rem 0; }
  .entry-diff { margin: 1rem 0; padding-top: 0.5rem; border-top: 1px solid var(--border); }

  pre.diff { background: var(--bg); border: 1px solid var(--border); border-radius: 6px;
             padding: 1rem; overflow-x: auto; font-size: 0.8rem; line-height: 1.5;
             max-height: 600px; overflow-y: auto; }
  .diff-header { color: var(--header-fg); font-weight: 600; }
  .diff-hunk { color: var(--hunk-fg); }
  .diff-add { background: var(--diff-add-bg); color: var(--pass-fg); }
  .diff-del { background: var(--diff-del-bg); color: var(--fail-fg); }
  .binary { color: var(--header-fg); }

  .zip-table td, .zip-table th { font-size: 0.85rem; }
  .zip-table td:nth-child(3), .zip-table td:nth-child(4) { text-align: right; font-variant-numeric: tabular-nums; }

  /* Side-by-side diff */
  .sbs-diff { overflow-x: auto; margin: 1rem 0; }
  .sbs-table { border-collapse: collapse; width: 100%; font-size: 0.75rem; font-family: 'SF Mono', Menlo, Consolas, monospace; table-layout: fixed; }
  .sbs-table colgroup .sbs-num { width: 3.5em; }
  .sbs-table colgroup .sbs-content { width: calc(50% - 3.5em); }
  .sbs-table thead th { background: var(--card-bg); text-align: center; padding: 0.3rem 0.5rem; font-size: 0.8rem; }
  .sbs-table td { padding: 0 0.5rem; vertical-align: top; border: none; border-right: 1px solid var(--border); white-space: pre-wrap; word-break: break-all; line-height: 1.5; }
  .sbs-table td.sbs-num { color: var(--header-fg); text-align: right; user-select: none; padding-right: 0.5em; border-right: 1px solid var(--border); min-width: 3em; white-space: nowrap; }
  .sbs-table tr.sbs-eq td.sbs-code { color: var(--fg); }
  .sbs-table tr.sbs-chg td.sbs-code:nth-child(2) { background: var(--diff-del-bg); }
  .sbs-table tr.sbs-chg td.sbs-code:nth-child(4) { background: var(--diff-add-bg); }
  .sbs-table tr.sbs-del td.sbs-code:nth-child(2) { background: var(--diff-del-bg); }
  .sbs-table tr.sbs-ins td.sbs-code:nth-child(4) { background: var(--diff-add-bg); }
  .sbs-char-del { background: #fb3e4480; border-radius: 2px; }
  .sbs-char-ins { background: #3fb95080; border-radius: 2px; }
  .sbs-table tr.sbs-skip td { text-align: center; color: var(--header-fg); padding: 0.3rem; font-style: italic; border-bottom: 1px solid var(--border); }
  @media (prefers-color-scheme: light) {
    .sbs-char-del { background: #ff818266; }
    .sbs-char-ins { background: #3fb95066; }
  }

  details { margin: 0.5rem 0; }
  summary { cursor: pointer; color: var(--link); font-size: 0.85rem; padding: 0.3rem 0; }

  /* Tabs */
  .tabs { display: flex; gap: 0; border-bottom: 2px solid var(--border); margin-bottom: 1rem; }
  .tab-btn { background: none; border: none; border-bottom: 2px solid transparent;
             padding: 0.5rem 1rem; margin-bottom: -2px; cursor: pointer;
             font-size: 0.85rem; font-weight: 500; color: var(--header-fg); }
  .tab-btn:hover { color: var(--fg); border-bottom-color: var(--header-fg); }
  .tab-btn.active { color: var(--fg); border-bottom-color: var(--link); }
  .tab-btn.fail.active { border-bottom-color: var(--fail-fg); }
  .tab-btn.info.active { border-bottom-color: var(--header-fg); }
  .tab-btn::before { content: ""; display: inline-block; width: 8px; height: 8px;
                     border-radius: 50%; margin-right: 0.4rem; vertical-align: middle; }
  .tab-btn.pass::before { background: var(--pass-fg); }
  .tab-btn.fail::before { background: var(--fail-fg); }
  .tab-btn.info::before { background: var(--header-fg); }

  /* View toggles */
  .view-toggles { display: flex; gap: 1rem; margin-bottom: 1rem; align-items: center; }
  .toggle-group { display: flex; gap: 0; }
  .toggle-group .view-btn { border-radius: 0; border-right-width: 0; }
  .toggle-group .view-btn:first-child { border-radius: 4px 0 0 4px; }
  .toggle-group .view-btn:last-child { border-radius: 0 4px 4px 0; border-right-width: 1px; }
  .view-btn { background: var(--card-bg); border: 1px solid var(--border);
              padding: 0.3rem 0.75rem; cursor: pointer; font-size: 0.8rem; color: var(--header-fg); }
  .view-btn:hover { color: var(--fg); border-color: var(--header-fg); }
  .view-btn.active { background: var(--bg); color: var(--fg); border-color: var(--link); }
</style>
</head>
<body>
<h1>` + html.EscapeString(title) + ` <small>native vs bridge vs tikal</small></h1>
`
}

const pageFooter = `
<script>
function switchTab(idx) {
  var panels = document.querySelectorAll('.tab-panel');
  var buttons = document.querySelectorAll('.tab-btn');
  for (var i = 0; i < panels.length; i++) panels[i].style.display = 'none';
  for (var i = 0; i < buttons.length; i++) buttons[i].classList.remove('active');
  var target = document.querySelector('.tab-panel[data-tab="' + idx + '"]');
  if (target) target.style.display = 'block';
  if (buttons[idx]) buttons[idx].classList.add('active');
}
function getActiveView(panelID) {
  var visible = document.querySelector('.view-panel[data-panel="' + panelID + '"][style*="display:block"], .view-panel[data-panel="' + panelID + '"][style*="display: block"]');
  if (!visible) return { layout: 'sbs', norm: 'raw' };
  return { layout: visible.getAttribute('data-layout') || 'sbs', norm: visible.getAttribute('data-norm') || 'raw' };
}
function showView(panelID, layout, norm) {
  var panels = document.querySelectorAll('.view-panel[data-panel="' + panelID + '"]');
  for (var i = 0; i < panels.length; i++) panels[i].style.display = 'none';
  var target = document.querySelector('.view-panel[data-panel="' + panelID + '"][data-layout="' + layout + '"][data-norm="' + norm + '"]');
  if (target) target.style.display = 'block';
  var hint = document.querySelector('.norm-hint[data-panel="' + panelID + '"]');
  if (hint) hint.style.display = (norm === 'norm') ? 'inline' : 'none';
}
function switchLayout(panelID, layout) {
  var cur = getActiveView(panelID);
  showView(panelID, layout, cur.norm);
  var container = document.querySelector('.view-panel[data-panel="' + panelID + '"]').parentElement;
  var btns = container.querySelectorAll('.toggle-group:first-child .view-btn');
  for (var i = 0; i < btns.length; i++) btns[i].classList.remove('active');
  for (var i = 0; i < btns.length; i++) {
    if (btns[i].getAttribute('onclick').indexOf("'" + layout + "'") >= 0) btns[i].classList.add('active');
  }
}
function switchNorm(panelID, norm) {
  var cur = getActiveView(panelID);
  showView(panelID, cur.layout, norm);
  var container = document.querySelector('.view-panel[data-panel="' + panelID + '"]').parentElement;
  var btns = container.querySelectorAll('[data-norm]');
  for (var i = 0; i < btns.length; i++) {
    if (btns[i].classList.contains('view-btn')) btns[i].classList.remove('active');
  }
  for (var i = 0; i < btns.length; i++) {
    if (btns[i].classList.contains('view-btn') && btns[i].getAttribute('data-norm') === norm) btns[i].classList.add('active');
  }
}
</script>
</body>
</html>
`
