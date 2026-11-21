#!/usr/bin/env python3
"""Run govulncheck over Go modules and fail on every advisory nobody has reviewed.

govulncheck fails on any advisory whose vulnerable code the module reaches. An
advisory with no fixed release then fails every run, and a job that fails every
run stops being read: the next advisory lands behind it unseen. This wrapper
keeps the job meaningful. It scans every module it is given, reports all of
them, and consults scripts/govulncheck-accepted.txt, where a person records an
advisory that does not apply, why, and the date the decision is looked at again.

A run fails when:

  - a module reaches a vulnerability that is not in the accepted list;
  - an accepted advisory has a fixed release (upgrade, then drop the entry);
  - an accepted advisory is past its review date;
  - an accepted advisory matches nothing any more (drop the entry).

Usage:
    scripts/govulncheck.py [--tags fts5] <module-dir>...
    scripts/govulncheck.py --self-test
"""

import argparse
import datetime
import json
import subprocess
import sys
from pathlib import Path

ACCEPTED = Path(__file__).with_name("govulncheck-accepted.txt")
TOOL = "golang.org/x/vuln/cmd/govulncheck@latest"


def read_accepted(text):
    """Parse `ID  review-by(YYYY-MM-DD)  reason` lines into {id: (date, reason)}."""
    accepted = {}
    for number, line in enumerate(text.splitlines(), 1):
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        parts = line.split(None, 2)
        if len(parts) < 3:
            raise ValueError(f"{ACCEPTED.name}:{number}: expected `ID review-by reason`")
        accepted[parts[0]] = (datetime.date.fromisoformat(parts[1]), parts[2])
    return accepted


def read_stream(text):
    """Yield each JSON object of govulncheck's concatenated JSON output."""
    decoder = json.JSONDecoder()
    index = 0
    while index < len(text):
        while index < len(text) and text[index].isspace():
            index += 1
        if index == len(text):
            return
        value, index = decoder.raw_decode(text, index)
        yield value


def called(stream):
    """Return {id: (summary, fixed version, where)} for the advisories reached.

    A finding counts when its first trace frame names a function: that is the
    vulnerable symbol, and the module's own code calls its way to it. Findings
    for a package or module the code never calls into are left out, as
    govulncheck's own exit status leaves them out.
    """
    summaries = {}
    reached = {}
    for item in stream:
        if "osv" in item:
            summaries[item["osv"]["id"]] = item["osv"].get("summary", "")
        finding = item.get("finding")
        if not finding:
            continue
        trace = finding.get("trace") or []
        if not trace or not trace[0].get("function"):
            continue
        caller = trace[-1]
        position = caller.get("position") or {}
        where = f"{position.get('filename', caller.get('package', '?'))}:{position.get('line', 0)}"
        entry = reached.setdefault(finding["osv"], {"fixed": "", "where": where})
        entry["fixed"] = entry["fixed"] or finding.get("fixed_version", "")
    return {key: (summaries.get(key, ""), entry["fixed"], entry["where"]) for key, entry in reached.items()}


def judge(reached_by_module, accepted, today):
    """Return the failure lines for the scan results against the accepted list."""
    failures = []
    seen = set()
    for module, reached in reached_by_module.items():
        for key, (summary, fixed, where) in sorted(reached.items()):
            seen.add(key)
            label = f"{module}: {key} {summary}".rstrip()
            if key not in accepted:
                fix = f"fixed in {fixed}" if fixed else "no fixed release"
                failures.append(f"{label}\n    {fix}; reached from {where}")
                continue
            review_by, _ = accepted[key]
            if fixed:
                failures.append(f"{label}\n    accepted, but {fixed} fixes it: upgrade and drop the entry")
            elif today > review_by:
                failures.append(f"{label}\n    accepted until {review_by}: review it again and move the date")
    for key in sorted(set(accepted) - seen):
        failures.append(f"{key}: accepted, but no module reaches it now: drop the entry")
    return failures


def scan(module, tags):
    command = ["go", "run", TOOL, "-format", "json"]
    if tags:
        command += ["-tags", tags]
    command.append("./...")
    result = subprocess.run(command, cwd=module, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        raise RuntimeError(f"{module}: govulncheck could not run\n{result.stderr.strip()}")
    return called(read_stream(result.stdout))


def self_test():
    stream = "\n".join(json.dumps(item) for item in [
        {"config": {"scanner_name": "govulncheck"}},
        {"osv": {"id": "GO-1", "summary": "Reached, fixed"}},
        {"osv": {"id": "GO-2", "summary": "Reached, no fix"}},
        {"osv": {"id": "GO-3", "summary": "Imported only"}},
        {"finding": {"osv": "GO-1", "fixed_version": "v1.2.3", "trace": [
            {"module": "m", "function": "Bad"},
            {"module": "app", "function": "main", "position": {"filename": "main.go", "line": 7}}]}},
        {"finding": {"osv": "GO-2", "trace": [
            {"module": "m", "function": "Worse"},
            {"module": "app", "function": "run", "position": {"filename": "run.go", "line": 3}}]}},
        {"finding": {"osv": "GO-3", "fixed_version": "v2.0.0", "trace": [{"module": "m", "package": "m/p"}]}},
    ])
    reached = called(read_stream(stream))
    expect = {"GO-1": ("Reached, fixed", "v1.2.3", "main.go:7"), "GO-2": ("Reached, no fix", "", "run.go:3")}
    if reached != expect:
        raise AssertionError(f"called: expected {expect}, found {reached}")
    today = datetime.date(2026, 6, 1)
    later = datetime.date(2026, 12, 1)
    cases = [
        ({}, 2, "nothing accepted: both reached advisories fail"),
        ({"GO-2": (later, "why")}, 1, "an accepted advisory with no fix passes"),
        ({"GO-1": (later, "why"), "GO-2": (later, "why")}, 1, "an accepted advisory with a fix fails"),
        ({"GO-2": (datetime.date(2026, 1, 1), "why")}, 2, "a past review date fails"),
        ({"GO-2": (later, "why"), "GO-9": (later, "why")}, 2, "an entry that matches nothing fails"),
    ]
    for accepted, count, name in cases:
        failures = judge({"app": reached}, accepted, today)
        if len(failures) != count:
            raise AssertionError(f"{name}: expected {count} failures, found {failures}")
    parsed = read_accepted("# note\nGO-2  2026-12-01  not reachable  # trailing\n\n")
    if parsed != {"GO-2": (later, "not reachable")}:
        raise AssertionError(f"read_accepted: found {parsed}")
    try:
        read_accepted("GO-2 2026-12-01\n")
    except ValueError:
        pass
    else:
        raise AssertionError("read_accepted took an entry with no reason")


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--tags", default="fts5")
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("modules", nargs="*")
    args = parser.parse_args()
    self_test()
    if args.self_test:
        print("govulncheck wrapper fixtures passed.")
        return 0
    if not args.modules:
        parser.error("name at least one module directory")
    accepted = read_accepted(ACCEPTED.read_text(encoding="utf-8"))
    reached = {}
    for module in args.modules:
        print(f"govulncheck: scanning {module}", flush=True)
        reached[module] = scan(module, args.tags)
    failures = judge(reached, accepted, datetime.date.today())
    for key in sorted({key for found in reached.values() for key in found} & set(accepted)):
        review_by, reason = accepted[key]
        print(f"govulncheck: {key} accepted until {review_by}: {reason}")
    if failures:
        print("\n".join(failures), file=sys.stderr)
        print(f"govulncheck: {len(failures)} finding(s) need a decision", file=sys.stderr)
        return 1
    print("govulncheck: no unreviewed vulnerability is reachable")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, RuntimeError, AssertionError) as error:
        sys.exit(f"govulncheck: {error}")
