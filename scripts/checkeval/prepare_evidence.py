#!/usr/bin/env python3
"""Prepare a targeted comparison of requirement review and anchored evidence.

No model calls. Existing corpus labels and candidate content are retained.
Repository-source blob verification runs through the original preparer.
"""

import argparse
import html
import json
import tempfile
from pathlib import Path

import prepare_actions
import prepare_documents


TARGETS = (
    ("actions", "museum-transfer-explicit", "Supported action guidance with a previous literal-quote failure."),
    ("actions", "editorial-correction-omitted", "A required action omission with a previous unsupported duplicate claim."),
    ("documents", "destination-guidance-faulty", "Known factual conflicts and a required saved-file check omission."),
)


def read(path):
    return json.loads(path.read_text(encoding="utf-8"))


def write(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def prepared_bundle(directory):
    lines = (directory / "subject" / "inputs.jsonl").read_text(encoding="utf-8").splitlines()
    cases = [json.loads(line) for line in lines if line.strip()]
    return {
        "inputs": {case["id"]: case for case in cases},
        "labels": read(directory / "labels.json"),
        "provenance": read(directory / "provenance.json"),
    }


def select_cases(bundles):
    inputs, labels = [], {}
    for corpus, variant, reason in TARGETS:
        bundle = bundles[corpus]
        matches = [case_id for case_id, label in bundle["labels"].items() if label["variant"] == variant]
        if len(matches) != 1 or matches[0] not in bundle["inputs"]:
            raise ValueError(f"Expected one prepared subject case for {variant}")
        case_id = matches[0]
        if case_id in labels:
            raise ValueError("Selected case ID collision")
        inputs.append(bundle["inputs"][case_id])
        labels[case_id] = {**bundle["labels"][case_id], "selection_reason": reason}
    return inputs, labels


def casebook(inputs, labels):
    sections = []
    for case in inputs:
        label = labels[case["id"]]
        requirements = "".join("<li>" + html.escape(r["description"]) + "</li>" for r in case["requirements"])
        sources = "".join(
            "<details><summary>" + html.escape(source["id"]) + "</summary>" +
            prepare_actions.prose(source["text"]) + "</details>" for source in case["sources"]
        )
        issues = "".join(
            "<li><strong>" + html.escape(issue["kind"]) + ":</strong> " + html.escape(issue["rationale"]) +
            " <span>Evidence: " + html.escape(", ".join(issue["evidence_ids"])) + "</span></li>"
            for issue in label["issues"]
        )
        sections.append(
            "<section><h2>" + html.escape(label["variant"]) + "</h2><p><strong>Selection:</strong> " +
            html.escape(label["selection_reason"]) + "</p><p><strong>Reader task:</strong> " + html.escape(case["reader_task"]) +
            "</p><p><strong>Audience:</strong> " + html.escape(case["audience"]) +
            "</p><p><strong>Destination:</strong> " + html.escape(case["destination"]) +
            "</p><h3>Required actions and decisions</h3><ul>" + requirements +
            "</ul><h3>Governing sources</h3>" + sources + "<article>" + prepare_actions.prose(case["candidate"]) +
            "</article><details><summary>Frozen provisional author judgments</summary><p>" +
            html.escape(label["validity_rationale"]) + "</p><ul>" + issues + "</ul></details></section>"
        )
    return """<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Anchored evidence development casebook</title><style>
body{font:17px/1.6 system-ui,sans-serif;color:#233c38;background:#f6f4ec;margin:0}
main{max-width:1000px;margin:auto;padding:28px 20px}h1,h2,h3,h4{line-height:1.3}h4{font-size:1.05rem}section{margin:48px 0}
article{background:white;border:1px solid #c5d8d0;border-radius:10px;padding:20px}details{background:#edf4ef;padding:12px;margin:10px 0}
summary{cursor:pointer}p,li{overflow-wrap:anywhere}span{display:block;font-size:.9rem}
</style><main><h1>Can evidence selection improve review reliability?</h1>
<p>This is a targeted development regression comparison, selected from known failures and controls. It is not held out or an independent benchmark.
The two fictional action procedures and the repository-grounded kapi procedure retain their existing candidates, sources, requirements and provisional author labels.</p>
<p>Each of the three candidates receives a requirements review and an anchored review with the same model and task data.
The anchored protocol derives paragraph selection IDs from the candidate. Protocol order alternates between cases; the schedule allows six fresh sessions.</p>
<p>Selection explanations and labels appear only in this evaluator casebook. This page reports no model results and asks for no grading.</p>
""" + "".join(sections) + "</main></html>\n"


def prepare(actions, documents, output, repo):
    with tempfile.TemporaryDirectory(prefix="kapi-evidence-preparation-") as temporary:
        temporary = Path(temporary)
        prepare_actions.prepare(actions, temporary / "actions")
        prepare_documents.prepare(documents, temporary / "documents", repo, requirements_comparison=True)
        bundles = {name: prepared_bundle(temporary / name) for name in ("actions", "documents")}
    inputs, labels = select_cases(bundles)
    sessions = []
    for index, case in enumerate(inputs):
        protocols = ["requirements", "anchored"] if index % 2 == 0 else ["anchored", "requirements"]
        sessions.extend({"id": f"review-{case['id']}-{protocol}", "host": "claude", "case_id": case["id"],
                         "protocol": protocol} for protocol in protocols)
    manifest = {"schema": 1, "study": "contextual-evidence-comparison", "billing": "subscription-only",
                "agents": [{"host": "claude", "model": "claude-sonnet-5", "effort": "high"}],
                "sessions": sessions, "attempt_timeout_seconds": 180, "max_turns": 3}
    output.mkdir(parents=True, exist_ok=False)
    subject = output / "subject"
    subject.mkdir()
    write(subject / "manifest.json", manifest)
    (subject / "inputs.jsonl").write_text("".join(json.dumps(case, ensure_ascii=False) + "\n" for case in inputs), encoding="utf-8")
    (subject / "instruction.txt").write_text(prepare_actions.INSTRUCTION, encoding="utf-8")
    write(output / "labels.json", labels)
    write(output / "provenance.json", {
        "selection": [{"corpus": corpus, "variant": variant, "reason": reason} for corpus, variant, reason in TARGETS],
        "preparer_sha256": prepare_actions.digest(Path(__file__).read_bytes()),
        "preparation_dependencies": {Path(module.__file__).name: prepare_actions.digest(Path(module.__file__).read_bytes())
                                     for module in (prepare_actions, prepare_documents)},
        "source_preparations": {name: bundle["provenance"] for name, bundle in bundles.items()},
        "evaluation_status": "targeted_development_regression",
    })
    (output / "review.html").write_text(casebook(inputs, labels), encoding="utf-8")
    return {"cases": len(inputs), "sessions": len(sessions), "model_calls": 0, "subject": str(subject)}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--actions", type=Path, default=Path(__file__).with_name("action-coverage.json"))
    parser.add_argument("--documents", type=Path, default=Path(__file__).with_name("contextual-documents"))
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(prepare(args.actions, args.documents, args.out, args.repo), indent=2))
