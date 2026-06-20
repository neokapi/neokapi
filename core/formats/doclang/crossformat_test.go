package doclang_test

import (
	"strings"
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	markdownfmt "github.com/neokapi/neokapi/core/formats/markdown"
)

// Producer path: projecting other neokapi formats to DocLang must follow the
// DocLang Recommendations — a code block becomes <code> (not <text>) and carries
// the recommended language <label> (Linguist key). These guard the markdown/html
// readers populating the structure layer (RoleCode + code language) so the
// DocLang writer can emit it. See https://doclang.ai spec — Recommendations.

func TestDocLangProjection_MarkdownCode(t *testing.T) {
	xmllint := xmllintPath(t)
	r := markdownfmt.NewReader()
	r.MarkdownConfig().TranslateCodeBlocks = true
	out := string(renderDocLang(t, r, "# Title\n\n```go\nfmt.Println(\"hi\")\n```\n"))

	if !strings.Contains(out, "<code>") {
		t.Errorf("markdown code block did not project to <code>:\n%s", out)
	}
	if !strings.Contains(out, `<label value="go"/>`) {
		t.Errorf("markdown code language did not project to the recommended <label>:\n%s", out)
	}
	assertValidDocLang(t, xmllint, []byte(out))
}

func TestDocLangProjection_HTMLCode(t *testing.T) {
	xmllint := xmllintPath(t)
	html := `<html><body><h1>Title</h1><pre><code class="language-python">print("hi")</code></pre></body></html>`
	out := string(renderDocLang(t, htmlfmt.NewReader(), html))

	if !strings.Contains(out, "<code>") {
		t.Errorf("html <pre> did not project to <code>:\n%s", out)
	}
	if !strings.Contains(out, `<label value="python"/>`) {
		t.Errorf("html code language did not project to the recommended <label>:\n%s", out)
	}
	assertValidDocLang(t, xmllint, []byte(out))
}
