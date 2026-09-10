#!/usr/bin/env python3
"""Exercise section replacement in real formats with the local kapi binary.

The revisions are authored fixtures, not model output. Python's standard library
creates the documents, records real CLI results, and checks preservation.
"""

import argparse
import base64
import difflib
import hashlib
import html
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
import zipfile


TITLE = "Share a preview"
BEFORE = "Local preview stays on this machine. Reload the browser after saving."
AFTER = "Revocation stops new requests. Downloaded copies remain with their holders."
ORIGINAL = """Utilize Pageglass to make review seamless. When the content looks right, send it to reviewers.

Remember the account and the internet. Anyone with the link can open it. Changes need another share.

### Things to remember

- Take the preview ID from the terminal.
- Choose 1h, 24h or 7d for expiry.

```sh
pageglass share pv_example --expires 24h
```
"""
FIRST = """Share a **fixed snapshot** of the latest successful local preview. Sharing requires network access and a configured workspace account.

Replace `pv_example` with the preview ID printed by the local preview command:

```sh
pageglass share pv_example --expires 24h
```

The command prints a link ID and an HTTPS preview URL. Anyone holding the URL can view the snapshot without signing in, including people who receive a forwarded link.

- Share content suitable for everyone who might receive the URL.
- Choose `1h`, `24h` or `7d` for expiry. The default is `24h`.
- After editing locally, share again to create a new link. Existing links keep their original snapshot.

### End access

Utilize the link ID to stop new requests before expiry. Replace `ln_example` with the printed link ID:

```sh
pageglass revoke ln_example
```

Revocation cannot remove copies already downloaded by a reviewer.
"""
FINAL = FIRST.replace("Utilize the link ID", "Use the link ID")
FACTS = """# Pageglass sharing task

Pageglass is fictional. These commands are documentation, not commands to run.
The reader has a successful local preview and wants to share it with reviewers.
Rewrite the complete sharing section as usable instructions. Preserve the local
preview and retention sections around it.

Sharing requires network access and an already configured workspace account.
`pageglass share pv_example --expires 24h` uploads the current successful
snapshot. Substitute the actual preview ID. Output includes a link ID and an
HTTPS preview URL. Anyone holding the URL can view it without signing in.
Forwarded links grant the same access. Share suitable content only.

The default expiry is 24h; the only accepted values are 1h, 24h and 7d.
Local edits never change an existing shared snapshot. Sharing again creates a
new link. `pageglass revoke ln_example` stops new requests to that link before
expiry. Substitute the actual link ID. Downloaded copies remain accessible.

The initial wording and both revisions are authored for this deterministic
demonstration. The first revision deliberately leaves one vocabulary defect.
This demonstrates the edit/check loop, not model writing ability or quality.
"""
VOICE = """name: Pageglass instructions
tone:
  formality: neutral
  guidelines: Explain the action, required inputs and observable result. Keep access and fixed-snapshot behavior explicit.
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: major
    - term: seamless
      severity: major
constraints:
  - id: pageglass/sharing-facts
    version: 1
    source: task.md
    kind: guidance
    statement: Preserve the fixed snapshot, link-holder access, expiry choices and revocation limits described in task.md.
"""
RECIPE = """version: v1
name: semantic-section-demo
defaults:
  source_language: en
  voice:
    profile_file: voice.yaml
collections:
  - name: instructions
    content:
      - path: documents/*
"""
W = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def blocks(markdown):
    """Parse only the deliberately small syntax used to author these fixtures."""
    lines = markdown.strip().splitlines()
    index = 0
    while index < len(lines):
        line = lines[index]
        if not line:
            index += 1
            continue
        if line.startswith("```"):
            end = index + 1
            while end < len(lines) and not lines[end].startswith("```"):
                end += 1
            if end == len(lines):
                raise ValueError("Unclosed fixture code fence")
            yield "code", "\n".join(lines[index + 1:end])
            index = end + 1
            continue
        if line.startswith("### "):
            yield "h3", line[4:]
        elif line.startswith("- "):
            yield "li", line[2:]
        else:
            yield "p", line
        index += 1


def html_body(markdown):
    result = []
    in_list = False
    for kind, text in blocks(markdown):
        if in_list and kind != "li":
            result.append("</ul>")
            in_list = False
        if kind == "li" and not in_list:
            result.append("<ul>")
            in_list = True
        if kind == "code":
            result.append("<pre><code>" + html.escape(text) + "</code></pre>")
        else:
            result.append(f"<{kind}>" + html.escape(text) + f"</{kind}>")
    if in_list:
        result.append("</ul>")
    return "\n".join(result) + "\n"


def paragraph(text, level=None, bullet=False):
    props = ""
    if level:
        props = f'<w:pStyle w:val="Heading{level}"/><w:outlineLvl w:val="{level - 1}"/>'
    elif bullet:
        props = '<w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr>'
    return '<w:p>' + (f'<w:pPr>{props}</w:pPr>' if props else '') + '<w:r><w:t xml:space="preserve">' + html.escape(text) + '</w:t></w:r></w:p>'


def make_docx(path):
    body = paragraph("Pageglass", 1) + paragraph("Local preview", 2) + paragraph(BEFORE)
    body += paragraph(TITLE, 2)
    for kind, text in blocks(ORIGINAL):
        body += paragraph(text, 3 if kind == "h3" else None, kind == "li")
    body += paragraph("Retention", 2) + paragraph(AFTER)
    body += '<w:sectPr><w:pgSz w:w="12240" w:h="15840"/></w:sectPr>'
    styles = ''.join(f'<w:style w:type="paragraph" w:styleId="Heading{n}"><w:name w:val="heading {n}"/><w:pPr><w:outlineLvl w:val="{n - 1}"/></w:pPr></w:style>' for n in range(1, 4))
    entries = {
        '[Content_Types].xml': '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Default Extension="bin" ContentType="application/octet-stream"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/></Types>',
        '_rels/.rels': f'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="{R}/officeDocument" Target="word/document.xml"/></Relationships>',
        'word/document.xml': f'<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="{W}" xmlns:r="{R}"><w:body>{body}</w:body></w:document>',
        'word/styles.xml': f'<w:styles xmlns:w="{W}"><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>{styles}</w:styles>',
        'word/numbering.xml': f'<w:numbering xmlns:w="{W}"><w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num></w:numbering>',
        'word/_rels/document.xml.rels': f'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rStyles" Type="{R}/styles" Target="styles.xml"/><Relationship Id="rNumbers" Type="{R}/numbering" Target="numbering.xml"/><Relationship Id="rImage" Type="{R}/image" Target="media/retained.png"/></Relationships>',
        'custom/preserved.bin': b'Pageglass untouched custom payload\x00\x01\xff',
        'word/media/retained.png': base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII='),
    }
    with zipfile.ZipFile(path, 'w') as archive:
        archive.comment = b'Pageglass fixture archive'
        for name, value in entries.items():
            info = zipfile.ZipInfo(name, date_time=(2020, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o644 << 16
            archive.writestr(info, value.encode('utf-8') if isinstance(value, str) else value)


def create_project(root):
    docs = root / "documents"
    docs.mkdir(parents=True)
    (root / "task.md").write_text(FACTS, encoding="utf-8")
    (root / "voice.yaml").write_text(VOICE, encoding="utf-8")
    (root / "kapi.yaml").write_text(RECIPE, encoding="utf-8")
    (docs / "sharing.md").write_text(f"# Pageglass\n\n## Local preview\n\n{BEFORE}\n\n## {TITLE}\n\n{ORIGINAL}\n## Retention\n\n{AFTER}\n", encoding="utf-8")
    (docs / "sharing.html").write_text('<!doctype html>\n<html lang="en" data-fixture="preserve-shell"><head><meta charset="utf-8"><title>Pageglass</title></head>\n<body class="docs"><main id="instructions"><h1>Pageglass</h1>\n<h2 id="local">Local preview</h2>\n<p data-preserve="before">' + BEFORE + '</p>\n<h2 id="share" class="task">' + TITLE + '</h2>\n' + html_body(ORIGINAL) + '<h2 id="retention">Retention</h2>\n<p data-preserve="after">' + AFTER + '</p>\n</main><footer data-preserve="footer">Pageglass documentation</footer></body></html>\n', encoding="utf-8")
    make_docx(docs / "sharing.docx")


def isolated_environment(root):
    env = dict(os.environ, KAPI_NO_PROJECT="1", KAPI_PLUGINS_DIR_ONLY="1")
    env.pop("KAPI_PROJECT", None)
    for key, folder in {"KAPI_CONFIG_DIR": "config", "XDG_DATA_HOME": "data", "XDG_CACHE_HOME": "cache", "KAPI_PLUGINS_DIR": "plugins"}.items():
        path = root / folder
        path.mkdir(parents=True)
        env[key] = str(path)
    return env


def selected(document):
    matches = [section for section in document["sections"] if section["title"] == TITLE]
    if len(matches) != 1:
        raise AssertionError(f"Expected exactly one {TITLE!r} section")
    if document.get("content_format") != "markdown":
        raise AssertionError("Section content must be a Markdown projection")
    return matches[0]


def preserve(before, after):
    """Verify source bytes outside the edited section and other ZIP parts."""
    if before.suffix == ".docx":
        with zipfile.ZipFile(before) as left, zipfile.ZipFile(after) as right:
            assert set(left.namelist()) == set(right.namelist()), "ZIP entry names changed"
            for name in left.namelist():
                if name != "word/document.xml":
                    assert left.read(name) == right.read(name), f"Changed unrelated ZIP part: {name}"
            a, b = left.read("word/document.xml"), right.read("word/document.xml")
            start = paragraph(TITLE, 2).encode()
            end = paragraph("Retention", 2).encode()
    else:
        a, b = before.read_bytes(), after.read_bytes()
        if before.suffix == ".md":
            start, end = b"## Share a preview\n", b"## Retention\n"
        else:
            start = b'<h2 id="share" class="task">Share a preview</h2>'
            end = b'<h2 id="retention">Retention</h2>'
    assert a.count(start) == b.count(start) == 1, "Selected heading changed"
    assert a.count(end) == b.count(end) == 1, "Unrelated following heading changed"
    assert a.split(start)[0] == b.split(start)[0], "Bytes before selected body changed"
    assert a.split(end)[1] == b.split(end)[1], "Bytes after selected body changed"


def assert_check(report, count, term=None):
    assert report["schema"] == "kapi.check/v1"
    findings = report.get("findings") or []
    assert len(findings) == count, f"Expected {count} findings, got {findings}"
    assert report["pass"] == (count == 0)
    if count:
        assert all(f["rule"].startswith("voice.") for f in findings), findings
    if term:
        assert term.casefold() in json.dumps(findings).casefold(), findings
    analyzers = report.get("execution", {}).get("analyzers", [])
    assert any(a["id"] == "voice.rules" and a["status"] == ("findings" if count else "passed") for a in analyzers), analyzers
    assert any(a["id"] == "voice.guidance" and a["status"] == "unsupported" for a in analyzers), analyzers
    return findings


def replay_plan(original, plan):
    """Independently replay immutable source offsets, without parsing content."""
    assert plan["snapshot"] == digest(original.read_bytes()), "Plan snapshot differs from the original"
    patches = plan["patches"]
    assert patches, "No patches in preview plan"
    if original.suffix == ".docx":
        with zipfile.ZipFile(original) as archive:
            sources = {"word/document.xml": archive.read("word/document.xml")}
    else:
        sources = {"": original.read_bytes()}
    outputs = {}
    for entry in {p.get("entry", "") for p in patches}:
        assert entry in sources, f"Unexpected patched ZIP entry: {entry}"
        source = sources[entry]
        cursor = 0
        chunks = []
        for patch in sorted((p for p in patches if p.get("entry", "") == entry), key=lambda p: p["start"]):
            start, end = patch["start"], patch["end"]
            assert cursor <= start <= end <= len(source), "Overlapping or out-of-bounds offsets"
            assert source[start:end] == patch["before"].encode("utf-8"), "Patch before text differs at original offsets"
            chunks.extend([source[cursor:start], patch["replacement"].encode("utf-8")])
            cursor = end
        chunks.append(source[cursor:])
        outputs[entry] = b"".join(chunks)
    return outputs


def assert_replayed(file, expected):
    if file.suffix == ".docx":
        with zipfile.ZipFile(file) as archive:
            for entry, content in expected.items():
                assert archive.read(entry) == content, "Applied ZIP entry differs from preview plan"
    else:
        assert file.read_bytes() == expected[""], "Applied file differs from preview plan"


class Runner:
    def __init__(self, binary, output):
        self.binary = str(binary)
        self.output = output
        self.project = output / "project"
        self.env = isolated_environment(output / "isolation")
        self.commands = []

    def run(self, folder, label, args, expected=(0,), parse=False):
        argv = [self.binary, "-p", str(self.project / "kapi.yaml"), *args]
        started = time.monotonic()
        result = subprocess.run(argv, cwd=self.project, env=self.env, capture_output=True, timeout=60)
        (folder / (label + ".stdout")).write_bytes(result.stdout)
        (folder / (label + ".stderr")).write_bytes(result.stderr)
        record = {"arguments": argv, "exit_code": result.returncode, "elapsed_ms": round((time.monotonic() - started) * 1000), "stdout_sha256": digest(result.stdout), "stderr_sha256": digest(result.stderr)}
        write_json(folder / (label + ".command.json"), record)
        self.commands.append(record)
        if result.returncode not in expected:
            raise AssertionError(f"{label}: unexpected exit {result.returncode}; inspect {folder}")
        if parse:
            value = json.loads(result.stdout)
            write_json(folder / (label + ".json"), value)
            return value
        return result.stdout.decode("utf-8", errors="replace")

    def inspect(self, folder, label, file):
        result = self.run(folder, label, ["inspect", str(file), "--sections"], parse=True)
        assert result["snapshot"] == digest(file.read_bytes()), "Inspect snapshot differs from source bytes"
        selected(result)
        return result

    def revision(self, folder, name, file, document, text):
        changes = folder / (name + ".jsonl")
        entry = {"kind": "section", "file": str(file), "id": selected(document)["id"], "snapshot": document["snapshot"], "text": text}
        changes.write_text(json.dumps(entry) + "\n", encoding="utf-8")
        previous = file.read_bytes()
        diff = self.run(folder, name + ".diff", ["apply", str(changes), "--diff"])
        assert file.read_bytes() == previous, "--diff mutated the source"
        assert diff.strip(), "Preview did not report a patch"
        preview = self.run(folder, name + ".plan", ["apply", str(changes), "--diff", "--json"], parse=True)
        assert file.read_bytes() == previous, "JSON preview mutated the source"
        expected = replay_plan(file, preview["plan"])
        self.run(folder, name + ".apply", ["apply", str(changes)])
        assert file.read_bytes() != previous, "Apply left the source unchanged"
        assert_replayed(file, expected)
        return changes

    def format(self, extension):
        folder = self.output / extension
        folder.mkdir()
        file = self.project / "documents" / ("sharing." + extension)
        original_file = folder / ("original." + extension)
        shutil.copyfile(file, original_file)
        context = self.run(folder, "context", ["context", str(file), "--json"], parse=True)
        assert context.get("voice", {}).get("name") == "Pageglass instructions", "Wrong or missing voice for destination"
        self.run(folder, "voice-guide", ["voice", "guide", str(file)])
        original = self.inspect(folder, "original.inspect", file)
        initial_check = self.run(folder, "original.check", ["check", str(file), "--json"], expected=(3,), parse=True)
        assert_check(initial_check, 2)
        first_changes = self.revision(folder, "first", file, original, FIRST)
        first = self.inspect(folder, "first.inspect", file)
        shutil.copyfile(file, folder / ("first." + extension))
        preserve(original_file, file)
        first_check = self.run(folder, "first.check", ["check", str(file), "--json"], expected=(3,), parse=True)
        assert_check(first_check, 1, "utilize")
        saved = file.read_bytes()
        self.run(folder, "stale.apply", ["apply", str(first_changes)], expected=(1,))
        assert file.read_bytes() == saved, "Rejected stale edit mutated the source"
        # The authored second response is conditional on the actual check finding.
        self.revision(folder, "final", file, first, FINAL)
        final = self.inspect(folder, "final.inspect", file)
        shutil.copyfile(file, folder / ("final." + extension))
        preserve(original_file, file)
        final_check = self.run(folder, "final.check", ["check", str(file), "--json"], parse=True)
        assert_check(final_check, 0)
        body = selected(final)["content"]
        for required in ["fixed snapshot", "pageglass share pv_example --expires 24h", "pageglass revoke ln_example", "End access", "without signing in"]:
            assert required in body.replace("**", ""), f"Final projection lost {required}"
        assert "Things to remember" not in body and "seamless" not in body.lower()
        record = {"format": extension, "original": selected(original)["content"], "first": selected(first)["content"], "final": body, "checks": {"original": initial_check, "first": first_check, "final": final_check}, "preservation": "Unrelated source spans and, for DOCX, all other ZIP payloads verified byte for byte", "diff_read_only": True, "stale_rejected": True, "offset_plan_replayed": True}
        write_json(folder / "result.json", record)
        return record


def report(output, results):
    sections = []
    for result in results:
        extension = result["format"]
        cells = ''.join('<article><h3>' + label.title() + '</h3><pre>' + html.escape(result[label]) + '</pre><p><a href="' + extension + '/' + label + '.' + extension + '">Download ' + label + ' file</a></p></article>' for label in ['original', 'first', 'final'])
        diff = ''.join(difflib.unified_diff(result['original'].splitlines(True), result['final'].splitlines(True), fromfile='original section', tofile='final section'))
        checks = ''.join('<details' + (' open' if stage == 'first' else '') + '><summary>' + stage.title() + ' check: ' + str(len(check.get('findings') or [])) + ' findings</summary><pre>' + html.escape(json.dumps(check, indent=2, ensure_ascii=False)) + '</pre></details>' for stage, check in result['checks'].items())
        sections.append('<section><h2>' + extension.upper() + '</h2><p>' + html.escape(result['preservation']) + '. Diff is read-only; stale edits are rejected. Independently replaying the offset plan reproduces the applied content.</p><p><a href="' + extension + '/context.json">Resolved context</a> · <a href="' + extension + '/voice-guide.stdout">Retrieved voice guide</a> · <a href="' + extension + '/first.plan.json">First offset plan</a> · <a href="' + extension + '/final.plan.json">Final offset plan</a></p><div class="versions">' + cells + '</div><details><summary>Full section difference</summary><pre>' + html.escape(diff) + '</pre></details>' + checks + '</section>')
    page = '''<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Pageglass section edit loop</title><style>body{font:16px/1.5 system-ui,sans-serif;max-width:1500px;margin:2rem auto;padding:0 1rem;color:#23332f;background:#fafaf7}h1,h2,h3{line-height:1.2}section{margin:3rem 0;border-top:2px solid #48776d}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#fff;border:1px solid #cbd6d1;padding:1rem;font:14px/1.5 ui-monospace,monospace}a{color:#155e51}summary{cursor:pointer;padding:.6rem 0}.versions{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:1rem}@media(max-width:1000px){.versions{grid-template-columns:1fr}}</style><h1>Rewrite a complete sharing section</h1><p>The same authored Pageglass documentation is stored as Markdown, HTML and Word. Each run inspects a semantic section, previews and applies a multi-block replacement, reads a remaining vocabulary finding, and replaces the section again.</p><p><strong>Authored demonstration:</strong> no model or API calls. The initial defects and both responses are deliberately supplied fixtures. A passing check establishes only the recorded deterministic coverage, not factual completeness, good style, or editorial quality. No Word layout rendering is measured.</p><details open><summary>Task and factual context</summary><pre>''' + html.escape(FACTS) + '</pre></details><details><summary>Bound voice profile</summary><pre>' + html.escape(VOICE) + '</pre></details>' + ''.join(sections) + '</html>'
    (output / 'report.html').write_text(page, encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--kapi", required=True, type=Path, help="Binary built with section inspection and replacement")
    parser.add_argument("--output", required=True, type=Path, help="New directory for isolated project and evidence")
    args = parser.parse_args()
    binary, output = args.kapi.resolve(), args.output.resolve()
    if not binary.is_file():
        parser.error("--kapi must name an existing binary")
    if output.exists():
        parser.error("--output must be new; existing evidence is never overwritten")
    output.mkdir(parents=True)
    runner = Runner(binary, output)
    create_project(runner.project)
    try:
        results = [runner.format(extension) for extension in ["md", "html", "docx"]]
        report(output, results)
        write_json(output / "summary.json", {"status": "passed", "model_calls": 0, "formats": [r["format"] for r in results], "coverage": "Actual deterministic vocabulary checks and explicit source-preservation assertions; no editorial or rendered-layout assessment"})
    except Exception as error:
        write_json(output / "summary.json", {"status": "failed", "error": str(error)})
        raise
    finally:
        write_json(output / "commands.json", runner.commands)
    print(output / "report.html")


if __name__ == "__main__":
    main()
