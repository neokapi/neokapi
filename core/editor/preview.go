package editor

import (
	"github.com/gokapi/gokapi/core/model"
)

// BuildPreview generates a preview HTML string for the given parts.
// The format determines which preview builder is used:
//   - "html" — skeleton-based HTML preview with <kat-block> markers
//   - "markdown" — Markdown→HTML rendered preview
//   - other — generic fallback
func BuildPreview(parts []*model.Part, sourceBytes []byte, format string, locale model.LocaleID) string {
	switch format {
	case "html", "htm", "xhtml":
		return buildHTMLPreview(parts, sourceBytes)
	case "markdown":
		return buildMarkdownPreview(parts, sourceBytes)
	default:
		return buildGenericPreview(parts)
	}
}

// previewBoilerplateStart returns the standard HTML preamble for preview documents.
func previewBoilerplateStart() string {
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; line-height: 1.6; padding: 16px; color: #1a1a2e; }
  kat-block { cursor: pointer; border-radius: 2px; transition: background-color 0.15s; display: inline; }
  kat-block:hover { background-color: rgba(59,130,246,0.15); }
  kat-block.kat-selected { background-color: rgba(59,130,246,0.25); outline: 2px solid #3b82f6; }
</style>
</head>
<body>
`
}

// previewBoilerplateEnd returns the standard HTML closing for preview documents.
func previewBoilerplateEnd() string {
	return `
<script>
  document.querySelectorAll('kat-block').forEach(el => {
    el.addEventListener('click', () => {
      window.parent.postMessage({ type: 'kat-block-click', blockId: el.id }, '*');
    });
  });
  window.addEventListener('message', (e) => {
    if (e.data?.type === 'kat-select-block') {
      document.querySelector('.kat-selected')?.classList.remove('kat-selected');
      const el = document.getElementById(e.data.blockId);
      if (el) { el.classList.add('kat-selected'); el.scrollIntoView({ block: 'center', behavior: 'smooth' }); }
    }
    if (e.data?.type === 'kat-update-block') {
      const el = document.getElementById(e.data.blockId);
      if (el) el.innerHTML = e.data.html;
    }
  });
  window.parent.postMessage({ type: 'kat-iframe-ready' }, '*');
</script>
</body>
</html>`
}
