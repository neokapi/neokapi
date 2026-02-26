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
  html, body { overflow: hidden; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; line-height: 1.6; padding: 16px; color: #1a1a2e; }
  kat-block { cursor: pointer; border-radius: 2px; transition: background-color 0.15s; display: inline; }
  kat-block:hover { background-color: rgba(59,130,246,0.15); }
  kat-block.kat-selected { background-color: rgba(59,130,246,0.25); outline: 2px solid #3b82f6; }
  kat-block.kat-presence { outline-offset: 2px; }
  .kat-presence-label { position: absolute; top: -18px; left: 0; font-size: 10px; padding: 1px 4px; border-radius: 3px; color: white; pointer-events: none; white-space: nowrap; z-index: 10; }
  #kat-editor-spacer { transition: height 0.15s ease-out; }
</style>
</head>
<body>
`
}

// previewBoilerplateEnd returns the standard HTML closing for preview documents.
// Note: The kat-update-block handler uses textContent for safety when possible,
// but uses innerHTML for the preview HTML rendered by the trusted server-side
// BuildPreview function. The iframe is sandboxed (sandbox="allow-scripts") with
// no access to the parent origin's cookies or storage.
func previewBoilerplateEnd() string {
	return `
<script>
  document.querySelectorAll('kat-block').forEach(el => {
    el.addEventListener('click', () => {
      window.parent.postMessage({ type: 'kat-block-click', blockId: el.id }, '*');
    });
  });

  // Report content height to parent
  function reportContentHeight() {
    window.parent.postMessage({ type: 'kat-content-height', height: document.body.scrollHeight }, '*');
  }
  new ResizeObserver(reportContentHeight).observe(document.body);
  reportContentHeight();

  window.addEventListener('message', (e) => {
    if (e.data?.type === 'kat-select-block') {
      document.querySelector('.kat-selected')?.classList.remove('kat-selected');
      const el = document.getElementById(e.data.blockId);
      if (el) { el.classList.add('kat-selected'); }
    }
    if (e.data?.type === 'kat-update-block') {
      const el = document.getElementById(e.data.blockId);
      if (el) el.textContent = e.data.text || '';
    }
    if (e.data?.type === 'kat-insert-spacer') {
      var old = document.getElementById('kat-editor-spacer');
      if (old) old.remove();
      var el = document.getElementById(e.data.blockId);
      if (el) {
        var ancestor = el.closest('p,div,li,h1,h2,h3,h4,h5,h6,td,th,blockquote') || el;
        var spacer = document.createElement('div');
        spacer.id = 'kat-editor-spacer';
        spacer.style.height = e.data.height + 'px';
        ancestor.parentNode.insertBefore(spacer, ancestor.nextSibling);
        var rect = spacer.getBoundingClientRect();
        var scrollY = window.pageYOffset || document.documentElement.scrollTop;
        reportContentHeight();
        window.parent.postMessage({ type: 'kat-spacer-position', y: rect.top + scrollY, contentHeight: document.body.scrollHeight }, '*');
      }
    }
    if (e.data?.type === 'kat-remove-spacer') {
      var old = document.getElementById('kat-editor-spacer');
      if (old) old.remove();
      reportContentHeight();
    }
    if (e.data?.type === 'kat-set-presence') {
      document.querySelectorAll('.kat-presence').forEach(el => {
        el.classList.remove('kat-presence');
        el.style.outline = '';
        el.style.outlineOffset = '';
        el.style.position = '';
      });
      document.querySelectorAll('.kat-presence-label').forEach(el => el.remove());
      (e.data.users || []).forEach(u => {
        const el = document.getElementById(u.blockId);
        if (el) {
          el.classList.add('kat-presence');
          el.style.outline = '2px solid ' + u.color;
          el.style.outlineOffset = '2px';
          el.style.position = 'relative';
          const label = document.createElement('span');
          label.className = 'kat-presence-label';
          label.style.backgroundColor = u.color;
          label.textContent = u.name;
          el.appendChild(label);
        }
      });
    }
  });
  window.parent.postMessage({ type: 'kat-iframe-ready' }, '*');
</script>
</body>
</html>`
}
