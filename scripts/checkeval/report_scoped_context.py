#!/usr/bin/env python3
"""Render saved scoped-style reviews with actual retrieved context; no model calls."""

import argparse
import hashlib
import html
import json
from pathlib import Path


def digest(data):
    return hashlib.sha256(data).hexdigest()


def read(path):
    return json.loads(path.read_text())


def esc(value):
    return html.escape(str(value))


def pre(value):
    return f"<pre>{esc(value)}</pre>"


def passages(spans, title):
    return f"<h4>{title}</h4>" + ("".join(
        f"<blockquote><small>{esc(s['id'])} · UTF-8 bytes {s['start']}–{s['end']}</small>{pre(s['text'])}</blockquote>"
        for s in spans
    ) or "<p>No candidate passage selected.</p>")


def evidence(items):
    return "".join(
        "<article class='finding'>" + passages(item["candidate_spans"], "Candidate evidence")
        + passages(item["guidance_spans"], "Applicable guidance")
        + f"<p><strong>Model interpretation:</strong> {esc(item['rationale'])}</p></article>"
        for item in items
    ) or "<p>None reported.</p>"


def validate_style_identity(style, case):
    snapshot = json.loads(style["evidence"])
    if digest(style["evidence"].encode()) != style["request_fingerprint"] or style["request_id"] != case["id"]:
        raise ValueError("Style result identity differs from case")
    stored = dict(snapshot["input"])
    for field in ("candidate_spans", "guidance_spans"):
        spans = stored.pop(field)
        if spans != style[field]:
            raise ValueError("Style evidence spans differ from snapshot")
        text = case["candidate"] if field == "candidate_spans" else next(s["text"] for s in case["sources"] if s["id"] == "resolved-voice")
        raw = text.encode()
        known = {}
        for span in spans:
            if not 0 <= span["start"] < span["end"] <= len(raw) or raw[span["start"]:span["end"]].decode() != span["text"] or span["id"] in known:
                raise ValueError("Style span does not match exact source bytes")
            known[span["id"]] = span
        ids_field = "candidate_ids" if field == "candidate_spans" else "guidance_ids"
        for item in style["findings"] + style["suggestions"]:
            if item[field] != [known[i] for i in item[ids_field]]:
                raise ValueError("Selected style evidence differs from frozen spans")
    # Go omits an empty optional requirements slice from its snapshot.
    expected = {k: v for k, v in case.items() if k != "requirements" or v}
    if stored != expected:
        raise ValueError("Style result belongs to different frozen input")


def load_records(prepared, study, assessment_path=None):
    for name in ("inputs.jsonl", "manifest.json", "instruction.txt"):
        if (prepared / "subject" / name).read_bytes() != (study / name).read_bytes():
            raise ValueError(f"Prepared {name} differs from frozen study")
    raw = (study / "inputs.jsonl").read_bytes()
    cases = {c["id"]: c for c in map(json.loads, raw.splitlines())}
    manifest = read(study / "manifest.json")
    labels = read(prepared / "labels.json")
    resolution = read(prepared / "resolution.json")
    resolved = {r["id"]: r for r in resolution["cases"]}
    assessment = read(assessment_path) if assessment_path else {}
    if assessment and assessment["inputs_sha256"] != digest(raw):
        raise ValueError("Assessment belongs to different inputs")
    records = []
    for session in manifest["sessions"]:
        if session["protocol"] != "style":
            raise ValueError("This report accepts only native style reviews")
        case = cases[session["case_id"]]
        resolved_case = resolved[case["id"]]
        if digest(case["candidate"].encode()) != resolved_case["candidate_sha256"]:
            raise ValueError("Resolved candidate differs from study")
        sources = [s["text"] for s in case["sources"] if s["id"] == "resolved-voice"]
        if sources != [resolved_case["voice"]["guide"]]:
            raise ValueError("Resolved guide differs from study")
        if case["variables"]["resolved_context"]["point"] != resolved_case["point"]:
            raise ValueError("Resolved coordinates differ from study")
        for command in ("context", "check"):
            path = prepared / "resolved" / resolved_case["case"] / f"{command}.json"
            if digest(path.read_bytes()) != resolved_case[command]["stdout_sha256"]:
                raise ValueError("Raw resolution or check evidence differs from recorded hash")
            if command == "context":
                actual = read(path)
                if actual["point"] != resolved_case["point"] or actual["voice"] != resolved_case["voice"]:
                    raise ValueError("Raw resolver selection differs from supplied context")
        result_path = study / "attempts" / session["id"] / "result.json"
        if not result_path.exists():
            raise ValueError("Study contains unfinished sessions")
        result = read(result_path)
        style = result["integrity"].get("style") if result["integrity"]["valid"] else None
        if style:
            validate_style_identity(style, case)
        records.append((session, case, labels[case["id"]], resolved_case, result, style))
    return records, resolution, assessment


def render(prepared, study, output, assessment_path=None):
    records, resolution, assessment = load_records(prepared, study, assessment_path)
    rows, sections = [], []
    for session, case, label, resolved, result, style in records:
        agent, integrity = result["agent"], result["integrity"]
        point = resolved["point"]
        title = f"{label['candidate'].capitalize()} · {point['ref']}"
        status = style["assessment"] if style else "no accepted review"
        format_status = "accepted after unwrapping" if style and integrity.get("transport") else "accepted bare" if style else "rejected"
        rows.append(f"<tr><td><a href='#{esc(session['id'])}'>{esc(title)}</a></td><td>{esc(status)}</td>"
                    f"<td>{len(style['findings']) if style else '—'}</td><td>{agent['duration_ms']/1000:.1f}s</td><td>{format_status}</td></tr>")
        notes = assessment.get("sessions", {}).get(session["id"], [])
        note_html = "".join(f"<li>{esc(note)}</li>" for note in notes)
        coverage = "".join(f"<li><strong>{esc(a['id'])}: {esc(a['status'])}</strong> — {esc(a.get('reason', ''))}</li>" for a in resolved["coverage"])
        findings = evidence(style["findings"]) if style else "<p>No accepted review. Inspect the retained output below.</p>"
        uncertainties = "".join(f"<li>{esc(u['rationale'])}</li>" for u in style["uncertainties"]) if style else ""
        suggestions = evidence(style["suggestions"]) if style else "<p>No accepted review.</p>"
        sections.append(
            f"<section id='{esc(session['id'])}'><h2>{esc(title)}</h2>"
            f"<p><strong>Destination:</strong> {esc(point['path'])}<br><strong>Resolved coordinates:</strong> {esc(json.dumps(point['coordinates']))}</p>"
            "<div class='columns'><article><h3>Candidate reviewed</h3>" + pre(case["candidate"])
            + "</article><article><h3>Guidance returned by kapi</h3>" + pre(resolved["voice"]["guide"]) + "</article></div>"
            f"<h3>Advisory result: {esc(status)}</h3><p>{esc(agent.get('actual_model', 'unverified model'))} · {agent['duration_ms']/1000:.1f}s · {format_status}</p>"
            + findings + f"<h3>Uncertainty</h3><ul>{uncertainties or '<li>None reported in an accepted review.</li>'}</ul>"
            + f"<details><summary>Optional suggestions</summary>{suggestions}</details>"
            + f"<h3>Independent agent assessment</h3><ul>{note_html or '<li>Not supplied.</li>'}</ul>"
            + f"<details><summary>Provisional authored expectation</summary><p>{esc(label['expected_style'])} · {esc(label['label_status'])}</p></details>"
            + f"<details><summary>Deterministic check coverage</summary><ul>{coverage}</ul></details>"
            + "<details><summary>Resolution provenance and identity</summary>" + pre(json.dumps(resolved, indent=2)) + "</details>"
            + "<details><summary>Raw final answer and validation issues</summary>" + pre(agent.get("final_text", ""))
            + pre(json.dumps({"errors": integrity["errors"], "transport": integrity.get("transport"), "host_status": agent["status"], "result_error": result.get("error")}, indent=2))
            + "</details></section>"
        )
    page = """<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Scoped context: saved reviews</title><style>
body{font:17px/1.55 system-ui,sans-serif;color:#233c38;background:#f6f4ec;margin:0}main{max-width:1280px;margin:auto;padding:28px 20px}
h1,h2,h3,h4{line-height:1.25}section{margin:48px 0;scroll-margin-top:20px}article{background:white;border:1px solid #c5d8d0;padding:20px;border-radius:10px;min-width:0}
.columns{display:grid;grid-template-columns:1fr 1fr;gap:20px}pre{font:inherit;white-space:pre-wrap;overflow-wrap:anywhere}p{overflow-wrap:anywhere}
details{padding:12px;background:#edf4ef;margin:12px 0;overflow-wrap:anywhere}summary{cursor:pointer}blockquote{margin:12px 0;padding-left:14px;border-left:3px solid #23766b}
.finding{margin:14px 0}li{margin:10px 0}a{color:#17675a}table{border-collapse:collapse;width:100%}td,th{padding:10px;border-bottom:1px solid #c5d8d0;text-align:left}.table{overflow-x:auto}
@media(max-width:850px){.columns{grid-template-columns:1fr}}
</style><main><h1>Does the review follow the context?</h1>
<p>Three messages about the same fictional service issue, each reviewed at two destinations.
kapi retrieves a warm voice for individual support replies and neutral operational guidance for the public status page.
The text stays identical when its destination changes.</p>
<p>Open a case to compare the candidate, the actual retrieved guide and the evidence behind each advisory departure.
“Aligned” means no evidenced departure was reported; it is not a quality score or approval to publish.</p>
"""
    if assessment.get("summary"):
        page += "<p><strong>Independent agent reading:</strong> " + esc(assessment["summary"]) + "</p>"
    page += "<div class='table'><table><thead><tr><th>Candidate / destination</th><th>Advisory assessment</th><th>Departures</th><th>Time</th><th>Response format</th></tr></thead><tbody>" + "".join(rows) + "</tbody></table></div>"
    page += ("<details><summary>What this exercise establishes</summary><p>This authored development fixture checks actual project/channel resolution and advisory context-sensitive review. "
             "It includes offline controls for unrelated-product exclusion and missing voice binding. Those controls do not test model behavior with missing guidance. "
             "Audience coordinates are metadata here; the product/channel binding selects the voice.</p><p>The experiment does not establish general style accuracy, savings in human review, "
             "or an advantage over an agent handed the same guidance. Independent agent assessment is not human adjudication. "
             "Mechanical checks do not verify the semantic style rules, even when they report no findings.</p>"
             + pre(assessment.get("method", "Independent assessment not supplied."))
             + "<p>Recorded kapi binary SHA-256: " + esc(resolution["binary_sha256"]) + "</p></details>")
    page += "".join(sections) + "</main></html>\n"
    with output.open("x", encoding="utf-8") as file:
        file.write(page)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prepared", type=Path, required=True)
    parser.add_argument("--study", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--assessment", type=Path)
    args = parser.parse_args()
    render(args.prepared, args.study, args.out, args.assessment)
