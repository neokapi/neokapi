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
	NativeOK bool // false if native roundtrip errored
	OtherOK  bool // false if the other engine errored
	Diff     string
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

func (rc *reportCollector) add(r comparisonResult) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.results = append(rc.results, r)
}

// compareAndRecord compares two outputs, records the result, and returns whether they match.
func (rc *reportCollector) compareText(format, file, pair string, a, b []byte) bool {
	if bytes.Equal(a, b) {
		rc.add(comparisonResult{Format: format, File: file, Pair: pair, Match: true, NativeOK: true, OtherOK: true})
		return true
	}
	diff := unifiedDiff(pair, string(a), string(b))
	rc.add(comparisonResult{Format: format, File: file, Pair: pair, Match: false, NativeOK: true, OtherOK: true, Diff: diff})
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

// writeReport writes the HTML report to the given path.
func (rc *reportCollector) writeReport(path string) error {
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

	var buf bytes.Buffer
	buf.WriteString(reportHeader)

	// Summary table.
	passCount, failCount := 0, 0
	for _, r := range rc.results {
		if r.Match {
			passCount++
		} else {
			failCount++
		}
	}
	fmt.Fprintf(&buf, `<div class="summary"><h2>Summary</h2>`)
	fmt.Fprintf(&buf, `<p><span class="badge pass">%d PASS</span> <span class="badge fail">%d FAIL</span> across %d comparisons</p>`,
		passCount, failCount, len(rc.results))

	// Summary table.
	buf.WriteString(`<table class="summary-table"><tr><th>Format</th><th>File</th><th>Pair</th><th>Result</th></tr>`)
	for i, r := range rc.results {
		cls := "pass"
		label := "PASS"
		if !r.Match {
			cls = "fail"
			label = "FAIL"
		}
		link := ""
		if !r.Match {
			link = fmt.Sprintf(` <a href="#detail-%d">details</a>`, i)
		}
		fmt.Fprintf(&buf, `<tr class="%s"><td>%s</td><td>%s</td><td>%s</td><td><span class="badge %s">%s</span>%s</td></tr>`,
			cls, html.EscapeString(r.Format), html.EscapeString(r.File), html.EscapeString(r.Pair), cls, label, link)
	}
	buf.WriteString(`</table></div>`)

	// Detail sections.
	buf.WriteString(`<div class="details"><h2>Diff Details</h2>`)
	for i, r := range rc.results {
		if r.Match {
			continue
		}
		fmt.Fprintf(&buf, `<div class="detail" id="detail-%d">`, i)
		fmt.Fprintf(&buf, `<h3>%s / %s — %s</h3>`, html.EscapeString(r.Format), html.EscapeString(r.File), html.EscapeString(r.Pair))

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
					link = fmt.Sprintf(` <a href="#entry-%d-%d">diff</a>`, i, j)
				}
				fmt.Fprintf(&buf, `<tr class="%s"><td>%s</td><td><span class="badge %s">%s</span>%s</td><td>%d</td><td>%d</td></tr>`,
					cls, html.EscapeString(e.Name), cls, label, link, e.SizeA, e.SizeB)
			}
			buf.WriteString(`</table>`)

			// Entry diffs.
			for j, e := range r.Entries {
				if e.Match {
					continue
				}
				fmt.Fprintf(&buf, `<div class="entry-diff" id="entry-%d-%d">`, i, j)
				fmt.Fprintf(&buf, `<h4>%s</h4>`, html.EscapeString(e.Name))
				if e.Binary {
					fmt.Fprintf(&buf, `<pre class="diff binary">%s</pre>`, html.EscapeString(e.Diff))
				} else {
					buf.WriteString(`<pre class="diff">`)
					writeDiffHTML(&buf, e.Diff)
					buf.WriteString(`</pre>`)
				}
				buf.WriteString(`</div>`)
			}
		} else {
			// Text diff.
			buf.WriteString(`<pre class="diff">`)
			writeDiffHTML(&buf, r.Diff)
			buf.WriteString(`</pre>`)
		}
		buf.WriteString(`</div>`)
	}
	buf.WriteString(`</div>`)
	buf.WriteString(reportFooter)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
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

const reportHeader = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Format Compatibility Report</title>
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
</style>
</head>
<body>
<h1>Format Compatibility Report <small>native vs bridge vs tikal</small></h1>
`

const reportFooter = `
</body>
</html>
`
