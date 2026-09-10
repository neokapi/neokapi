#!/usr/bin/env python3
"""Render verified context-transfer artifacts as a standalone Markdown comparison.

Requires the repository's installed marked, DOMPurify and jsdom packages. No
models, network assets or browser-side Markdown parser are used.
"""

import argparse
import difflib
import hashlib
import html
import json
import math
import re
import subprocess
from pathlib import Path
from urllib.parse import urlparse


DOCUMENTS = {"README.md", "docs/getting-started.md", "docs/preview-links.md", "docs/troubleshooting.md"}
ARMS = {"references", "guidance", "kapi"}
MARKDOWN_RENDERER = r"""
const base = process.argv[1];
const dependency = name => require(require.resolve(name, {paths: [base]}));
const {marked} = dependency('marked');
const {JSDOM} = dependency('jsdom');
const purify = dependency('dompurify')(new JSDOM('').window);
const escape = value => value.replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;');
const renderer = new marked.Renderer();
renderer.html = token => escape(token.text);
renderer.image = token => '<span class="image-alt">[Image: ' + escape(token.text || '') + ']</span>';
let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => input += chunk);
process.stdin.on('end', () => {
  const output = JSON.parse(input).map(text => purify.sanitize(marked.parse(text, {renderer, gfm:true, breaks:false}), {
    FORBID_TAGS:['script','style','iframe','object','embed','form','input','button','img','video','audio','link','meta'],
    FORBID_ATTR:['style','src','srcset'], ALLOW_DATA_ATTR:false
  }));
  process.stdout.write(JSON.stringify(output));
});
"""


def esc(value):
    return html.escape(str(value), quote=True)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def read_artifact(root, artifact):
    relative = Path(artifact["path"])
    if relative.is_absolute():
        raise ValueError("Artifact path must be relative to the index")
    path = (root / relative).resolve(strict=True)
    if not path.is_relative_to(root):
        raise ValueError("Artifact path escapes the index directory")
    expected = artifact["sha256"]
    if not re.fullmatch(r"[0-9a-f]{64}", expected):
        raise ValueError("Artifact needs a lowercase SHA-256 digest")
    raw = path.read_bytes()
    if digest(raw) != expected:
        raise ValueError(f"Artifact hash differs: {relative}")
    return raw.decode("utf-8")


def load_index(path):
    root = path.resolve(strict=True).parent
    raw = path.read_bytes()
    index = json.loads(raw)
    documents = index["documents"]
    if len(documents) != 4 or {doc["id"] for doc in documents} != DOCUMENTS:
        raise ValueError("Comparison needs the four declared document paths")
    if len(index["adaptations"]) != 3 or {arm["id"] for arm in index["adaptations"]} != ARMS:
        raise ValueError("Comparison needs references, guidance and kapi arms")
    texts, artifacts = {}, []

    def accept(key, artifact):
        texts[key] = read_artifact(root, artifact)
        artifacts.append({"role": key, **artifact})

    accept("brief", index["brief"])
    for position, reference in enumerate(index.get("references", [])):
        accept(f"reference-{position}", reference["artifact"])
    for position, guidance in enumerate(index.get("guidance", [])):
        accept(f"guidance-{position}", guidance["artifact"])
    for arm in [{**index["original"], "id": "original"}, *index["adaptations"]]:
        if set(arm["documents"]) - DOCUMENTS:
            raise ValueError("Unexpected document in an arm")
        for document, artifact in arm["documents"].items():
            accept(arm["id"] + ":" + document, artifact)
        validate_observation(arm.get("observation", {}))
    for stage in index.get("stages", []):
        validate_observation(stage.get("observation", {}))
    return index, texts, {"index_sha256": digest(raw), "artifacts": artifacts}


def validate_observation(observation):
    duration = observation.get("duration_ms")
    if duration is not None and (not isinstance(duration, (int, float)) or not math.isfinite(duration) or duration < 0):
        raise ValueError("Observed duration must be finite and nonnegative")


def render_markdown(texts, repo):
    completed = subprocess.run(["node", "-e", MARKDOWN_RENDERER, str(repo.resolve() / "node_modules")],
                               input=json.dumps(texts), text=True, capture_output=True, check=False)
    if completed.returncode:
        raise ValueError("Markdown rendering failed; install repository dependencies or set --repo. " + completed.stderr.strip())
    rendered = json.loads(completed.stdout)
    if len(rendered) != len(texts) or not all(isinstance(value, str) for value in rendered):
        raise ValueError("Markdown renderer returned unexpected output")
    return rendered


def observation_html(observation):
    duration = observation.get("duration_ms")
    elapsed = f"{duration / 1000:.1f}s" if duration is not None else "duration not reported"
    tools = observation.get("tools")
    tool_text = ", ".join(map(str, tools)) if tools else "none recorded" if tools == [] else "not reported"
    return (f"<p class='observation'><strong>{esc(observation.get('status', 'status not reported'))}</strong> · "
            f"{esc(observation.get('model', 'model not reported'))} · {elapsed}</p>"
            f"<p class='tools'><strong>Observed tools:</strong> {esc(tool_text)}</p>")


def notes_html(notes):
    return "<ul>" + "".join("<li>" + esc(note) + "</li>" for note in notes) + "</ul>" if notes else ""


def exact_diff(original, adapted, document, arm):
    lines = difflib.unified_diff(original.splitlines(keepends=True), adapted.splitlines(keepends=True),
                                 fromfile="original/" + document, tofile=arm + "/" + document)
    output = []
    for line in lines:
        output.append(line)
        if not line.endswith("\n"):
            output.append("\n\\ No newline at end of file\n")
    return "".join(output)


def safe_reference(reference):
    title, url = reference["title"], reference.get("url", "")
    parsed = urlparse(url)
    if parsed.scheme in {"http", "https"} and parsed.netloc:
        return f"<a href='{esc(url)}' target='_blank' rel='noopener noreferrer'>{esc(title)}</a>"
    return esc(title) + (" · " + esc(url) if url else "")


def render(index_path, output, repo):
    index, texts, provenance = load_index(index_path)
    source_root = index_path.resolve().parent
    protected = {index_path.resolve(), *((source_root / item["path"]).resolve() for item in provenance["artifacts"])}
    if output.resolve() in protected:
        raise ValueError("Report output would overwrite a verified artifact")
    markdown_keys = [key for key in texts if not key.startswith("reference-")]
    rendered = dict(zip(markdown_keys, render_markdown([texts[key] for key in markdown_keys], repo), strict=True))
    arms = [{**index["original"], "id": "original"}, *index["adaptations"]]
    summaries = []
    for arm in arms:
        changed = sum(texts.get(arm["id"] + ":" + document) != texts.get("original:" + document)
                      for document in DOCUMENTS if arm["id"] + ":" + document in texts and "original:" + document in texts)
        available = len(arm["documents"])
        count = f"{available}/4 artifacts retained" + (f" · {changed} changed from original" if arm["id"] != "original" else "")
        summaries.append("<article><h3>" + esc(arm["title"]) + "</h3>" + observation_html(arm.get("observation", {})) +
                         "<p>" + count + "</p>" + notes_html(arm.get("notes", [])) + "</article>")
    views = []
    for position, document in enumerate(index["documents"]):
        doc_id = document["id"]
        panels = []
        for arm in arms:
            key = arm["id"] + ":" + doc_id
            if key in texts:
                body = "<div class='rendered'>" + rendered[key] + "</div>"
                if arm["id"] != "original" and "original:" + doc_id in texts:
                    diff = exact_diff(texts["original:" + doc_id], texts[key], doc_id, arm["id"])
                    body += "<details class='diff'><summary>Exact Markdown diff</summary><pre>" + esc(diff or "No changes.") + "</pre></details>"
                body += "<details><summary>Raw Markdown</summary><pre>" + esc(texts[key]) + "</pre></details>"
            else:
                body = "<p class='missing'>No artifact retained for this document. No output has been inferred.</p>"
            panels.append(f"<article class='document-panel' data-arm='{esc(arm['id'])}'><header><h3>{esc(arm['title'])}</h3>" +
                          observation_html(arm.get("observation", {})) + "</header>" + body + "</article>")
        views.append(f"<section class='document-view' data-document='{esc(doc_id)}'" + (" hidden" if position else "") + ">" +
                     "<h2>" + esc(document["title"]) + " <small>" + esc(doc_id) + "</small></h2><div class='comparison'>" + "".join(panels) + "</div></section>")
    references = "".join("<article><h3>" + safe_reference(reference) + "</h3><details><summary>Full retained reference source</summary><pre>" +
                         esc(texts[f"reference-{position}"]) + "</pre></details></article>"
                         for position, reference in enumerate(index.get("references", [])))
    guidance = "".join("<article><h3>" + esc(item["title"]) + "</h3><div class='rendered'>" + rendered[f"guidance-{position}"] +
                       "</div><details><summary>Raw learned guidance</summary><pre>" + esc(texts[f"guidance-{position}"]) + "</pre></details></article>"
                       for position, item in enumerate(index.get("guidance", [])))
    assessment = index.get("assessment")
    assessment_html = ("<section><h2>Assessment</h2><p>" + esc(assessment["summary"]) +
                       "</p><details><summary>Assessment reasoning and limitations</summary><p><strong>Method:</strong> " +
                       esc(assessment["method"]) + "</p>" + notes_html(assessment.get("notes", [])) + "</details></section>") if assessment else ""
    stages = "".join("<article><h3>" + esc(stage["title"]) + "</h3>" + observation_html(stage.get("observation", {})) +
                     notes_html(stage.get("notes", [])) + "</article>" for stage in index.get("stages", []))
    page = (PAGE_START.replace("{{TITLE}}", esc(index["title"])) + "<main><h1>" + esc(index["title"]) + "</h1><p class='intro'>" +
        esc(index.get("description", "")) + "</p><p class='explanation'>The same visual theme is used for every version. Counts describe retained artifacts and byte changes; they are not quality scores.</p>")
    page += "<section class='overview'><h2>Observed runs</h2><div class='summary-grid'>" + "".join(summaries) + "</div></section>" + assessment_html
    page += "<section class='toolbar' aria-label='Comparison controls'><label>Document <select id='document-choice'>" + "".join(
        "<option value='" + esc(doc["id"]) + "'>" + esc(doc["title"]) + " · " + esc(doc["id"]) + "</option>" for doc in index["documents"]) + "</select></label>"
    page += ("<label>Adaptation <select id='arm-choice'>" + "".join("<option value='" + esc(arm["id"]) + "'>" + esc(arm["title"]) + "</option>" for arm in index["adaptations"]) +
        "</select></label><label class='all-toggle'><input type='checkbox' id='all-versions'> Show all four versions</label></section>" + "".join(views))
    page += "<section class='context'><h2>Brief and context</h2><details open><summary>Full brief</summary><div class='rendered'>" + rendered["brief"] + "</div></details>"
    page += "<details><summary>Reference documents and source links</summary>" + (references or "<p>No reference artifacts supplied.</p>") + "</details>"
    page += "<details><summary>Learned guidance supplied to the adaptation runs</summary>" + (guidance or "<p>No guidance artifact supplied.</p>") + "</details></section>"
    if stages:
        page += "<section><h2>Preparation runs</h2><div class='summary-grid'>" + stages + "</div></section>"
    page += "<details class='provenance'><summary>Verified artifact provenance</summary><pre>" + esc(json.dumps(provenance, indent=2)) + "</pre></details></main>" + SCRIPT + "</body></html>\n"
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(page, encoding="utf-8")
    return {"documents": 4, "adaptations": 3, "verified_artifacts": len(provenance["artifacts"]), "model_calls": 0, "output": str(output)}


PAGE_START = """<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src 'none'; connect-src 'none'; base-uri 'none'; form-action 'none'">
<title>{{TITLE}}</title><style>
*{box-sizing:border-box}body{margin:0;background:#f6f4ec;color:#233c38;font:16px/1.6 system-ui,sans-serif}main{max-width:1800px;margin:auto;padding:30px 24px 70px}
h1,h2,h3,h4{line-height:1.25;overflow-wrap:anywhere}h1{font-size:clamp(1.8rem,3vw,2.6rem)}h2{margin-top:2rem}h3{font-size:1.1rem}h2 small{display:block;font-size:.85rem;font-weight:400;margin-top:8px}
a{color:#126858;text-underline-offset:3px}.intro,.explanation{max-width:100ch}.explanation,.tools,.observation{font-size:.88rem}section{margin:28px 0}
.summary-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px}.summary-grid article{background:#edf3ef;padding:16px;border:1px solid #d0dfd7;border-radius:10px}
.toolbar{display:flex;align-items:end;gap:18px;flex-wrap:wrap;background:#e4efe8;padding:18px;border-radius:10px;border:1px solid #c5d8d0}
label{display:block;font-weight:650}select{display:block;max-width:100%;font:inherit;border:1px solid #93aea1;border-radius:6px;padding:9px;background:white;color:inherit;margin-top:4px}
.all-toggle{font-size:.9rem;padding-bottom:11px}input{accent-color:#126858}input:focus-visible,select:focus-visible,a:focus-visible,summary:focus-visible{outline:3px solid #247e6d;outline-offset:3px}
.comparison{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px;align-items:start}.all .comparison{grid-template-columns:repeat(4,minmax(0,1fr))}
.document-panel{min-width:0;background:white;border:1px solid #c5d8d0;border-radius:12px;padding:24px}.document-panel>header{border-bottom:1px solid #dce6df;margin-bottom:24px;padding-bottom:10px}
.rendered{overflow-wrap:anywhere}.rendered h1{font-size:1.75rem}.rendered h2{font-size:1.4rem}.rendered h3{font-size:1.15rem}.rendered p:first-child{margin-top:0}
pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#f0f3ef;padding:14px;border-radius:6px;font:13px/1.6 ui-monospace,monospace;max-width:100%}code{font: .9em ui-monospace,monospace;background:#f0f3ef;padding:2px 4px;border-radius:3px}pre code{padding:0}
blockquote{margin-left:0;border-left:3px solid #a8c4b4;padding-left:15px}table{border-collapse:collapse;font-size:.9rem;max-width:100%;table-layout:fixed;width:100%}th,td{border:1px solid #cbd9d0;padding:8px;overflow-wrap:anywhere;text-align:left}
details{margin:18px 0;border:1px solid #d0dfd7;border-radius:8px;padding:13px;background:#f8faf7}summary{cursor:pointer;font-weight:600;overflow-wrap:anywhere}.context>details{padding:18px}.missing{background:#fff0d3;padding:16px;border-radius:8px}.image-alt{font-style:italic}
[hidden]{display:none!important}@media(max-width:1400px){.summary-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.all .comparison{grid-template-columns:repeat(2,minmax(0,1fr))}}
@media(max-width:750px){main{padding:22px 14px 45px}.comparison,.all .comparison,.summary-grid{grid-template-columns:1fr}.toolbar{display:block}.toolbar label{margin:10px 0}.toolbar select{width:100%;font-size:.9rem}.document-panel{padding:18px}.all-toggle{padding:8px 0}h2{font-size:1.4rem}}
</style></head><body>"""

SCRIPT = """<script>
const documentChoice = document.querySelector('#document-choice');
const armChoice = document.querySelector('#arm-choice');
const allVersions = document.querySelector('#all-versions');
function update() {
  document.body.classList.toggle('all', allVersions.checked);
  for (const section of document.querySelectorAll('.document-view')) {
    section.hidden = section.dataset.document !== documentChoice.value;
    for (const panel of section.querySelectorAll('.document-panel')) {
      panel.hidden = !allVersions.checked && panel.dataset.arm !== 'original' && panel.dataset.arm !== armChoice.value;
    }
  }
  armChoice.disabled = allVersions.checked;
}
for (const control of [documentChoice, armChoice, allVersions]) control.addEventListener('change', update);
document.addEventListener('click', event => {
  const anchor = event.target.closest('.document-panel .rendered a');
  if (!anchor) return;
  const raw = anchor.getAttribute('href');
  if (!raw || raw.startsWith('#')) return;
  const target = new URL(raw, 'https://context-report.invalid/' + documentChoice.value);
  if (target.origin === 'https://context-report.invalid') {
    const doc = target.pathname.slice(1);
    if (Array.from(documentChoice.options).some(option => option.value === doc)) {
      event.preventDefault(); documentChoice.value = doc; update();
      documentChoice.scrollIntoView({block:'nearest'});
    }
  } else { anchor.target = '_blank'; anchor.rel = 'noopener noreferrer'; }
});
update();
</script>"""


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--index", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[2])
    args = parser.parse_args()
    print(json.dumps(render(args.index, args.out, args.repo), indent=2))
