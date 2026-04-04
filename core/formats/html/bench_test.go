package html_test

import (
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/internal/testutil"
)

func drainParts(ch <-chan model.PartResult) {
	for range ch {
	}
}

// BenchmarkHTMLTokenizer benchmarks reading and parsing a moderate HTML document.
func BenchmarkHTMLTokenizer(b *testing.B) {
	input := `<!DOCTYPE html>
<html lang="en">
<head><title>Benchmark Page</title></head>
<body>
<h1>Welcome to the Benchmark</h1>
<p>This is a <strong>paragraph</strong> with <em>inline</em> elements and a <a href="#">link</a>.</p>
<div>
  <h2>Section One</h2>
  <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>
  <ul>
    <li>First item with <code>inline code</code></li>
    <li>Second item with <b>bold</b> and <i>italic</i></li>
    <li>Third item</li>
  </ul>
</div>
<div>
  <h2>Section Two</h2>
  <p>Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.</p>
  <table>
    <tr><th>Header 1</th><th>Header 2</th></tr>
    <tr><td>Cell 1</td><td>Cell 2</td></tr>
    <tr><td>Cell 3</td><td>Cell 4</td></tr>
  </table>
</div>
<footer><p>Copyright 2024</p></footer>
</body>
</html>`

	ctx := b.Context()

	b.ResetTimer()
	for b.Loop() {
		reader := htmlfmt.NewReader()
		_ = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
		drainParts(reader.Read(ctx))
		reader.Close()
	}
}
