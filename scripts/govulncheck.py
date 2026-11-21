#!/usr/bin/env python3
"""Run govulncheck over Go modules and fail on every advisory nobody has reviewed.

govulncheck fails on any advisory whose vulnerable code the module reaches. An
advisory with no fixed release then fails every run, and a job that fails every
run stops being read: the next advisory lands behind it unseen. This wrapper
keeps the job meaningful. It scans every module it is given, reports all of
them, and consults scripts/govulncheck-accepted.txt, where a person records an
advisory that does not apply to one module, why, and the date the decision is
looked at again.

A run fails when:

  - a module reaches a vulnerability that is not accepted for that module;
  - an accepted advisory has a fixed release (upgrade, then drop the entry);
  - an accepted advisory has reached its review date;
  - an accepted advisory matches nothing any more (drop the entry);
  - the scanner's output is not the JSON this script was written against.

The scanner version is pinned, and its output is checked against the protocol
it declares, so a change of format fails the run. An unrecognised stream never
reads as "nothing found".

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
TOOL = "golang.org/x/vuln/cmd/govulncheck@v1.10.0"
PROTOCOL = "v1."
RECORDS = {"config", "SBOM", "progress", "osv", "finding"}


def read_accepted(text):
    """Parse `module  ID  review-by  reason` lines into {(module, id): (date, reason)}."""
    accepted = {}
    for number, line in enumerate(text.splitlines(), 1):
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        parts = line.split(None, 3)
        if len(parts) < 4:
            raise ValueError(f"{ACCEPTED.name}:{number}: expected `module ID review-by reason`")
        accepted[(parts[0], parts[1])] = (datetime.date.fromisoformat(parts[2]), parts[3])
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
    configs = 0
    for item in stream:
        if not isinstance(item, dict) or not set(item) <= RECORDS:
            raise ValueError(f"unrecognised govulncheck record: {str(item)[:120]}")
        if "config" in item:
            configs += 1
            config = item["config"]
            if not str(config.get("protocol_version", "")).startswith(PROTOCOL) or config.get("scan_level") != "symbol":
                raise ValueError(f"unexpected govulncheck configuration: {config}")
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
    if configs != 1:
        raise ValueError(f"expected one govulncheck config record, found {configs}")
    return {key: (summaries.get(key, ""), entry["fixed"], entry["where"]) for key, entry in reached.items()}


def judge(reached_by_module, accepted, today):
    """Return the failure lines for the scan results against the accepted list."""
    failures = []
    seen = set()
    for module, reached in reached_by_module.items():
        for key, (summary, fixed, where) in sorted(reached.items()):
            label = f"{module}: {key} {summary}".rstrip()
            seen.add((module, key))
            if (module, key) not in accepted:
                fix = f"fixed in {fixed}" if fixed else "no fixed release"
                failures.append(f"{label}\n    {fix}; reached from {where}")
                continue
            review_by, _ = accepted[(module, key)]
            if fixed:
                failures.append(f"{label}\n    accepted, but {fixed} fixes it: upgrade and drop the entry")
            elif today >= review_by:
                failures.append(f"{label}\n    accepted until {review_by}: review it again and move the date")
    for module, key in sorted(set(accepted) - seen):
        if module in reached_by_module:
            failures.append(f"{module}: {key} accepted, but the module does not reach it now: drop the entry")
        else:
            failures.append(f"{module}: {key} accepted, but the module was not scanned")
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
    records = [
        {"config": {"scanner_name": "govulncheck", "protocol_version": "v1.0.0", "scan_level": "symbol"}},
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
    ]

    def encode(items):
        return "\n".join(json.dumps(item) for item in items)

    reached = called(read_stream(encode(records)))
    expect = {"GO-1": ("Reached, fixed", "v1.2.3", "main.go:7"), "GO-2": ("Reached, no fix", "", "run.go:3")}
    if reached != expect:
        raise AssertionError(f"called: expected {expect}, found {reached}")
    today = datetime.date(2026, 6, 1)
    later = datetime.date(2026, 12, 1)
    cases = [
        ({}, 2, "nothing accepted: both reached advisories fail"),
        ({("app", "GO-2"): (later, "why")}, 1, "an accepted advisory with no fix passes"),
        ({("app", "GO-1"): (later, "why"), ("app", "GO-2"): (later, "why")}, 1, "an accepted advisory with a fix fails"),
        ({("app", "GO-2"): (datetime.date(2026, 1, 1), "why")}, 2, "a past review date fails"),
        ({("app", "GO-2"): (today, "why")}, 2, "the review date itself fails"),
        ({("app", "GO-2"): (later, "why"), ("app", "GO-9"): (later, "why")}, 2, "an entry that matches nothing fails"),
        ({("other", "GO-2"): (later, "why")}, 3, "an entry for another module accepts nothing here"),
    ]
    for accepted, count, name in cases:
        failures = judge({"app": reached}, accepted, today)
        if len(failures) != count:
            raise AssertionError(f"{name}: expected {count} failures, found {failures}")
    parsed = read_accepted("# note\napp  GO-2  2026-12-01  not reachable  # trailing\n\n")
    if parsed != {("app", "GO-2"): (later, "not reachable")}:
        raise AssertionError(f"read_accepted: found {parsed}")
    refused = [
        ("an entry with no reason", lambda: read_accepted("app GO-2 2026-12-01\n")),
        ("a stream with no config record", lambda: called(read_stream(encode(records[1:])))),
        ("an unknown record", lambda: called(read_stream(encode(records + [{"vulnerability": {}}])))),
        ("another protocol", lambda: called(read_stream(encode(
            [{"config": {"protocol_version": "v2.0.0", "scan_level": "symbol"}}])))),
        ("a module-level scan", lambda: called(read_stream(encode(
            [{"config": {"protocol_version": "v1.0.0", "scan_level": "module"}}])))),
    ]
    for name, attempt in refused:
        try:
            attempt()
        except ValueError:
            continue
        raise AssertionError(f"self-test: accepted {name}")


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
    for module, key in sorted(set(accepted)):
        if key in reached.get(module, {}):
            review_by, reason = accepted[(module, key)]
            print(f"govulncheck: {module}: {key} accepted until {review_by}: {reason}")
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
