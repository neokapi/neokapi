#!/usr/bin/env python3
"""Record deterministic kapi coverage for prepared document inputs, without AI.

Governing passages are supplied as semantic guidance, without encoding the
expected answers as regexes. This records executed coverage, not meaning scores.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time


def run(inputs_path, binary, output):
    inputs = [json.loads(line) for line in inputs_path.read_text().splitlines() if line.strip()]
    binary = binary.resolve(strict=True)
    output.mkdir(parents=True, exist_ok=False)
    records = []
    for index, case in enumerate(inputs):
        work = output / f"case-{index + 1}"
        work.mkdir()
        (work / "guide.md").write_text(case["candidate"], encoding="utf-8")
        (work / "sources.json").write_text(json.dumps(case["sources"], indent=2), encoding="utf-8")
        profile = {
            "name": "Document evidence", "description": "Preserve the supplied product behavior and required reader actions.",
            "constraints": [{"id": "document/" + source["id"], "version": 1,
                             "kind": "guidance", "source": "sources.json#" + source["id"],
                             "statement": source["text"]} for source in case["sources"]],
        }
        (work / "voice.yaml").write_text(json.dumps(profile), encoding="utf-8")
        env = dict(os.environ)
        env.update({"KAPI_NO_PROJECT": "1", "KAPI_PLUGINS_DIR_ONLY": "1"})
        for variable, name in {"KAPI_CONFIG_DIR": "config", "XDG_DATA_HOME": "data", "XDG_CACHE_HOME": "cache", "KAPI_PLUGINS_DIR": "plugins"}.items():
            folder = work / name
            folder.mkdir()
            env[variable] = str(folder.resolve())
        command = [str(binary), "check", "guide.md", "--profile-file", "voice.yaml", "--json"]
        started = time.monotonic()
        try:
            result = subprocess.run(command, cwd=work, env=env, capture_output=True, text=True, timeout=60)
        except subprocess.TimeoutExpired as error:
            for name, partial in (("stdout.json", error.stdout), ("stderr.txt", error.stderr)):
                data = partial.decode("utf-8", errors="replace") if isinstance(partial, bytes) else partial or ""
                (work / name).write_text(data, encoding="utf-8")
            records.append({"id": case["id"], "status": "timeout", "meaning_accuracy": "unmeasured",
                            "elapsed_ms": (time.monotonic() - started) * 1000, "command": command[1:]})
            continue
        elapsed_ms = (time.monotonic() - started) * 1000
        (work / "stdout.json").write_text(result.stdout, encoding="utf-8")
        (work / "stderr.txt").write_text(result.stderr, encoding="utf-8")
        record = {"id": case["id"], "exit_code": result.returncode, "elapsed_ms": elapsed_ms,
                  "status": "operational_error", "meaning_accuracy": "unmeasured", "command": command[1:]}
        if result.returncode in (0, 3):
            try:
                report = json.loads(result.stdout)
                if report.get("schema") != "kapi.check/v1":
                    raise ValueError("Unexpected report schema")
                record.update({"status": "reported", "findings": report["findings"], "execution": report.get("execution"), "gate": report.get("gate")})
            except (ValueError, KeyError):
                record["status"] = "invalid_report"
        records.append(record)
    summary = {"binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
               "inputs_sha256": hashlib.sha256(inputs_path.read_bytes()).hexdigest(),
               "runner_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
               "model_calls": 0, "meaning_accuracy": "unmeasured", "cases": records}
    (output / "coverage.json").write_text(json.dumps(summary, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    return {"cases": len(records), "reported": sum(r["status"] == "reported" for r in records), "model_calls": 0}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--inputs", type=Path, required=True)
    parser.add_argument("--kapi", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    result = run(args.inputs, args.kapi, args.out)
    print(json.dumps(result, indent=2))
    sys.exit(0 if result["reported"] == result["cases"] else 1)
