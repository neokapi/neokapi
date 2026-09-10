#!/usr/bin/env python3
"""Render saved document reviews with their full context, without scoring them."""

import argparse
import hashlib
import html
import json
from pathlib import Path


def render(prepared, study, checks, output, assessment_path=None):
    raw = (prepared / "subject/inputs.jsonl").read_bytes()
    if raw != (study / "inputs.jsonl").read_bytes():
        raise ValueError("Prepared inputs differ from the saved study")
    cases = {case["id"]: case for case in map(json.loads, raw.splitlines())}
    labels = json.loads((prepared / "labels.json").read_text())
    manifest = json.loads((study / "manifest.json").read_text())
    coverage = json.loads(checks.read_text())
    if coverage["inputs_sha256"] != hashlib.sha256(raw).hexdigest():
        raise ValueError("Deterministic coverage belongs to different inputs")
    assessment = {}
    if assessment_path:
        assessment = json.loads(assessment_path.read_text())
        if assessment["inputs_sha256"] != hashlib.sha256(raw).hexdigest():
            raise ValueError("Assessment belongs to different inputs")
    coverage_by_id = {case["id"]: case for case in coverage["cases"]}
    escape = html.escape
    rows, sections = [], []
    for session in manifest["sessions"]:
        case = cases[session["case_id"]]
        label = labels[case["id"]]
        result_path = study / "attempts" / session["id"] / "result.json"
        if not result_path.exists():
            raise ValueError("Study still contains unfinished sessions")
        result = json.loads(result_path.read_text())
        agent, integrity = result["agent"], result["integrity"]
        review = integrity.get("review") or {}
        parsed = integrity.get("review") is not None
        findings = review.get("findings") or []
        abstentions = review.get("abstentions") or []
        deterministic = coverage_by_id[case["id"]]
        analyses = (deterministic.get("execution") or {}).get("analyzers", [])
        statuses = ", ".join(a["id"] + ": " + a["status"] for a in analyses)
        title = escape(label["variant"])
        rows.append(
            f"<tr><td><a href='#{case['id']}'>{title}</a></td><td>{escape(agent['status'])}</td>"
            f"<td>{agent['duration_ms']/1000:.1f}s</td><td>{len(findings) if parsed else 'unparsed'}</td><td>{len(abstentions) if parsed else 'unparsed'}</td>"
            f"<td>{len(label['issues'])}</td></tr>"
        )
        finding_html = "".join(
            f"<li><strong>{escape(f['kind'])}</strong><blockquote>{escape(f['candidate_quote']) or '(missing guidance)'}</blockquote>"
            f"<p>{escape(f['rationale'])}</p><p>Evidence: {escape(', '.join(f['source_ids'] or []))}</p></li>"
            for f in findings
        ) or ("<li>No findings reported.</li>" if parsed else "<li>Output could not be parsed; inspect the raw answer below.</li>")
        abstention_html = "".join(
            f"<li>{escape(a['missing_evidence'])}</li>" for a in abstentions
        )
        source_html = "".join(
            f"<details><summary>{escape(s['id'])}</summary><pre>{escape(s['text'])}</pre></details>"
            for s in case["sources"]
        )
        expected = "".join(f"<li>{escape(i['rationale'])}</li>" for i in label["issues"])
        notes = "".join(f"<li>{escape(note)}</li>" for note in assessment.get("cases", {}).get(case["id"], []))
        sections.append(
            f"<section id='{case['id']}'><h2>{title}</h2><p><strong>Reader task:</strong> {escape(case['reader_task'])}</p>"
            f"<p><strong>Audience:</strong> {escape(case['audience'])}<br><strong>Surface:</strong> {escape(case['surface'])}</p>"
            f"<details><summary>Governing sources</summary>{source_html}</details><div class='columns'>"
            f"<article><h3>Candidate reviewed</h3><pre>{escape(case['candidate'])}</pre></article>"
            f"<article><h3>Model findings</h3><p>{escape(agent.get('actual_model', 'unverified model'))} · {agent['duration_ms']/1000:.1f}s · "
            f"output integrity: {str(integrity['valid']).lower()}</p><ol>{finding_html}</ol>"
            f"<h3>Abstentions</h3><ul>{abstention_html or ('<li>None reported.</li>' if parsed else '<li>Unparsed.</li>')}</ul>"
            f"<h3>Independent agent assessment</h3><ul>{notes or '<li>Not supplied.</li>'}</ul>"
            f"<details><summary>Provisional authored issues</summary><p>{escape(label['validity_rationale'])}</p><ul>{expected}</ul></details>"
            f"<details><summary>Raw final answer and protocol issues</summary><pre>{escape(agent.get('final_text',''))}</pre>"
            f"<pre>{escape(json.dumps(integrity['errors']))}</pre></details></article></div>"
            f"<details><summary>Deterministic kapi coverage</summary><p>Status: {escape(deterministic['status'])}; "
            f"findings: {len(deterministic.get('findings',[]))}</p><p>{escape(statuses)}</p></details></section>"
        )
    page = """<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Contextual review: saved results</title><style>
body{font:17px/1.55 system-ui,sans-serif;color:#233c38;background:#f6f4ec;margin:0}main{max-width:1360px;margin:auto;padding:28px 20px}
h1,h2,h3{line-height:1.25}section{margin:50px 0;scroll-margin-top:20px}article{background:white;border:1px solid #c5d8d0;padding:20px;border-radius:10px;min-width:0}
.columns{display:grid;grid-template-columns:1fr 1fr;gap:20px;margin-top:20px}pre{font:inherit;white-space:pre-wrap;overflow-wrap:anywhere}
details{padding:12px;background:#edf4ef;margin:12px 0;overflow-wrap:anywhere}summary{cursor:pointer}blockquote{margin:12px 0;padding-left:14px;border-left:3px solid #23766b}
li{margin:16px 0}a{color:#17675a}table{border-collapse:collapse;width:100%}td,th{padding:10px;border-bottom:1px solid #c5d8d0;text-align:left}.table{overflow-x:auto}
@media(max-width:850px){.columns{grid-template-columns:1fr}}
</style><main><h1>Contextual review: what the six attempts reported</h1>
<p>Each review uses a fixed guide and supplied source evidence. These are synthetic development documents based on repository documentation.
The model is reviewing directly; kapi's deterministic checks are recorded separately. This does not measure kapi's incremental benefit.</p>
<p>Finding counts are observations, not correctness scores. Expected issues are provisional authored labels, independently checked by an agent.
Output integrity validates JSON, references and quotations only. Additional findings require source-based adjudication; human acceptance and review time remain unmeasured.</p>
<div class="table"><table><thead><tr><th>Case</th><th>Host outcome</th><th>Time</th><th>Findings</th><th>Abstentions</th><th>Authored issues</th></tr></thead><tbody>
""" + "".join(rows) + "</tbody></table></div>" + "".join(sections) + "</main></html>\n"
    with output.open("x", encoding="utf-8") as file:
        file.write(page)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prepared", type=Path, required=True)
    parser.add_argument("--study", type=Path, required=True)
    parser.add_argument("--checks", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--assessment", type=Path, help="Optional separately recorded agent adjudication; never changes saved results")
    args = parser.parse_args()
    render(args.prepared, args.study, args.checks, args.out, args.assessment)
